// Package ledger berisi aturan inti: jenis transaksi, cara membaca pesan,
// pengelompokan kategori, dan format rupiah. Tidak tahu apa-apa soal
// Telegram maupun database, jadi gampang dites.
package ledger

import (
	"errors"
	"time"
)

type Kind string

const (
	Expense Kind = "out"
	Income  Kind = "in"
)

func (k Kind) Label() string {
	if k == Income {
		return "Pemasukan"
	}
	return "Pengeluaran"
}

// Entry adalah hasil membaca satu pesan, sebelum disimpan.
type Entry struct {
	Kind     Kind
	Amount   int64
	Category string
	Note     string
}

// Tx adalah Entry yang sudah tersimpan.
type Tx struct {
	Entry
	ID        int64
	CreatedAt time.Time
}

type CategoryTotal struct {
	Kind     Kind
	Category string
	Total    int64
}

// ErrDuplicate dikembalikan saat Telegram mengirim ulang update yang sama.
var ErrDuplicate = errors.New("update sudah pernah diproses")
