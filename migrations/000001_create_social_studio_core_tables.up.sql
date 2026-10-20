CREATE TABLE constraint_profiles (
    platform TEXT PRIMARY KEY CHECK (platform IN ('X', 'LINKEDIN', 'TELEGRAM')),
    max_length INTEGER NOT NULL CHECK (max_length > 0),
    tone TEXT NOT NULL CHECK (tone IN ('CONCISE', 'PROFESSIONAL', 'CONVERSATIONAL')),
    max_hashtags INTEGER NOT NULL CHECK (max_hashtags >= 0)
);

INSERT INTO constraint_profiles (platform, max_length, tone, max_hashtags) VALUES
    ('X', 280, 'CONCISE', 3),
    ('LINKEDIN', 3000, 'PROFESSIONAL', 5),
    ('TELEGRAM', 4096, 'CONVERSATIONAL', 5);

CREATE TABLE posts (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    source_type TEXT NOT NULL CHECK (source_type IN ('URL', 'MARKDOWN')),
    source_url TEXT,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version BIGINT NOT NULL DEFAULT 1
);

CREATE TABLE variants (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    post_id UUID NOT NULL REFERENCES posts(id),
    platform TEXT NOT NULL REFERENCES constraint_profiles(platform),
    content TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('DRAFT', 'APPROVED', 'REJECTED', 'PUBLISHED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version BIGINT NOT NULL DEFAULT 1
);

CREATE TABLE schedules (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    variant_id UUID NOT NULL REFERENCES variants(id),
    scheduled_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    version BIGINT NOT NULL DEFAULT 1,
    UNIQUE (variant_id, scheduled_at)
);

CREATE TABLE publish_attempts (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    schedule_id UUID NOT NULL REFERENCES schedules(id),
    variant_id UUID NOT NULL REFERENCES variants(id),
    idempotency_key TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('SUCCESS', 'FAILED')),
    external_post_id TEXT,
    error_message TEXT,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_schedules_due ON schedules (status, scheduled_at);
CREATE INDEX idx_publish_attempts_schedule ON publish_attempts (schedule_id);
