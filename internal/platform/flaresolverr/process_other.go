//go:build !windows

package flaresolverr

import "os/exec"

func hiddenCommand(executable string) *exec.Cmd {
	return exec.Command(executable)
}
