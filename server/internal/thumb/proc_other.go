//go:build !windows

package thumb

import "os/exec"

// configureCmd 非 Windows 平台无需额外设置（可配合 nice 运行）。
func configureCmd(cmd *exec.Cmd) {}
