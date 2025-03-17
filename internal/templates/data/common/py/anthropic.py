from agentuity import AgentRequest, AgentResponse, AgentContext
from anthropic import Anthropic

client = Anthropic()


async def run(request: AgentRequest, response: AgentResponse, context: AgentContext):
    result = client.messages.create(
        max_tokens=1024,
        messages=[
            {
                "role": "user",
                "content": request.data.text or "Hello, Claude",
            }
        ],
        model="claude-3-5-sonnet-latest",
    )
    return response.text(result.content[0].text)
