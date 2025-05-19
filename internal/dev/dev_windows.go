//go:build windows
// +build windows

package dev

import (
	"fmt"
	"os/exec"
	"unsafe"

	"github.com/agentuity/go-common/logger"
	"golang.org/x/sys/windows"
)

// getProcessTree returns all descendant PIDs for a given parent PID
func getProcessTree(logger logger.Logger, pid int) ([]int, error) {
	var pids []int

	// Create a snapshot of all processes
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to create process snapshot: %v", err)
	}
	defer windows.CloseHandle(snapshot)

	// Initialize process entry
	var processEntry windows.ProcessEntry32
	processEntry.Size = uint32(unsafe.Sizeof(processEntry))

	// Get first process
	err = windows.Process32First(snapshot, &processEntry)
	if err != nil {
		return nil, fmt.Errorf("failed to get first process: %v", err)
	}

	// Create a map to track all processes and their parent-child relationships
	processMap := make(map[uint32][]uint32)
	processNames := make(map[uint32]string)

	// First pass: build the process tree
	for {
		parentID := processEntry.ParentProcessID
		processID := processEntry.ProcessID

		// Convert the process name from UTF-16 to string
		name := windows.UTF16ToString(processEntry.ExeFile[:])

		// Skip the System process (PID 4) as it's a special case
		if processID != 4 {
			processMap[parentID] = append(processMap[parentID], processID)
			processNames[processID] = name
		}

		err = windows.Process32Next(snapshot, &processEntry)
		if err != nil {
			break
		}
	}

	// Function to recursively get all descendants
	var getDescendants func(parentID uint32, depth int)
	getDescendants = func(parentID uint32, depth int) {
		children, exists := processMap[parentID]
		if !exists {
			return
		}

		for _, childID := range children {
			// Skip the System process (PID 4) and the parent process itself
			if childID != 4 && childID != uint32(pid) {
				pids = append(pids, int(childID))
				// Log the process tree structure
				logger.Debug("Found process: %s (pid: %d, parent: %d, depth: %d)",
					processNames[childID], childID, parentID, depth)
				getDescendants(childID, depth+1)
			}
		}
	}

	// Start the recursive search from the given PID
	logger.Debug("Starting process tree search from PID: %d", pid)
	getDescendants(uint32(pid), 0)

	return pids, nil
}

// kill terminates a process by PID
func kill(logger logger.Logger, pid int) error {
	// Open the process with terminate access
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("failed to open process: %v", err)
	}
	defer windows.CloseHandle(handle)

	// Get process name for logging
	var name [windows.MAX_PATH]uint16
	var size uint32 = windows.MAX_PATH
	err = windows.QueryFullProcessImageName(handle, 0, &name[0], &size)
	processName := "unknown"
	if err == nil {
		processName = windows.UTF16ToString(name[:size])
	}

	logger.Debug("Killing process: %s (pid: %d)", processName, pid)

	// Terminate the process
	err = windows.TerminateProcess(handle, 1)
	if err != nil {
		return fmt.Errorf("failed to terminate process: %v", err)
	}

	return nil
}

func terminateProcess(logger logger.Logger, cmd *exec.Cmd) error {
	logger.Debug("terminateProcess: %s", cmd)
	if cmd.Process != nil {
		// Create a job object
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			return fmt.Errorf("failed to create job object: %v", err)
		}
		defer windows.CloseHandle(job)

		// Configure the job object to terminate all processes when the job is terminated
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}

		// Set the job object information
		_, err = windows.SetInformationJobObject(
			job,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		)
		if err != nil {
			return fmt.Errorf("failed to set job object information: %v", err)
		}

		// Assign the process to the job object
		err = windows.AssignProcessToJobObject(job, windows.Handle(cmd.Process.Pid))
		if err != nil {
			return fmt.Errorf("failed to assign process to job object: %v", err)
		}

		// Terminate the job object, which will kill all processes in the job
		err = windows.TerminateJobObject(job, 1)
		if err != nil {
			return fmt.Errorf("failed to terminate job object: %v", err)
		}
	}
	return nil
}
