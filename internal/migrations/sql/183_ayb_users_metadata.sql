-- Defaulted metadata columns keep Firebase/PocketBase auth inserts compatible
-- while preserving Supabase GoTrue user and app metadata during migration.
ALTER TABLE _ayb_users ADD COLUMN IF NOT EXISTS raw_user_meta_data JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE _ayb_users ADD COLUMN IF NOT EXISTS raw_app_meta_data JSONB NOT NULL DEFAULT '{}'::jsonb;
