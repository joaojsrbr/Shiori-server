//go:build windows

package flaresolverr

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func hiddenCommand(executable string) *exec.Cmd {
	command := exec.Command(executable)
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
	return command
}
