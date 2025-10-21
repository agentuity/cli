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
							const sessionId = 'sess_' + traceId;
							const promptMetadataRaw = this.attributes['@agentuity/prompts'];
							
							// Use internal logger from SDK
							const logger = globalThis.__agentuityInternalLogger || console;
							logger.debug('[AGENTUITY] 🔍 Span attributes:', this.attributes);
							// Create eval job with output if promptMetadata exists
							if (globalThis.__evalJobSchedulerInstance && promptMetadataRaw) {								
								try {
									// Parse the JSON string to get the actual prompt metadata array
									const promptMetadata = JSON.parse(promptMetadataRaw);
									
									// Count total evals across all prompt metadata
									const totalEvals = promptMetadata.reduce((count, meta) => count + (meta.evals?.length || 0), 0);
									logger.info('[AGENTUITY] 📦 Creating eval job with output:', {
										spanId,
										sessionId,
										totalEvals,
										outputLength: value?.length || 0
									});
									
									// Create job with output included
									const jobWithOutput = {
										spanId,
										sessionId,
										promptMetadata,
										output: value,
										createdAt: new Date().toISOString()
									};								
									globalThis.__evalJobSchedulerInstance.pendingJobs.set(spanId, jobWithOutput);
									logger.info('[AGENTUITY] ✅ Eval job created successfully');
								} catch (error) {
									logger.warn('[AGENTUITY] ❌ Failed to create eval job:', error);
								}
							} else {
								logger.debug('[AGENTUITY] ⏭️  Skipping eval job creation - no promptMetadata or scheduler instance');
							}
						}
						`,
					},
				},
			},
		},
	}
	patches["@opentelemetry/sdk-trace-base"] = openTelemetryPatches
}
