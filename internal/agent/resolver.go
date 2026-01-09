package agent

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	DefaultCatalogURL = "https://github.com/agentuity/agents"
	DefaultBranch     = "main"
)

type SourceResolver struct {
	catalogURL string
}

func NewSourceResolver() *SourceResolver {
	return &SourceResolver{
		catalogURL: DefaultCatalogURL,
	}
}

func NewSourceResolverWithCatalog(catalogURL string) *SourceResolver {
	return &SourceResolver{
		catalogURL: catalogURL,
	}
}

func (r *SourceResolver) Resolve(source string) (*AgentSource, error) {
	if source == "" {
		return nil, fmt.Errorf("source cannot be empty")
	}

	originalSource := source

	// Check if it's a local path
	if r.isLocalPath(source) {
		return &AgentSource{
			Type:     SourceTypeLocal,
			Location: source,
			Path:     "",
			Raw:      originalSource,
		}, nil
	}

	// Check if it's a URL
	if r.isURL(source) {
		return r.parseURL(source, originalSource)
	}

	// Check if it's a Git repository reference
	if r.isGitRepo(source) {
		return r.parseGitRepo(source, originalSource)
	}

	// Assume it's a catalog reference
	return r.parseCatalogRef(source, originalSource)
}

func (r *SourceResolver) isLocalPath(source string) bool {
	return strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../") ||
		strings.HasPrefix(source, "/") ||
		strings.HasPrefix(source, "~")
}

func (r *SourceResolver) isURL(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}

func (r *SourceResolver) isGitRepo(source string) bool {
	// Match patterns like: github.com/user/repo, gitlab.com/user/repo, etc.
	gitPattern := regexp.MustCompile(`^([a-zA-Z0-9.-]+\.[a-zA-Z]{2,})/([a-zA-Z0-9._-]+)/([a-zA-Z0-9._-]+)`)
	return gitPattern.MatchString(source)
}

func (r *SourceResolver) parseURL(source, originalSource string) (*AgentSource, error) {
	u, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Extract branch from fragment
	branch := ""
	if u.Fragment != "" {
		branch = u.Fragment
		u.Fragment = ""
	}

	return &AgentSource{
		Type:     SourceTypeURL,
		Location: u.String(),
		Branch:   branch,
		Path:     "",
		Raw:      originalSource,
	}, nil
}

func (r *SourceResolver) parseGitRepo(source, originalSource string) (*AgentSource, error) {
	parts := strings.Split(source, " ")
	gitPart := parts[0]
	agentPath := ""

	if len(parts) > 1 {
		agentPath = strings.Join(parts[1:], " ")
	}

	// Parse branch from gitPart
	branch := DefaultBranch
	if strings.Contains(gitPart, "#") {
		gitBranchParts := strings.Split(gitPart, "#")
		gitPart = gitBranchParts[0]
		if len(gitBranchParts) > 1 {
			branch = gitBranchParts[1]
		}
	}

	// Ensure HTTPS URL format
	gitURL := fmt.Sprintf("https://%s", gitPart)

	return &AgentSource{
		Type:     SourceTypeGit,
		Location: gitURL,
		Branch:   branch,
		Path:     agentPath,
		Raw:      originalSource,
	}, nil
}

func (r *SourceResolver) parseCatalogRef(source, originalSource string) (*AgentSource, error) {
	// Parse catalog reference like "memory/vector-store"
	parts := strings.Split(source, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid catalog reference: %s (expected format: category/agent-name)", source)
	}

	// Validate catalog reference format
	catalogPattern := regexp.MustCompile(`^[a-zA-Z0-9._-]+/[a-zA-Z0-9._-]+$`)
	if !catalogPattern.MatchString(source) {
		return nil, fmt.Errorf("invalid catalog reference format: %s", source)
	}

	return &AgentSource{
		Type:     SourceTypeCatalog,
		Location: r.catalogURL,
		Branch:   DefaultBranch,
		Path:     source,
		Raw:      originalSource,
	}, nil
}

func (r *SourceResolver) GetCacheKey(source *AgentSource) string {
	switch source.Type {
	case SourceTypeLocal:
		abs, _ := filepath.Abs(source.Location)
		return fmt.Sprintf("local_%s", abs)
	case SourceTypeURL:
		return fmt.Sprintf("url_%s_%s", source.Location, source.Branch)
	case SourceTypeGit:
		return fmt.Sprintf("git_%s_%s_%s", source.Location, source.Branch, source.Path)
	case SourceTypeCatalog:
		return fmt.Sprintf("catalog_%s_%s_%s", source.Location, source.Branch, source.Path)
	default:
		return fmt.Sprintf("unknown_%s", source.Raw)
	}
}

func (r *SourceResolver) GetDownloadURL(source *AgentSource) (string, error) {
	switch source.Type {
	case SourceTypeURL:
		return source.Location, nil
	case SourceTypeGit, SourceTypeCatalog:
		// Convert to GitHub archive URL
		u, err := url.Parse(source.Location)
		if err != nil {
			return "", fmt.Errorf("invalid repository URL: %w", err)
		}

		// Extract owner and repo from path
		pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(pathParts) < 2 {
			return "", fmt.Errorf("invalid repository path: %s", u.Path)
		}

		owner := pathParts[0]
		repo := pathParts[1]
		branch := source.Branch
		if branch == "" {
			branch = DefaultBranch
		}

		return fmt.Sprintf("https://%s/%s/%s/archive/%s.zip", u.Host, owner, repo, branch), nil
	case SourceTypeLocal:
		return "", fmt.Errorf("local sources don't have download URLs")
	default:
		return "", fmt.Errorf("unsupported source type: %s", source.Type)
	}
}
