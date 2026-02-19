-- Pixel Canvas schema for AYB
-- Run: psql $DATABASE_URL -f schema.sql

CREATE TABLE pixels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    x INT NOT NULL,
    y INT NOT NULL,
    color SMALLINT NOT NULL DEFAULT 0,
    user_id UUID REFERENCES _ayb_users(id),
    placed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (x, y)
);

-- Public read, authenticated write.
ALTER TABLE pixels ENABLE ROW LEVEL SECURITY;
CREATE POLICY pixels_read ON pixels FOR SELECT USING (true);
CREATE POLICY pixels_insert ON pixels FOR INSERT WITH CHECK (
    user_id::text = current_setting('ayb.user_id', true)
);
CREATE POLICY pixels_update ON pixels FOR UPDATE USING (true) WITH CHECK (
    user_id::text = current_setting('ayb.user_id', true)
);

-- RPC function for atomic upsert (avoids two-call create-or-update).
CREATE OR REPLACE FUNCTION place_pixel(px INT, py INT, pcolor SMALLINT)
RETURNS TABLE(id UUID, x INT, y INT, color SMALLINT, user_id UUID, placed_at TIMESTAMPTZ) AS $$
BEGIN
    RETURN QUERY
    INSERT INTO pixels (x, y, color, user_id, placed_at)
    VALUES (px, py, pcolor, current_setting('ayb.user_id')::uuid, now())
    ON CONFLICT (x, y) DO UPDATE SET
        color = EXCLUDED.color,
        user_id = EXCLUDED.user_id,
        placed_at = EXCLUDED.placed_at
    RETURNING pixels.id, pixels.x, pixels.y, pixels.color, pixels.user_id, pixels.placed_at;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER;
