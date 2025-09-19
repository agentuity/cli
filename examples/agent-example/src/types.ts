export interface VectorStoreConfig {
  pinecone: {
    index_name: string;
    dimension: number;
    metric: string;
  };
  openai: {
    model: string;
    max_tokens: number;
  };
}

export interface MemoryEntry {
  id: string;
  content: string;
  score: number;
  metadata: Record<string, any>;
}

export interface SearchResult {
  entries: MemoryEntry[];
  query: string;
  timestamp: number;
}