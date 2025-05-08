//go:build !windows
// +build !windows

package dev

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/agentuity/go-common/logger"
)

func terminateProcess(logger logger.Logger, cmd *exec.Cmd) error {
	logger.Debug("terminateProcess: %s", cmd)
	if cmd.Process != nil {
		// Get the process group ID (negative PID)
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err != nil {
			// If we can't get the process group, just kill the process directly
			cmd.Process.Signal(syscall.SIGINT)
		} else {
			// Kill the entire process group
			syscall.Kill(-pgid, syscall.SIGINT)
		}

		// Wait a short time for graceful shutdown
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()

		select {
		case <-time.After(5 * time.Second):
			// If process hasn't terminated, use SIGKILL on the process group
			if err == nil {
				// Kill the entire process group with SIGKILL
				syscall.Kill(-pgid, syscall.SIGKILL)
			} else {
				// Fallback to just killing the process
				cmd.Process.Signal(syscall.SIGKILL)
			}
		case <-done:
			// Process terminated gracefully
		}
	}
	return nil
}

// getProcessTree returns a list of all descendant PIDs of the given parent PID.
func getProcessTree(logger logger.Logger, parentPID int) ([]int, error) {
	logger.Debug("getting process tree for parent (pid: %d)", parentPID)
	cmd := exec.Command("ps", "-eo", "pid,ppid") // works on both macOS and Linux
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		logger.Debug("failed to run ps: %s", err)
		return nil, fmt.Errorf("failed to run ps: %w", err)
	}

	lines := strings.Split(out.String(), "\n")
	pidMap := make(map[int][]int) // PPID -> []PID

	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}

		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}

		pidMap[ppid] = append(pidMap[ppid], pid)
	}

	// Recursively collect descendants
	var collect func(int)
	descendants := []int{}
	collect = func(ppid int) {
		for _, child := range pidMap[ppid] {
			descendants = append(descendants, child)
			collect(child)
		}
	}
	collect(parentPID)

	return descendants, nil
}

func kill(logger logger.Logger, pid int) error {
	logger.Debug("killing process (pid: %d)", pid)
	return syscall.Kill(pid, syscall.SIGTERM)
}
