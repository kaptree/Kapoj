CREATE TABLE public.judge_settings (
    id bigint PRIMARY KEY,
    output_floor_bytes bigint NOT NULL DEFAULT 1048576,
    output_slack_bytes bigint NOT NULL DEFAULT 1048576,
    max_output_bytes bigint NOT NULL DEFAULT 16777216,
    max_input_file_bytes bigint NOT NULL DEFAULT 16777216,
    max_output_file_bytes bigint NOT NULL DEFAULT 16777216,
    max_batch_total_bytes bigint NOT NULL DEFAULT 209715200,
    max_problem_total_bytes bigint NOT NULL DEFAULT 209715200,
    updated_by bigint NOT NULL DEFAULT 0,
    updated_at timestamp with time zone NOT NULL
);

INSERT INTO public.judge_settings (
    id, output_floor_bytes, output_slack_bytes, max_output_bytes,
    max_input_file_bytes, max_output_file_bytes, max_batch_total_bytes,
    max_problem_total_bytes, updated_by, updated_at
)
VALUES (1, 1048576, 1048576, 16777216, 16777216, 16777216, 209715200, 209715200, 0, now())
ON CONFLICT (id) DO NOTHING;
