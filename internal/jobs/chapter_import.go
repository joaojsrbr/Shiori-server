package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"log/slog"
	"mime"
	"net/url"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/html"

	"github.com/joaojsr/shiori-server/internal/library"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	platformstorage "github.com/joaojsr/shiori-server/internal/platform/storage"
)

type extractedUnit struct{ Number, Title, URL string }

func importExtractedUnits(ctx context.Context, b browser.Provider, files platformstorage.Provider, repo library.ChapterRepository, media *library.Media, payload ExtractPayload, result map[string]interface{}, progress func(string, any)) []string {
	field := "chapters"
	if media.Type == library.MediaTypeAnime {
		field = "episodes"
	}
	units := readExtractedUnits(result[field], payload.URL)
	warnings := make([]string, 0)
	for index, unit := range units {
		progress("progress", map[string]string{"step": "importing_chapters", "message": fmt.Sprintf("Importing %s %d of %d...", strings.TrimSuffix(field, "s"), index+1, len(units))})
		nav, err := b.Navigate(ctx, browser.NavigateRequest{URL: unit.URL, ProfileURL: payload.URL, RequiresLogin: payload.RequiresLogin, AutoScroll: payload.AutoScroll})
		if err != nil {
			warnings = append(warnings, unit.URL+": "+err.Error())
			continue
		}
		snapshot, err := b.Snapshot(ctx, nav.SessionID)
		_ = b.CloseSession(context.Background(), nav.SessionID)
		if err != nil {
			warnings = append(warnings, unit.URL+": "+err.Error())
			continue
		}

		request := library.ChapterCreateRequest{MediaID: media.ID, Number: unit.Number, Title: unit.Title, SourceURL: unit.URL}
		if media.Type == library.MediaTypeAnime {
			request.VideoURL = findVideoURL(snapshot.HTML, unit.URL)
			if request.VideoURL == "" {
				warnings = append(warnings, unit.URL+": no video URL found")
			}
		} else {
			request.Images, err = saveChapterImages(ctx, b, files, media.ID, payload.URL, unit.URL, snapshot.HTML, func(saved, total int) {
				progress("progress", map[string]string{"step": "saving_chapter_images", "message": fmt.Sprintf("Saved image %d of %d from chapter %s", saved, total, unit.Number)})
			})
			if err != nil {
				warnings = append(warnings, unit.URL+": "+err.Error())
			}
			if len(request.Images) == 0 {
				warnings = append(warnings, unit.URL+": no chapter images found")
			}
		}
		if _, err := repo.UpsertChapter(ctx, request); err != nil {
			warnings = append(warnings, unit.URL+": "+err.Error())
		}
	}
	return warnings
}

func readExtractedUnits(value interface{}, baseURL string) []extractedUnit {
	items, _ := value.([]interface{})
	base, _ := url.Parse(baseURL)
	seen := make(map[string]bool)
	result := make([]extractedUnit, 0, len(items))
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		rawURL := scalarString(item["url"])
		if rawURL == "" {
			continue
		}
		ref, err := url.Parse(rawURL)
		if err != nil {
			continue
		}
		if base != nil {
			ref = base.ResolveReference(ref)
		}
		absolute := ref.String()
		if ref.Scheme != "http" && ref.Scheme != "https" || seen[absolute] {
			continue
		}
		seen[absolute] = true
		result = append(result, extractedUnit{Number: scalarString(item["number"]), Title: scalarString(item["title"]), URL: absolute})
	}
	return result
}

