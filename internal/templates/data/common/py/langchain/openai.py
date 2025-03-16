from agentuity import AgentRequest, AgentResponse, AgentContext
from langchain_openai import ChatOpenAI
from langchain_core.prompts import ChatPromptTemplate
from langchain_core.output_parsers import StrOutputParser

llm = ChatOpenAI()


async def run(request: AgentRequest, response: AgentResponse, context: AgentContext):
    prompt = ChatPromptTemplate.from_messages(
        [
            (
                "system",
                "You are an expert in world knowledge and all things in general.",
            ),
            ("user", "{input}"),
        ]
    )
    output_parser = StrOutputParser()
    chain = prompt | llm | output_parser
    result = chain.invoke({"input": request.data.text})

    return response.text(result)
