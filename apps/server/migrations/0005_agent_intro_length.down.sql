ALTER TABLE public.agents
    ALTER COLUMN model_intro TYPE character varying(500)
    USING left(model_intro, 500);
