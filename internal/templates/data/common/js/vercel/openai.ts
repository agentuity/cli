import type { AgentRequest, AgentResponse, AgentContext } from "@agentuity/sdk";
import { generateText } from "ai";
import { openai } from "@ai-sdk/openai";

export default async function Agent(
	req: AgentRequest,
	resp: AgentResponse,
	ctx: AgentContext,
) {
	const res = await generateText({
		model: openai("gpt-4o"),
		system: "You are a friendly assistant!",
		prompt: req.data.text ?? "Why is the sky blue?",
	});
	return resp.text(res.text);
}