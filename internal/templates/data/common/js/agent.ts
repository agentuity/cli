import type { AgentRequest, AgentResponse, AgentContext } from "@agentuity/sdk";

export default async function Agent(
	req: AgentRequest,
	resp: AgentResponse,
	ctx: AgentContext,
) {
	return resp.text('Hi from Agentuity!');
}