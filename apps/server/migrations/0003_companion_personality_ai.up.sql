ALTER TABLE public.companion_states
    ADD COLUMN IF NOT EXISTS personality_prompt varchar(1000) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS ai_dialogue_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS ai_daily_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS ai_daily_date varchar(10) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS last_ai_at timestamptz;
