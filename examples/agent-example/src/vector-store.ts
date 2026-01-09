import { Pinecone } from '@pinecone-database/pinecone';
import OpenAI from 'openai';
import { v4 as uuidv4 } from 'uuid';
import { MemoryEntry, VectorStoreConfig } from './types';

export class VectorStore {
  private pinecone: Pinecone;
  private openai: OpenAI;
  private config: VectorStoreConfig;

  constructor(config: VectorStoreConfig) {
    this.config = config;
    this.pinecone = new Pinecone({
      apiKey: process.env.PINECONE_API_KEY!,
    });
    this.openai = new OpenAI({
      apiKey: process.env.OPENAI_API_KEY!,
    });
  }

  async store(content: string, metadata: Record<string, any> = {}): Promise<string> {
    // Generate embedding
    const embedding = await this.generateEmbedding(content);
    
    // Generate unique ID
    const id = uuidv4();
    
    // Store in Pinecone
    const index = this.pinecone.index(this.config.pinecone.index_name);
    await index.upsert([{
      id,
      values: embedding,
      metadata: {
        content,
        timestamp: Date.now(),
        ...metadata,
      },
    }]);

    return id;
  }

  async search(query: string, topK: number = 5): Promise<MemoryEntry[]> {
    // Generate query embedding
    const embedding = await this.generateEmbedding(query);
    
    // Search in Pinecone
    const index = this.pinecone.index(this.config.pinecone.index_name);
    const results = await index.query({
      vector: embedding,
      topK,
      includeMetadata: true,
    });

    return results.matches?.map(match => ({
      id: match.id!,
      content: match.metadata?.content as string,
      score: match.score!,
      metadata: match.metadata!,
    })) || [];
  }

  async delete(id: string): Promise<void> {
    const index = this.pinecone.index(this.config.pinecone.index_name);
    await index.deleteOne(id);
  }

  private async generateEmbedding(text: string): Promise<number[]> {
    const response = await this.openai.embeddings.create({
      model: this.config.openai.model,
      input: text,
    });

    return response.data[0].embedding;
  }
}