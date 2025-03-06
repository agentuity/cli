from openai import OpenAI

client = OpenAI()


def run(request, response, context):
    chat_completion = client.chat.completions.create(
        messages=[
            {
                "role": "system",
                "content": "You are a friendly assistant!",
            },
            {
                "role": "user",
                "content": request.text or "Why is the sky blue?",
            },
        ],
        model="gpt-4o",
    )
    return response.text(chat_completion.choices[0].message.content)
