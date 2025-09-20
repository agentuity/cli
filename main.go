/*
Copyright © 2025 Agentuity, Inc.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package main

import (
	"runtime/debug"

	"github.com/agentuity/cli/cmd"
	"github.com/agentuity/cli/internal/bundler"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// goreleaser will set version using ldflags to the latest tag (eg. v0.0.59)
	if version == "dev" {
		// if dev use git sha (build info is only present from go build not go run)
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, s := range info.Settings {
				if s.Key == "vcs.revision" {
					version = s.Value
				}
			}
		}
	}
	cmd.Version = version
	cmd.Commit = commit
	cmd.Date = date
	errsystem.Version = version
	project.Version = version
	util.Version = version
	util.Commit = commit
	bundler.Version = version
	cmd.Execute()
}
