// Package flaresolverr installs and manages the bundled portable FlareSolverr process.
package flaresolverr

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

const (
	Version     = "3.5.0"
	DownloadURL = "https://github.com/FlareSolverr/FlareSolverr/releases/download/v3.5.0/flaresolverr_windows_x64.zip"

	installDirectoryName = "FlareSolverr"
	executableName       = "flaresolverr.exe"
	listenAddress        = "127.0.0.1:8191"
	maxDownloadBytes     = 1 << 30
	maxExtractedBytes    = 2 << 30
)

// Manager owns a FlareSolverr process started by Shiori. If the port was
// already in use, Manager does not take ownership of the existing process.
type Manager struct {
	command  *exec.Cmd
	done     chan error
	logger   *slog.Logger
	stopping atomic.Bool
}

// EnsureAndStart downloads the Windows x64 release when necessary and starts
// FlareSolverr without opening a console window.
func EnsureAndStart(ctx context.Context, dataFolder string, logger *slog.Logger) (*Manager, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		return nil, fmt.Errorf("flaresolverr portable requires windows/amd64, got %s/%s", runtime.GOOS, runtime.GOARCH)
	}

	executable, err := ensureInstalled(ctx, dataFolder, logger)
	if err != nil {
		return nil, err
	}
	if isListening(listenAddress) {
		logger.Info("flaresolverr already listening", "address", listenAddress)
		return &Manager{}, nil
	}

	logger.Info("starting flaresolverr", "executable", executable, "address", listenAddress)
	command := hiddenCommand(executable)
	command.Dir = filepath.Dir(executable)
	command.Env = append(os.Environ(),
		"HOST=127.0.0.1",
		"PORT=8191",
		"LOG_LEVEL=info",
	)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("capturing flaresolverr stdout: %w", err)
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("capturing flaresolverr stderr: %w", err)
	}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("starting flaresolverr: %w", err)
	}

	manager := &Manager{command: command, done: make(chan error, 1), logger: logger}
	go streamLogs(stdout, logger.Info)
	go streamLogs(stderr, logger.Warn)
	go func() {
		err := command.Wait()
		if err != nil && !manager.stopping.Load() {
			logger.Warn("flaresolverr exited", "error", err)
		} else {
			logger.Info("flaresolverr stopped")
		}
		manager.done <- err
	}()

	if err := manager.waitUntilReady(ctx, 30*time.Second); err != nil {
		_ = manager.Close()
		return nil, err
	}
	logger.Info("flaresolverr ready", "address", listenAddress)
	return manager, nil
}

func ensureInstalled(ctx context.Context, dataFolder string, logger *slog.Logger) (string, error) {
	installDirectory := filepath.Join(dataFolder, installDirectoryName)
	if executable, err := findExecutable(installDirectory); err == nil {
		logger.Info("flaresolverr installation found", "version", Version, "directory", installDirectory)
		return executable, nil
	}
	if err := os.MkdirAll(installDirectory, 0o700); err != nil {
		return "", fmt.Errorf("creating flaresolverr directory: %w", err)
	}

	archive, err := os.CreateTemp(dataFolder, ".flaresolverr-*.zip")
	if err != nil {
		return "", fmt.Errorf("creating flaresolverr download: %w", err)
	}
	archivePath := archive.Name()
	if err := archive.Close(); err != nil {
		return "", fmt.Errorf("closing flaresolverr download: %w", err)
	}
	defer os.Remove(archivePath)

	logger.Info("downloading flaresolverr", "version", Version, "url", DownloadURL)
	if err := download(ctx, archivePath, logger); err != nil {
		return "", err
	}
	logger.Info("extracting flaresolverr", "directory", installDirectory)
	if err := extract(archivePath, installDirectory); err != nil {
		return "", err
	}
	executable, err := findExecutable(installDirectory)
	if err != nil {
		return "", fmt.Errorf("locating flaresolverr after extraction: %w", err)
	}
	logger.Info("flaresolverr installed", "version", Version, "executable", executable)
	return executable, nil
}

func download(ctx context.Context, destination string, logger *slog.Logger) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, DownloadURL, nil)
	if err != nil {
		return fmt.Errorf("creating flaresolverr download request: %w", err)
	}
	response, err := (&http.Client{Timeout: 10 * time.Minute}).Do(request)
	if err != nil {
		return fmt.Errorf("downloading flaresolverr %s: %w", Version, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("downloading flaresolverr: unexpected HTTP status %d", response.StatusCode)
	}
	if response.ContentLength > maxDownloadBytes {
		return fmt.Errorf("flaresolverr download declares %d bytes, exceeding 1 GiB", response.ContentLength)
	}

	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("opening flaresolverr download: %w", err)
	}
	progress := newDownloadProgress(logger, response.ContentLength)
	written, copyErr := io.Copy(io.MultiWriter(file, progress), io.LimitReader(response.Body, maxDownloadBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("saving flaresolverr download: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing flaresolverr download: %w", closeErr)
	}
	if written > maxDownloadBytes {
		return errors.New("flaresolverr download exceeds 1 GiB")
	}
	progress.complete()
	return nil
}

