-- Kanban Board Demo Schema for Allyourbase
-- Run against the AYB-managed Postgres:
--   psql "postgresql://ayb:ayb@localhost:15432/ayb" -f schema.sql

CREATE TABLE IF NOT EXISTS boards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  title TEXT NOT NULL CHECK (length(title) > 0),
  user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS columns (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
  title TEXT NOT NULL CHECK (length(title) > 0),
  position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
  created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cards (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  column_id UUID NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  title TEXT NOT NULL CHECK (length(title) > 0),
  description TEXT DEFAULT '',
  position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS attachments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  card_id UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
  bucket TEXT NOT NULL,
  object_name TEXT NOT NULL,
  file_name TEXT NOT NULL,
  content_type TEXT NOT NULL,
  size BIGINT NOT NULL,
  user_id UUID NOT NULL REFERENCES _ayb_users(id),
  created_at TIMESTAMPTZ DEFAULT now()
);

-- Row-Level Security: collaborative boards — all authenticated users can
-- read and contribute, but only the board owner can modify/delete the board itself.

-- Drop all policies for idempotency (re-running schema on existing DB).
DROP POLICY IF EXISTS boards_owner ON boards;
DROP POLICY IF EXISTS boards_select ON boards;
DROP POLICY IF EXISTS boards_insert ON boards;
DROP POLICY IF EXISTS boards_update ON boards;
DROP POLICY IF EXISTS boards_delete ON boards;
DROP POLICY IF EXISTS columns_owner ON columns;
DROP POLICY IF EXISTS columns_select ON columns;
DROP POLICY IF EXISTS columns_insert ON columns;
DROP POLICY IF EXISTS columns_update ON columns;
DROP POLICY IF EXISTS columns_delete ON columns;
DROP POLICY IF EXISTS cards_owner ON cards;
DROP POLICY IF EXISTS cards_select ON cards;
DROP POLICY IF EXISTS cards_insert ON cards;
DROP POLICY IF EXISTS cards_update ON cards;
DROP POLICY IF EXISTS cards_delete ON cards;
DROP POLICY IF EXISTS attachments_select ON attachments;
DROP POLICY IF EXISTS attachments_insert ON attachments;
DROP POLICY IF EXISTS attachments_update ON attachments;
DROP POLICY IF EXISTS attachments_delete ON attachments;

ALTER TABLE boards ENABLE ROW LEVEL SECURITY;
CREATE POLICY boards_select ON boards FOR SELECT USING (true);
CREATE POLICY boards_insert ON boards FOR INSERT WITH CHECK (
  user_id::text = current_setting('ayb.user_id', true)
);
CREATE POLICY boards_update ON boards FOR UPDATE USING (
  user_id::text = current_setting('ayb.user_id', true)
);
CREATE POLICY boards_delete ON boards FOR DELETE USING (
  user_id::text = current_setting('ayb.user_id', true)
);

ALTER TABLE columns ENABLE ROW LEVEL SECURITY;
CREATE POLICY columns_select ON columns FOR SELECT USING (true);
CREATE POLICY columns_insert ON columns FOR INSERT WITH CHECK (true);
CREATE POLICY columns_update ON columns FOR UPDATE USING (true);
CREATE POLICY columns_delete ON columns FOR DELETE USING (true);

ALTER TABLE cards ENABLE ROW LEVEL SECURITY;
CREATE POLICY cards_select ON cards FOR SELECT USING (true);
CREATE POLICY cards_insert ON cards FOR INSERT WITH CHECK (true);
CREATE POLICY cards_update ON cards FOR UPDATE USING (true);
CREATE POLICY cards_delete ON cards FOR DELETE USING (true);

ALTER TABLE attachments ENABLE ROW LEVEL SECURITY;
CREATE POLICY attachments_select ON attachments FOR SELECT USING (true);
CREATE POLICY attachments_insert ON attachments FOR INSERT WITH CHECK (
  user_id::text = current_setting('ayb.user_id', true)
);
CREATE POLICY attachments_delete ON attachments FOR DELETE USING (true);
