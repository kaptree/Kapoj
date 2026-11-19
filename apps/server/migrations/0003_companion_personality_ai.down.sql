ALTER TABLE public.companion_states
    DROP COLUMN IF EXISTS last_ai_at,
    DROP COLUMN IF EXISTS ai_daily_date,
    DROP COLUMN IF EXISTS ai_daily_count,
    DROP COLUMN IF EXISTS ai_dialogue_enabled,
    DROP COLUMN IF EXISTS personality_prompt;
