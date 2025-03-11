import type { AgentRequest, AgentResponse, AgentContext } from "@agentuity/sdk";
import OpenAI from 'openai';

const client = new OpenAI();

export default async function Agent(
	req: AgentRequest,
	resp: AgentResponse,
	ctx: AgentContext,
) {
	const completion = await client.chat.completions.create({
    messages: [{ role: 'user', content: req.data.text ?? 'Say this is a test' }],
    model: 'gpt-4o',
  });
	const message = completion.choices[0]?.message;
	return resp.text(message?.content ?? 'Something went wrong');
}