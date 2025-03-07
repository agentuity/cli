from agentuity.server.types import AgentRequest, AgentResponse, AgentContext
def run(request: AgentRequest, response: AgentResponse, context: AgentContext):
    return response.text("Hi there!")