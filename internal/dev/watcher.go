package dev

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/agentuity/go-common/logger"
	"github.com/fsnotify/fsnotify"
)

type FileWatcher struct {
	watcher  *fsnotify.Watcher
	patterns []string
	callback func(string)
	dir      string
}

func NewWatcher(logger logger.Logger, dir string, patterns []string, callback func(string)) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	fw := &FileWatcher{
		watcher:  watcher,
		patterns: patterns,
		callback: callback,
		dir:      dir,
	}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fw.matchesPattern(logger, path) {
			logger.Trace("Adding path to watcher: %s", path)
			return watcher.Add(path)
		}
		return nil
	})

	go fw.watch(logger)
	return fw, err
}

func (fw *FileWatcher) watch(logger logger.Logger) {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			// Watch new directories
			if event.Op&fsnotify.Create == fsnotify.Create {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					fw.watcher.Add(event.Name)
				}
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				if fw.matchesPattern(logger, event.Name) {
					fw.callback(event.Name)
				}
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			// Handle error if needed
			_ = err
		}
	}
}

func (fw *FileWatcher) matchesPattern(logger logger.Logger, filename string) bool {
	for _, pattern := range fw.patterns {
		// Make pattern relative to watched directory
		if ok, _ := doubleStarMatch(logger, pattern, filename, fw.dir); ok {
			return true
		}
	}
	return false
}

func doubleStarMatch(logger logger.Logger, pattern, path, baseDir string) (bool, error) {

	// Convert absolute path to relative path from baseDir
	relPath, err := filepath.Rel(baseDir, path)
	if err != nil {
		logger.Error("Failed to get relative path: %v", err)
		return false, err
	}

	// Clean and split paths
	relPath = filepath.ToSlash(relPath)
	pattern = filepath.ToSlash(pattern)

	patternParts := strings.Split(pattern, "/")

	// Base cases
	if pattern == "**" {
		return true, nil
	}

	// If pattern ends with **, it matches any path that starts with the pattern prefix
	if patternParts[len(patternParts)-1] == "**" {
		prefix := strings.Join(patternParts[:len(patternParts)-1], "/")
		return strings.HasPrefix(relPath, prefix), nil
	}

	// If pattern starts with **, it matches any path that ends with the pattern suffix
	if patternParts[0] == "**" {
		suffix := strings.Join(patternParts[1:], "/")
		return strings.HasSuffix(relPath, suffix), nil
	}

	// Regular path matching
	matched, err := filepath.Match(pattern, relPath)
	return matched, err
}

func (fw *FileWatcher) Close(logger logger.Logger) error {
	return fw.watcher.Close()
}
