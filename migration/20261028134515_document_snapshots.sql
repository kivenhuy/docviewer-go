-- +goose Up

-- One row per (vin, source): the latest successful fetch from that source.
-- Read before fan-out (cache hit if fetched_at is within TTL) and used as a
-- stale fallback when the source fails. Upserted after every successful fetch.
CREATE TABLE document_snapshots (
    vin        TEXT        NOT NULL,
    source     TEXT        NOT NULL,
    documents  JSONB       NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (vin, source)
);

-- Audit log: one row per call to an external source.
CREATE TABLE source_call_log (
    id          BIGSERIAL   PRIMARY KEY,
    vin         TEXT        NOT NULL,
    source      TEXT        NOT NULL,
    outcome     TEXT        NOT NULL,
    duration_ms INT         NOT NULL,
    error       TEXT,
    called_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT chk_outcome CHECK (outcome IN ('ok', 'empty', 'error', 'timeout'))
);

CREATE INDEX idx_source_call_log_vin_called_at ON source_call_log (vin, called_at DESC);

-- +goose Down
DROP TABLE IF EXISTS source_call_log;
DROP TABLE IF EXISTS document_snapshots;
