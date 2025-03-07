from litellm import completion
from agentuity.server.types import AgentRequest, AgentResponse, AgentContext


def run(request: AgentRequest, response: AgentResponse, context: AgentContext):
    messages = [{"content": request.text or "Hello, how are you?", "role": "user"}]
    result = completion(model="openai/gpt-4o", messages=messages)
    return response.text(result.choices[0].message.content)
