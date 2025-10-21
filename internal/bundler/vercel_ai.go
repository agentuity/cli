package bundler

import "fmt"

func generateVercelAIProvider(name string, envkey string) string {
	return generateJSArgsPatch(0, "") + fmt.Sprintf(`const opts = {...(_args[0] ?? {}) };
if (!opts.baseURL) {
	const apikey = process.env.AGENTUITY_SDK_KEY;
	const url = process.env.AGENTUITY_TRANSPORT_URL;
	if (url && apikey) {
		opts.apiKey = apikey;
		opts.baseURL = url + '/gateway/%[1]s';
		_args[0] = opts;
	} else {
	  %[2]s
	}
}`, name, generateEnvWarning(envkey))
}

func createVercelAIProviderPatch(module string, createFn string, envkey string, provider string) patchModule {
	return patchModule{
		Module: module,
		Functions: map[string]patchAction{
			createFn: {
				Before: generateEnvGuard(envkey,
					generateVercelAIProvider(provider, envkey),
				),
			},
		},
	}
}

func init() {

	var vercelTelemetryPatch = generateJSArgsPatch(0, ` `+"")

	var enableTelemetryPatch = `
		// Enable experimental telemetry to capture response text
		const opts = {...(_args[0] ?? {}) };
		opts.experimental_telemetry = { isEnabled: true };
		_args[0] = opts;
		`

	vercelAIPatches := patchModule{
		Module: "ai",
		Functions: map[string]patchAction{
			"generateText": {
				Before: vercelTelemetryPatch + enableTelemetryPatch,
			},
			"streamText": {
				Before: vercelTelemetryPatch + enableTelemetryPatch,
			},
			"generateObject": {
				Before: vercelTelemetryPatch + enableTelemetryPatch,
			},
			"streamObject": {
				Before: vercelTelemetryPatch + enableTelemetryPatch,
			},
			"embed": {
				Before: vercelTelemetryPatch + enableTelemetryPatch,
			},
			"embedMany": {
				Before: vercelTelemetryPatch + enableTelemetryPatch,
			},
			"recordSpan": {
				Before: `
				if (_args[0]?.name && ['ai.generateText', 'ai.generateObject', 'ai.streamText', 'ai.streamObject'].includes(_args[0].name)) {
					// Add our custom attributes to the span configuration
					const originalAttributes = _args[0].attributes || {};
					
					// Extract system and prompt from the span attributes
					let systemString = '';
					let promptString = '';
					
					if (_args[0]?.attributes) {
						// Try to extract from span attributes
						systemString = _args[0].attributes['ai.system'] || _args[0].attributes['system'] || '';
						promptString = _args[0].attributes['ai.prompt'] || _args[0].attributes['prompt'] || '';
						
						// If prompt is a JSON object, extract the individual fields
						if (typeof promptString === 'string' && promptString.startsWith('{')) {
							try {
								const promptObj = JSON.parse(promptString);
								systemString = promptObj.system || systemString;
								promptString = promptObj.prompt || promptString;
							} catch (e) {
								// If parsing fails, keep the original string
							}
						}
					}
					
					// Generate hashes using SDK utility
					const { hashSync } = require('@agentuity/sdk');
					let compiledSystemHash = '';
					let compiledPromptHash = '';
					
					if (systemString) {
						compiledSystemHash = hashSync(systemString);
						console.log('🔑 System hash:', compiledSystemHash);
					}
					
					if (promptString) {
						compiledPromptHash = hashSync(promptString);
						console.log('🔑 Prompt hash:', compiledPromptHash);
					}
					
					// Access PatchPortal state synchronously
					const agentuityPromptMetadata = [];
					
					if (globalThis.__patchPortalInstance) {
						if (systemString) {
							const key = 'prompt:' + compiledSystemHash;
							const patchData = globalThis.__patchPortalInstance.state[key];
							if (patchData) {
								agentuityPromptMetadata.push(...patchData);
							}
						}
						
						if (promptString) {
							const key = 'prompt:' + compiledPromptHash;
							const patchData = globalThis.__patchPortalInstance.state[key];
							if (patchData) {
								agentuityPromptMetadata.push(...patchData);
							}
						}
					}
					
					
				
					
					// Add attributes to span configuration
					if (agentuityPromptMetadata.length > 0) {
						_args[0].attributes = {
							...originalAttributes,
							'@agentuity/prompts': JSON.stringify(agentuityPromptMetadata)
						};
					}

				}
				`,
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
