package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEvalMetadata(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected *EvalMetadata
		wantErr  bool
	}{
		{
			name: "coherence-check.ts",
			content: `import type { EvalContext, EvalRequest, EvalResponse } from '@agentuity/sdk';

export const metadata = {
  id: 'eval_3b74dc768cbdec883d857c011bb85925',
  slug: 'coherence-check',
  name: 'Coherence Check',
  description: 'Evaluates if the response is coherent and logically consistent'
};

/**
 * Coherence Check
 * Evaluates if the response is coherent and logically consistent
 */
export default async function evaluate(
  _ctx: EvalContext,
  req: EvalRequest,
  res: EvalResponse
) {
  const { input, output } = req;

  // TODO: Implement your evaluation logic here
  // Example: Score the output based on some criteria
  
  const score = 0.8; // Replace with your actual scoring logic
  const metadata = {
    reasoning: 'Replace with your evaluation reasoning'
  };

  res.score(score, metadata);
}`,
			expected: &EvalMetadata{
				ID:          "eval_3b74dc768cbdec883d857c011bb85925",
				Slug:        "coherence-check",
				Name:        "Coherence Check",
				Description: "Evaluates if the response is coherent and logically consistent",
			},
			wantErr: false,
		},
		{
			name: "politeness.ts",
			content: `import type { EvalContext, EvalRequest, EvalResponse } from '@agentuity/sdk';
import { groq } from '@ai-sdk/groq';
import { generateObject } from 'ai';
import { z } from 'zod';

export const metadata = {
  id: 'eval_politeness_123456789',
  slug: 'politeness',
  name: 'Politeness Evaluation',
  description: 'Evaluates the politeness of AI responses on a scale of 0 to 1',
};

export default async function evaluate(
  _ctx: EvalContext,
  req: EvalRequest,
  res: EvalResponse
) {
  const { input, output } = req;
  const prompt = "You are a human judge evaluating the politeness of an AI response. Please rate the politeness of the following response on a scale of 0 to 1, where: 0.0-0.3: Not polite (rude, dismissive, or inappropriate), 0.4-0.6: Moderately polite (neutral tone, some courtesy), 0.7-0.9: Polite (courteous, respectful, helpful tone), 1.0: Very polite (exceptionally courteous, warm, and respectful). Consider these factors: Greeting and closing politeness, Use of please, thank you, and other courtesy words, Overall tone and respectfulness, Helpful and considerate language, Professional yet friendly demeanor. User Input: \"" + input + "\" AI Response: \"" + output + "\" Please provide a JSON response with a score (0-1) and reasoning.";

  const result = await generateObject({
    model: groq('llama-3.1-8b-instant'),
    prompt,
    schema: z.object({
      score: z.number().min(0).max(1),
      reasoning: z.string(),
    }),
  });

  const { score, reasoning } = result.object;
  res.score(score, { reasoning: reasoning || 'No reasoning provided' });
}`,
			expected: &EvalMetadata{
				ID:          "eval_politeness_123456789",
				Slug:        "politeness",
				Name:        "Politeness Evaluation",
				Description: "Evaluates the politeness of AI responses on a scale of 0 to 1",
			},
			wantErr: false,
		},
		{
			name: "missing metadata",
			content: `import type { EvalContext, EvalRequest, EvalResponse } from '@agentuity/sdk';

export default async function evaluate(
  _ctx: EvalContext,
  req: EvalRequest,
  res: EvalResponse
) {
  res.score(0.8, { reasoning: 'test' });
}`,
			expected: nil,
			wantErr:  true,
		},
		{
			name: "malformed metadata",
			content: `import type { EvalContext, EvalRequest, EvalResponse } from '@agentuity/sdk';

export const metadata = {
  id: 'test',
  slug: 'test',
  name: 'Test',
  description: 'Test description'
  // Missing closing brace
};

export default async function evaluate(
  _ctx: EvalContext,
  req: EvalRequest,
  res: EvalResponse
) {
  res.score(0.8, { reasoning: 'test' });
}`,
			expected: nil,
			wantErr:  true,
		},
		{
			name: "nested objects in metadata",
			content: `import type { EvalContext, EvalRequest, EvalResponse } from '@agentuity/sdk';

export const metadata = {
  id: 'eval_nested_123',
  slug: 'nested-test',
  name: 'Nested Test',
  description: 'Test with nested objects',
  config: {
    threshold: 0.5,
    enabled: true
  }
};

export default async function evaluate(
  _ctx: EvalContext,
  req: EvalRequest,
  res: EvalResponse
) {
  res.score(0.8, { reasoning: 'test' });
}`,
			expected: &EvalMetadata{
				ID:          "eval_nested_123",
				Slug:        "nested-test",
				Name:        "Nested Test",
				Description: "Test with nested objects",
			},
			wantErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := ParseEvalMetadata(test.content)

			if test.wantErr {
				require.Error(t, err)
				assert.Nil(t, result)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, test.expected.ID, result.ID)
				assert.Equal(t, test.expected.Slug, result.Slug)
				assert.Equal(t, test.expected.Name, result.Name)
				assert.Equal(t, test.expected.Description, result.Description)
			}
		})
	}
}
