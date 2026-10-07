// Package bot menghubungkan pesan Telegram dengan pencatatan transaksi.
// Dipakai sama persis oleh webhook (Vercel) dan mode polling lokal.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ayndri/dompetku/ledger"
	"github.com/ayndri/dompetku/telegram"
)

type Store interface {
	Add(ctx context.Context, chatID, updateID int64, e ledger.Entry) (ledger.Tx, error)
	DeleteLast(ctx context.Context, chatID int64) (ledger.Tx, bool, error)
	Totals(ctx context.Context, chatID int64, from, to time.Time) ([]ledger.CategoryTotal, error)
	Recent(ctx context.Context, chatID int64, limit int) ([]ledger.Tx, error)
	Balance(ctx context.Context, chatID int64) (int64, error)
}

type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string) error
}

type Bot struct {
	Store  Store
	Sender Sender
	// Allowed membatasi siapa yang boleh memakai bot. Kosong = semua orang.
	Allowed map[int64]bool
	// Now bisa diganti di tes. Nil = time.Now.
	Now func() time.Time
}

// Commands ditampilkan sebagai menu "/" di Telegram.
var Commands = []telegram.BotCommand{
	{Command: "hariini", Description: "Ringkasan hari ini"},
	{Command: "mingguini", Description: "Ringkasan minggu ini"},
	{Command: "bulanini", Description: "Ringkasan bulan ini"},
	{Command: "saldo", Description: "Pemasukan dikurangi pengeluaran sejak awal"},
	{Command: "riwayat", Description: "10 catatan terakhir"},
	{Command: "batal", Description: "Hapus catatan terakhir"},
	{Command: "bantuan", Description: "Cara pakai"},
}

func (b *Bot) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

func (b *Bot) HandleUpdate(ctx context.Context, u telegram.Update) error {
	m := u.Message
	if m == nil || strings.TrimSpace(m.Text) == "" {
		return nil
	}
	chatID := m.Chat.ID

	if len(b.Allowed) > 0 && !b.Allowed[chatID] {
		return b.Sender.SendMessage(ctx, chatID,
			fmt.Sprintf("Maaf, bot ini pribadi. ID chat kamu: <code>%d</code>", chatID))
	}

	reply, err := b.respond(ctx, u.UpdateID, chatID, strings.TrimSpace(m.Text))
	if errors.Is(err, ledger.ErrDuplicate) {
		return nil // sudah dibalas waktu kiriman pertama
	}
	if err != nil {
		log.Printf("update %d: %v", u.UpdateID, err)
		reply = "Waduh, lagi ada gangguan. Coba kirim lagi sebentar ya."
	}
	return b.Sender.SendMessage(ctx, chatID, reply)
}

func (b *Bot) respond(ctx context.Context, updateID, chatID int64, text string) (string, error) {
	if strings.HasPrefix(text, "/") {
		switch command(text) {
		case "/start", "/bantuan", "/help":
			return helpText, nil
		case "/hariini":
			return b.summary(ctx, chatID, Today)
		case "/mingguini":
			return b.summary(ctx, chatID, ThisWeek)
		case "/bulanini":
			return b.summary(ctx, chatID, ThisMonth)
		case "/saldo":
			return b.balance(ctx, chatID)
		case "/riwayat":
			return b.history(ctx, chatID)
		case "/batal":
			return b.undo(ctx, chatID)
		default:
			return "Perintah itu belum ada. Ketik /bantuan buat lihat daftarnya.", nil
		}
	}

	entry, err := ledger.Parse(text)
	if err != nil {
		return parseHint, nil
	}
	tx, err := b.Store.Add(ctx, chatID, updateID, entry)
	if err != nil {
		return "", err
	}
	return formatAdded(tx), nil
}

// command mengubah "/HariIni@dompetku_bot besok" menjadi "/hariini".
func command(text string) string {
	cmd, _, _ := strings.Cut(strings.Fields(text)[0], "@")
	return strings.ToLower(cmd)
}

func (b *Bot) summary(ctx context.Context, chatID int64, p Period) (string, error) {
	from, to := p.Range(b.now())
	totals, err := b.Store.Totals(ctx, chatID, from, to)
	if err != nil {
		return "", err
	}
	return formatSummary(p, from, to, totals), nil
}

func (b *Bot) balance(ctx context.Context, chatID int64) (string, error) {
	v, err := b.Store.Balance(ctx, chatID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("💼 Saldo: <b>%s</b>\n<i>Total pemasukan dikurangi pengeluaran sejak awal pakai bot.</i>", ledger.Rupiah(v)), nil
}

func (b *Bot) history(ctx context.Context, chatID int64) (string, error) {
	txs, err := b.Store.Recent(ctx, chatID, 10)
	if err != nil {
		return "", err
	}
	return formatHistory(txs), nil
}

func (b *Bot) undo(ctx context.Context, chatID int64) (string, error) {
	tx, ok, err := b.Store.DeleteLast(ctx, chatID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "Belum ada catatan yang bisa dihapus.", nil
	}
	return "🗑️ Dihapus: " + formatLine(tx), nil
}
