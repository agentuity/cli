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
	Classes   map[string]patchClass
	Body      *patchAction
}

type patchClass struct {
	Methods map[string]patchAction
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
					isJS := strings.HasSuffix(args.Path, ".js")
					for fn, mod := range mod.Functions {
						fnname := "function " + fn
						index := strings.Index(contents, fnname)
						var isConstVariable bool
						if index == -1 {
							fnname = "const " + fn + " = "
							index = strings.Index(contents, fnname)
							isConstVariable = true
							if index == -1 {
								continue
							}
						}
						eol := searchBackwards(contents, index, '\n')
						if eol < 0 {
							continue
						}
						prefix := strings.TrimSpace(contents[eol+1 : index])
						isAsync := strings.Contains(prefix, "async")
						isExport := strings.Contains(prefix, "export")
						newname := "__agentuity_" + fn
						var newfnname string
						if isConstVariable {
							newfnname = "const " + newname + " = "
						} else {
							newfnname = "function " + newname
						}
						var fnprefix string
						if isAsync {
							fnprefix = "async "
						}
						if isExport {
							fnprefix += "export " + fnprefix
						}
						contents = strings.Replace(contents, fnname, newfnname, 1)
						if isJS {
							suffix.WriteString(fnprefix + "function " + fn + "() {\n")
							suffix.WriteString("let args = arguments;\n")
						} else {
							suffix.WriteString(fnprefix + fnname + "(...args) {\n")
						}
						suffix.WriteString("\tlet _args = args;\n")
						if mod.Before != "" {
							suffix.WriteString(mod.Before)
							suffix.WriteString("\n")
						}

						if isJS {
							// For JS: use .apply to preserve 'this' context
							suffix.WriteString("\tlet result = " + newname + ".apply(this, _args);\n")
						} else {
							// For TS: use spread operator
							suffix.WriteString("\tlet result = " + newname + "(..._args);\n")
						}

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

					// Handle class method patching
					for className, class := range mod.Classes {
						for methodName, method := range class.Methods {
							logger.Debug("attempting to patch class %s method %s", className, methodName)

							// Look for class definition
							classPattern := "class " + className
							classIndex := strings.Index(contents, classPattern)
							if classIndex == -1 {
								logger.Debug("class %s not found", className)
								continue
							}
							logger.Debug("found class %s at index %d", className, classIndex)

							// Look for method definition within the class
							methodPattern := methodName + "("
							methodIndex := strings.Index(contents[classIndex:], methodPattern)
							if methodIndex == -1 {
								logger.Debug("method %s not found in class %s", methodName, className)
								continue
							}
							methodIndex += classIndex
							logger.Debug("found method %s at index %d", methodName, methodIndex)

							// Find the start of the method
							braceIndex := strings.LastIndex(contents[:methodIndex], "{")
							if braceIndex == -1 {
								logger.Debug("opening brace not found for method %s", methodName)
								continue
							}

							// Find the end of the method
							braceCount := 0
							endIndex := braceIndex
							for i := braceIndex; i < len(contents); i++ {
								if contents[i] == '{' {
									braceCount++
								} else if contents[i] == '}' {
									braceCount--
									if braceCount == 0 {
										endIndex = i
										break
									}
								}
							}

							// Extract method content
							methodContent := contents[braceIndex+1 : endIndex]

							// Create patched method
							patchedMethod := fmt.Sprintf(`{
								// Store original method
								if (!%s.prototype.__agentuity_%s) {
									%s.prototype.__agentuity_%s = %s.prototype.%s;
								}
								
								// Create wrapper
								%s.prototype.%s = function(...args) {
									%s
									return this.__agentuity_%s.apply(this, args);
								};
								
								// Original method implementation
								%s
							}`, className, methodName, className, methodName, className, methodName, className, methodName, method.Before, methodName, methodContent)

							// Replace the method in the content
							contents = contents[:braceIndex] + patchedMethod + contents[endIndex+1:]

							logger.Debug("patched class method %s.%s", className, methodName)
						}
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
					if !isJS {
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
