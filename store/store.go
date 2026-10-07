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

// Add menyimpan transaksi. Kalau update yang sama datang lagi (Telegram
// mengirim ulang saat webhook lambat), hasilnya ledger.ErrDuplicate.
func (s *Store) Add(ctx context.Context, chatID, updateID int64, e ledger.Entry) (ledger.Tx, error) {
	tx := ledger.Tx{Entry: e}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO transactions (chat_id, update_id, kind, amount, category, note)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (chat_id, update_id) DO NOTHING
		RETURNING id, created_at`,
		chatID, updateID, string(e.Kind), e.Amount, e.Category, e.Note,
	).Scan(&tx.ID, &tx.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx, ledger.ErrDuplicate
	}
	return tx, err
}

// DeleteLast menghapus transaksi terakhir milik chat. ok=false kalau kosong.
func (s *Store) DeleteLast(ctx context.Context, chatID int64) (ledger.Tx, bool, error) {
	tx, err := scanTx(s.pool.QueryRow(ctx, `
		DELETE FROM transactions WHERE id = (
			SELECT id FROM transactions WHERE chat_id = $1
			ORDER BY created_at DESC, id DESC LIMIT 1
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
		ORDER BY created_at DESC, id DESC LIMIT $2`, chatID, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ledger.Tx, error) { return scanTx(row) })
}

// Balance adalah total pemasukan dikurangi pengeluaran sejak awal.
func (s *Store) Balance(ctx context.Context, chatID int64) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(CASE WHEN kind = 'in' THEN amount ELSE -amount END), 0)::bigint
		FROM transactions WHERE chat_id = $1`, chatID).Scan(&v)
	return v, err
}
