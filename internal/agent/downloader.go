package agent

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/util"
	"gopkg.in/yaml.v3"
)

const (
	DefaultCacheTTL = 24 * time.Hour
	MaxPackageSize  = 100 * 1024 * 1024 // 100MB
)

type AgentDownloader struct {
	cacheDir   string
	httpClient *http.Client
	resolver   *SourceResolver
}

func NewAgentDownloader(cacheDir string) *AgentDownloader {
	return &AgentDownloader{
		cacheDir: cacheDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		resolver: NewSourceResolver(),
	}
}

func (d *AgentDownloader) Download(source *AgentSource) (*AgentPackage, error) {
	// Check cache first
	if pkg, err := d.getFromCache(source); err == nil && pkg != nil {
		return pkg, nil
	}

	switch source.Type {
	case SourceTypeLocal:
		return d.downloadLocal(source)
	case SourceTypeURL, SourceTypeGit, SourceTypeCatalog:
		return d.downloadRemote(source)
	default:
		return nil, fmt.Errorf("unsupported source type: %s", source.Type)
	}
}

func (d *AgentDownloader) downloadLocal(source *AgentSource) (*AgentPackage, error) {
	if !util.Exists(source.Location) {
		return nil, fmt.Errorf("local path does not exist: %s", source.Location)
	}

	// For local sources, we don't cache but read directly
	return d.loadPackageFromPath(source, source.Location)
}

func (d *AgentDownloader) downloadRemote(source *AgentSource) (*AgentPackage, error) {
	downloadURL, err := d.resolver.GetDownloadURL(source)
	if err != nil {
		return nil, fmt.Errorf("failed to get download URL: %w", err)
	}

	// Create cache directory
	cacheKey := d.resolver.GetCacheKey(source)
	cacheDir := filepath.Join(d.cacheDir, cacheKey)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Download and extract
	zipPath := filepath.Join(cacheDir, "source.zip")
	extractPath := filepath.Join(cacheDir, "extracted")

	if err := d.downloadFile(downloadURL, zipPath); err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}

	if err := d.extractZip(zipPath, extractPath); err != nil {
		return nil, fmt.Errorf("failed to extract: %w", err)
	}

	// Find the agent path within the extracted content
	agentPath, err := d.findAgentPath(extractPath, source)
	if err != nil {
		return nil, fmt.Errorf("failed to find agent: %w", err)
	}

	// Load the package
	pkg, err := d.loadPackageFromPath(source, agentPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load package: %w", err)
	}

	// Save cache entry
	if err := d.saveToCache(source, cacheDir, pkg); err != nil {
		// Log warning but don't fail
		fmt.Printf("Warning: failed to save to cache: %v\n", err)
	}

	return pkg, nil
}

func (d *AgentDownloader) downloadFile(url, filepath string) error {
	resp, err := d.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}

	// Check content length
	if resp.ContentLength > MaxPackageSize {
		return fmt.Errorf("package too large: %d bytes (max %d)", resp.ContentLength, MaxPackageSize)
	}

	file, err := os.Create(filepath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy with size limit
	_, err = io.CopyN(file, resp.Body, MaxPackageSize)
	if err != nil && err != io.EOF {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

func (d *AgentDownloader) extractZip(zipPath, extractPath string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	if err := os.MkdirAll(extractPath, 0755); err != nil {
		return fmt.Errorf("failed to create extract directory: %w", err)
	}

	for _, file := range reader.File {
		// Security check: prevent path traversal
		if strings.Contains(file.Name, "..") {
			continue
		}

		path := filepath.Join(extractPath, file.Name)

		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.FileInfo().Mode())
			continue
		}

		// Create directory for file
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}

		// Extract file
		fileReader, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file in zip: %w", err)
		}
		defer fileReader.Close()

		targetFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.FileInfo().Mode())
		if err != nil {
			return fmt.Errorf("failed to create target file: %w", err)
		}
		defer targetFile.Close()

		_, err = io.Copy(targetFile, fileReader)
		if err != nil {
			return fmt.Errorf("failed to copy file contents: %w", err)
		}
	}

	return nil
}

