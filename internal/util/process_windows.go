//go:build windows

package util

import (
	"os/exec"
	"syscall"
)

func ProcessKill(projectServerCmd *exec.Cmd) {
	if projectServerCmd.Process != nil {
		// Get the process handle using syscall
		handle, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, uint32(projectServerCmd.Process.Pid))
		if err == nil {
			syscall.TerminateProcess(handle, 0)
			syscall.CloseHandle(handle)
		}
		projectServerCmd.Process.Release()
	}
}

func ProcessSetup(cmd *exec.Cmd) {
	// set this process as the parent of the process group since it will likely span child processes
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000,
		HideWindow:    true,
	}
}
