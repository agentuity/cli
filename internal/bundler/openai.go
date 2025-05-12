package bundler

func init() {
	patches["openai"] = patchModule{
		Module:   "openai",
		Filename: "index",
		Body: &patchAction{
			Before: generateEnvGuard("OPENAI_API_KEY", generateGatewayEnvGuard("OPENAI_API_KEY", "process.env.AGENTUITY_API_KEY || process.env.AGENTUITY_SDK_KEY", "OPENAI_BASE_URL", "openai")),
		},
	}
}
