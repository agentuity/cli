package bundler

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/sys"
	"github.com/evanw/esbuild/pkg/api"
	"gopkg.in/yaml.v3"
)

func makePath(args api.OnResolveArgs) string {
	p := args.Path
	if !filepath.IsAbs(p) {
		p = filepath.Join(args.ResolveDir, p)
	} else {
		p = filepath.Clean(p)
	}
	return p
}

func isNodeModulesPath(p string) bool {
	return strings.Contains(filepath.ToSlash(p), "/node_modules/")
}

func createYAMLImporter(logger logger.Logger) api.Plugin {
	return api.Plugin{
		Name: "yaml",
		Setup: func(build api.PluginBuild) {
			filter := "\\.ya?ml$"
			build.OnResolve(api.OnResolveOptions{Filter: filter, Namespace: "file"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				p := makePath(args)
				if isNodeModulesPath(p) {
					return api.OnResolveResult{}, nil
				}
				return api.OnResolveResult{Path: p, Namespace: "yaml"}, nil
			})

			build.OnLoad(api.OnLoadOptions{Filter: filter, Namespace: "yaml"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
				of, err := os.Open(args.Path)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				defer of.Close()
				var kv any
				err = yaml.NewDecoder(of).Decode(&kv)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				js := "export default " + cstr.JSONStringify(kv)
				logger.Debug("bundling yaml file from %s", args.Path)
				return api.OnLoadResult{Contents: &js, Loader: api.LoaderJS}, nil
			})

		},
	}

}

func createJSONImporter(logger logger.Logger) api.Plugin {
	return api.Plugin{
		Name: "json",
		Setup: func(build api.PluginBuild) {
			filter := "\\.json$"
			build.OnResolve(api.OnResolveOptions{Filter: filter, Namespace: "file"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				p := makePath(args)
				if isNodeModulesPath(p) {
					return api.OnResolveResult{}, nil
				}
				return api.OnResolveResult{Path: p, Namespace: "json"}, nil
			})

			build.OnLoad(api.OnLoadOptions{Filter: filter, Namespace: "json"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
				of, err := os.Open(args.Path)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				defer of.Close()
				var kv any
				err = json.NewDecoder(of).Decode(&kv)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				js := "export default " + cstr.JSONStringify(kv)
				logger.Debug("bundling json file from %s", args.Path)
				return api.OnLoadResult{Contents: &js, Loader: api.LoaderJS}, nil
			})

		},
	}

}

func createFileImporter(logger logger.Logger) api.Plugin {
	return api.Plugin{
		Name: "file",
		Setup: func(build api.PluginBuild) {
			filter := "\\.(gif|png|jpg|jpeg|svg|webp|pdf)$"
			build.OnResolve(api.OnResolveOptions{Filter: filter, Namespace: "file"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				p := makePath(args)
				if isNodeModulesPath(p) {
					return api.OnResolveResult{}, nil
				}
				return api.OnResolveResult{Path: p, Namespace: "file"}, nil
			})

			build.OnLoad(api.OnLoadOptions{Filter: filter, Namespace: "file"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
				data, err := os.ReadFile(args.Path)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				base64Data := base64.StdEncoding.EncodeToString(data)
				js := "export default new Uint8Array(Buffer.from(" + cstr.JSONStringify(base64Data) + ", \"base64\"));"
				logger.Debug("bundling binary file from %s", args.Path)
				return api.OnLoadResult{Contents: &js, Loader: api.LoaderJS}, nil
			})

		},
	}

}

func createTextImporter(logger logger.Logger) api.Plugin {
	return api.Plugin{
		Name: "text",
		Setup: func(build api.PluginBuild) {
			filter := "\\.(txt|md|csv|xml|sql)$"
			build.OnResolve(api.OnResolveOptions{Filter: filter, Namespace: "file"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
				p := makePath(args)
				if isNodeModulesPath(p) {
					return api.OnResolveResult{}, nil
				}
				return api.OnResolveResult{Path: p, Namespace: "text"}, nil
			})

			build.OnLoad(api.OnLoadOptions{Filter: filter, Namespace: "text"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
				data, err := os.ReadFile(args.Path)
				if err != nil {
					return api.OnLoadResult{}, err
				}
				js := "export default " + cstr.JSONStringify(string(data))
				logger.Debug("bundling text file from %s", args.Path)
				return api.OnLoadResult{Contents: &js, Loader: api.LoaderJS}, nil
			})

		},
	}

}

func needsDeclarationUpdate(filePath string, expectedHash string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return true // File doesn't exist or can't be read, needs update
	}
	defer file.Close()

	// Read first 100 bytes to check for hash comment
	buffer := make([]byte, 100)
	n, err := file.Read(buffer)
	if err != nil || n == 0 {
		return true // Can't read file, needs update
	}

	content := string(buffer[:n])
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return true
	}

	// Check if first line contains our hash
	firstLine := strings.TrimSpace(lines[0])
	expectedPrefix := "// agentuity-types-hash:"
	if !strings.HasPrefix(firstLine, expectedPrefix) {
		return true // No hash found, needs update
	}

	currentHash := strings.TrimPrefix(firstLine, expectedPrefix)
	return currentHash != expectedHash
}

