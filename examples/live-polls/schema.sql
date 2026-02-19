-- Live Polls schema for AYB
-- Run: psql $DATABASE_URL -f schema.sql

CREATE TABLE polls (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    question TEXT NOT NULL,
    is_closed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE poll_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    label TEXT NOT NULL,
    position SMALLINT NOT NULL DEFAULT 0
);

CREATE TABLE votes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    poll_id UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id UUID NOT NULL REFERENCES poll_options(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES _ayb_users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (poll_id, user_id)
);

-- Indexes for common queries.
CREATE INDEX idx_poll_options_poll_id ON poll_options(poll_id);
CREATE INDEX idx_votes_poll_id ON votes(poll_id);
CREATE INDEX idx_votes_option_id ON votes(option_id);

-- RLS: polls
ALTER TABLE polls ENABLE ROW LEVEL SECURITY;
CREATE POLICY polls_read ON polls FOR SELECT USING (true);
CREATE POLICY polls_insert ON polls FOR INSERT WITH CHECK (
    user_id::text = current_setting('ayb.user_id', true)
);
CREATE POLICY polls_update ON polls FOR UPDATE USING (
    user_id::text = current_setting('ayb.user_id', true)
);

-- RLS: poll_options
ALTER TABLE poll_options ENABLE ROW LEVEL SECURITY;
CREATE POLICY options_read ON poll_options FOR SELECT USING (true);
CREATE POLICY options_insert ON poll_options FOR INSERT WITH CHECK (
    poll_id IN (
        SELECT id FROM polls
        WHERE user_id::text = current_setting('ayb.user_id', true)
    )
);

-- RLS: votes
ALTER TABLE votes ENABLE ROW LEVEL SECURITY;
CREATE POLICY votes_read ON votes FOR SELECT USING (true);
CREATE POLICY votes_insert ON votes FOR INSERT WITH CHECK (
    user_id::text = current_setting('ayb.user_id', true)
);

-- RPC: atomic vote cast (enforces one vote per user per poll, rejects closed polls).
CREATE OR REPLACE FUNCTION cast_vote(p_poll_id UUID, p_option_id UUID)
RETURNS TABLE(id UUID, poll_id UUID, option_id UUID, user_id UUID, created_at TIMESTAMPTZ) AS $$
DECLARE
    v_user_id UUID := current_setting('ayb.user_id')::uuid;
BEGIN
    -- Reject if poll is closed.
    IF (SELECT is_closed FROM polls WHERE polls.id = p_poll_id) THEN
        RAISE EXCEPTION 'poll is closed';
    END IF;

    RETURN QUERY
    INSERT INTO votes (poll_id, option_id, user_id)
    VALUES (p_poll_id, p_option_id, v_user_id)
    ON CONFLICT (poll_id, user_id) DO UPDATE SET
        option_id = EXCLUDED.option_id,
        created_at = now()
    RETURNING votes.id, votes.poll_id, votes.option_id, votes.user_id, votes.created_at;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
