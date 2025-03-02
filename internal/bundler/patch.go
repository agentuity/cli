package bundler

import (
	"fmt"
	"os"
	"strings"

	"github.com/agentuity/go-common/logger"
	"github.com/evanw/esbuild/pkg/api"
)

type patchModule struct {
	Module    string                 `json:"module"`
	Functions map[string]patchAction `json:"functions"`
}

type patchAction struct {
	Before string
	After  string
}

func generateJSArgsPatch(index int, inject string) string {
	return fmt.Sprintf(`const _newargs = [...(_args ?? [])];
_newargs[%[1]d] = {..._newargs[%[1]d], %[2]s};
_args = _newargs;`, index, inject)
}

func generateEnvGuard(name string, inject string) string {
	return fmt.Sprintf(`if (!process.env.%[1]s) {
%[2]s
}`, name, inject)
}

var patches = map[string]patchModule{}

func searchBackwards(contents string, offset int, val byte) int {
	for i := offset; i >= 0; i-- {
		if contents[i] == val {
			return i
		}
	}
	return -1
}

func createPlugin(logger logger.Logger) api.Plugin {
	return api.Plugin{
		Name: "inject-agentuity",
		Setup: func(build api.PluginBuild) {
			for name, mod := range patches {
				build.OnLoad(api.OnLoadOptions{Filter: "node_modules/" + mod.Module + "/.*", Namespace: "file"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					logger.Debug("re-writing %s for %s", args.Path, name)
					buf, err := os.ReadFile(args.Path)
					if err != nil {
						return api.OnLoadResult{}, err
					}
					contents := string(buf)
					var suffix strings.Builder
					for fn, mod := range mod.Functions {
						fnname := "function " + fn
						index := strings.Index(contents, fnname)
						if index == -1 {
							continue
						}
						eol := searchBackwards(contents, index, '\n')
						if eol < 0 {
							continue
						}
						prefix := strings.TrimSpace(contents[eol+1 : index])
						isAsync := strings.Contains(prefix, "async")
						newname := "__agentuity_" + fn
						newfnname := "function " + newname
						var fnprefix string
						if isAsync {
							fnprefix = "async "
						}
						contents = strings.Replace(contents, fnname, newfnname, 1)
						suffix.WriteString(fnprefix + fnname + "(...args) {\n")
						suffix.WriteString("\tlet _args = args;\n")
						if mod.Before != "" {
							suffix.WriteString(mod.Before)
							suffix.WriteString("\n")
						}
						suffix.WriteString("\tlet result = " + newname + "(..._args);\n")
						if isAsync {
							suffix.WriteString("\tif (result instanceof Promise) {\n")
							suffix.WriteString("\t\tresult = await result;\n")
							suffix.WriteString("\t}\n")
						}
						if mod.After != "" {
							suffix.WriteString(mod.After)
							suffix.WriteString("\n")
						}
						suffix.WriteString("\treturn result;\n")
						suffix.WriteString("}\n")
						logger.Debug("patched %s -> %s", name, fn)
					}
					contents = contents + "\n" + suffix.String()
					loader := api.LoaderJS
					if strings.HasSuffix(args.Path, ".ts") {
						loader = api.LoaderTS
					}
					return api.OnLoadResult{
						Contents: &contents,
						Loader:   loader,
					}, nil
				})
			}
		},
	}
}
