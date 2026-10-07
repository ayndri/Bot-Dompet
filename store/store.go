// Package store menyimpan transaksi di Postgres (Neon).
package store

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/ayndri/dompetku/ledger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schema string

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL tidak valid: %w", err)
	}
	// Fungsi serverless bisa jalan di banyak instance sekaligus. Koneksi per
	// instance dibuat sedikit supaya kuota koneksi Neon tidak habis.
	cfg.MaxConns = 2
	// Tanpa cache prepared statement, biar aman lewat connection pooler Neon.
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeExec

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	// schema.sql berisi beberapa statement; hanya simple protocol yang bisa.
	_, err := s.pool.Exec(ctx, schema, pgx.QueryExecModeSimpleProtocol)
	return err
}

type scanner interface{ Scan(dest ...any) error }

func scanTx(row scanner) (ledger.Tx, error) {
	var tx ledger.Tx
	var kind string
	err := row.Scan(&tx.ID, &kind, &tx.Amount, &tx.Category, &tx.Note, &tx.CreatedAt)
	tx.Kind = ledger.Kind(kind)
	return tx, err
}

const txColumns = "id, kind, amount, category, note, created_at"

// Add menyimpan beberapa transaksi dari satu pesan sekaligus, semuanya atau
// tidak sama sekali. CreatedAt tiap Tx dipakai sebagai waktu transaksi.
// Kalau update yang sama datang lagi (Telegram mengirim ulang saat webhook
// lambat), hasilnya ledger.ErrDuplicate.
func (s *Store) Add(ctx context.Context, chatID, updateID int64, txs []ledger.Tx) ([]ledger.Tx, error) {
	dbtx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer dbtx.Rollback(ctx)

	saved := make([]ledger.Tx, 0, len(txs))
	for i, tx := range txs {
		err := dbtx.QueryRow(ctx, `
			INSERT INTO transactions (chat_id, update_id, line, kind, amount, category, note, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (chat_id, update_id, line) DO NOTHING
			RETURNING id`,
			chatID, updateID, i, string(tx.Kind), tx.Amount, tx.Category, tx.Note, tx.CreatedAt,
		).Scan(&tx.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ledger.ErrDuplicate
		}
		if err != nil {
			return nil, err
		}
		saved = append(saved, tx)
	}
	return saved, dbtx.Commit(ctx)
}

func (s *Store) GetTx(ctx context.Context, chatID, id int64) (ledger.Tx, bool, error) {
	tx, err := scanTx(s.pool.QueryRow(ctx, `
		SELECT `+txColumns+` FROM transactions WHERE chat_id = $1 AND id = $2`, chatID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return tx, false, nil
	}
	return tx, err == nil, err
}

func (s *Store) SetCategory(ctx context.Context, chatID, id int64, category string) (ledger.Tx, bool, error) {
	tx, err := scanTx(s.pool.QueryRow(ctx, `
		UPDATE transactions SET category = $3 WHERE chat_id = $1 AND id = $2
		RETURNING `+txColumns, chatID, id, category))
	if errors.Is(err, pgx.ErrNoRows) {
		return tx, false, nil
	}
	return tx, err == nil, err
}

// DeleteLast menghapus transaksi yang terakhir diketik (bukan yang tanggalnya
// paling baru, karena catatan bisa bertanggal mundur). ok=false kalau kosong.
func (s *Store) DeleteLast(ctx context.Context, chatID int64) (ledger.Tx, bool, error) {
	tx, err := scanTx(s.pool.QueryRow(ctx, `
		DELETE FROM transactions WHERE id = (
			SELECT id FROM transactions WHERE chat_id = $1
			ORDER BY id DESC LIMIT 1
		)
		RETURNING `+txColumns, chatID))
	if errors.Is(err, pgx.ErrNoRows) {
		return tx, false, nil
	}
	return tx, err == nil, err
}

// Totals menjumlahkan per jenis dan kategori dalam rentang [from, to),
// diurutkan dari yang terbesar.
func (s *Store) Totals(ctx context.Context, chatID int64, from, to time.Time) ([]ledger.CategoryTotal, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT kind, category, SUM(amount)::bigint
		FROM transactions
		WHERE chat_id = $1 AND created_at >= $2 AND created_at < $3
		GROUP BY kind, category
		ORDER BY 3 DESC`, chatID, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ledger.CategoryTotal, error) {
		var t ledger.CategoryTotal
		var kind string
		err := row.Scan(&kind, &t.Category, &t.Total)
		t.Kind = ledger.Kind(kind)
		return t, err
	})
}

func (s *Store) Recent(ctx context.Context, chatID int64, limit int) ([]ledger.Tx, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+txColumns+` FROM transactions
		WHERE chat_id = $1
		ORDER BY id DESC LIMIT $2`, chatID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ledger.Tx, error) { return scanTx(row) })
}

// ActiveChats mengembalikan chat yang punya catatan sejak waktu tertentu.
func (s *Store) ActiveChats(ctx context.Context, since time.Time) ([]int64, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT chat_id FROM transactions WHERE created_at >= $1`, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// Balance adalah total pemasukan dikurangi pengeluaran sejak awal.
func (s *Store) Balance(ctx context.Context, chatID int64) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(CASE WHEN kind = 'in' THEN amount ELSE -amount END), 0)::bigint
		FROM transactions WHERE chat_id = $1`, chatID).Scan(&v)
	return v, err
}

func (s *Store) LearnRule(ctx context.Context, chatID int64, kind ledger.Kind, keyword, category string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO category_rules (chat_id, kind, keyword, category) VALUES ($1, $2, $3, $4)
		ON CONFLICT (chat_id, kind, keyword) DO UPDATE SET category = EXCLUDED.category`,
		chatID, string(kind), keyword, category)
	return err
}

func (s *Store) Rules(ctx context.Context, chatID int64) (ledger.Learned, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT kind, keyword, category FROM category_rules WHERE chat_id = $1`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	learned := ledger.Learned{}
	for rows.Next() {
		var kind, keyword, category string
		if err := rows.Scan(&kind, &keyword, &category); err != nil {
			return nil, err
		}
		k := ledger.Kind(kind)
		if learned[k] == nil {
			learned[k] = map[string]string{}
		}
		learned[k][keyword] = category
	}
	return learned, rows.Err()
}

func (s *Store) SetBudget(ctx context.Context, chatID int64, b ledger.Budget) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO budgets (chat_id, category, amount) VALUES ($1, $2, $3)
		ON CONFLICT (chat_id, category) DO UPDATE SET amount = EXCLUDED.amount`,
		chatID, b.Category, b.Amount)
	return err
}

func (s *Store) DeleteBudget(ctx context.Context, chatID int64, category string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM budgets WHERE chat_id = $1 AND category = $2`, chatID, category)
	return tag.RowsAffected() > 0, err
}

func (s *Store) Budgets(ctx context.Context, chatID int64) ([]ledger.Budget, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT category, amount FROM budgets WHERE chat_id = $1 ORDER BY category`, chatID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[ledger.Budget])
}
