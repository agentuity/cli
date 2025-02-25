import type {
	AgentConfig,
	AgentRequest,
	AgentResponse,
	AgentContext,
} from "@agentuity/sdk";
import { generateText } from "ai";
import { openai } from "@ai-sdk/openai";

export const config: AgentConfig = {
	"name": "MyFirstAgent",
	
	'description': "A simple agent that can generate text",
};

export default async function Agent(
	req: AgentRequest,
	resp: AgentResponse,
	ctx: AgentContext,
) {
	const res = await generateText({
		model: openai("gpt-4o"),
		system: "You are a friendly assistant!",
		prompt: req.text() ?? "Why is the sky blue?",
	});
	return resp.text(res.text);
}
