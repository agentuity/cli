//go:build windows
// +build windows

package util

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWindowsProcessSetupAndKill(t *testing.T) {
	cmd := exec.Command("timeout", "10")
	
	ProcessSetup(cmd)
	
	err := cmd.Start()
	assert.NoError(t, err)
	
	assert.NotNil(t, cmd.Process)
	assert.NotEqual(t, 0, cmd.Process.Pid)
	
	time.Sleep(100 * time.Millisecond)
	
	ProcessKill(cmd)
	
	time.Sleep(100 * time.Millisecond)
	
	_, err = os.FindProcess(cmd.Process.Pid)
	
	cmd.Process.Release()
}
