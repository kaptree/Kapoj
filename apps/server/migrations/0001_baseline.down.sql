-- 0001_baseline is a one-way baseline for the initial production schema.
-- Restoring a baseline requires a backup/restore; silently dropping the
-- public schema here would be destructive.
DO $$
BEGIN
    RAISE EXCEPTION '0001_baseline is irreversible; restore from a backup before downgrading';
END
$$;
