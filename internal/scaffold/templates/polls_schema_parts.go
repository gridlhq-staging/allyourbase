package templates

const pollsSchemaPart1 = `-- Polls domain schema
-- Apply with: ayb sql < schema.sql

CREATE TABLE IF NOT EXISTS polls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    creator_id UUID NOT NULL REFERENCES _ayb_users(id),
    multiple_choice BOOLEAN NOT NULL DEFAULT false,
    closes_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS poll_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    text TEXT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (poll_id, id)
);

CREATE TABLE IF NOT EXISTS votes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id UUID NOT NULL,
    user_id UUID NOT NULL REFERENCES _ayb_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (poll_id, option_id) REFERENCES poll_options(poll_id, id) ON DELETE CASCADE,
    UNIQUE (poll_id, user_id)
);

ALTER TABLE polls ENABLE ROW LEVEL SECURITY;
ALTER TABLE poll_options ENABLE ROW LEVEL SECURITY;
ALTER TABLE votes ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS polls_select ON polls;
CREATE POLICY polls_select ON polls FOR SELECT
    USING (true);

DROP POLICY IF EXISTS polls_insert ON polls;
CREATE POLICY polls_insert ON polls FOR INSERT
    WITH CHECK (creator_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS polls_update ON polls;
CREATE POLICY polls_update ON polls FOR UPDATE
    USING (creator_id = current_setting('ayb.user_id', true)::uuid)
    WITH CHECK (creator_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS polls_delete ON polls;
CREATE POLICY polls_delete ON polls FOR DELETE
    USING (creator_id = current_setting('ayb.user_id', true)::uuid);
`

const pollsSchemaPart2 = `
DROP POLICY IF EXISTS poll_options_select ON poll_options;
CREATE POLICY poll_options_select ON poll_options FOR SELECT
    USING (true);

DROP POLICY IF EXISTS poll_options_insert ON poll_options;
CREATE POLICY poll_options_insert ON poll_options FOR INSERT
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM polls p
            WHERE p.id = poll_options.poll_id
              AND p.creator_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS poll_options_update ON poll_options;
CREATE POLICY poll_options_update ON poll_options FOR UPDATE
    USING (
        EXISTS (
            SELECT 1
            FROM polls p
            WHERE p.id = poll_options.poll_id
              AND p.creator_id = current_setting('ayb.user_id', true)::uuid
        )
    )
    WITH CHECK (
        EXISTS (
            SELECT 1
            FROM polls p
            WHERE p.id = poll_options.poll_id
              AND p.creator_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS poll_options_delete ON poll_options;
CREATE POLICY poll_options_delete ON poll_options FOR DELETE
    USING (
        EXISTS (
            SELECT 1
            FROM polls p
            WHERE p.id = poll_options.poll_id
              AND p.creator_id = current_setting('ayb.user_id', true)::uuid
        )
    );

DROP POLICY IF EXISTS votes_select ON votes;
CREATE POLICY votes_select ON votes FOR SELECT
    USING (true);

DROP POLICY IF EXISTS votes_insert ON votes;
CREATE POLICY votes_insert ON votes FOR INSERT
    WITH CHECK (user_id = current_setting('ayb.user_id', true)::uuid);

DROP POLICY IF EXISTS votes_update ON votes;
DROP POLICY IF EXISTS votes_delete ON votes;
`
