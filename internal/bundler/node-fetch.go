package bundler

func init() {
	patches["node-fetch"] = patchModule{
		Module:   "node-fetch",
		Filename: "src/utils/is",
		Functions: map[string]patchAction{
			"isAbortSignal": {
				After: `if (result) { return true; }
				if (_args[0] && _args[0].constructor.name === 'AbortSignal') {
					return true;
				}
				`,
			},
		},
	}
}
