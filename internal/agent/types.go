package agent

import (
	"time"
)

type SourceType string

const (
	SourceTypeCatalog SourceType = "catalog"
	SourceTypeGit     SourceType = "git"
	SourceTypeLocal   SourceType = "local"
	SourceTypeURL     SourceType = "url"
)

type AgentSource struct {
	Type     SourceType `json:"type"`
	Location string     `json:"location"`
	Branch   string     `json:"branch,omitempty"`
	Path     string     `json:"path,omitempty"`
	Raw      string     `json:"raw"`
}

type AgentMetadata struct {
	Name         string                 `yaml:"name" json:"name"`
	Version      string                 `yaml:"version" json:"version"`
	Description  string                 `yaml:"description" json:"description"`
	Author       string                 `yaml:"author,omitempty" json:"author,omitempty"`
	Language     string                 `yaml:"language" json:"language"`
	Dependencies *AgentDependencies     `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	Files        []string               `yaml:"files" json:"files"`
	Config       map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
}

type AgentDependencies struct {
	NPM []string `yaml:"npm,omitempty" json:"npm,omitempty"`
	Pip []string `yaml:"pip,omitempty" json:"pip,omitempty"`
	Go  []string `yaml:"go,omitempty" json:"go,omitempty"`
}

type AgentPackage struct {
	Source   *AgentSource      `json:"source"`
	Metadata *AgentMetadata    `json:"metadata"`
	Files    map[string][]byte `json:"files"`
	RootPath string            `json:"root_path"`
	CachedAt time.Time         `json:"cached_at"`
}

type InstallOptions struct {
	LocalName   string `json:"local_name,omitempty"`
	NoInstall   bool   `json:"no_install"`
	Force       bool   `json:"force"`
	ProjectRoot string `json:"project_root"`
}

type CacheEntry struct {
	Source    *AgentSource `json:"source"`
	Path      string       `json:"path"`
	ETag      string       `json:"etag,omitempty"`
	GitSHA    string       `json:"git_sha,omitempty"`
	CachedAt  time.Time    `json:"cached_at"`
	ExpiresAt time.Time    `json:"expires_at"`
}

type ValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}
