package bundler

func init() {
	patches["node-fetch"] = patchModule{
		Module:   "node-fetch",
		Filename: "lib/index",
		Functions: map[string]patchAction{
			"isAbortSignal": {
				After: `if (result) { return true; }
				if (typeof _args[0] === 'object') {
					return true;
				}
				`,
			},
		},
	}

}
