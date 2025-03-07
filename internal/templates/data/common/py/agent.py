from agentuity import AgentRequest, AgentResponse, AgentContext


async def run(request: AgentRequest, response: AgentResponse, context: AgentContext):
    return response.text("Hi there!")