func possiblyCreateDeclarationFile(logger logger.Logger, dir string) error {
	// Generate hash of declaration content
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(declaration)))

	// Create declaration with hash header
	declarationWithHash := fmt.Sprintf("// agentuity-types-hash:%s\n%s", hash, declaration)

	mfp := filepath.Join(dir, "node_modules", "@agentuity", "sdk", "dist", "file_types.d.ts")
	fp := filepath.Join(dir, "node_modules", "@types", "agentuity")
	fn := filepath.Join(fp, "index.d.ts")

	// Check if files need updates
	mfpNeedsUpdate := needsDeclarationUpdate(mfp, hash)
	fnNeedsUpdate := needsDeclarationUpdate(fn, hash)

	if !mfpNeedsUpdate && !fnNeedsUpdate {
		logger.Debug("declaration files are up to date")
		return nil
	}

	// Create directory if needed
	if !sys.Exists(fp) {
		if err := os.MkdirAll(fp, 0755); err != nil {
			return fmt.Errorf("cannot create directory: %s. %w", fp, err)
		}
		logger.Debug("created directory %s", fp)
	}

	// Create/update the @types/agentuity declaration file
	if fnNeedsUpdate {
		err := os.WriteFile(fn, []byte(declarationWithHash), 0644)
		if err != nil {
			return fmt.Errorf("cannot create file: %s. %w", fn, err)
		}
		logger.Debug("updated declaration file at %s", fn)
	}

	// Create/update the SDK file_types.d.ts
	if mfpNeedsUpdate {
		// Ensure SDK directory exists
		sdkDir := filepath.Dir(mfp)
		if !sys.Exists(sdkDir) {
			if err := os.MkdirAll(sdkDir, 0755); err != nil {
				return fmt.Errorf("cannot create SDK directory: %s. %w", sdkDir, err)
			}
			logger.Debug("created SDK directory %s", sdkDir)
		}

		err := os.WriteFile(mfp, []byte(declarationWithHash), 0644)
		if err != nil {
			return fmt.Errorf("cannot create file: %s. %w", mfp, err)
		}
		logger.Debug("updated declaration file at %s", mfp)
	}

	// Patch the SDK's main index.d.ts to import file_types if it exists and doesn't already import it
	sdkIndexPath := filepath.Join(dir, "node_modules", "@agentuity", "sdk", "dist", "index.d.ts")
	if sys.Exists(sdkIndexPath) {
		content, err := os.ReadFile(sdkIndexPath)
		if err == nil {
			contentStr := string(content)
			// Only add the import if it's not already there
			if !strings.Contains(contentStr, "import './file_types'") && !strings.Contains(contentStr, "import \"./file_types\"") {
				// Find where to insert the import (after the first relative export)
				lines := strings.Split(contentStr, "\n")
				var newLines []string
				inserted := false
				
				for _, line := range lines {
					newLines = append(newLines, line)
					// Insert after the first export with relative import
					if !inserted && strings.HasPrefix(strings.TrimSpace(line), "export ") &&
						(strings.Contains(line, "from './") || strings.Contains(line, "from \"./")) {
						newLines = append(newLines, "import './file_types';")
						inserted = true
					}
				}
				// If we didn't insert it yet, add it after the exports
				if !inserted && len(newLines) > 0 {
					// Find the position after existing exports
					for i, line := range newLines {
						if strings.HasPrefix(strings.TrimSpace(line), "export ") {
							continue
						}
						// Insert before the first non-export line
						newContent := append(newLines[:i], append([]string{"import './file_types';"}, newLines[i:]...)...)
						contentStr = strings.Join(newContent, "\n")
						inserted = true
						break
					}
				}
				// Update contentStr with the modified lines if we inserted something in the first loop
				if inserted && contentStr == string(content) {
					contentStr = strings.Join(newLines, "\n")
				} else if !inserted && len(newLines) > 0 {
					// If we still haven't inserted and there are only exports, append at the end
					newLines = append(newLines, "import './file_types';")
					contentStr = strings.Join(newLines, "\n")
				}
				
				err = os.WriteFile(sdkIndexPath, []byte(contentStr), 0644)
				if err != nil {
					logger.Debug("failed to patch SDK index.d.ts: %v", err)
				} else {
					logger.Debug("patched SDK index.d.ts to include file_types import")
				}
			}
		}
	}

	return nil
}

var declaration = `
declare module '*.yml' {
  const value: any;
  export default value;
}

declare module '*.yaml' {
  const value: any;
  export default value;
}

declare module '*.json' {
  const value: any;
  export default value;
}

declare module '*.png' {
  const value: Uint8Array;
  export default value;
}

declare module '*.gif' {
  const value: Uint8Array;
  export default value;
}

declare module '*.jpg' {
  const value: Uint8Array;
  export default value;
}

declare module '*.jpeg' {
  const value: Uint8Array;
  export default value;
}

declare module '*.svg' {
  const value: Uint8Array;
  export default value;
}

declare module '*.webp' {
  const value: Uint8Array;
  export default value;
}

declare module '*.pdf' {
  const value: Uint8Array;
  export default value;
}

declare module '*.txt' {
  const value: string;
  export default value;
}

declare module '*.md' {
  const value: string;
  export default value;
}

declare module '*.csv' {
  const value: string;
  export default value;
}

declare module '*.xml' {
  const value: string;
  export default value;
}

declare module '*.sql' {
  const value: string;
  export default value;
}
`
