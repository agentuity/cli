package bundler

func init() {
	// Patch OpenTelemetry SDK Span class to intercept setAttribute calls
	// This allows us to capture ai.response.text when it's set on spans
	openTelemetryPatches := patchModule{
		Module: "@opentelemetry/sdk-trace-base",
		Classes: map[string]patchClass{
			"Span": {
				Methods: map[string]patchAction{
					"setAttribute": {
						Before: `
						const key = args[0];
						const value = args[1];
						if (key === 'ai.response.text') {
							const spanId = this.spanContext().spanId;
							const traceId = this.spanContext().traceId;
							console.log('[AGENTUITY DEBUG] Captured ai.response.text:', value);
							console.log('[AGENTUITY DEBUG] Span ID:', spanId);
							console.log('[AGENTUITY DEBUG] Trace ID:', traceId);
						}
						`,
					},
				},
			},
		},
	}

	patches["@opentelemetry/sdk-trace-base"] = openTelemetryPatches
}
