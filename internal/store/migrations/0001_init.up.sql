-- Phase 1 schema: Scan and Report.

CREATE TABLE admin_users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username      TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per machine, however many sources have seen it. Hardware columns
-- hold the merged view; the per-source observations live in machine_sources.
CREATE TABLE machines (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name              TEXT,
    hostname          TEXT,
    os                TEXT,
    system_uuid       TEXT,
    cloud_provider    TEXT,
    cloud_instance_id TEXT,
    cloud_region      TEXT,
    instance_type     TEXT,
    state             TEXT,
    cpu_model         TEXT,
    cpu_cores         INTEGER,
    ram_bytes         BIGINT,
    disk_bytes_free   BIGINT,
    agent_version     TEXT,
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE gpus (
    id           BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    machine_id   UUID    NOT NULL REFERENCES machines (id) ON DELETE CASCADE,
    idx          INTEGER NOT NULL,
    vendor       TEXT,
    name         TEXT,
    uuid         TEXT,
    memory_bytes BIGINT,
    driver       TEXT,
    UNIQUE (machine_id, idx)
);

CREATE TABLE enrolment_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash BYTEA       NOT NULL UNIQUE,
    label      TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ
);

CREATE TABLE agents (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash          BYTEA       NOT NULL UNIQUE,
    enrolment_token_id  UUID REFERENCES enrolment_tokens (id) ON DELETE SET NULL,
    machine_id          UUID REFERENCES machines (id) ON DELETE SET NULL,
    reported_machine_id TEXT,
    hostname            TEXT,
    agent_version       TEXT,
    enrolled_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at        TIMESTAMPTZ
);

CREATE TABLE connectors (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind                  TEXT        NOT NULL CHECK (kind IN ('kubernetes', 'aws')),
    name                  TEXT        NOT NULL,
    config                JSONB       NOT NULL DEFAULT '{}',
    -- AES-GCM: nonce followed by ciphertext. Never returned by the API.
    credentials_encrypted BYTEA,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE scans (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    connector_id   UUID        NOT NULL REFERENCES connectors (id) ON DELETE CASCADE,
    status         TEXT        NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    started_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at    TIMESTAMPTZ,
    machines_found INTEGER,
    error          TEXT
);

CREATE INDEX scans_connector_started_idx ON scans (connector_id, started_at DESC);

-- What one source (an agent, or one connector) last saw for a machine.
-- The identity columns are what the merge matches on.
CREATE TABLE machine_sources (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    machine_id        UUID        NOT NULL REFERENCES machines (id) ON DELETE CASCADE,
    kind              TEXT        NOT NULL CHECK (kind IN ('agent', 'kubernetes', 'aws')),
    connector_id      UUID REFERENCES connectors (id) ON DELETE CASCADE,
    agent_id          UUID REFERENCES agents (id) ON DELETE CASCADE,
    external_id       TEXT        NOT NULL,
    cloud_instance_id TEXT,
    system_uuid       TEXT,
    hostname          TEXT,
    data              JSONB       NOT NULL,
    first_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((kind = 'agent') = (connector_id IS NULL))
);

CREATE UNIQUE INDEX machine_sources_agent_key
    ON machine_sources (external_id) WHERE kind = 'agent';
CREATE UNIQUE INDEX machine_sources_connector_key
    ON machine_sources (connector_id, external_id) WHERE kind <> 'agent';
CREATE INDEX machine_sources_machine_idx ON machine_sources (machine_id);
CREATE INDEX machine_sources_instance_idx
    ON machine_sources (cloud_instance_id) WHERE cloud_instance_id IS NOT NULL;
CREATE INDEX machine_sources_system_uuid_idx
    ON machine_sources (system_uuid) WHERE system_uuid IS NOT NULL;
CREATE INDEX machine_sources_hostname_idx
    ON machine_sources (hostname) WHERE hostname IS NOT NULL;

-- One row per machine per minute. Kept for 14 days by the retention job.
-- gpu_count, gpus_idle and idle are worked out when the sample arrives so
-- that idle reports are a plain aggregate.
CREATE TABLE utilisation_samples (
    machine_id     UUID        NOT NULL REFERENCES machines (id) ON DELETE CASCADE,
    at             TIMESTAMPTZ NOT NULL,
    cpu_pct        REAL,
    ram_used_bytes BIGINT,
    gpus           JSONB       NOT NULL DEFAULT '[]',
    gpu_count      SMALLINT    NOT NULL DEFAULT 0,
    gpus_idle      SMALLINT    NOT NULL DEFAULT 0,
    idle           BOOLEAN     NOT NULL,
    PRIMARY KEY (machine_id, at)
);

CREATE INDEX utilisation_samples_at_idx ON utilisation_samples USING brin (at);
