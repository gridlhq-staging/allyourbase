-- Kanban Board Demo Schema for AllYourBase
-- Run against the AYB-managed Postgres:
--   psql "postgresql://ayb:ayb@localhost:15432/ayb" -f schema.sql

CREATE TABLE boards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL,
  user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE columns (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  position INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE cards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  column_id UUID NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  description TEXT DEFAULT '',
  position INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

-- Row-Level Security: users only see their own boards and nested data
ALTER TABLE boards ENABLE ROW LEVEL SECURITY;
CREATE POLICY boards_owner ON boards
  FOR ALL USING (user_id::text = current_setting('ayb.user_id', true));

ALTER TABLE columns ENABLE ROW LEVEL SECURITY;
CREATE POLICY columns_owner ON columns
  FOR ALL USING (board_id IN (
    SELECT id FROM boards WHERE user_id::text = current_setting('ayb.user_id', true)
  ));

ALTER TABLE cards ENABLE ROW LEVEL SECURITY;
CREATE POLICY cards_owner ON cards
  FOR ALL USING (column_id IN (
    SELECT id FROM columns WHERE board_id IN (
      SELECT id FROM boards WHERE user_id::text = current_setting('ayb.user_id', true)
    )
  ));
