CREATE TABLE IF NOT EXISTS transactions (
    id         BIGSERIAL PRIMARY KEY,
    chat_id    BIGINT      NOT NULL,
    -- update_id dari Telegram, untuk menolak kiriman ulang yang sama.
    update_id  BIGINT,
    kind       TEXT        NOT NULL CHECK (kind IN ('in', 'out')),
    amount     BIGINT      NOT NULL CHECK (amount > 0),
    category   TEXT        NOT NULL,
    note       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (chat_id, update_id)
);

CREATE INDEX IF NOT EXISTS transactions_chat_created_idx
    ON transactions (chat_id, created_at DESC);
