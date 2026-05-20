package templates

const chatSchemaPart1 = `-- Chat domain schema
-- Apply with: ayb sql < schema.sql

CREATE TABLE IF NOT EXISTS rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'group' CHECK (type IN ('direct', 'group', 'channel')),
    created_by UUID NOT NULL REFERENCES _ayb_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS participants (
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    role TEXT NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, user_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    sender_id UUID NOT NULL REFERENCES _ayb_users(id),
    body TEXT NOT NULL,
    edited_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS read_receipts (
    room_id UUID NOT NULL REFERENCES rooms(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    last_read_message_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, user_id)
);

ALTER TABLE rooms ENABLE ROW LEVEL SECURITY;
ALTER TABLE participants ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE read_receipts ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS rooms_select ON rooms;
CREATE POLICY rooms_select ON rooms FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = rooms.id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS rooms_insert ON rooms;
CREATE POLICY rooms_insert ON rooms FOR INSERT
    WITH CHECK (created_by = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS rooms_update ON rooms;
CREATE POLICY rooms_update ON rooms FOR UPDATE
    USING (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = rooms.id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
              AND p.role IN ('owner', 'admin')
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = rooms.id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
              AND p.role IN ('owner', 'admin')
        )
    );
`

const chatSchemaPart2 = `
DROP POLICY IF EXISTS rooms_delete ON rooms;
CREATE POLICY rooms_delete ON rooms FOR DELETE
    USING (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = rooms.id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
              AND p.role IN ('owner', 'admin')
        )
    );

DROP POLICY IF EXISTS participants_select ON participants;
CREATE POLICY participants_select ON participants FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = participants.room_id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS participants_insert ON participants;
CREATE POLICY participants_insert ON participants FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM rooms r
            WHERE r.id = participants.room_id
              AND r.created_by = current_setting('ayb.user_id', true)::uuid
        )
        OR EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = participants.room_id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
              AND p.role IN ('owner', 'admin')
        )
    );

DROP POLICY IF EXISTS participants_delete ON participants;
CREATE POLICY participants_delete ON participants FOR DELETE
    USING (
        participants.user_id = current_setting('ayb.user_id', true)::uuid
        OR EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = participants.room_id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
              AND p.role IN ('owner', 'admin')
        )
    );

DROP POLICY IF EXISTS messages_select ON messages;
CREATE POLICY messages_select ON messages FOR SELECT
    USING (
        EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = messages.room_id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS messages_insert ON messages;
CREATE POLICY messages_insert ON messages FOR INSERT
    WITH CHECK (
        sender_id = current_setting('ayb.user_id', true)::uuid
        AND EXISTS (
            SELECT 1
            FROM participants p
            WHERE p.room_id = messages.room_id
              AND p.user_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS messages_update ON messages;
CREATE POLICY messages_update ON messages FOR UPDATE
    USING (sender_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (sender_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS messages_delete ON messages;
CREATE POLICY messages_delete ON messages FOR DELETE
    USING (sender_id = current_setting('ayb.user_id', true)::uuid);
`

const chatSchemaPart3 = `
DROP POLICY IF EXISTS read_receipts_select ON read_receipts;
CREATE POLICY read_receipts_select ON read_receipts FOR SELECT
    USING (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS read_receipts_insert ON read_receipts;
CREATE POLICY read_receipts_insert ON read_receipts FOR INSERT
    WITH CHECK (
        user_id = current_setting('ayb.user_id', true)::uuid
        AND (
            last_read_message_id IS NULL
            OR EXISTS (
                SELECT 1
                FROM messages m
                WHERE m.id = read_receipts.last_read_message_id
                  AND m.room_id = read_receipts.room_id
            )
        )
    );

DROP POLICY IF EXISTS read_receipts_update ON read_receipts;
CREATE POLICY read_receipts_update ON read_receipts FOR UPDATE
    USING (user_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (
        user_id = current_setting('ayb.user_id', true)::uuid
        AND (
            last_read_message_id IS NULL
            OR EXISTS (
                SELECT 1
                FROM messages m
                WHERE m.id = read_receipts.last_read_message_id
                  AND m.room_id = read_receipts.room_id
            )
        )
    );
`

const chatClientCodePart1 = `import { ayb } from "./ayb";

export interface Room {
  id: string;
  name: string;
  type: "direct" | "group" | "channel";
  created_by: string;
  created_at: string;
}

export interface Participant {
  room_id: string;
  user_id: string;
  role: "owner" | "admin" | "member";
  joined_at: string;
}

export interface Message {
  id: string;
  room_id: string;
  sender_id: string;
  body: string;
  edited_at: string | null;
  created_at: string;
}

export interface ReadReceipt {
  room_id: string;
  user_id: string;
  last_read_message_id: string | null;
  updated_at: string;
}

export interface CreateRoomInput {
  name: string;
  type?: "direct" | "group" | "channel";
  created_by: string;
}

async function requireCurrentUserID(): Promise<string> {
  const me = await ayb.auth.me();
  const userId = (me as { id?: string; user?: { id?: string } }).id
    ?? (me as { id?: string; user?: { id?: string } }).user?.id;
  if (!userId) {
    throw new Error("Cannot continue without an authenticated user");
  }
  return userId;
}

export function listRooms() {
  return ayb.records.list("rooms", { sort: "-created_at" });
}

export function createRoom(data: CreateRoomInput) {
  return ayb.records.create("rooms", {
    type: "group",
    ...data,
  });
}

export function getRoom(id: string) {
  return ayb.records.get("rooms", id);
}

export function listParticipants(roomId: string) {
  return ayb.records.list("participants", {
    filter: "room_id='" + roomId + "'",
    sort: "joined_at",
  });
}

export function addParticipant(roomId: string, userId: string, role: "owner" | "admin" | "member" = "member") {
  return ayb.records.create("participants", {
    room_id: roomId,
    user_id: userId,
    role,
  });
}
`

const chatClientCodePart2 = `
export async function removeParticipant(roomId: string, userId: string) {
  const res = await ayb.records.list<Participant>("participants", {
    filter: "room_id='" + roomId + "' && user_id='" + userId + "'",
    limit: 1,
  });
  if (!res.items?.length) {
    return;
  }
  return ayb.records.delete("participants", roomId + "," + userId);
}

export function listMessages(roomId: string) {
  return ayb.records.list("messages", {
    filter: "room_id='" + roomId + "'",
    sort: "created_at",
  });
}

export async function sendMessage(roomId: string, body: string) {
  const userId = await requireCurrentUserID();

  return ayb.records.create("messages", {
    room_id: roomId,
    sender_id: userId,
    body,
  });
}

export function editMessage(id: string, body: string) {
  return ayb.records.update("messages", id, {
    body,
    edited_at: new Date().toISOString(),
  });
}

export async function markRead(roomId: string, messageId: string) {
  const userId = await requireCurrentUserID();

  const existing = await ayb.records.list<ReadReceipt>("read_receipts", {
    filter: "room_id='" + roomId + "' && user_id='" + userId + "'",
    limit: 1,
  });
  if (existing.items?.length) {
    return ayb.records.update("read_receipts", roomId + "," + userId, {
      last_read_message_id: messageId,
      updated_at: new Date().toISOString(),
    });
  }

  return ayb.records.create("read_receipts", {
    room_id: roomId,
    user_id: userId,
    last_read_message_id: messageId,
  });
}
`
