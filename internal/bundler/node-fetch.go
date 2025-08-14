package bundler

var patchCode = `
if (result) { return true; }
if (typeof _args[0] === 'object') {
	return true;
}
`

func init() {
	patches["node-fetch-2.7.0"] = patchModule{
		Module:   "node-fetch",
		Filename: "lib/index",
		Functions: map[string]patchAction{
			"isAbortSignal": {
				After: patchCode,
			},
		},
	}
	patches["node-fetch-3.3.2"] = patchModule{
		Module:   "node-fetch",
		Filename: "src/utils/is",
		Functions: map[string]patchAction{
			"isAbortSignal": {
				After: patchCode,
			},
		},
	}

}
