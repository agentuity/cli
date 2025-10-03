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
		console.log('🔧 generateText patch executing...');
		const { PatchPortal } = await import('@agentuity/sdk');
		const crypto = await import('crypto');
		const patchPortal = await PatchPortal.getInstance();
		console.log('✅ PatchPortal instance created');
		
		// Extract prompt from arguments
		const prompt = _args[0]?.prompt || _args[0]?.messages || '';
		const promptString = typeof prompt === 'string' ? prompt : JSON.stringify(prompt);
		console.log('📝 Extracted prompt:', promptString.substring(0, 100) + '...');
		
		// Generate hash for the compiled prompt (same as processPromptMetadata uses)
		const compiledHash = crypto.createHash('sha256').update(promptString).digest('hex');
		console.log('🔑 Generated compiled hash:', compiledHash);
		
		// Print current PatchPortal state
		patchPortal.printState();
		
		// Get patch data using the same key format as processPromptMetadata
		const key = 'prompt:' + compiledHash;
		console.log('🔍 Looking for key:', key);
		const patchData = await patchPortal.get(key);
		console.log('🔍 Retrieved patch data:', patchData);
		
		// Prepare telemetry metadata with PatchPortal data
		const opts = {...(_args[0] ?? {}) };
		const metadata = { 
			promptId: opts.prompt?.id || compiledHash,
			patchPortalData: patchData || null,
			compiledHash: compiledHash,
			patchPortalKey: key
		};
		opts.experimental_telemetry = { isEnabled: true, metadata: metadata };
		opts.prompt = opts.prompt.toString();
		if (opts.system) {
			opts.system = opts.system.toString();
		}
		_args[0] = opts;
		
		if (patchData) {
			console.log('✅ Patch data found for compiled hash:', compiledHash, patchData);
		} else {
			console.log('ℹ️ No patch data found for compiled hash:', compiledHash);
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
