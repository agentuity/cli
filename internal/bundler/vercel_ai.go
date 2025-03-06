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

func createVercelAIProviderPatch(module string, createFn string, envkey string, provider string) patchModule {
	return patchModule{
		Module: module,
		Functions: map[string]patchAction{
			createFn: {
				Before: generateEnvGuard(envkey,
					generateVercelAIProvider(provider),
				),
			},
		},
	}
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
	patches["@vercel/ai"] = vercelAIPatches

	// register all the providers that we support in our Agentuity AI Gateway
	patches["@vercel/openai"] = createVercelAIProviderPatch("@ai-sdk/openai", "createOpenAI", "OPENAI_API_KEY", "openai")
	patches["@vercel/anthropic"] = createVercelAIProviderPatch("@ai-sdk/anthropic", "createAnthropic", "ANTHROPIC_API_KEY", "anthropic")
	patches["@vercel/cohere"] = createVercelAIProviderPatch("@ai-sdk/cohere", "createCohere", "COHERE_API_KEY", "cohere")
	patches["@vercel/deepseek"] = createVercelAIProviderPatch("@ai-sdk/deepseek", "createDeepSeek", "DEEPSEEK_API_KEY", "deepseek")
	patches["@vercel/google"] = createVercelAIProviderPatch("@ai-sdk/google", "createGoogleGenerativeAI", "GOOGLE_GENERATIVE_AI_API_KEY", "google-ai-studio")
	patches["@vercel/xai"] = createVercelAIProviderPatch("@ai-sdk/xai", "createXai", "XAI_API_KEY", "grok")
	patches["@vercel/groq"] = createVercelAIProviderPatch("@ai-sdk/groq", "createGroq", "GROQ_API_KEY", "groq")
	patches["@vercel/mistral"] = createVercelAIProviderPatch("@ai-sdk/mistral", "createMistral", "MISTRAL_API_KEY", "mistral")
	patches["@vercel/perplexity"] = createVercelAIProviderPatch("@ai-sdk/perplexity", "createPerplexity", "PERPLEXITY_API_KEY", "perplexity-ai")
}
