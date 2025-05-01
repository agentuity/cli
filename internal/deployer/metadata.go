package deployer

import (
	"os"
	"runtime"
	"strings"

	"github.com/agentuity/go-common/logger"
	"github.com/go-git/go-git/v5"
)

// GitInfo contains basic git repository information
type GitInfo struct {
	RemoteURL     string `json:"remoteUrl"`
	Branch        string `json:"branch"`
	Commit        string `json:"commit"`
	CommitMessage string `json:"commitMessage"`
	IsRepo        bool   `json:"isRepo"`
	Host          *string
}

type MetadataOrigin struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type Metadata struct {
	Origin MetadataOrigin `json:"origin,omitempty"`
}

type MachineInfo struct {
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	CPUs     int    `json:"cpus"`
	Hostname string `json:"hostname"`
	Username string `json:"username"`
}

func GetMachineInfo() *MachineInfo {
	hostname, _ := os.Hostname()
	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME")
	}

	info := MachineInfo{
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		Version:  runtime.Version(),
		CPUs:     runtime.NumCPU(),
		Hostname: hostname,
		Username: username,
	}

	return &info
}

// GetGitInfo extracts git information from a directory
func GetGitInfo(logger logger.Logger, dir string) (*GitInfo, error) {
	info := &GitInfo{}

	repo, err := git.PlainOpen(dir)
	if err != nil {
		return info, nil
	}

	// Get remote URL
	remote, err := repo.Remote("origin")
	if err == nil {
		info.RemoteURL = remote.Config().URLs[0]
		// re-write the github url to be https so they display correctly in the UI
		// git@github.com:agentuity/agent-changelog.git
		if strings.HasPrefix(info.RemoteURL, "git@github.com:") {
			info.RemoteURL = strings.Replace(info.RemoteURL, "git@github.com:", "https://github.com/", 1)
			if strings.HasSuffix(info.RemoteURL, ".git") {
				info.RemoteURL = strings.TrimSuffix(info.RemoteURL, ".git")
			}
		}
	}

	// Get current branch and commit
	head, err := repo.Head()
	if err == nil {
		info.Branch = head.Name().Short()
		info.Commit = head.Hash().String()
		info.IsRepo = true
		commit, err := repo.CommitObject(head.Hash())
		if err == nil {
			info.CommitMessage = strings.TrimSpace(commit.Message)
		}
	}
	if err != nil {
		logger.Trace(err.Error())
	}

	return info, nil
}
