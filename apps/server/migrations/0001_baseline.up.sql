
SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

SET default_tablespace = '';

SET default_table_access_method = heap;

CREATE TABLE public.achievements (
    id bigint NOT NULL,
    code character varying(64) NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(500) DEFAULT ''::character varying NOT NULL,
    icon_url character varying(512) DEFAULT ''::character varying NOT NULL,
    condition_type character varying(20) NOT NULL,
    condition_config_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    reward_coins bigint DEFAULT 0 NOT NULL,
    reward_badge_id bigint,
    sort_order bigint DEFAULT 0 NOT NULL,
    is_active boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.achievements_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.achievements_id_seq OWNED BY public.achievements.id;

CREATE TABLE public.activity_logs (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    action character varying(20) NOT NULL,
    points bigint NOT NULL,
    ref_type character varying(20) NOT NULL,
    ref_id bigint NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.activity_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.activity_logs_id_seq OWNED BY public.activity_logs.id;

CREATE TABLE public.admin_audit_logs (
    id bigint NOT NULL,
    actor_id bigint NOT NULL,
    action character varying(64) NOT NULL,
    target_type character varying(32) DEFAULT ''::character varying NOT NULL,
    target_id bigint,
    summary character varying(500) DEFAULT ''::character varying NOT NULL,
    ip character varying(45) DEFAULT ''::character varying NOT NULL,
    user_agent character varying(512) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.admin_audit_logs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.admin_audit_logs_id_seq OWNED BY public.admin_audit_logs.id;

CREATE TABLE public.agent_problem_attempts (
    id bigint NOT NULL,
    agent_id bigint NOT NULL,
    problem_id bigint NOT NULL,
    attempts_used bigint DEFAULT 0 NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    claimed_at timestamp with time zone NOT NULL,
    deadline_at timestamp with time zone NOT NULL,
    last_submission_at timestamp with time zone,
    solved_at timestamp with time zone,
    solve_time_ms bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.agent_problem_attempts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.agent_problem_attempts_id_seq OWNED BY public.agent_problem_attempts.id;

CREATE TABLE public.agent_rating_history (
    id bigint NOT NULL,
    agent_id bigint NOT NULL,
    problem_id bigint,
    submission_id bigint,
    kind character varying(20) NOT NULL,
    difficulty bigint DEFAULT 0 NOT NULL,
    rating_before bigint DEFAULT 0 NOT NULL,
    rating_after bigint DEFAULT 0 NOT NULL,
    delta bigint DEFAULT 0 NOT NULL,
    solve_time_ms bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.agent_rating_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.agent_rating_history_id_seq OWNED BY public.agent_rating_history.id;

CREATE TABLE public.agent_tokens (
    id bigint NOT NULL,
    agent_id bigint NOT NULL,
    token_hash character(64) NOT NULL,
    token_prefix character varying(24) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone NOT NULL,
    last_used_at timestamp with time zone,
    revoked_at timestamp with time zone
);

CREATE SEQUENCE public.agent_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.agent_tokens_id_seq OWNED BY public.agent_tokens.id;

CREATE TABLE public.agents (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    name character varying(100) NOT NULL,
    model_name character varying(100) DEFAULT ''::character varying NOT NULL,
    model_intro character varying(500) DEFAULT ''::character varying NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    rating bigint DEFAULT 1500 NOT NULL,
    passed_count bigint DEFAULT 0 NOT NULL,
    attempted_count bigint DEFAULT 0 NOT NULL,
    avg_solve_time_ms bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.agents_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.agents_id_seq OWNED BY public.agents.id;

CREATE TABLE public.ai_hints (
    id bigint NOT NULL,
    problem_id bigint NOT NULL,
    content text NOT NULL,
    created_by bigint NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.ai_hints_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.ai_hints_id_seq OWNED BY public.ai_hints.id;

CREATE TABLE public.ai_platform_models (
    id bigint NOT NULL,
    name character varying(100) NOT NULL,
    protocol character varying(20) DEFAULT 'openai'::character varying NOT NULL,
    base_url character varying(512) NOT NULL,
    model_name character varying(100) NOT NULL,
    api_key_encrypted text DEFAULT ''::text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    last_test_at timestamp with time zone,
    last_test_ok boolean DEFAULT false NOT NULL,
    last_test_error character varying(500) DEFAULT ''::character varying NOT NULL,
    last_test_latency_ms bigint DEFAULT 0 NOT NULL,
    created_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.ai_platform_models_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.ai_platform_models_id_seq OWNED BY public.ai_platform_models.id;

CREATE TABLE public.ai_providers (
    id bigint NOT NULL,
    code character varying(64) NOT NULL,
    name character varying(100) NOT NULL,
    base_url character varying(512) NOT NULL,
    model_name character varying(100) NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    api_key_ref character varying(255) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.ai_providers_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.ai_providers_id_seq OWNED BY public.ai_providers.id;

CREATE TABLE public.ai_settings (
    id smallint NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    key_fingerprint character varying(64) DEFAULT ''::character varying NOT NULL,
    platform_model_id bigint,
    updated_by bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.ai_settings_id_seq
    AS smallint
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.ai_settings_id_seq OWNED BY public.ai_settings.id;

CREATE TABLE public.announcements (
    id bigint NOT NULL,
    contest_id bigint NOT NULL,
    title character varying(100) NOT NULL,
    content_md text DEFAULT ''::text NOT NULL,
    created_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.announcements_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.announcements_id_seq OWNED BY public.announcements.id;

CREATE TABLE public.backup_packages (
    id bigint NOT NULL,
    kind character varying(20) NOT NULL,
    user_id bigint,
    filename character varying(255) DEFAULT ''::character varying NOT NULL,
    size_bytes bigint DEFAULT 0 NOT NULL,
    status character varying(20) NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    schema_hash character varying(80) DEFAULT ''::character varying NOT NULL,
    created_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone
);

CREATE SEQUENCE public.backup_packages_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.backup_packages_id_seq OWNED BY public.backup_packages.id;

CREATE TABLE public.backup_restore_jobs (
    id bigint NOT NULL,
    kind character varying(20) NOT NULL,
    package_id bigint,
    uploaded_file character varying(255) DEFAULT ''::character varying NOT NULL,
    target_user_id bigint,
    status character varying(20) NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    created_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone,
    restart_required boolean DEFAULT false NOT NULL
);

CREATE SEQUENCE public.backup_restore_jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.backup_restore_jobs_id_seq OWNED BY public.backup_restore_jobs.id;

CREATE TABLE public.backup_schedules (
    id bigint NOT NULL,
    kind character varying(20) NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    frequency character varying(20) DEFAULT 'daily'::character varying NOT NULL,
    run_hour bigint DEFAULT 3 NOT NULL,
    run_minute bigint DEFAULT 0 NOT NULL,
    weekday bigint DEFAULT 1 NOT NULL,
    day_of_month bigint DEFAULT 1 NOT NULL,
    next_run_at timestamp with time zone,
    last_run_at timestamp with time zone,
    last_status character varying(20) DEFAULT ''::character varying NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    updated_by bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.backup_schedules_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.backup_schedules_id_seq OWNED BY public.backup_schedules.id;

CREATE TABLE public.badges (
    id bigint NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(500) DEFAULT ''::character varying NOT NULL,
    image_url character varying(512) DEFAULT ''::character varying NOT NULL,
    price_coins bigint DEFAULT 0 NOT NULL,
    category character varying(20) DEFAULT '其他'::character varying NOT NULL,
    is_limited boolean DEFAULT false NOT NULL,
    stock bigint DEFAULT 0 NOT NULL,
    sort_order bigint DEFAULT 0 NOT NULL,
    is_active boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.badges_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.badges_id_seq OWNED BY public.badges.id;

CREATE TABLE public.certificate_types (
    id bigint NOT NULL,
    name character varying(50) NOT NULL,
    description character varying(200) DEFAULT ''::character varying NOT NULL,
    is_active boolean NOT NULL,
    sort_order bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.certificate_types_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.certificate_types_id_seq OWNED BY public.certificate_types.id;

CREATE TABLE public.certificates (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    certificate_type_id bigint NOT NULL,
    certificate_type_name character varying(50) NOT NULL,
    level character varying(50) NOT NULL,
    image_key character varying(255) NOT NULL,
    image_content_type character varying(100) NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    reject_reason character varying(500) DEFAULT ''::character varying NOT NULL,
    verify_code character varying(32),
    submitted_at timestamp with time zone NOT NULL,
    reviewed_at timestamp with time zone,
    reviewed_by bigint,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.certificates_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.certificates_id_seq OWNED BY public.certificates.id;

CREATE TABLE public.coin_transactions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    amount bigint NOT NULL,
    type character varying(20) NOT NULL,
    reason character varying(40) NOT NULL,
    ref_type character varying(20) DEFAULT ''::character varying NOT NULL,
    ref_id bigint DEFAULT 0 NOT NULL,
    note character varying(200) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.coin_transactions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.coin_transactions_id_seq OWNED BY public.coin_transactions.id;

CREATE TABLE public.companion_events (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    key character varying(64) NOT NULL,
    type character varying(20) NOT NULL,
    bond bigint DEFAULT 0 NOT NULL,
    event_date character varying(10) DEFAULT ''::character varying NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    read_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.companion_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.companion_events_id_seq OWNED BY public.companion_events.id;

CREATE TABLE public.companion_states (
    user_id bigint NOT NULL,
    bond bigint DEFAULT 0 NOT NULL,
    level bigint DEFAULT 0 NOT NULL,
    daily_gain bigint DEFAULT 0 NOT NULL,
    daily_date character varying(10) DEFAULT ''::character varying NOT NULL,
    activity_level character varying(10) DEFAULT 'normal'::character varying NOT NULL,
    dock_enabled boolean DEFAULT true NOT NULL,
    public_profile boolean DEFAULT false NOT NULL,
    equipped_skin character varying(40) DEFAULT ''::character varying NOT NULL,
    unlocked_skins_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    last_pet_date character varying(10) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone,
    updated_at timestamp with time zone
);

CREATE SEQUENCE public.companion_states_user_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.companion_states_user_id_seq OWNED BY public.companion_states.user_id;

CREATE TABLE public.contest_participants (
    id bigint NOT NULL,
    contest_id bigint NOT NULL,
    user_id bigint NOT NULL,
    register_time timestamp with time zone NOT NULL,
    extra_info_json jsonb DEFAULT '[]'::jsonb NOT NULL
);

CREATE SEQUENCE public.contest_participants_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.contest_participants_id_seq OWNED BY public.contest_participants.id;

CREATE TABLE public.contest_problems (
    id bigint NOT NULL,
    contest_id bigint NOT NULL,
    problem_id bigint NOT NULL,
    display_id character varying(1) NOT NULL,
    sort_order bigint DEFAULT 1 NOT NULL,
    score bigint DEFAULT 100 NOT NULL
);

CREATE SEQUENCE public.contest_problems_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.contest_problems_id_seq OWNED BY public.contest_problems.id;

CREATE TABLE public.contest_similarity_matches (
    id bigint NOT NULL,
    run_id bigint NOT NULL,
    contest_id bigint NOT NULL,
    problem_id bigint NOT NULL,
    original_submission_id bigint NOT NULL,
    suspect_submission_id bigint NOT NULL,
    original_user_id bigint NOT NULL,
    suspect_user_id bigint NOT NULL,
    language character varying(20) NOT NULL,
    similarity_percent double precision NOT NULL,
    dice_percent double precision NOT NULL,
    shared_fingerprints bigint NOT NULL,
    original_fingerprints bigint NOT NULL,
    suspect_fingerprints bigint NOT NULL,
    original_created_at timestamp with time zone NOT NULL,
    suspect_created_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.contest_similarity_matches_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.contest_similarity_matches_id_seq OWNED BY public.contest_similarity_matches.id;

CREATE TABLE public.contest_similarity_runs (
    id bigint NOT NULL,
    contest_id bigint NOT NULL,
    problem_id bigint NOT NULL,
    status character varying(20) NOT NULL,
    threshold_percent bigint NOT NULL,
    ignore_common_code boolean DEFAULT true NOT NULL,
    base_language character varying(20) DEFAULT ''::character varying NOT NULL,
    base_code text DEFAULT ''::text NOT NULL,
    engine character varying(30) NOT NULL,
    engine_version character varying(20) NOT NULL,
    requested_by bigint DEFAULT 0 NOT NULL,
    submission_count bigint DEFAULT 0 NOT NULL,
    match_count bigint DEFAULT 0 NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    started_at timestamp with time zone,
    finished_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.contest_similarity_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.contest_similarity_runs_id_seq OWNED BY public.contest_similarity_runs.id;

CREATE TABLE public.contest_similarity_settings (
    id bigint NOT NULL,
    contest_id bigint NOT NULL,
    problem_id bigint NOT NULL,
    threshold_percent bigint DEFAULT 80 NOT NULL,
    ignore_common_code boolean DEFAULT true NOT NULL,
    base_language character varying(20) DEFAULT ''::character varying NOT NULL,
    base_code text DEFAULT ''::text NOT NULL,
    updated_by bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.contest_similarity_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.contest_similarity_settings_id_seq OWNED BY public.contest_similarity_settings.id;

CREATE TABLE public.contests (
    id bigint NOT NULL,
    title character varying(100) NOT NULL,
    description_md text DEFAULT ''::text NOT NULL,
    start_time timestamp with time zone NOT NULL,
    end_time timestamp with time zone NOT NULL,
    mode character varying(10) NOT NULL,
    is_rated boolean DEFAULT false NOT NULL,
    rating_settled_at timestamp with time zone,
    is_public boolean DEFAULT false NOT NULL,
    disable_ai boolean DEFAULT true NOT NULL,
    password_hash character varying(100) DEFAULT ''::character varying NOT NULL,
    password_salt character varying(32) DEFAULT ''::character varying NOT NULL,
    invite_code_hash character varying(100) DEFAULT ''::character varying NOT NULL,
    invite_code_salt character varying(32) DEFAULT ''::character varying NOT NULL,
    register_type character varying(20) DEFAULT 'open'::character varying NOT NULL,
    created_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.contests_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.contests_id_seq OWNED BY public.contests.id;

CREATE TABLE public.db_migrations (
    key character varying(100) NOT NULL,
    applied_at timestamp with time zone NOT NULL
);

CREATE TABLE public.difficulty_settings (
    id bigint NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    default_score bigint DEFAULT 50 NOT NULL,
    min_sample bigint DEFAULT 10 NOT NULL,
    prior_strength bigint DEFAULT 10 NOT NULL,
    max_step bigint DEFAULT 5 NOT NULL,
    window_days bigint DEFAULT 365 NOT NULL,
    include_contest boolean DEFAULT false NOT NULL,
    curve_gamma numeric(3,2) DEFAULT 1.5 NOT NULL,
    ai_assess_enabled boolean DEFAULT true NOT NULL,
    ai_timeout_seconds bigint DEFAULT 15 NOT NULL,
    updated_by bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.difficulty_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.difficulty_settings_id_seq OWNED BY public.difficulty_settings.id;

CREATE TABLE public.learning_path_nodes (
    id bigint NOT NULL,
    path_id bigint NOT NULL,
    parent_id bigint,
    title character varying(200) NOT NULL,
    description_md text DEFAULT ''::text NOT NULL,
    problem_ids_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    sort_order bigint DEFAULT 0 NOT NULL,
    required_count bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.learning_path_nodes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.learning_path_nodes_id_seq OWNED BY public.learning_path_nodes.id;

CREATE TABLE public.learning_path_purchases (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    path_id bigint NOT NULL,
    price_paid bigint NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.learning_path_purchases_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.learning_path_purchases_id_seq OWNED BY public.learning_path_purchases.id;

CREATE TABLE public.learning_path_recommendations (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    path_id bigint NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.learning_path_recommendations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.learning_path_recommendations_id_seq OWNED BY public.learning_path_recommendations.id;

CREATE TABLE public.learning_paths (
    id bigint NOT NULL,
    title character varying(100) NOT NULL,
    description_md text DEFAULT ''::text NOT NULL,
    cover_url character varying(512) DEFAULT ''::character varying NOT NULL,
    is_official boolean NOT NULL,
    is_public boolean NOT NULL,
    is_active boolean NOT NULL,
    created_by bigint NOT NULL,
    sort_order bigint DEFAULT 0 NOT NULL,
    price_coins bigint DEFAULT 0 NOT NULL,
    participant_count bigint DEFAULT 0 NOT NULL,
    recommend_count bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.learning_paths_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.learning_paths_id_seq OWNED BY public.learning_paths.id;

CREATE TABLE public.mail_settings (
    id smallint NOT NULL,
    provider character varying(32) DEFAULT 'qq'::character varying NOT NULL,
    smtp_host character varying(255) DEFAULT 'smtp.qq.com'::character varying NOT NULL,
    smtp_port bigint DEFAULT 465 NOT NULL,
    smtp_tls character varying(16) DEFAULT 'ssl'::character varying NOT NULL,
    smtp_username character varying(255) DEFAULT ''::character varying NOT NULL,
    smtp_password_encrypted text DEFAULT ''::text NOT NULL,
    from_email character varying(254) DEFAULT ''::character varying NOT NULL,
    from_name character varying(100) DEFAULT 'kapOJ'::character varying NOT NULL,
    reply_to character varying(254) DEFAULT ''::character varying NOT NULL,
    public_base_url character varying(500) DEFAULT ''::character varying NOT NULL,
    sending_enabled boolean DEFAULT false NOT NULL,
    require_email_verification boolean DEFAULT true NOT NULL,
    updated_by bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.mail_settings_id_seq
    AS smallint
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.mail_settings_id_seq OWNED BY public.mail_settings.id;

CREATE TABLE public.notifications (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    actor_id bigint NOT NULL,
    type character varying(20) NOT NULL,
    dedupe_key character varying(100) NOT NULL,
    target_type character varying(20) NOT NULL,
    target_id bigint NOT NULL,
    link character varying(255) NOT NULL,
    read_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.notifications_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.notifications_id_seq OWNED BY public.notifications.id;

CREATE TABLE public.permissions (
    id bigint NOT NULL,
    code character varying(64) NOT NULL,
    name character varying(100) NOT NULL,
    group_name character varying(64) NOT NULL,
    description character varying(500) DEFAULT ''::character varying NOT NULL,
    sort_order bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.permissions_id_seq OWNED BY public.permissions.id;

CREATE TABLE public.problem_attachments (
    id bigint NOT NULL,
    problem_id bigint NOT NULL,
    kind character varying(20) NOT NULL,
    filename character varying(255) NOT NULL,
    storage_key character varying(255) NOT NULL,
    content_type character varying(100) DEFAULT ''::character varying NOT NULL,
    size bigint DEFAULT 0 NOT NULL,
    created_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.problem_attachments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.problem_attachments_id_seq OWNED BY public.problem_attachments.id;

CREATE TABLE public.problem_difficulty_history (
    id bigint NOT NULL,
    problem_id bigint NOT NULL,
    old_score bigint NOT NULL,
    new_score bigint NOT NULL,
    prior bigint NOT NULL,
    source character varying(20) NOT NULL,
    sample_users bigint DEFAULT 0 NOT NULL,
    accepted_users bigint DEFAULT 0 NOT NULL,
    pass_rate numeric(6,5) DEFAULT 0 NOT NULL,
    smoothed_pass_rate numeric(6,5) DEFAULT 0 NOT NULL,
    target_score bigint DEFAULT 0 NOT NULL,
    confidence numeric(4,3) DEFAULT 0 NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    run_id bigint,
    created_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.problem_difficulty_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.problem_difficulty_history_id_seq OWNED BY public.problem_difficulty_history.id;

CREATE TABLE public.problem_solution_post_likes (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    post_id bigint NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.problem_solution_post_likes_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.problem_solution_post_likes_id_seq OWNED BY public.problem_solution_post_likes.id;

CREATE TABLE public.problem_solution_posts (
    id bigint NOT NULL,
    problem_id bigint NOT NULL,
    user_id bigint NOT NULL,
    title character varying(100) NOT NULL,
    content_md text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    deleted_at timestamp with time zone
);

CREATE SEQUENCE public.problem_solution_posts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.problem_solution_posts_id_seq OWNED BY public.problem_solution_posts.id;

CREATE TABLE public.problem_solution_replies (
    id bigint NOT NULL,
    post_id bigint NOT NULL,
    user_id bigint NOT NULL,
    content_md text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    deleted_at timestamp with time zone
);

CREATE SEQUENCE public.problem_solution_replies_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.problem_solution_replies_id_seq OWNED BY public.problem_solution_replies.id;

CREATE TABLE public.problem_solution_unlocks (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    problem_id bigint NOT NULL,
    price_paid bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.problem_solution_unlocks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.problem_solution_unlocks_id_seq OWNED BY public.problem_solution_unlocks.id;

CREATE TABLE public.problem_testcases (
    id bigint NOT NULL,
    problem_id bigint NOT NULL,
    input_file character varying(255) NOT NULL,
    output_file character varying(255) NOT NULL,
    input_size bigint DEFAULT 0 NOT NULL,
    output_size bigint DEFAULT 0 NOT NULL,
    score bigint DEFAULT 10 NOT NULL,
    is_sample boolean DEFAULT false NOT NULL,
    sort_order bigint NOT NULL
);

CREATE SEQUENCE public.problem_testcases_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.problem_testcases_id_seq OWNED BY public.problem_testcases.id;

CREATE TABLE public.problems (
    id bigint NOT NULL,
    title character varying(200) NOT NULL,
    slug character varying(63) NOT NULL,
    statement_md text DEFAULT ''::text NOT NULL,
    input_format_md text DEFAULT ''::text NOT NULL,
    output_format_md text DEFAULT ''::text NOT NULL,
    samples_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    hint_md text DEFAULT ''::text NOT NULL,
    solution_md text DEFAULT ''::text NOT NULL,
    solution_code text DEFAULT ''::text NOT NULL,
    solution_price_coins bigint DEFAULT 0 NOT NULL,
    type character varying(20) DEFAULT 'traditional'::character varying NOT NULL,
    answers_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    tags_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    difficulty bigint DEFAULT 50 NOT NULL,
    difficulty_prior bigint DEFAULT 50 NOT NULL,
    difficulty_prior_source character varying(20) DEFAULT 'default'::character varying NOT NULL,
    difficulty_auto boolean DEFAULT true NOT NULL,
    difficulty_source character varying(20) DEFAULT 'default'::character varying NOT NULL,
    difficulty_reason text DEFAULT ''::text NOT NULL,
    difficulty_updated_at timestamp with time zone DEFAULT now() NOT NULL,
    time_limit_ms bigint DEFAULT 1000 NOT NULL,
    memory_limit_mb bigint DEFAULT 256 NOT NULL,
    checker_path character varying(255) DEFAULT ''::character varying NOT NULL,
    is_public boolean DEFAULT false NOT NULL,
    source character varying(200) DEFAULT ''::character varying NOT NULL,
    submit_count bigint DEFAULT 0 NOT NULL,
    accept_count bigint DEFAULT 0 NOT NULL,
    created_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.problems_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.problems_id_seq OWNED BY public.problems.id;

CREATE TABLE public.rating_history (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    contest_id bigint,
    kind character varying(20) NOT NULL,
    rank bigint NOT NULL,
    rating_before bigint NOT NULL,
    rating_after bigint NOT NULL,
    period_key character varying(64) DEFAULT ''::character varying NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.rating_history_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.rating_history_id_seq OWNED BY public.rating_history.id;

CREATE TABLE public.rating_runs (
    key character varying(64) NOT NULL,
    kind character varying(20) NOT NULL,
    processed_at timestamp with time zone,
    created_at timestamp with time zone NOT NULL
);

CREATE TABLE public.rating_settings (
    id bigint NOT NULL,
    decay_enabled boolean DEFAULT true NOT NULL,
    decay_threshold bigint DEFAULT 2400 NOT NULL,
    decay_inactive_days bigint DEFAULT 60 NOT NULL,
    decay_percent bigint DEFAULT 3 NOT NULL,
    decay_max bigint DEFAULT 100 NOT NULL,
    season_enabled boolean DEFAULT true NOT NULL,
    season_months bigint DEFAULT 6 NOT NULL,
    season_reset_percent bigint DEFAULT 30 NOT NULL,
    season_anchor date NOT NULL,
    updated_by bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.rating_settings_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.rating_settings_id_seq OWNED BY public.rating_settings.id;

CREATE TABLE public.rbac_meta (
    key character varying(64) NOT NULL,
    value character varying(255) DEFAULT ''::character varying NOT NULL
);

CREATE TABLE public.role_permissions (
    id bigint NOT NULL,
    role_id bigint NOT NULL,
    permission_id bigint NOT NULL
);

CREATE SEQUENCE public.role_permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.role_permissions_id_seq OWNED BY public.role_permissions.id;

CREATE TABLE public.roles (
    id bigint NOT NULL,
    code character varying(64) NOT NULL,
    name character varying(100) NOT NULL,
    description character varying(500) DEFAULT ''::character varying NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.roles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.roles_id_seq OWNED BY public.roles.id;

CREATE TABLE public.scheduled_job_runs (
    id bigint NOT NULL,
    job_key character varying(100) NOT NULL,
    trigger character varying(20) NOT NULL,
    status character varying(20) NOT NULL,
    progress_current bigint DEFAULT 0 NOT NULL,
    progress_total bigint DEFAULT 0 NOT NULL,
    stats_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    created_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    started_at timestamp with time zone,
    heartbeat_at timestamp with time zone,
    finished_at timestamp with time zone
);

CREATE SEQUENCE public.scheduled_job_runs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.scheduled_job_runs_id_seq OWNED BY public.scheduled_job_runs.id;

CREATE TABLE public.scheduled_jobs (
    id bigint NOT NULL,
    key character varying(100) NOT NULL,
    name character varying(100) NOT NULL,
    description text DEFAULT ''::text NOT NULL,
    cron_expr character varying(100) NOT NULL,
    timezone character varying(64) DEFAULT 'Asia/Shanghai'::character varying NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    next_run_at timestamp with time zone,
    last_run_at timestamp with time zone,
    last_status character varying(20) DEFAULT ''::character varying NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    updated_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.scheduled_jobs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.scheduled_jobs_id_seq OWNED BY public.scheduled_jobs.id;

CREATE TABLE public.shared_items (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    type character varying(20) NOT NULL,
    ref_id bigint NOT NULL,
    like_count bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.shared_items_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.shared_items_id_seq OWNED BY public.shared_items.id;

CREATE TABLE public.site_settings (
    id smallint NOT NULL,
    logo_key character varying(255) DEFAULT ''::character varying NOT NULL,
    logo_content_type character varying(100) DEFAULT ''::character varying NOT NULL,
    favicon_key character varying(255) DEFAULT ''::character varying NOT NULL,
    favicon_content_type character varying(100) DEFAULT ''::character varying NOT NULL,
    started_at timestamp with time zone NOT NULL,
    footer_icp character varying(100) DEFAULT ''::character varying NOT NULL,
    footer_police character varying(100) DEFAULT ''::character varying NOT NULL,
    footer_police_url character varying(500) DEFAULT ''::character varying NOT NULL,
    footer_copyright character varying(200) DEFAULT ''::character varying NOT NULL,
    footer_text text DEFAULT ''::text NOT NULL,
    show_visits boolean DEFAULT true NOT NULL,
    badge_size bigint DEFAULT 22 NOT NULL,
    home_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    companion_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    deployment_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    updated_by bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.site_settings_id_seq
    AS smallint
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.site_settings_id_seq OWNED BY public.site_settings.id;

CREATE TABLE public.site_visit_counters (
    id smallint NOT NULL,
    count bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.site_visit_counters_id_seq
    AS smallint
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.site_visit_counters_id_seq OWNED BY public.site_visit_counters.id;

CREATE TABLE public.submissions (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    problem_id bigint NOT NULL,
    contest_id bigint,
    agent_id bigint,
    agent_attempt_id bigint,
    agent_settled boolean DEFAULT false NOT NULL,
    language character varying(20) NOT NULL,
    code text NOT NULL,
    answer text DEFAULT ''::text NOT NULL,
    status character varying(30) DEFAULT 'pending'::character varying NOT NULL,
    time_used_ms bigint DEFAULT 0 NOT NULL,
    memory_used_kb bigint DEFAULT 0 NOT NULL,
    score bigint DEFAULT 0 NOT NULL,
    code_length bigint DEFAULT 0 NOT NULL,
    accepted_counted boolean DEFAULT false NOT NULL,
    rejudged_at timestamp with time zone,
    error_message text DEFAULT ''::text NOT NULL,
    judge_result_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    created_at timestamp with time zone NOT NULL,
    judged_at timestamp with time zone
);

CREATE SEQUENCE public.submissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.submissions_id_seq OWNED BY public.submissions.id;

CREATE TABLE public.tasks (
    id bigint NOT NULL,
    title character varying(100) NOT NULL,
    description_md text DEFAULT ''::text NOT NULL,
    problem_ids_json jsonb DEFAULT '[]'::jsonb NOT NULL,
    reward_coins bigint DEFAULT 0 NOT NULL,
    start_time timestamp with time zone NOT NULL,
    end_time timestamp with time zone NOT NULL,
    is_active boolean NOT NULL,
    created_by bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.tasks_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.tasks_id_seq OWNED BY public.tasks.id;

CREATE TABLE public.testcase_generation_cases (
    id bigint NOT NULL,
    generation_id bigint NOT NULL,
    sort_order bigint NOT NULL,
    input_file character varying(255) NOT NULL,
    output_file character varying(255) NOT NULL,
    input_size bigint DEFAULT 0 NOT NULL,
    output_size bigint DEFAULT 0 NOT NULL
);

CREATE SEQUENCE public.testcase_generation_cases_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.testcase_generation_cases_id_seq OWNED BY public.testcase_generation_cases.id;

CREATE TABLE public.testcase_generations (
    id bigint NOT NULL,
    problem_id bigint NOT NULL,
    user_id bigint NOT NULL,
    status character varying(20) NOT NULL,
    language character varying(20) NOT NULL,
    standard_code text NOT NULL,
    generator_code text NOT NULL,
    testcase_count bigint NOT NULL,
    time_limit_ms bigint NOT NULL,
    memory_limit_mb bigint NOT NULL,
    error_message text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.testcase_generations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.testcase_generations_id_seq OWNED BY public.testcase_generations.id;

CREATE TABLE public.titles (
    id bigint NOT NULL,
    name character varying(100) NOT NULL,
    color character varying(7) NOT NULL,
    unlock_condition_type character varying(30) NOT NULL,
    unlock_condition_json jsonb DEFAULT '{}'::jsonb NOT NULL,
    price_coins bigint DEFAULT 0 NOT NULL,
    sort_order bigint DEFAULT 0 NOT NULL,
    is_active boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.titles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.titles_id_seq OWNED BY public.titles.id;

CREATE TABLE public.user_achievements (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    achievement_id bigint NOT NULL,
    unlocked_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.user_achievements_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_achievements_id_seq OWNED BY public.user_achievements.id;

CREATE TABLE public.user_ai_configs (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    provider character varying(64) NOT NULL,
    model_name character varying(100) NOT NULL,
    api_key_encrypted character varying(1024) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.user_ai_configs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_ai_configs_id_seq OWNED BY public.user_ai_configs.id;

CREATE TABLE public.user_ai_models (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    name character varying(100) NOT NULL,
    protocol character varying(20) DEFAULT 'openai'::character varying NOT NULL,
    base_url character varying(512) NOT NULL,
    model_name character varying(100) NOT NULL,
    api_key_encrypted text DEFAULT ''::text NOT NULL,
    is_active boolean DEFAULT false NOT NULL,
    last_test_at timestamp with time zone,
    last_test_ok boolean DEFAULT false NOT NULL,
    last_test_error character varying(500) DEFAULT ''::character varying NOT NULL,
    last_test_latency_ms bigint DEFAULT 0 NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.user_ai_models_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_ai_models_id_seq OWNED BY public.user_ai_models.id;

CREATE TABLE public.user_badges (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    badge_id bigint NOT NULL,
    is_equipped boolean DEFAULT false NOT NULL,
    purchased_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.user_badges_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_badges_id_seq OWNED BY public.user_badges.id;

CREATE TABLE public.user_evaluations (
    id bigint NOT NULL,
    target_user_id bigint NOT NULL,
    author_user_id bigint NOT NULL,
    content character varying(40) NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.user_evaluations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_evaluations_id_seq OWNED BY public.user_evaluations.id;

CREATE TABLE public.user_follows (
    id bigint NOT NULL,
    follower_id bigint NOT NULL,
    followee_id bigint NOT NULL,
    created_at timestamp with time zone NOT NULL,
    CONSTRAINT chk_user_follows_no_self CHECK ((follower_id <> followee_id))
);

CREATE SEQUENCE public.user_follows_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_follows_id_seq OWNED BY public.user_follows.id;

CREATE TABLE public.user_learning_progress (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    path_id bigint NOT NULL,
    node_id bigint NOT NULL,
    status character varying(20) NOT NULL,
    solved_count bigint DEFAULT 0 NOT NULL,
    completed_at timestamp with time zone,
    updated_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.user_learning_progress_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_learning_progress_id_seq OWNED BY public.user_learning_progress.id;

CREATE TABLE public.user_login_days (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    login_date date NOT NULL,
    created_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.user_login_days_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_login_days_id_seq OWNED BY public.user_login_days.id;

CREATE TABLE public.user_roles (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    role_id bigint NOT NULL,
    assigned_by bigint,
    assigned_at timestamp with time zone NOT NULL
);

CREATE SEQUENCE public.user_roles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_roles_id_seq OWNED BY public.user_roles.id;

CREATE TABLE public.user_task_records (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    task_id bigint NOT NULL,
    status character varying(20) NOT NULL,
    claimed_at timestamp with time zone NOT NULL,
    completed_at timestamp with time zone
);

CREATE SEQUENCE public.user_task_records_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_task_records_id_seq OWNED BY public.user_task_records.id;

CREATE TABLE public.user_titles (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    title_id bigint NOT NULL,
    is_equipped boolean DEFAULT false NOT NULL,
    unlocked_at timestamp with time zone NOT NULL,
    purchased_at timestamp with time zone
);

CREATE SEQUENCE public.user_titles_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.user_titles_id_seq OWNED BY public.user_titles.id;

CREATE TABLE public.users (
    id bigint NOT NULL,
    username character varying(32) NOT NULL,
    nickname character varying(64) NOT NULL,
    email character varying(254) NOT NULL,
    email_verified_at timestamp with time zone,
    password_hash character varying(100) NOT NULL,
    salt character varying(32) DEFAULT ''::character varying NOT NULL,
    avatar_url character varying(512) DEFAULT ''::character varying NOT NULL,
    avatar_key character varying(255) DEFAULT ''::character varying NOT NULL,
    avatar_content_type character varying(100) DEFAULT ''::character varying NOT NULL,
    role character varying(20) DEFAULT 'user'::character varying NOT NULL,
    rating bigint DEFAULT 1500 NOT NULL,
    coins bigint DEFAULT 0 NOT NULL,
    bio character varying(800) DEFAULT ''::character varying NOT NULL,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone NOT NULL,
    CONSTRAINT chk_users_coins_non_negative CHECK ((coins >= 0))
);

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;

ALTER TABLE ONLY public.achievements ALTER COLUMN id SET DEFAULT nextval('public.achievements_id_seq'::regclass);

ALTER TABLE ONLY public.activity_logs ALTER COLUMN id SET DEFAULT nextval('public.activity_logs_id_seq'::regclass);

ALTER TABLE ONLY public.admin_audit_logs ALTER COLUMN id SET DEFAULT nextval('public.admin_audit_logs_id_seq'::regclass);

ALTER TABLE ONLY public.agent_problem_attempts ALTER COLUMN id SET DEFAULT nextval('public.agent_problem_attempts_id_seq'::regclass);

ALTER TABLE ONLY public.agent_rating_history ALTER COLUMN id SET DEFAULT nextval('public.agent_rating_history_id_seq'::regclass);

ALTER TABLE ONLY public.agent_tokens ALTER COLUMN id SET DEFAULT nextval('public.agent_tokens_id_seq'::regclass);

ALTER TABLE ONLY public.agents ALTER COLUMN id SET DEFAULT nextval('public.agents_id_seq'::regclass);

ALTER TABLE ONLY public.ai_hints ALTER COLUMN id SET DEFAULT nextval('public.ai_hints_id_seq'::regclass);

ALTER TABLE ONLY public.ai_platform_models ALTER COLUMN id SET DEFAULT nextval('public.ai_platform_models_id_seq'::regclass);

ALTER TABLE ONLY public.ai_providers ALTER COLUMN id SET DEFAULT nextval('public.ai_providers_id_seq'::regclass);

ALTER TABLE ONLY public.ai_settings ALTER COLUMN id SET DEFAULT nextval('public.ai_settings_id_seq'::regclass);

ALTER TABLE ONLY public.announcements ALTER COLUMN id SET DEFAULT nextval('public.announcements_id_seq'::regclass);

ALTER TABLE ONLY public.backup_packages ALTER COLUMN id SET DEFAULT nextval('public.backup_packages_id_seq'::regclass);

ALTER TABLE ONLY public.backup_restore_jobs ALTER COLUMN id SET DEFAULT nextval('public.backup_restore_jobs_id_seq'::regclass);

ALTER TABLE ONLY public.backup_schedules ALTER COLUMN id SET DEFAULT nextval('public.backup_schedules_id_seq'::regclass);

ALTER TABLE ONLY public.badges ALTER COLUMN id SET DEFAULT nextval('public.badges_id_seq'::regclass);

ALTER TABLE ONLY public.certificate_types ALTER COLUMN id SET DEFAULT nextval('public.certificate_types_id_seq'::regclass);

ALTER TABLE ONLY public.certificates ALTER COLUMN id SET DEFAULT nextval('public.certificates_id_seq'::regclass);

ALTER TABLE ONLY public.coin_transactions ALTER COLUMN id SET DEFAULT nextval('public.coin_transactions_id_seq'::regclass);

ALTER TABLE ONLY public.companion_events ALTER COLUMN id SET DEFAULT nextval('public.companion_events_id_seq'::regclass);

ALTER TABLE ONLY public.companion_states ALTER COLUMN user_id SET DEFAULT nextval('public.companion_states_user_id_seq'::regclass);

ALTER TABLE ONLY public.contest_participants ALTER COLUMN id SET DEFAULT nextval('public.contest_participants_id_seq'::regclass);

ALTER TABLE ONLY public.contest_problems ALTER COLUMN id SET DEFAULT nextval('public.contest_problems_id_seq'::regclass);

ALTER TABLE ONLY public.contest_similarity_matches ALTER COLUMN id SET DEFAULT nextval('public.contest_similarity_matches_id_seq'::regclass);

ALTER TABLE ONLY public.contest_similarity_runs ALTER COLUMN id SET DEFAULT nextval('public.contest_similarity_runs_id_seq'::regclass);

ALTER TABLE ONLY public.contest_similarity_settings ALTER COLUMN id SET DEFAULT nextval('public.contest_similarity_settings_id_seq'::regclass);

ALTER TABLE ONLY public.contests ALTER COLUMN id SET DEFAULT nextval('public.contests_id_seq'::regclass);

ALTER TABLE ONLY public.difficulty_settings ALTER COLUMN id SET DEFAULT nextval('public.difficulty_settings_id_seq'::regclass);

ALTER TABLE ONLY public.learning_path_nodes ALTER COLUMN id SET DEFAULT nextval('public.learning_path_nodes_id_seq'::regclass);

ALTER TABLE ONLY public.learning_path_purchases ALTER COLUMN id SET DEFAULT nextval('public.learning_path_purchases_id_seq'::regclass);

ALTER TABLE ONLY public.learning_path_recommendations ALTER COLUMN id SET DEFAULT nextval('public.learning_path_recommendations_id_seq'::regclass);

ALTER TABLE ONLY public.learning_paths ALTER COLUMN id SET DEFAULT nextval('public.learning_paths_id_seq'::regclass);

ALTER TABLE ONLY public.mail_settings ALTER COLUMN id SET DEFAULT nextval('public.mail_settings_id_seq'::regclass);

ALTER TABLE ONLY public.notifications ALTER COLUMN id SET DEFAULT nextval('public.notifications_id_seq'::regclass);

ALTER TABLE ONLY public.permissions ALTER COLUMN id SET DEFAULT nextval('public.permissions_id_seq'::regclass);

ALTER TABLE ONLY public.problem_attachments ALTER COLUMN id SET DEFAULT nextval('public.problem_attachments_id_seq'::regclass);

ALTER TABLE ONLY public.problem_difficulty_history ALTER COLUMN id SET DEFAULT nextval('public.problem_difficulty_history_id_seq'::regclass);

ALTER TABLE ONLY public.problem_solution_post_likes ALTER COLUMN id SET DEFAULT nextval('public.problem_solution_post_likes_id_seq'::regclass);

ALTER TABLE ONLY public.problem_solution_posts ALTER COLUMN id SET DEFAULT nextval('public.problem_solution_posts_id_seq'::regclass);

ALTER TABLE ONLY public.problem_solution_replies ALTER COLUMN id SET DEFAULT nextval('public.problem_solution_replies_id_seq'::regclass);

ALTER TABLE ONLY public.problem_solution_unlocks ALTER COLUMN id SET DEFAULT nextval('public.problem_solution_unlocks_id_seq'::regclass);

ALTER TABLE ONLY public.problem_testcases ALTER COLUMN id SET DEFAULT nextval('public.problem_testcases_id_seq'::regclass);

ALTER TABLE ONLY public.problems ALTER COLUMN id SET DEFAULT nextval('public.problems_id_seq'::regclass);

ALTER TABLE ONLY public.rating_history ALTER COLUMN id SET DEFAULT nextval('public.rating_history_id_seq'::regclass);

ALTER TABLE ONLY public.rating_settings ALTER COLUMN id SET DEFAULT nextval('public.rating_settings_id_seq'::regclass);

ALTER TABLE ONLY public.role_permissions ALTER COLUMN id SET DEFAULT nextval('public.role_permissions_id_seq'::regclass);

ALTER TABLE ONLY public.roles ALTER COLUMN id SET DEFAULT nextval('public.roles_id_seq'::regclass);

ALTER TABLE ONLY public.scheduled_job_runs ALTER COLUMN id SET DEFAULT nextval('public.scheduled_job_runs_id_seq'::regclass);

ALTER TABLE ONLY public.scheduled_jobs ALTER COLUMN id SET DEFAULT nextval('public.scheduled_jobs_id_seq'::regclass);

ALTER TABLE ONLY public.shared_items ALTER COLUMN id SET DEFAULT nextval('public.shared_items_id_seq'::regclass);

ALTER TABLE ONLY public.site_settings ALTER COLUMN id SET DEFAULT nextval('public.site_settings_id_seq'::regclass);

ALTER TABLE ONLY public.site_visit_counters ALTER COLUMN id SET DEFAULT nextval('public.site_visit_counters_id_seq'::regclass);

ALTER TABLE ONLY public.submissions ALTER COLUMN id SET DEFAULT nextval('public.submissions_id_seq'::regclass);

ALTER TABLE ONLY public.tasks ALTER COLUMN id SET DEFAULT nextval('public.tasks_id_seq'::regclass);

ALTER TABLE ONLY public.testcase_generation_cases ALTER COLUMN id SET DEFAULT nextval('public.testcase_generation_cases_id_seq'::regclass);

ALTER TABLE ONLY public.testcase_generations ALTER COLUMN id SET DEFAULT nextval('public.testcase_generations_id_seq'::regclass);

ALTER TABLE ONLY public.titles ALTER COLUMN id SET DEFAULT nextval('public.titles_id_seq'::regclass);

ALTER TABLE ONLY public.user_achievements ALTER COLUMN id SET DEFAULT nextval('public.user_achievements_id_seq'::regclass);

ALTER TABLE ONLY public.user_ai_configs ALTER COLUMN id SET DEFAULT nextval('public.user_ai_configs_id_seq'::regclass);

ALTER TABLE ONLY public.user_ai_models ALTER COLUMN id SET DEFAULT nextval('public.user_ai_models_id_seq'::regclass);

ALTER TABLE ONLY public.user_badges ALTER COLUMN id SET DEFAULT nextval('public.user_badges_id_seq'::regclass);

ALTER TABLE ONLY public.user_evaluations ALTER COLUMN id SET DEFAULT nextval('public.user_evaluations_id_seq'::regclass);

ALTER TABLE ONLY public.user_follows ALTER COLUMN id SET DEFAULT nextval('public.user_follows_id_seq'::regclass);

ALTER TABLE ONLY public.user_learning_progress ALTER COLUMN id SET DEFAULT nextval('public.user_learning_progress_id_seq'::regclass);

ALTER TABLE ONLY public.user_login_days ALTER COLUMN id SET DEFAULT nextval('public.user_login_days_id_seq'::regclass);

ALTER TABLE ONLY public.user_roles ALTER COLUMN id SET DEFAULT nextval('public.user_roles_id_seq'::regclass);

ALTER TABLE ONLY public.user_task_records ALTER COLUMN id SET DEFAULT nextval('public.user_task_records_id_seq'::regclass);

ALTER TABLE ONLY public.user_titles ALTER COLUMN id SET DEFAULT nextval('public.user_titles_id_seq'::regclass);

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);

ALTER TABLE ONLY public.achievements
    ADD CONSTRAINT achievements_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.activity_logs
    ADD CONSTRAINT activity_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.admin_audit_logs
    ADD CONSTRAINT admin_audit_logs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_problem_attempts
    ADD CONSTRAINT agent_problem_attempts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_rating_history
    ADD CONSTRAINT agent_rating_history_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agent_tokens
    ADD CONSTRAINT agent_tokens_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.ai_hints
    ADD CONSTRAINT ai_hints_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.ai_platform_models
    ADD CONSTRAINT ai_platform_models_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.ai_providers
    ADD CONSTRAINT ai_providers_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.ai_settings
    ADD CONSTRAINT ai_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.announcements
    ADD CONSTRAINT announcements_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.backup_packages
    ADD CONSTRAINT backup_packages_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.backup_restore_jobs
    ADD CONSTRAINT backup_restore_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.backup_schedules
    ADD CONSTRAINT backup_schedules_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.badges
    ADD CONSTRAINT badges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.certificate_types
    ADD CONSTRAINT certificate_types_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.certificates
    ADD CONSTRAINT certificates_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.coin_transactions
    ADD CONSTRAINT coin_transactions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.companion_events
    ADD CONSTRAINT companion_events_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.companion_states
    ADD CONSTRAINT companion_states_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.contest_participants
    ADD CONSTRAINT contest_participants_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.contest_problems
    ADD CONSTRAINT contest_problems_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.contest_similarity_matches
    ADD CONSTRAINT contest_similarity_matches_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.contest_similarity_runs
    ADD CONSTRAINT contest_similarity_runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.contest_similarity_settings
    ADD CONSTRAINT contest_similarity_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.contests
    ADD CONSTRAINT contests_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.db_migrations
    ADD CONSTRAINT db_migrations_pkey PRIMARY KEY (key);

ALTER TABLE ONLY public.difficulty_settings
    ADD CONSTRAINT difficulty_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.learning_path_nodes
    ADD CONSTRAINT learning_path_nodes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.learning_path_purchases
    ADD CONSTRAINT learning_path_purchases_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.learning_path_recommendations
    ADD CONSTRAINT learning_path_recommendations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.learning_paths
    ADD CONSTRAINT learning_paths_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.mail_settings
    ADD CONSTRAINT mail_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.problem_attachments
    ADD CONSTRAINT problem_attachments_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.problem_difficulty_history
    ADD CONSTRAINT problem_difficulty_history_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.problem_solution_post_likes
    ADD CONSTRAINT problem_solution_post_likes_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.problem_solution_posts
    ADD CONSTRAINT problem_solution_posts_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.problem_solution_replies
    ADD CONSTRAINT problem_solution_replies_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.problem_solution_unlocks
    ADD CONSTRAINT problem_solution_unlocks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.problem_testcases
    ADD CONSTRAINT problem_testcases_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.problems
    ADD CONSTRAINT problems_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rating_history
    ADD CONSTRAINT rating_history_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rating_runs
    ADD CONSTRAINT rating_runs_pkey PRIMARY KEY (key);

ALTER TABLE ONLY public.rating_settings
    ADD CONSTRAINT rating_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.rbac_meta
    ADD CONSTRAINT rbac_meta_pkey PRIMARY KEY (key);

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.scheduled_job_runs
    ADD CONSTRAINT scheduled_job_runs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.scheduled_jobs
    ADD CONSTRAINT scheduled_jobs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.shared_items
    ADD CONSTRAINT shared_items_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.site_settings
    ADD CONSTRAINT site_settings_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.site_visit_counters
    ADD CONSTRAINT site_visit_counters_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.submissions
    ADD CONSTRAINT submissions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.tasks
    ADD CONSTRAINT tasks_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.testcase_generation_cases
    ADD CONSTRAINT testcase_generation_cases_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.testcase_generations
    ADD CONSTRAINT testcase_generations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.titles
    ADD CONSTRAINT titles_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_achievements
    ADD CONSTRAINT user_achievements_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_ai_configs
    ADD CONSTRAINT user_ai_configs_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_ai_models
    ADD CONSTRAINT user_ai_models_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_badges
    ADD CONSTRAINT user_badges_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_evaluations
    ADD CONSTRAINT user_evaluations_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_follows
    ADD CONSTRAINT user_follows_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_learning_progress
    ADD CONSTRAINT user_learning_progress_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_login_days
    ADD CONSTRAINT user_login_days_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_task_records
    ADD CONSTRAINT user_task_records_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.user_titles
    ADD CONSTRAINT user_titles_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);

CREATE INDEX idx_activity_logs_created ON public.activity_logs USING btree (created_at);

CREATE INDEX idx_activity_logs_user_created ON public.activity_logs USING btree (user_id, created_at);

CREATE INDEX idx_admin_audit_logs_action ON public.admin_audit_logs USING btree (action);

CREATE INDEX idx_admin_audit_logs_actor ON public.admin_audit_logs USING btree (actor_id);

CREATE INDEX idx_admin_audit_logs_created ON public.admin_audit_logs USING btree (created_at);

CREATE INDEX idx_agent_problem_attempts_agent_status ON public.agent_problem_attempts USING btree (agent_id, status);

CREATE INDEX idx_agent_rating_history_agent_created ON public.agent_rating_history USING btree (agent_id, created_at);

CREATE INDEX idx_agent_tokens_agent_id ON public.agent_tokens USING btree (agent_id);

CREATE UNIQUE INDEX idx_agent_tokens_token_hash ON public.agent_tokens USING btree (token_hash);

CREATE INDEX idx_announcements_contest ON public.announcements USING btree (contest_id);

CREATE INDEX idx_backup_packages_kind_created ON public.backup_packages USING btree (kind, user_id);

CREATE INDEX idx_backup_packages_status ON public.backup_packages USING btree (status);

CREATE INDEX idx_certificates_status_submitted ON public.certificates USING btree (status, submitted_at);

CREATE INDEX idx_certificates_type ON public.certificates USING btree (certificate_type_id);

CREATE INDEX idx_certificates_user_submitted ON public.certificates USING btree (user_id, submitted_at);

CREATE INDEX idx_coin_transactions_user_created ON public.coin_transactions USING btree (user_id, created_at);

CREATE INDEX idx_companion_events_user_date ON public.companion_events USING btree (user_id, type, event_date);

CREATE INDEX idx_companion_events_user_unread ON public.companion_events USING btree (user_id, type, read_at);

CREATE INDEX idx_contest_similarity_matches_contest_id ON public.contest_similarity_matches USING btree (contest_id);

CREATE INDEX idx_contest_similarity_matches_problem_id ON public.contest_similarity_matches USING btree (problem_id);

CREATE INDEX idx_contest_similarity_runs_status ON public.contest_similarity_runs USING btree (status);

CREATE INDEX idx_contests_end ON public.contests USING btree (end_time);

CREATE INDEX idx_contests_is_public ON public.contests USING btree (is_public);

CREATE INDEX idx_contests_start ON public.contests USING btree (start_time);

CREATE INDEX idx_generations_problem_created ON public.testcase_generations USING btree (problem_id, created_at);

CREATE INDEX idx_learning_path_nodes_parent ON public.learning_path_nodes USING btree (parent_id);

CREATE INDEX idx_learning_path_nodes_path ON public.learning_path_nodes USING btree (path_id);

CREATE INDEX idx_learning_path_purchases_path ON public.learning_path_purchases USING btree (user_id, path_id);

CREATE INDEX idx_learning_path_recommendations_path ON public.learning_path_recommendations USING btree (path_id);

CREATE INDEX idx_learning_paths_active ON public.learning_paths USING btree (is_active);

CREATE INDEX idx_learning_paths_created_by ON public.learning_paths USING btree (created_by);

CREATE INDEX idx_notifications_user_created ON public.notifications USING btree (user_id, created_at);

CREATE INDEX idx_notifications_user_read ON public.notifications USING btree (user_id, read_at);

CREATE INDEX idx_problem_attachments_problem_kind ON public.problem_attachments USING btree (problem_id, kind);

CREATE INDEX idx_problem_difficulty_history_problem ON public.problem_difficulty_history USING btree (problem_id, created_at DESC);

CREATE INDEX idx_problem_difficulty_history_run ON public.problem_difficulty_history USING btree (run_id);

CREATE INDEX idx_problem_solution_posts_deleted_at ON public.problem_solution_posts USING btree (deleted_at);

CREATE INDEX idx_problem_solution_posts_problem_id ON public.problem_solution_posts USING btree (problem_id);

CREATE INDEX idx_problem_solution_posts_user_id ON public.problem_solution_posts USING btree (user_id);

CREATE INDEX idx_problem_solution_replies_deleted_at ON public.problem_solution_replies USING btree (deleted_at);

CREATE INDEX idx_problem_solution_replies_post_id ON public.problem_solution_replies USING btree (post_id);

CREATE INDEX idx_problem_solution_replies_user_id ON public.problem_solution_replies USING btree (user_id);

CREATE INDEX idx_problems_difficulty ON public.problems USING btree (difficulty);

CREATE INDEX idx_problems_is_public ON public.problems USING btree (is_public);

CREATE INDEX idx_problems_tags ON public.problems USING gin (tags_json);

CREATE INDEX idx_scheduled_job_runs_job_created ON public.scheduled_job_runs USING btree (job_key, created_at DESC);

CREATE INDEX idx_scheduled_job_runs_status ON public.scheduled_job_runs USING btree (status);

CREATE INDEX idx_scheduled_jobs_due ON public.scheduled_jobs USING btree (enabled, next_run_at);

CREATE UNIQUE INDEX idx_scheduled_jobs_key ON public.scheduled_jobs USING btree (key);

CREATE INDEX idx_similarity_runs_problem ON public.contest_similarity_runs USING btree (contest_id, problem_id);

CREATE INDEX idx_solution_post_likes_post ON public.problem_solution_post_likes USING btree (post_id);

CREATE INDEX idx_submissions_agent_attempt_id ON public.submissions USING btree (agent_attempt_id);

CREATE INDEX idx_submissions_agent_id ON public.submissions USING btree (agent_id);

CREATE INDEX idx_submissions_agent_settled ON public.submissions USING btree (agent_id, agent_settled);

CREATE INDEX idx_submissions_agent_unsettled ON public.submissions USING btree (agent_id) WHERE ((agent_id IS NOT NULL) AND (agent_settled = false));

CREATE INDEX idx_submissions_contest_user ON public.submissions USING btree (contest_id, user_id);

CREATE INDEX idx_submissions_problem_status ON public.submissions USING btree (problem_id, status);

CREATE INDEX idx_submissions_user_created ON public.submissions USING btree (user_id, created_at);

CREATE INDEX idx_user_ai_models_user_active ON public.user_ai_models USING btree (user_id, is_active);

CREATE INDEX idx_user_evaluations_target_updated ON public.user_evaluations USING btree (target_user_id, updated_at);

CREATE INDEX idx_user_follows_followee ON public.user_follows USING btree (followee_id);

CREATE INDEX idx_user_learning_progress_path ON public.user_learning_progress USING btree (path_id);

CREATE INDEX idx_user_learning_progress_user ON public.user_learning_progress USING btree (user_id);

CREATE INDEX idx_user_roles_role_id ON public.user_roles USING btree (role_id);

CREATE INDEX idx_users_status_coins ON public.users USING btree (status, coins);

CREATE UNIQUE INDEX uniq_achievements_code ON public.achievements USING btree (code);

CREATE UNIQUE INDEX uniq_activity_logs_dedupe ON public.activity_logs USING btree (user_id, action, ref_type, ref_id);

CREATE UNIQUE INDEX uniq_agent_attempts_active ON public.agent_problem_attempts USING btree (agent_id) WHERE ((status)::text = 'active'::text);

CREATE UNIQUE INDEX uniq_agent_problem_attempts ON public.agent_problem_attempts USING btree (agent_id, problem_id);

CREATE UNIQUE INDEX uniq_agent_rating_fail ON public.agent_rating_history USING btree (agent_id, problem_id) WHERE ((kind)::text = ANY ((ARRAY['abandon'::character varying, 'timeout'::character varying, 'exhausted'::character varying])::text[]));

CREATE UNIQUE INDEX uniq_agent_rating_solve ON public.agent_rating_history USING btree (agent_id, problem_id) WHERE ((kind)::text = 'solve'::text);

CREATE UNIQUE INDEX uniq_agent_rating_submission ON public.agent_rating_history USING btree (submission_id) WHERE (submission_id IS NOT NULL);

CREATE UNIQUE INDEX uniq_agent_tokens_active ON public.agent_tokens USING btree (agent_id) WHERE (revoked_at IS NULL);

CREATE UNIQUE INDEX uniq_agents_name ON public.agents USING btree (name);

CREATE UNIQUE INDEX uniq_agents_user ON public.agents USING btree (user_id);

CREATE UNIQUE INDEX uniq_ai_hints_problem ON public.ai_hints USING btree (problem_id);

CREATE UNIQUE INDEX uniq_ai_platform_models_name ON public.ai_platform_models USING btree (name);

CREATE UNIQUE INDEX uniq_ai_providers_code ON public.ai_providers USING btree (code);

CREATE UNIQUE INDEX uniq_backup_schedules_kind ON public.backup_schedules USING btree (kind);

CREATE UNIQUE INDEX uniq_certificate_types_name ON public.certificate_types USING btree (name);

CREATE UNIQUE INDEX uniq_certificates_verify_code ON public.certificates USING btree (verify_code);

CREATE UNIQUE INDEX uniq_coin_transactions_idem ON public.coin_transactions USING btree (user_id, reason, ref_type, ref_id) WHERE ((ref_type)::text <> ''::text);

CREATE UNIQUE INDEX uniq_companion_events_user_key ON public.companion_events USING btree (user_id, key);

CREATE UNIQUE INDEX uniq_contest_participants_user ON public.contest_participants USING btree (contest_id, user_id);

CREATE UNIQUE INDEX uniq_contest_problems_display ON public.contest_problems USING btree (contest_id, display_id);

CREATE UNIQUE INDEX uniq_contest_problems_problem ON public.contest_problems USING btree (contest_id, problem_id);

CREATE UNIQUE INDEX uniq_generation_cases_order ON public.testcase_generation_cases USING btree (generation_id, sort_order);

CREATE UNIQUE INDEX uniq_learning_path_purchases ON public.learning_path_purchases USING btree (user_id, path_id);

CREATE UNIQUE INDEX uniq_learning_path_recommendations ON public.learning_path_recommendations USING btree (user_id, path_id);

CREATE UNIQUE INDEX uniq_notifications_dedupe ON public.notifications USING btree (dedupe_key);

CREATE UNIQUE INDEX uniq_permissions_code ON public.permissions USING btree (code);

CREATE UNIQUE INDEX uniq_problem_solution_unlocks ON public.problem_solution_unlocks USING btree (user_id, problem_id);

CREATE UNIQUE INDEX uniq_problems_slug ON public.problems USING btree (slug);

CREATE UNIQUE INDEX uniq_rating_history_user_contest ON public.rating_history USING btree (user_id, contest_id) WHERE (contest_id IS NOT NULL);

CREATE UNIQUE INDEX uniq_rating_history_user_period ON public.rating_history USING btree (user_id, kind, period_key) WHERE ((period_key)::text <> ''::text);

CREATE UNIQUE INDEX uniq_role_permissions ON public.role_permissions USING btree (role_id, permission_id);

CREATE UNIQUE INDEX uniq_roles_code ON public.roles USING btree (code);

CREATE UNIQUE INDEX uniq_scheduled_job_runs_active ON public.scheduled_job_runs USING btree (job_key) WHERE ((status)::text = ANY ((ARRAY['queued'::character varying, 'running'::character varying])::text[]));

CREATE UNIQUE INDEX uniq_shared_items_type_ref ON public.shared_items USING btree (type, ref_id);

CREATE UNIQUE INDEX uniq_similarity_match_pair ON public.contest_similarity_matches USING btree (run_id, original_submission_id, suspect_submission_id);

CREATE UNIQUE INDEX uniq_similarity_settings_problem ON public.contest_similarity_settings USING btree (contest_id, problem_id);

CREATE UNIQUE INDEX uniq_solution_post_likes ON public.problem_solution_post_likes USING btree (user_id, post_id);

CREATE UNIQUE INDEX uniq_testcases_problem_order ON public.problem_testcases USING btree (problem_id, sort_order);

CREATE UNIQUE INDEX uniq_user_achievements ON public.user_achievements USING btree (user_id, achievement_id);

CREATE UNIQUE INDEX uniq_user_ai_configs_user ON public.user_ai_configs USING btree (user_id);

CREATE UNIQUE INDEX uniq_user_ai_models_active ON public.user_ai_models USING btree (user_id) WHERE is_active;

CREATE UNIQUE INDEX uniq_user_ai_models_user_name ON public.user_ai_models USING btree (user_id, name);

CREATE UNIQUE INDEX uniq_user_badges ON public.user_badges USING btree (user_id, badge_id);

CREATE UNIQUE INDEX uniq_user_evaluations_author_target ON public.user_evaluations USING btree (author_user_id, target_user_id);

CREATE UNIQUE INDEX uniq_user_follows ON public.user_follows USING btree (follower_id, followee_id);

CREATE UNIQUE INDEX uniq_user_learning_progress ON public.user_learning_progress USING btree (user_id, node_id);

CREATE UNIQUE INDEX uniq_user_login_days ON public.user_login_days USING btree (user_id, login_date);

CREATE UNIQUE INDEX uniq_user_roles ON public.user_roles USING btree (user_id, role_id);

CREATE UNIQUE INDEX uniq_user_task_records ON public.user_task_records USING btree (user_id, task_id);

CREATE UNIQUE INDEX uniq_user_titles ON public.user_titles USING btree (user_id, title_id);

CREATE UNIQUE INDEX uniq_users_email ON public.users USING btree (email);

CREATE UNIQUE INDEX uniq_users_username ON public.users USING btree (username);

ALTER TABLE ONLY public.contest_problems
    ADD CONSTRAINT fk_contest_problems_problem FOREIGN KEY (problem_id) REFERENCES public.problems(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.announcements
    ADD CONSTRAINT fk_contests_announcements FOREIGN KEY (contest_id) REFERENCES public.contests(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.contest_participants
    ADD CONSTRAINT fk_contests_participants FOREIGN KEY (contest_id) REFERENCES public.contests(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.contest_problems
    ADD CONSTRAINT fk_contests_problems FOREIGN KEY (contest_id) REFERENCES public.contests(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.problem_testcases
    ADD CONSTRAINT fk_problems_testcases FOREIGN KEY (problem_id) REFERENCES public.problems(id) ON DELETE CASCADE;

ALTER TABLE ONLY public.submissions
    ADD CONSTRAINT fk_submissions_contest FOREIGN KEY (contest_id) REFERENCES public.contests(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.submissions
    ADD CONSTRAINT fk_submissions_problem FOREIGN KEY (problem_id) REFERENCES public.problems(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.submissions
    ADD CONSTRAINT fk_submissions_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE RESTRICT;