type downloadProgress struct {
	logger     *slog.Logger
	total      int64
	downloaded atomic.Int64
	lastLog    atomic.Int64
}

func newDownloadProgress(logger *slog.Logger, total int64) *downloadProgress {
	progress := &downloadProgress{logger: logger, total: total}
	progress.lastLog.Store(time.Now().UnixNano())
	progress.log(0, false)
	return progress
}

func (p *downloadProgress) Write(data []byte) (int, error) {
	downloaded := p.downloaded.Add(int64(len(data)))
	now := time.Now().UnixNano()
	last := p.lastLog.Load()
	if now-last >= int64(time.Second) && p.lastLog.CompareAndSwap(last, now) {
		p.log(downloaded, false)
	}
	return len(data), nil
}

func (p *downloadProgress) complete() {
	p.log(p.downloaded.Load(), true)
}

func (p *downloadProgress) log(downloaded int64, complete bool) {
	attributes := []any{
		"downloaded_bytes", downloaded,
		"downloaded_mib", float64(downloaded) / (1024 * 1024),
		"complete", complete,
	}
	if p.total > 0 {
		attributes = append(attributes,
			"total_bytes", p.total,
			"total_mib", float64(p.total)/(1024*1024),
			"percent", float64(downloaded)*100/float64(p.total),
		)
	}
	p.logger.Info("flaresolverr download progress", attributes...)
}

func streamLogs(reader io.Reader, logLine func(string, ...any)) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			logLine("flaresolverr", "message", line)
		}
	}
}

func extract(archivePath, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("opening flaresolverr archive: %w", err)
	}
	defer archive.Close()

	var extracted uint64
	for _, entry := range archive.File {
		extracted += entry.UncompressedSize64
		if extracted > maxExtractedBytes {
			return errors.New("flaresolverr archive exceeds 2 GiB after extraction")
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("flaresolverr archive contains unsupported symlink %q", entry.Name)
		}

		target, err := safeArchivePath(destination, entry.Name)
		if err != nil {
			return err
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return fmt.Errorf("creating flaresolverr directory: %w", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("creating flaresolverr parent directory: %w", err)
		}
		if err := extractFile(entry, target); err != nil {
			return err
		}
	}
	return nil
}

func safeArchivePath(destination, name string) (string, error) {
	cleanName := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(cleanName) || filepath.VolumeName(cleanName) != "" {
		return "", fmt.Errorf("flaresolverr archive contains absolute path %q", name)
	}
	target := filepath.Join(destination, cleanName)
	relative, err := filepath.Rel(destination, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("flaresolverr archive path escapes destination: %q", name)
	}
	return target, nil
}

func extractFile(entry *zip.File, destination string) error {
	source, err := entry.Open()
	if err != nil {
		return fmt.Errorf("opening flaresolverr archive entry %q: %w", entry.Name, err)
	}
	defer source.Close()

	mode := entry.Mode().Perm()
	if mode == 0 {
		mode = 0o600
	}
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("creating flaresolverr file %q: %w", entry.Name, err)
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		return fmt.Errorf("extracting flaresolverr file %q: %w", entry.Name, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing flaresolverr file %q: %w", entry.Name, closeErr)
	}
	return nil
}

func findExecutable(directory string) (string, error) {
	var executable string
	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(entry.Name(), executableName) {
			executable = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if executable == "" {
		return "", os.ErrNotExist
	}
	return executable, nil
}

func (m *Manager) waitUntilReady(ctx context.Context, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	for {
		if isListening(listenAddress) {
			return nil
		}
		select {
		case err := <-m.done:
			if err == nil {
				return errors.New("flaresolverr exited before becoming ready")
			}
			return fmt.Errorf("flaresolverr exited before becoming ready: %w", err)
		case <-ctx.Done():
			return fmt.Errorf("waiting for flaresolverr: %w", ctx.Err())
		case <-timer.C:
			return errors.New("timed out waiting for flaresolverr on 127.0.0.1:8191")
		case <-ticker.C:
		}
	}
}

func isListening(address string) bool {
	connection, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}

// Close stops only the process that this manager started.
func (m *Manager) Close() error {
	if m == nil || m.command == nil || m.command.Process == nil {
		return nil
	}
	if m.command.ProcessState != nil {
		return nil
	}
	m.stopping.Store(true)
	if m.logger != nil {
		m.logger.Info("stopping flaresolverr")
	}
	if err := m.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("stopping flaresolverr: %w", err)
	}
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
	}
	return nil
}
