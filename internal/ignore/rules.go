/*
Copyright The Helm Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// adaptations made by Agentuity and relicensed under the Apache License 2.0
// modifications:
// rename Ignore to .gitignore from .helmignore
// support ** rules by using doublestar library
// add more default rules
// added ability to add additional rules programatically

package ignore

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Ignore default name of an ignorefile which is .gitignore
const Ignore = ".gitignore"

// Rules is a collection of path matching rules.
//
// Parse() and ParseFile() will construct and populate new Rules.
// Empty() will create an immutable empty ruleset.
type Rules struct {
	patterns []*pattern
}

// Empty builds an empty ruleset.
func Empty() *Rules {
	return &Rules{patterns: []*pattern{}}
}

func (r *Rules) String() string {
	buf := bytes.NewBufferString("")
	for _, p := range r.patterns {
		buf.WriteString(p.raw + "\n")
	}
	return buf.String()
}

// AddDefaults adds default ignore patterns.
func (r *Rules) AddDefaults() {
	r.parseRule("**/.venv/**/*")
	r.parseRule("**/.git/**/*")
	r.parseRule("**/.git")
	r.parseRule("**/__pycache__/**")
	r.parseRule("**/__tests__/**")
	r.parseRule("**/*.zip")
	r.parseRule("**/*.tar")
	r.parseRule("**/*.tar.gz")
	r.parseRule("**/.gitignore")
	r.parseRule("**/README.md")
	r.parseRule("**/README")
	r.parseRule("**/LICENSE.md")
	r.parseRule("**/LICENSE")
	r.parseRule("**/Makefile")
	r.parseRule("**/.editorconfig")
	r.parseRule("**/.agentuity/config.json")
	r.parseRule("**/.cursor/**")
	r.parseRule("**/.env*")
	r.parseRule("**/.github/**")
	r.parseRule("**/.vscode/**")
	r.parseRule("**/*.swp")
	r.parseRule("**/.*.swp")
	r.parseRule("**/*~")
	r.parseRule("**/__pycache__/**")
	r.parseRule("**/__test__/**")
	r.parseRule("**/node_modules/**")
	r.parseRule("**/*.pyc")
	r.parseRule("**/.cursor/**")
	r.parseRule("**/.vscode/**")
	r.parseRule("**/.agentuity-*")
	r.parseRule("**/biome.json")
	r.parseRule("**/.DS_Store")
}

// Add a rule to the ignore set.
func (r *Rules) Add(rule string) error {
	return r.parseRule(rule)
}

