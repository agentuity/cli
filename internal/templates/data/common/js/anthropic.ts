import type { AgentRequest, AgentResponse, AgentContext } from "@agentuity/sdk";
import Anthropic from '@anthropic-ai/sdk';

const anthropic = new Anthropic();

export default async function Agent(
	req: AgentRequest,
	resp: AgentResponse,
	ctx: AgentContext,
) {
	const message = await anthropic.messages.create({
    model: 'claude-3-5-sonnet-latest',
    max_tokens: 1024,
    messages: [{ role: 'user', content: req.data.text ??'Hello, Claude' }],
  });
	return resp.text(message.content[0].text);
}