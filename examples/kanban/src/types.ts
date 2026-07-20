export interface Board {
  id: string;
  title: string;
  user_id: string;
  created_at: string;
  updated_at: string;
}

export interface Column {
  id: string;
  board_id: string;
  title: string;
  position: number;
  created_at: string;
}

export interface Card {
  id: string;
  column_id: string;
  title: string;
  description: string;
  position: number;
  created_at: string;
  updated_at: string;
}

export interface Attachment {
  id: string;
  card_id: string;
  bucket: string;
  object_name: string;
  file_name: string;
  content_type: string;
  size: number;
  user_id: string;
  created_at: string;
}
