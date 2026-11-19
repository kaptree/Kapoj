ALTER TABLE public.submissions
    ADD COLUMN IF NOT EXISTS cpu_time_ms bigint NOT NULL DEFAULT 0;
