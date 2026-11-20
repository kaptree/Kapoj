CREATE TABLE public.problem_import_settings (
    id bigint PRIMARY KEY,
    max_package_mb integer NOT NULL DEFAULT 200,
    updated_by bigint NOT NULL DEFAULT 0,
    updated_at timestamp with time zone NOT NULL
);

INSERT INTO public.problem_import_settings (id, max_package_mb, updated_by, updated_at)
VALUES (1, 200, 0, now())
ON CONFLICT (id) DO NOTHING;