func scalarString(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func saveChapterImages(ctx context.Context, b browser.Provider, files platformstorage.Provider, mediaID, profileURL, chapterURL, document string, onSaved func(int, int)) ([]library.ChapterImage, error) {
	sources := findImageSources(document, chapterURL)
	images := make([]library.ChapterImage, 0, len(sources))
	fetcher, canFetch := b.(browser.AssetFetcher)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(chapterURL)))[:20]
	var lastErr error
	for _, source := range sources {
		var data []byte
		var contentType string
		var err error
		if strings.HasPrefix(strings.ToLower(source), "data:") {
			data, contentType, err = decodeDataImage(source)
		} else if canFetch {
			data, contentType, err = fetcher.FetchAsset(ctx, profileURL, source, chapterURL)
		} else {
			continue
		}
		if err != nil {
			lastErr = err
			continue
		}
		if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
			continue
		}
		ext := extensionForImage(contentType, source)
		key := fmt.Sprintf("media/%s/chapters/%s/%04d%s", mediaID, hash, len(images)+1, ext)
		if err := files.Put(ctx, key, bytes.NewReader(data)); err != nil {
			lastErr = err
			continue
		}
		images = append(images, library.ChapterImage{Position: len(images) + 1, StorageKey: key, ContentType: contentType})
		slog.Info("chapter image saved", "chapter_url", chapterURL, "position", len(images), "storage_key", key)
		if onSaved != nil {
			onSaved(len(images), len(sources))
		}
	}
	return images, lastErr
}

func findImageSources(document, baseURL string) []string {
	doc, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return nil
	}
	base, _ := url.Parse(baseURL)
	type imageCandidate struct {
		url, group string
		score      int
	}
	seen := make(map[string]bool)
	candidates := make([]imageCandidate, 0)
	attrs := map[string]bool{"src": true, "data-src": true, "data-lazy-src": true, "data-original": true, "srcset": true, "data-srcset": true}
	addCandidate := func(raw, marker string) {
		if raw == "" {
			return
		}
		resolved := raw
		if !strings.HasPrefix(strings.ToLower(raw), "data:") {
			ref, parseErr := url.Parse(raw)
			if parseErr != nil || base == nil {
				return
			}
			resolved = base.ResolveReference(ref).String()
		}
		if seen[resolved] {
			return
		}
		score := chapterImageScore(resolved, marker)
		if score < 0 {
			return
		}
		seen[resolved] = true
		group := "data:"
		if parsed, parseErr := url.Parse(resolved); parseErr == nil && parsed.Host != "" {
			group = strings.ToLower(parsed.Scheme+"://"+parsed.Host) + path.Dir(parsed.Path)
		}
		candidates = append(candidates, imageCandidate{url: resolved, group: group, score: score})
	}
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.ElementNode && (node.Data == "img" || node.Data == "source") {
			var candidate, marker string
			for _, attr := range node.Attr {
				key := strings.ToLower(attr.Key)
				if attrs[key] && strings.TrimSpace(attr.Val) != "" && (candidate == "" || key != "src") {
					candidate = imageCandidateFromAttribute(key, attr.Val)
				}
				if key == "class" || key == "id" || key == "alt" {
					marker += " " + strings.ToLower(attr.Val)
				}
			}
			addCandidate(candidate, marker)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	// Hydration payloads often contain the complete page array even when lazy
	// loading leaves only the first few <img> nodes in the rendered HTML.
	for _, embedded := range embeddedImageURLPattern.FindAllString(strings.ReplaceAll(document, `\/`, `/`), -1) {
		addCandidate(embedded, "embedded image list")
	}
	if len(candidates) == 0 {
		return nil
	}

	type groupScore struct{ count, score int }
	groups := make(map[string]groupScore)
	for _, candidate := range candidates {
		group := groups[candidate.group]
		group.count++
		group.score += candidate.score
		groups[candidate.group] = group
	}
	bestGroup, bestValue, bestCount := "", -1, 0
	for group, value := range groups {
		rank := value.count*100 + value.score
		if rank > bestValue {
			bestGroup, bestValue, bestCount = group, rank, value.count
		}
	}
	result := make([]string, 0, bestCount)
	if bestCount >= 2 {
		for _, candidate := range candidates {
			if candidate.group == bestGroup {
				result = append(result, candidate.url)
			}
		}
		return result
	}
	// A one-page chapter is accepted only with reader/page evidence.
	for _, candidate := range candidates {
		if candidate.score >= 6 {
			result = append(result, candidate.url)
		}
	}
	return result
}

