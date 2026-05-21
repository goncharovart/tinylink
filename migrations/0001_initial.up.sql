CREATE TABLE IF NOT EXISTS links (
    code        text PRIMARY KEY,
    url         text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    hit_count   bigint NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS links_created_at_idx ON links (created_at);
