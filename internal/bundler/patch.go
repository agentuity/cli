package bundler

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/agentuity/go-common/logger"
	"github.com/evanw/esbuild/pkg/api"
)

type patchModule struct {
	Module    string
	Filename  string
	Functions map[string]patchAction
	Body      *patchAction
}

type patchAction struct {
	Before string
	After  string
}

func generateEnvWarning(envkey string) string {
	return fmt.Sprintf(`if (process.env.AGENTUITY_ENVIRONMENT === 'development') {
	  console.warn('\nYou have not set the environment variable %[1]s in your project .env file.\n');
	 } else {
	  console.warn('\nYou have not set the environment variable %[1]s in your project. Use "agentuity env set %[1]s" to set it and redeploy your project.\n');
	 }
	 process.exit(1);`, envkey)
}

func generateJSArgsPatch(index int, inject string) string {
	return fmt.Sprintf(`const _newargs = [...(_args ?? [])];
_newargs[%[1]d] = {..._newargs[%[1]d], %[2]s};
_args = _newargs;`, index, inject)
}

func generateEnvGuard(name string, inject string) string {
	return fmt.Sprintf(`if (!process.env.%[1]s || process.env.%[1]s  ===  process.env.AGENTUITY_SDK_KEY) {
%[2]s
}`, name, inject)
}

func generateGatewayEnvGuard(apikey string, apikeyval string, apibase string, provider string) string {
	return fmt.Sprintf(`{
	const apikey =  process.env.AGENTUITY_SDK_KEY;
	const url = process.env.AGENTUITY_TRANSPORT_URL;
	if (url && apikey) {
		process.env.%[1]s = %[2]s;
		process.env.%[3]s = url + '/gateway/%[4]s';
		console.debug('Enabled Agentuity AI Gateway for %[4]s');
	} else {
	 %[5]s
	}
}
`, apikey, apikeyval, apibase, provider, generateEnvWarning(apikey))
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

func createPlugin(logger logger.Logger, dir string, shimSourceMap bool) api.Plugin {
	return api.Plugin{
		Name: "inject-agentuity",
		Setup: func(build api.PluginBuild) {
			if shimSourceMap {
				build.OnLoad(api.OnLoadOptions{Filter: path.Join(dir, "index.ts"), Namespace: "file"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					logger.Debug("adding source map import to %s", args.Path)
					buf, err := os.ReadFile(args.Path)
					if err != nil {
						return api.OnLoadResult{}, err
					}
					contents := string(buf)
					contents = sourceMapShim + "\n" + contents
					return api.OnLoadResult{
						Contents: &contents,
						Loader:   api.LoaderTS,
					}, nil
				})
			}
			for name, mod := range patches {
				path := "node_modules/" + mod.Module + "/.*"
				if mod.Filename != "" {
					path = "node_modules/" + mod.Module + "/" + mod.Filename + ".*"
				}
				build.OnLoad(api.OnLoadOptions{Filter: path, Namespace: "file"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
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
					if mod.Body != nil {
						if mod.Body.Before != "" {
							contents = mod.Body.Before + "\n" + contents
						}
						if mod.Body.After != "" {
							contents = contents + "\n" + mod.Body.After
						}
					}
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
