-- Enable pg_trgm extension for fuzzy text search.
-- Uses IF NOT EXISTS for idempotency. If extension installation is unavailable
-- for the connected role/environment, keep migrations running and degrade
-- gracefully by leaving fuzzy support unavailable at runtime.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_trgm extension not available: %. Fuzzy search features will be disabled.', SQLERRM;
END;
$$;
