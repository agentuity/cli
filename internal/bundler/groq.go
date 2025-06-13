package bundler

func init() {
	patches["groq-sdk"] = patchModule{
		Module:   "groq-sdk",
		Filename: "index",
		Body: &patchAction{
			Before: generateEnvGuard("GROQ_API_KEY", generateGatewayEnvGuard("GROQ_API_KEY", "process.env.AGENTUITY_SDK_KEY", "GROQ_BASE_URL", "groq")),
		},
	}
} 