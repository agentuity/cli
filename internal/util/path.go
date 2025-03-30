package util

import (
	"os"
	"os/exec"
	"strings"
)

func GetAgentuityCommand() string {
	exe, _ := os.Executable()
	if !strings.Contains(exe, "agentuity") {
		exe, _ = exec.LookPath("agentuity")
	}
	return exe
}

func GetFormattedMCPCommand() string {
	exe := GetAgentuityCommand()
	
	return exe
}
