package templates

const kanbanSchemaPart1 = `-- Kanban domain schema
-- Apply with: ayb sql < schema.sql

CREATE TABLE IF NOT EXISTS boards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    owner_id UUID REFERENCES _ayb_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS columns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    column_id UUID NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
    assignee_id UUID REFERENCES _ayb_users(id),
    due_date TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    board_id UUID NOT NULL REFERENCES boards(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    color TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS card_labels (
    card_id UUID NOT NULL REFERENCES cards(id) ON DELETE CASCADE,
    label_id UUID NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (card_id, label_id)
);

ALTER TABLE boards ENABLE ROW LEVEL SECURITY;
ALTER TABLE columns ENABLE ROW LEVEL SECURITY;
ALTER TABLE cards ENABLE ROW LEVEL SECURITY;
ALTER TABLE labels ENABLE ROW LEVEL SECURITY;
ALTER TABLE card_labels ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS boards_select ON boards;
CREATE POLICY boards_select ON boards FOR SELECT
    USING (true);

DROP POLICY IF EXISTS boards_insert ON boards;
CREATE POLICY boards_insert ON boards FOR INSERT
    WITH CHECK (owner_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS boards_update ON boards;
CREATE POLICY boards_update ON boards FOR UPDATE
    USING (owner_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (owner_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS boards_delete ON boards;
CREATE POLICY boards_delete ON boards FOR DELETE
    USING (owner_id = current_setting('ayb.user_id', true)::uuid);
`

const kanbanSchemaPart2 = `
DROP POLICY IF EXISTS columns_select ON columns;
CREATE POLICY columns_select ON columns FOR SELECT
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS columns_insert ON columns;
CREATE POLICY columns_insert ON columns FOR INSERT
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS columns_update ON columns;
CREATE POLICY columns_update ON columns FOR UPDATE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL)
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS columns_delete ON columns;
CREATE POLICY columns_delete ON columns FOR DELETE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS cards_select ON cards;
CREATE POLICY cards_select ON cards FOR SELECT
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS cards_insert ON cards;
CREATE POLICY cards_insert ON cards FOR INSERT
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS cards_update ON cards;
CREATE POLICY cards_update ON cards FOR UPDATE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL)
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS cards_delete ON cards;
CREATE POLICY cards_delete ON cards FOR DELETE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS labels_select ON labels;
CREATE POLICY labels_select ON labels FOR SELECT
    USING (true);

DROP POLICY IF EXISTS labels_insert ON labels;
CREATE POLICY labels_insert ON labels FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM boards b
            WHERE b.id = labels.board_id
              AND b.owner_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS labels_update ON labels;
CREATE POLICY labels_update ON labels FOR UPDATE
    USING (
        EXISTS (
            SELECT 1
            FROM boards b
            WHERE b.id = labels.board_id
              AND b.owner_id = current_setting('ayb.user_id', true)::uuid
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM boards b
            WHERE b.id = labels.board_id
              AND b.owner_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS labels_delete ON labels;
CREATE POLICY labels_delete ON labels FOR DELETE
    USING (
        EXISTS (
            SELECT 1
            FROM boards b
            WHERE b.id = labels.board_id
              AND b.owner_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS card_labels_select ON card_labels;
CREATE POLICY card_labels_select ON card_labels FOR SELECT
    USING (true);

DROP POLICY IF EXISTS card_labels_insert ON card_labels;
CREATE POLICY card_labels_insert ON card_labels FOR INSERT
    WITH CHECK (current_setting('ayb.user_id', true)::uuid IS NOT NULL);

DROP POLICY IF EXISTS card_labels_delete ON card_labels;
CREATE POLICY card_labels_delete ON card_labels FOR DELETE
    USING (current_setting('ayb.user_id', true)::uuid IS NOT NULL);
`

const kanbanClientCodePart1 = `import { ayb } from "./ayb";

export interface Board {
  id: string;
  name: string;
  owner_id: string;
  created_at: string;
}

export interface Column {
  id: string;
  board_id: string;
  name: string;
  position: number;
  created_at: string;
}

export interface Card {
  id: string;
  column_id: string;
  title: string;
  description: string;
  position: number;
  assignee_id: string | null;
  due_date: string | null;
  created_at: string;
  updated_at: string;
}

export interface Label {
  id: string;
  board_id: string;
  name: string;
  color: string;
  created_at: string;
}

export interface CreateBoardInput {
  name: string;
  owner_id: string;
}

export interface CreateColumnInput {
  name: string;
  position?: number;
}

export interface CreateCardInput {
  title: string;
  description?: string;
  position?: number;
  assignee_id?: string | null;
  due_date?: string | null;
}

export function listBoards() {
  return ayb.records.list("boards", { sort: "created_at" });
}

export function createBoard(data: CreateBoardInput) {
  return ayb.records.create("boards", data);
}

export function listColumns(boardId: string) {
  return ayb.records.list("columns", {
    filter: "board_id='" + boardId + "'",
    sort: "position",
  });
}
`

const kanbanClientCodePart2 = `
export function createColumn(boardId: string, data: CreateColumnInput) {
  return ayb.records.create("columns", {
    board_id: boardId,
    ...data,
  });
}

export function listCards(columnId: string) {
  return ayb.records.list("cards", {
    filter: "column_id='" + columnId + "'",
    sort: "position",
  });
}

export function createCard(columnId: string, data: CreateCardInput) {
  return ayb.records.create("cards", {
    column_id: columnId,
    description: "",
    ...data,
  });
}

export function moveCard(id: string, columnId: string, position: number) {
  return ayb.records.update("cards", id, {
    column_id: columnId,
    position,
  });
}

export function listLabels(boardId: string) {
  return ayb.records.list("labels", {
    filter: "board_id='" + boardId + "'",
    sort: "name",
  });
}

export function addLabel(cardId: string, labelId: string) {
  return ayb.records.create("card_labels", {
    card_id: cardId,
    label_id: labelId,
  });
}
`
