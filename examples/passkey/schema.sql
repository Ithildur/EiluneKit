CREATE TABLE passkeys (
    rp_id text NOT NULL,
    id text NOT NULL,
    user_id text NOT NULL,
    name text NOT NULL,
    created_at timestamptz NOT NULL,
    last_used_at timestamptz,
    credential jsonb NOT NULL,
    PRIMARY KEY (rp_id, id)
);
CREATE INDEX passkeys_user ON passkeys (rp_id, user_id);
