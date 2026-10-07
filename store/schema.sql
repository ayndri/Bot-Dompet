CREATE TABLE IF NOT EXISTS transactions (
    id         BIGSERIAL PRIMARY KEY,
    chat_id    BIGINT      NOT NULL,
    -- update_id dari Telegram plus nomor baris, untuk menolak kiriman ulang
    -- yang sama. Satu pesan bisa berisi beberapa catatan.
    update_id  BIGINT,
    line       SMALLINT    NOT NULL DEFAULT 0,
    kind       TEXT        NOT NULL CHECK (kind IN ('in', 'out')),
    amount     BIGINT      NOT NULL CHECK (amount > 0),
    category   TEXT        NOT NULL,
    note       TEXT        NOT NULL DEFAULT '',
    -- Waktu transaksi terjadi; bisa mundur kalau dicatat "kemarin".
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Migrasi dari versi pertama, yang hanya mengizinkan satu catatan per pesan.
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS line SMALLINT NOT NULL DEFAULT 0;
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_chat_id_update_id_key;

CREATE UNIQUE INDEX IF NOT EXISTS transactions_update_line_idx
    ON transactions (chat_id, update_id, line);

CREATE INDEX IF NOT EXISTS transactions_chat_created_idx
    ON transactions (chat_id, created_at DESC);

-- Kategori yang diajarkan pengguna lewat tombol "Ganti kategori".
CREATE TABLE IF NOT EXISTS category_rules (
    chat_id  BIGINT NOT NULL,
    kind     TEXT   NOT NULL CHECK (kind IN ('in', 'out')),
    keyword  TEXT   NOT NULL,
    category TEXT   NOT NULL,
    PRIMARY KEY (chat_id, kind, keyword)
);

-- Batas pengeluaran bulanan per kategori.
CREATE TABLE IF NOT EXISTS budgets (
    chat_id  BIGINT NOT NULL,
    category TEXT   NOT NULL,
    amount   BIGINT NOT NULL CHECK (amount > 0),
    PRIMARY KEY (chat_id, category)
);
