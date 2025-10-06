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
	// Generate PatchPortal integration patch with hashing and telemetry
	var patchPortalPatch = `
		const { PatchPortal } = await import('@agentuity/sdk');
		const { internal } = await import('@agentuity/sdk/logger/internal');
		const crypto = await import('node:crypto');
		internal.debug('🔧 generateText patch executing...');
		const patchPortal = await PatchPortal.getInstance();
		internal.debug('✅ PatchPortal instance created');

		let compiledSystemHash = '';
		let compiledPromptHash = '';
		const metadata = [];
		patchPortal.printState();
		
		if (_args[0]?.system) {
			// Extract prompt from arguments
			const systemString = _args[0]?.system;
			internal.debug('📝 Extracted system:', systemString.substring(0, 100) + '...');
			compiledSystemHash = crypto.createHash('sha256').update(systemString).digest('hex');
			internal.debug('🔑 SYSTEM Generated compiled hash:', compiledSystemHash);

			// Get patch data using the same key format as processPromptMetadata
			const key = 'prompt:' + compiledPromptHash;
			internal.debug('🔍 Looking for key:', key);
			const patchData = await patchPortal.get(key);
			internal.debug('🔍 Retrieved patch data:', patchData);
			metadata.push(patchData);
		}


		if (_args[0]?.prompt) {
			const prompt = _args[0]?.prompt || _args[0]?.messages || '';
			const promptString = typeof prompt === 'string' ? prompt : JSON.stringify(prompt);
			internal.debug('📝 Extracted prompt:', promptString.substring(0, 100) + '...');
			// Generate hash for the compiled prompt (same as processPromptMetadata uses)
			compiledPromptHash = crypto.createHash('sha256').update(promptString).digest('hex');
			internal.debug('🔑 PROMPT Generated compiled hash:', compiledPromptHash);

			// Get patch data using the same key format as processPromptMetadata
			const key = 'prompt:' + compiledPromptHash;
			internal.debug('🔍 Looking for key:', key);
			const patchData = await patchPortal.get(key);
			internal.debug('🔍 Retrieved patch data:', patchData);
			metadata.push(patchData);
		}
		
		if (metadata) {
			// Prepare telemetry metadata with PatchPortal data
			const opts = {...(_args[0] ?? {}) };
			opts.experimental_telemetry = { isEnabled: true, metadata };
			_args[0] = opts;
			internal.debug('✅ Patch data found for compiled hash:', compiledHash, patchData);
		} else {
			internal.debug('ℹ️ No patch data found for compiled hash:', compiledHash);
		}
	`

	var vercelTelemetryPatch = generateJSArgsPatch(0, ``)

	vercelAIPatches := patchModule{
		Module: "ai",
		Functions: map[string]patchAction{
			"generateText": {
				Before: vercelTelemetryPatch + "\n" + patchPortalPatch,
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
