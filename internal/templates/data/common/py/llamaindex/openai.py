from llama_index.core.agent.workflow import AgentWorkflow
from llama_index.llms.openai import OpenAI
from agentuity import AgentRequest, AgentResponse, AgentContext


# Define a simple calculator tool
def multiply(a: float, b: float) -> float:
    """Useful for multiplying two numbers."""
    return a * b


# Create an agent workflow with our calculator tool
agent = AgentWorkflow.from_tools_or_functions(
    [multiply],
    llm=OpenAI(model="gpt-4o-mini"),
    system_prompt="You are a helpful assistant that can multiply two numbers.",
)


async def run(request: AgentRequest, response: AgentResponse, context: AgentContext):
    result = await agent.run(request.data.text or "What is 1234 * 4567?")
    return response.text(str(result))
