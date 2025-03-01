package bundler

import "fmt"

func generateVercelAIProvider(name string) string {
	return generateJSArgsPatch(0, "") + fmt.Sprintf(`const opts = {...(_args[0] ?? {}) };
if (!opts.baseURL) {
	const apikey = process.env.AGENTUITY_API_KEY;
	const url = process.env.AGENTUITY_URL;
	if (url && apikey) {
		opts.apiKey = 'x';
		opts.baseURL = url + '/sdk/gateway/%s';
		opts.headers = {
			...(opts.headers ?? {}),
			Authorization: 'Bearer ' + apikey,
		};
		_args[0] = opts;
	}
}`, name)
}

func init() {
	var vercelTelemetryPatch = generateJSArgsPatch(0, `experimental_telemetry: { isEnabled: true }`)
	vercelAIPatches := patchModule{
		Module: "ai",
		Functions: map[string]patchAction{
			"generateText": {
				Before: vercelTelemetryPatch,
			},
			"streamText": {
				Before: vercelTelemetryPatch,
			},
			"generateObject": {
				Before: vercelTelemetryPatch,
			},
			"streamObject": {
				Before: vercelTelemetryPatch,
			},
			"embed": {
				Before: vercelTelemetryPatch,
			},
			"embedMany": {
				Before: vercelTelemetryPatch,
			},
		},
	}
	vercelOpenAIPatches := patchModule{
		Module: "@ai-sdk/openai",
		Functions: map[string]patchAction{
			"createOpenAI": {
				Before: generateEnvGuard("OPENAI_API_KEY",
					generateVercelAIProvider("openai"),
				),
			},
		},
	}
	patches["@vercel/ai"] = vercelAIPatches
	patches["@vercel/openai"] = vercelOpenAIPatches
}
