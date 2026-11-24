//go:build windows

package thumb

import (
	"os/exec"
	"syscall"
)

// configureCmd 让 ffmpeg 以隐藏窗口 + 最低优先级运行，避免干扰前台应用（N-01/N-03）。
func configureCmd(cmd *exec.Cmd) {
	const idlePriorityClass = 0x00000040
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: idlePriorityClass,
	}
}
