// Package templates Chat scaffolds a room-based messaging system with SQL schema, seed data, TypeScript client library, and documentation.
package templates

type chatTemplate struct{}

func init() {
	Register(chatTemplate{})
}

func (chatTemplate) Name() string {
	return "chat"
}

// Schema returns the SQL data definition language for the chat domain, including rooms, participants, messages, and read_receipts tables with row-level security policies that enforce participant scoping.
func (chatTemplate) Schema() string {
	return chatSchemaPart1 + chatSchemaPart2 + chatSchemaPart3
}

// SeedData returns sample SQL INSERT statements including three test users, two chat rooms, room participants with various roles, and example messages.
func (chatTemplate) SeedData() string {
	return `-- Chat domain seed data
-- Apply with: ayb sql < seed.sql

INSERT INTO _ayb_users (id, email, password_hash)
VALUES
    ('a1111111-1111-1111-1111-111111111111', 'chat.alex@example.com', 'seeded-password-hash'),
    ('a2222222-2222-2222-2222-222222222222', 'chat.sam@example.com', 'seeded-password-hash'),
    ('a3333333-3333-3333-3333-333333333333', 'chat.jordan@example.com', 'seeded-password-hash')
ON CONFLICT DO NOTHING;

INSERT INTO rooms (id, name, type, created_by)
VALUES
    ('b1000000-0000-0000-0000-000000000001', 'Platform Team', 'group', 'a1111111-1111-1111-1111-111111111111'),
    ('b1000000-0000-0000-0000-000000000002', 'Alex & Sam', 'direct', 'a1111111-1111-1111-1111-111111111111')
ON CONFLICT DO NOTHING;

INSERT INTO participants (room_id, user_id, role)
VALUES
    ('b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'owner'),
    ('b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'admin'),
    ('b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'member'),
    ('b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'member'),
    ('b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'member')
ON CONFLICT DO NOTHING;

INSERT INTO messages (id, room_id, sender_id, body)
VALUES
    ('c1000000-0000-0000-0000-000000000001', 'b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'Morning team, standup starts in 10 minutes.'),
    ('c1000000-0000-0000-0000-000000000002', 'b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'On it, posting blocker updates now.'),
    ('c1000000-0000-0000-0000-000000000003', 'b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'I can take the webhook retry task.'),
    ('c1000000-0000-0000-0000-000000000004', 'b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'Great, can you also own the test coverage follow-up?'),
    ('c1000000-0000-0000-0000-000000000005', 'b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'Yes, I will open a PR by this afternoon.'),
    ('c1000000-0000-0000-0000-000000000006', 'b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'I pushed the migration draft, please review.'),
    ('c1000000-0000-0000-0000-000000000007', 'b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'Reviewing now; policy naming looks consistent.'),
    ('c1000000-0000-0000-0000-000000000008', 'b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'Should we split chat and polls into separate PRs?'),
    ('c1000000-0000-0000-0000-000000000009', 'b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'Yes, smaller reviews will move faster.'),
    ('c1000000-0000-0000-0000-000000000010', 'b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'Agreed. Let us ship polls first.'),
    ('c1000000-0000-0000-0000-000000000011', 'b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'Polls PR opened: please review when free.'),
    ('c1000000-0000-0000-0000-000000000012', 'b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'Reviewed and left two comments on seed data.'),
    ('c1000000-0000-0000-0000-000000000013', 'b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'Can we sync on the release checklist after lunch?'),
    ('c1000000-0000-0000-0000-000000000014', 'b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'Yes, 1:30 PM works for me.'),
    ('c1000000-0000-0000-0000-000000000015', 'b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'Perfect, I will bring rollout metrics.'),
    ('c1000000-0000-0000-0000-000000000016', 'b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'Also need to confirm support runbook updates.'),
    ('c1000000-0000-0000-0000-000000000017', 'b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'Good call, adding that to agenda.'),
    ('c1000000-0000-0000-0000-000000000018', 'b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'Do you want me to draft the announcement copy?'),
    ('c1000000-0000-0000-0000-000000000019', 'b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'Yes please, that would help a lot.'),
    ('c1000000-0000-0000-0000-000000000020', 'b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'Done. Shared in the docs channel.')
ON CONFLICT DO NOTHING;

INSERT INTO read_receipts (room_id, user_id, last_read_message_id)
VALUES
    ('b1000000-0000-0000-0000-000000000001', 'a1111111-1111-1111-1111-111111111111', 'c1000000-0000-0000-0000-000000000012'),
    ('b1000000-0000-0000-0000-000000000001', 'a2222222-2222-2222-2222-222222222222', 'c1000000-0000-0000-0000-000000000011'),
    ('b1000000-0000-0000-0000-000000000001', 'a3333333-3333-3333-3333-333333333333', 'c1000000-0000-0000-0000-000000000010'),
    ('b1000000-0000-0000-0000-000000000002', 'a1111111-1111-1111-1111-111111111111', 'c1000000-0000-0000-0000-000000000020'),
    ('b1000000-0000-0000-0000-000000000002', 'a2222222-2222-2222-2222-222222222222', 'c1000000-0000-0000-0000-000000000019')
ON CONFLICT DO NOTHING;
`
}

// ClientCode returns a map of TypeScript client files providing type-safe interfaces and helper functions for room and message operations, participant management, and read receipt tracking.
func (chatTemplate) ClientCode() map[string]string {
	return map[string]string{
		"src/lib/chat.ts": chatClientCodePart1 + chatClientCodePart2,
	}
}

// Readme returns markdown documentation describing the template schema structure, access control model, realtime subscription patterns, and usage examples.
func (chatTemplate) Readme() string {
	return `# Chat Template

This scaffold provisions a room-based chat schema with participant-scoped access control.

## Included schema

- ` + "`rooms`" + `: chat rooms with ` + "`direct`" + `, ` + "`group`" + `, or ` + "`channel`" + ` types
- ` + "`participants`" + `: room membership and role (` + "`owner`" + `, ` + "`admin`" + `, ` + "`member`" + `)
- ` + "`messages`" + `: room messages with sender ownership and optional edit timestamp
- ` + "`read_receipts`" + `: per-user read position per room

## RLS model

Room, participant, and message visibility is scoped to room participants. Room management and participant management are restricted to owner/admin roles (with creator bootstrap support). Messages are editable/deletable by sender only.

## Realtime hint

Use AYB realtime subscriptions on the ` + "`messages`" + ` table to drive live updates:

` + "```ts" + `
const unsubscribe = ayb.realtime.subscribe(["messages"], (event) => {
  const roomId = (event.record as { room_id?: string }).room_id;
  if (roomId === "<room-id>") {
    // update local chat timeline
  }
});
` + "```" + `

## Apply schema and seed data

` + "```bash" + `
ayb sql < schema.sql && ayb sql < seed.sql
` + "```" + `

## SDK usage example

` + "```ts" + `
import {
  createRoom,
  addParticipant,
  sendMessage,
  markRead,
} from "./src/lib/chat";

const room = await createRoom({
  name: "Incident War Room",
  type: "group",
  created_by: "<current-user-id>",
});
await addParticipant(room.id, "<teammate-user-id>", "member");
const message = await sendMessage(room.id, "Starting timeline doc now.");
await markRead(room.id, message.id);
` + "```" + `

## Quick start

1. Start AYB with ` + "`ayb start`" + `.
2. Apply schema and seed data.
3. Use ` + "`src/lib/chat.ts`" + ` helpers to build room, messaging, and read-state flows.
`
}
