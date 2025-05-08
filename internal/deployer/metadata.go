package deployer

import (
	"os"
	"runtime"
	"strings"

	"github.com/agentuity/go-common/logger"
	"github.com/go-git/go-git/v5"
)

type CIInfo struct {
	LogsURL string `json:"logsUrl"`
}

// GitInfo contains basic git repository information
type GitInfo struct {
	RemoteURL     *string `json:"remoteUrl"`
	Branch        *string `json:"branch"`
	Commit        *string `json:"commit"`
	CommitMessage *string `json:"commitMessage"`
	IsRepo        bool    `json:"isRepo"`
	GitProvider   *string `json:"gitProvider"`
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
	if err == nil && len(remote.Config().URLs) > 0 {
		remoteURL := remote.Config().URLs[0]
		// re-write the github url to be https so they display correctly in the UI
		if strings.HasPrefix(remoteURL, "git@github.com:") {
			remoteURL = strings.Replace(remoteURL, "git@github.com:", "https://github.com/", 1)
			if strings.HasSuffix(remoteURL, ".git") {
				remoteURL = strings.TrimSuffix(remoteURL, ".git")
			}
		}
		info.RemoteURL = &remoteURL
	}

	// Get current branch and commit
	head, err := repo.Head()
	if err == nil {
		branch := head.Name().Short()
		commitHash := head.Hash().String()
		info.Branch = &branch
		info.Commit = &commitHash
		info.IsRepo = true
		commit, err := repo.CommitObject(head.Hash())
		if err == nil {
			msg := strings.TrimSpace(commit.Message)
			info.CommitMessage = &msg
		}
	}
	if err != nil {
		logger.Trace(err.Error())
	}

	return info, nil
}

// GetGitInfoRecursive walks up directories until it finds a git repo and returns its info
func GetGitInfoRecursive(logger logger.Logger, startDir string) (*GitInfo, error) {
	depth := 0
	dir := startDir
	for {
		if depth >= 100 {
			logger.Warn("Max depth reached while trying to find git dir")
			return &GitInfo{}, nil
		}
		info, err := GetGitInfo(logger, dir)
		if err != nil {
			return nil, err
		}
		if info != nil && info.IsRepo {
			return info, nil
		}

		parent := parentDir(dir)
		if parent == dir {
			break
		}
		dir = parent
		depth++
	}
	return &GitInfo{}, nil
}

// parentDir returns the parent directory of the given path
func parentDir(path string) string {
	if path == "/" {
		return path
	}
	cleaned := strings.TrimRight(path, "/")
	idx := strings.LastIndex(cleaned, "/")
	if idx <= 0 {
		return "/"
	}
	return cleaned[:idx]
}
