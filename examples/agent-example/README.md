# Vector Store Agent

A semantic memory agent that provides vector-based storage and retrieval using Pinecone and OpenAI embeddings.

## Features

- Store text content with semantic embeddings
- Search for similar content using natural language queries
- Configurable vector dimensions and similarity metrics
- Metadata support for rich context

## Setup

1. Install dependencies:
   ```bash
   npm install @pinecone-database/pinecone openai uuid
   ```

2. Set environment variables:
   ```bash
   export PINECONE_API_KEY="your-pinecone-api-key"
   export PINECONE_ENVIRONMENT="your-pinecone-environment"
   export OPENAI_API_KEY="your-openai-api-key"
   ```

3. Create a Pinecone index with 1536 dimensions (for OpenAI embeddings)

## Usage

```typescript
import { VectorStore } from './src/vector-store';
import config from './config/vector-store.yaml';

const vectorStore = new VectorStore(config);

// Store content
const id = await vectorStore.store(
  "The capital of France is Paris",
  { topic: "geography", source: "knowledge-base" }
);

// Search for similar content
const results = await vectorStore.search("What is the capital of France?");
console.log(results[0].content); // "The capital of France is Paris"

// Delete content
await vectorStore.delete(id);
```

## Configuration

Edit `config/vector-store.yaml` to customize:

- `pinecone.index_name`: Name of your Pinecone index
- `pinecone.dimension`: Vector dimension (1536 for OpenAI ada-002)
- `pinecone.metric`: Distance metric (cosine, euclidean, dotproduct)
- `openai.model`: OpenAI embedding model to use
- `openai.max_tokens`: Maximum tokens for text processing

## API

### `store(content: string, metadata?: Record<string, any>): Promise<string>`
Store text content and return a unique ID.

### `search(query: string, topK?: number): Promise<MemoryEntry[]>`  
Search for similar content and return top matches.

### `delete(id: string): Promise<void>`
Delete stored content by ID.