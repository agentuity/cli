package dev

import (
	"os"
	"path/filepath"
	"time"

	"github.com/agentuity/cli/internal/ignore"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	"github.com/fsnotify/fsnotify"
)

const debugLogging = false

type FileWatcher struct {
	watcher  *fsnotify.Watcher
	ignore   *ignore.Rules
	callback func(string)
	dir      string
}

var ignorePatterns = []string{
	"**/.agentuity/**",
	"**/package-lock.json",
	"**/package.json",
	"**/yarn.lock",
	"**/pnpm-lock.yaml",
	"**/bun.lock",
	"**/bun.lockb",
	"**/tsconfig.json",
	"**/agentuity.yaml",
	"**/.agentuity",
}

func NewWatcher(logger logger.Logger, dir string, rules *ignore.Rules, callback func(string)) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	for _, pattern := range ignorePatterns {
		rules.Add(pattern)
	}

	fw := &FileWatcher{
		watcher:  watcher,
		callback: callback,
		dir:      dir,
		ignore:   rules,
	}

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if rules.Ignore(path, info) {
			if debugLogging {
				logger.Trace("Ignoring path: %s", path)
			}
			return nil
		}
		logger.Trace("Adding path to watcher: %s", path)
		fw.watcher.Add(path)
		return nil
	})

	go fw.watch(logger)
	return fw, err
}

func (fw *FileWatcher) watch(logger logger.Logger) {
	t := time.NewTicker(250 * time.Millisecond) // how long to debounce changes
	defer t.Stop()
	pending := make(map[string]bool)
	for {
		select {
		case <-t.C:
			for path := range pending {
				fw.callback(path)
				delete(pending, path)
			}
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			logger.Trace("Event: %s => %s", event.Op, event.Name)
			if !util.Exists(event.Name) {
				logger.Trace("File %s no longer exists", event.Name)
				continue
			}
			fi, err := os.Stat(event.Name)
			if err != nil {
				logger.Error("Error statting %s: %s", event.Name, err)
				continue
			}
			// Watch new directories
			if event.Op&fsnotify.Create == fsnotify.Create {
				if fi.IsDir() && !fw.ignore.Ignore(event.Name, fi) {
					logger.Trace("Adding directory to watcher: %s", event.Name)
					fw.watcher.Add(event.Name)
				}
			}
			if (event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create) && !fw.ignore.Ignore(event.Name, fi) {
				logger.Trace("Write detected for %s", event.Name)
				pending[event.Name] = true
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			logger.Error("watcher error: %s", err)
		}
	}
}

func (fw *FileWatcher) Close(logger logger.Logger) error {
	return fw.watcher.Close()
}
