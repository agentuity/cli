package bundler

import (
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
				js := "export default new Uint8Array(atob(" + cstr.JSONStringify(base64Data) + ").split('').map(c => c.charCodeAt(0)));"
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

func possiblyCreateDeclarationFile(logger logger.Logger, dir string) error {
	mfp := filepath.Join(dir, "node_modules", "@agentuity", "sdk", "dist", "file_types.d.ts")
	if sys.Exists(mfp) {
		logger.Debug("found existing declaration file at %s", mfp)
		return nil
	}
	fp := filepath.Join(dir, "node_modules", "@types", "agentuity")
	fn := filepath.Join(fp, "index.d.ts")
	if sys.Exists(fn) {
		logger.Debug("declaration file already exists at %s", fn)
		return nil
	}
	if !sys.Exists(fp) {
		if err := os.MkdirAll(fp, 0755); err != nil {
			return fmt.Errorf("cannot create directory: %s. %w", fp, err)
		}
		logger.Debug("created directory %s", fp)
	}
	err := os.WriteFile(fn, []byte(declaration), 0644)
	if err != nil {
		return fmt.Errorf("cannot create file: %s. %w", fn, err)
	}
	logger.Debug("created declaration file at %s", mfp)
	err = os.WriteFile(mfp, []byte(declaration), 0644)
	if err != nil {
		return fmt.Errorf("cannot create file: %s. %w", fn, err)
	}
	logger.Debug("created declaration file at %s", fn)
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