func (d *AgentDownloader) findAgentPath(extractPath string, source *AgentSource) (string, error) {
	if source.Path == "" {
		// Look for agent.yaml in the root
		agentYaml := filepath.Join(extractPath, "agent.yaml")
		if util.Exists(agentYaml) {
			return extractPath, nil
		}

		// If it's a GitHub archive, there should be a single directory
		entries, err := os.ReadDir(extractPath)
		if err != nil {
			return "", fmt.Errorf("failed to read extract directory: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				candidatePath := filepath.Join(extractPath, entry.Name())
				agentYaml := filepath.Join(candidatePath, "agent.yaml")
				if util.Exists(agentYaml) {
					return candidatePath, nil
				}
			}
		}

		return "", fmt.Errorf("agent.yaml not found in extracted content")
	}

	// For catalog sources, the path is relative to the repository root
	if source.Type == SourceTypeCatalog {
		// Find the repository root (should be the single directory in extract)
		entries, err := os.ReadDir(extractPath)
		if err != nil {
			return "", fmt.Errorf("failed to read extract directory: %w", err)
		}

		var repoRoot string
		for _, entry := range entries {
			if entry.IsDir() {
				repoRoot = filepath.Join(extractPath, entry.Name())
				break
			}
		}

		if repoRoot == "" {
			return "", fmt.Errorf("repository root not found")
		}

		agentPath := filepath.Join(repoRoot, source.Path)
		agentYaml := filepath.Join(agentPath, "agent.yaml")

		if !util.Exists(agentYaml) {
			return "", fmt.Errorf("agent.yaml not found at path: %s", source.Path)
		}

		return agentPath, nil
	}

	// For git sources with path
	agentPath := filepath.Join(extractPath, source.Path)
	agentYaml := filepath.Join(agentPath, "agent.yaml")

	if !util.Exists(agentYaml) {
		return "", fmt.Errorf("agent.yaml not found at path: %s", source.Path)
	}

	return agentPath, nil
}

func (d *AgentDownloader) loadPackageFromPath(source *AgentSource, agentPath string) (*AgentPackage, error) {
	// Load agent.yaml
	metadataPath := filepath.Join(agentPath, "agent.yaml")
	if !util.Exists(metadataPath) {
		return nil, fmt.Errorf("agent.yaml not found at: %s", metadataPath)
	}

	metadataBytes, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read agent.yaml: %w", err)
	}

	var metadata AgentMetadata
	if err := yaml.Unmarshal(metadataBytes, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse agent.yaml: %w", err)
	}

	// Load all files specified in metadata
	files := make(map[string][]byte)
	for _, file := range metadata.Files {
		filePath := filepath.Join(agentPath, file)
		if !util.Exists(filePath) {
			return nil, fmt.Errorf("file not found: %s", file)
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", file, err)
		}

		files[file] = content
	}

	return &AgentPackage{
		Source:   source,
		Metadata: &metadata,
		Files:    files,
		RootPath: agentPath,
		CachedAt: time.Now(),
	}, nil
}

func (d *AgentDownloader) getFromCache(source *AgentSource) (*AgentPackage, error) {
	cacheKey := d.resolver.GetCacheKey(source)
	cachePath := filepath.Join(d.cacheDir, cacheKey, "cache.json")

	if !util.Exists(cachePath) {
		return nil, fmt.Errorf("cache not found")
	}

	cacheBytes, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache: %w", err)
	}

	var entry CacheEntry
	if err := json.Unmarshal(cacheBytes, &entry); err != nil {
		return nil, fmt.Errorf("failed to parse cache: %w", err)
	}

	// Check if cache is expired
	if time.Now().After(entry.ExpiresAt) {
		return nil, fmt.Errorf("cache expired")
	}

	// Load package from cached path
	return d.loadPackageFromPath(source, entry.Path)
}

func (d *AgentDownloader) saveToCache(source *AgentSource, cacheDir string, pkg *AgentPackage) error {
	entry := CacheEntry{
		Source:    source,
		Path:      pkg.RootPath,
		CachedAt:  time.Now(),
		ExpiresAt: time.Now().Add(DefaultCacheTTL),
	}

	cacheBytes, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	cachePath := filepath.Join(cacheDir, "cache.json")
	if err := os.WriteFile(cachePath, cacheBytes, 0644); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}

	return nil
}

func (d *AgentDownloader) hashSource(source *AgentSource) string {
	data := fmt.Sprintf("%s:%s:%s:%s", source.Type, source.Location, source.Branch, source.Path)
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)[:16]
}