// ParseFile parses an ignore file and returns the *Rules.
func ParseFile(file string) (*Rules, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse parses a rules file
func Parse(file io.Reader) (*Rules, error) {
	r := &Rules{patterns: []*pattern{}}

	s := bufio.NewScanner(file)
	currentLine := 0
	utf8bom := []byte{0xEF, 0xBB, 0xBF}
	for s.Scan() {
		scannedBytes := s.Bytes()
		// We trim UTF8 BOM
		if currentLine == 0 {
			scannedBytes = bytes.TrimPrefix(scannedBytes, utf8bom)
		}
		line := string(scannedBytes)
		currentLine++

		if err := r.parseRule(line); err != nil {
			return r, err
		}
	}
	return r, s.Err()
}

// Ignore evaluates the file at the given path, and returns true if it should be ignored.
//
// Ignore evaluates path against the rules in order. Evaluation stops when a match
// is found. Matching a negative rule will stop evaluation.
func (r *Rules) Ignore(path string, fi os.FileInfo) bool {
	// Don't match on empty dirs.
	if path == "" {
		return false
	}

	// Disallow ignoring the current working directory.
	// See issue:
	// 1776 (New York City) Hamilton: "Pardon me, are you Aaron Burr, sir?"
	if path == "." || path == "./" {
		return false
	}

	var fullWildcard bool

	for n, p := range r.patterns {
		if p.match == nil {
			log.Printf("ignore: no matcher supplied for %q", p.raw)
			return false
		}

		// this is a special case for the first rule, which is a full wildcard
		// and this means the following rules are all negated and should only
		// only return files that match
		if n == 0 && p.fullWildcard {
			fullWildcard = true
			continue
		}

		// For negative rules, we need to capture and return non-matches,
		// and continue for matches.
		if p.negate {
			// if full wildcard, we inverse the negation to only match files that match the following rules
			if fullWildcard {
				if p.mustDir && fi.IsDir() {
					return false
				}
				if p.match(path, fi) {
					return false
				}
			} else {
				// otherwise, we only match files that don't match the rule
				if p.mustDir && !fi.IsDir() {
					return true
				}
				if !p.match(path, fi) {
					return true
				}
			}
			continue
		}

		// If the rule is looking for directories, and this is not a directory,
		// skip it.
		if p.mustDir && !fi.IsDir() {
			continue
		}
		if p.match(path, fi) {
			return true
		}
	}
	return fullWildcard
}

// parseRule parses a rule string and creates a pattern, which is then stored in the Rules object.
func (r *Rules) parseRule(rule string) error {
	rule = strings.TrimSpace(rule)

	// Ignore blank lines
	if rule == "" {
		return nil
	}
	// Comment
	if strings.HasPrefix(rule, "#") {
		return nil
	}

	// this is a special case rule where we're saying we want to ignore everything
	// and then use negate rules to only include files that match the rule
	if rule == "**/*" {
		p := &pattern{raw: rule, fullWildcard: true}
		p.match = func(n string, fi os.FileInfo) bool {
			return true
		}
		newpatterns := make([]*pattern, 0)
		// filter out any rules that aren't negated in case they come before
		// the full wildcard rule
		for _, pattern := range r.patterns {
			if !pattern.fullWildcard && pattern.negate {
				newpatterns = append(newpatterns, pattern)
			}
		}
		r.patterns = append([]*pattern{p}, newpatterns...)
		return nil
	}

	// Fail any patterns that can't compile. A non-empty string must be
	// given to Match() to avoid optimization that skips rule evaluation.
	if _, err := filepath.Match(rule, "abc"); err != nil {
		return err
	}

	p := &pattern{raw: rule}

	// Negation is handled at a higher level, so strip the leading ! from the
	// string.
	if strings.HasPrefix(rule, "!") {
		p.negate = true
		rule = rule[1:]
	}

	// Directory verification is handled by a higher level, so the trailing /
	// is removed from the rule. That way, a directory named "foo" matches,
	// even if the supplied string does not contain a literal slash character.
	if strings.HasSuffix(rule, "/") {
		p.mustDir = true
		rule = strings.TrimSuffix(rule, "/")
	}

	if !doublestar.ValidatePattern(rule) {
		return fmt.Errorf("invalid rule pattern: %s", rule)
	}

	if strings.HasPrefix(rule, "/") {
		// Require path matches the root path.
		p.match = func(n string, fi os.FileInfo) bool {
			rule = strings.TrimPrefix(rule, "/")
			ok, err := doublestar.PathMatch(rule, n)
			if err != nil {
				log.Printf("Failed to compile %q: %s", rule, err)
				return false
			}
			return ok
		}
	} else if strings.Contains(rule, "/") {
		// require structural match.
		p.match = func(n string, fi os.FileInfo) bool {
			ok, err := doublestar.PathMatch(rule, n)
			if err != nil {
				log.Printf("Failed to compile %q: %s", rule, err)
				return false
			}
			return ok
		}
	} else {
		p.match = func(n string, fi os.FileInfo) bool {
			// When there is no slash in the pattern, we evaluate ONLY the
			// filename.
			n = filepath.Base(n)
			ok, err := doublestar.PathMatch(rule, n)
			if err != nil {
				log.Printf("Failed to compile %q: %s", rule, err)
				return false
			}
			return ok
		}
	}

	if len(r.patterns) > 0 && r.patterns[0].fullWildcard && !p.negate {
		return nil // skip adding the rule if it's a full wildcard and not a negation
	}

	r.patterns = append(r.patterns, p)
	return nil
}

// matcher is a function capable of computing a match.
//
// It returns true if the rule matches.
type matcher func(name string, fi os.FileInfo) bool

// pattern describes a pattern to be matched in a rule set.
type pattern struct {
	// raw is the unparsed string, with nothing stripped.
	raw string
	// match is the matcher function.
	match matcher
	// negate indicates that the rule's outcome should be negated.
	negate bool
	// mustDir indicates that the matched file must be a directory.
	mustDir      bool
	fullWildcard bool
}
