export interface MovieListItem {
  slug: string;
  title: string;
  overview: string;
  release_year: number;
  primary_genre: string;
  _highlight?: string;
}

export interface NoteEmbedResponse {
  id: string;
  movie_slug: string;
  embedding: number[];
}

export interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: string;
}

export interface ChatStartEvent {
  provider: string;
  model: string;
  session_id: string;
}

export interface ChatChunkEvent {
  text: string;
}

export interface ChatDoneEvent {
  session_id: string;
  text: string;
}

export interface ChatErrorEvent {
  code: number;
  message: string;
}

export type BYOKProvider = "openai" | "anthropic" | "ollama";
