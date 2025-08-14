package bundler

import "fmt"

func init() {
	fmt.Println("patching node-fetch")
	patches["node-fetch"] = patchModule{
		Module:   "node-fetch",
		Filename: "lib/index",
		Functions: map[string]patchAction{
			"isAbortSignal": {
				After: `if (result) { return true; }
				console.log(_args[0]);
				if (typeof _args[0] === 'object') {
					return true;
				}
				`,
			},
		},
	}

}
