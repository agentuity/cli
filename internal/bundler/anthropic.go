package bundler

func init() {
	patches["anthropic"] = patchModule{
		Module:   "@anthropic-ai",
		Filename: "sdk\\/index.*",
		Body: &patchAction{
			Before: generateEnvGuard("ANTHROPIC_API_KEY", generateGatewayEnvGuard("ANTHROPIC_API_KEY", "process.env.AGENTUITY_API_KEY", "ANTHROPIC_BASE_URL", "anthropic")),
		},
	}
}
