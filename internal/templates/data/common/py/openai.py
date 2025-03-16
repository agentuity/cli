from openai import OpenAI
from agentuity import AgentRequest, AgentResponse, AgentContext

client = OpenAI()


async def run(request: AgentRequest, response: AgentResponse, context: AgentContext):
    chat_completion = client.chat.completions.create(
        messages=[
            {
                "role": "system",
                "content": "You are a friendly assistant!",
            },
            {
                "role": "user",
                "content": request.data.text or "Why is the sky blue?",
            },
        ],
        model="gpt-4o",
    )
    return response.text(chat_completion.choices[0].message.content)