var embeddedImageURLPattern = regexp.MustCompile(`(?i)https?[^"'\\\s<>]+\.(?:avif|gif|jpe?g|png|webp)(?:\?[^"'\\\s<>]*)?`)

func chapterImageScore(source, marker string) int {
	value := strings.ToLower(source + " " + marker)
	for _, excluded := range []string{"avatar", "profile-cover", "/covers/", "cover-", "favicon", "logo", "banner", "advert", "adsbygoogle"} {
		if strings.Contains(value, excluded) {
			return -1
		}
	}
	score := 1
	for _, signal := range []string{"página", "pagina", " page ", "reader", "leitor"} {
		if strings.Contains(value, signal) {
			score += 10
			break
		}
	}
	for _, signal := range []string{"/page-", "/pages/", "/chapter", "/cap-", "/capitulo"} {
		if strings.Contains(value, signal) {
			score += 6
			break
		}
	}
	return score
}

func imageCandidateFromAttribute(key, value string) string {
	value = strings.TrimSpace(value)
	if !strings.Contains(strings.ToLower(key), "srcset") {
		return value
	}
	// Prefer the largest candidate, conventionally the final srcset entry.
	entries := strings.Split(value, ",")
	for i := len(entries) - 1; i >= 0; i-- {
		fields := strings.Fields(strings.TrimSpace(entries[i]))
		if len(fields) > 0 && fields[0] != "" {
			return fields[0]
		}
	}
	return ""
}

func decodeDataImage(source string) ([]byte, string, error) {
	comma := strings.IndexByte(source, ',')
	if comma < 0 {
		return nil, "", fmt.Errorf("invalid data image")
	}
	header, payload := source[5:comma], source[comma+1:]
	parts := strings.Split(header, ";")
	contentType := parts[0]
	if !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return nil, "", fmt.Errorf("data URL is not an image")
	}
	if strings.Contains(strings.ToLower(header), ";base64") {
		data, err := base64.StdEncoding.DecodeString(payload)
		return data, contentType, err
	}
	decoded, err := url.PathUnescape(payload)
	return []byte(decoded), contentType, err
}

func extensionForImage(contentType, source string) string {
	if extensions, _ := mime.ExtensionsByType(contentType); len(extensions) > 0 {
		return extensions[0]
	}
	if parsed, err := url.Parse(source); err == nil {
		ext := strings.ToLower(path.Ext(parsed.Path))
		if len(ext) <= 6 {
			return ext
		}
	}
	return ".img"
}

var videoURLPattern = regexp.MustCompile(`(?i)https?[^"'\\\s<>]+\.(?:m3u8|mp4|webm)(?:\?[^"'\\\s<>]*)?`)

func findVideoURL(document, baseURL string) string {
	doc, err := html.Parse(strings.NewReader(document))
	if err == nil {
		base, _ := url.Parse(baseURL)
		var found string
		var walk func(*html.Node)
		walk = func(node *html.Node) {
			if found != "" {
				return
			}
			if node.Type == html.ElementNode && (node.Data == "video" || node.Data == "source" || node.Data == "iframe") {
				for _, attr := range node.Attr {
					if strings.EqualFold(attr.Key, "src") && strings.TrimSpace(attr.Val) != "" {
						if ref, e := url.Parse(attr.Val); e == nil && base != nil {
							found = base.ResolveReference(ref).String()
						}
						return
					}
				}
			}
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				walk(child)
			}
		}
		walk(doc)
		if found != "" {
			return found
		}
	}
	return strings.ReplaceAll(videoURLPattern.FindString(document), `\/`, `/`)
}
