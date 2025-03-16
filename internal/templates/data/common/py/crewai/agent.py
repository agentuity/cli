from agentuity import AgentRequest, AgentResponse, AgentContext
from agents.{{ .AgentName | safe_filename }}.crew import MyCrew


async def run(request: AgentRequest, response: AgentResponse, context: AgentContext):
    inputs = {"topic": request.data.text or "AI LLMs"}
    result = MyCrew().crew().kickoff(inputs=inputs)
    return response.text(str(result))
