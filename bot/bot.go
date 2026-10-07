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
	Add(ctx context.Context, chatID, updateID int64, txs []ledger.Tx) ([]ledger.Tx, error)
	GetTx(ctx context.Context, chatID, id int64) (ledger.Tx, bool, error)
	SetCategory(ctx context.Context, chatID, id int64, category string) (ledger.Tx, bool, error)
	DeleteLast(ctx context.Context, chatID int64) (ledger.Tx, bool, error)
	Totals(ctx context.Context, chatID int64, from, to time.Time) ([]ledger.CategoryTotal, error)
	Recent(ctx context.Context, chatID int64, limit int) ([]ledger.Tx, error)
	Balance(ctx context.Context, chatID int64) (int64, error)
	ActiveChats(ctx context.Context, since time.Time) ([]int64, error)
	LearnRule(ctx context.Context, chatID int64, kind ledger.Kind, keyword, category string) error
	Rules(ctx context.Context, chatID int64) (ledger.Learned, error)
	SetBudget(ctx context.Context, chatID int64, b ledger.Budget) error
	DeleteBudget(ctx context.Context, chatID int64, category string) (bool, error)
	Budgets(ctx context.Context, chatID int64) ([]ledger.Budget, error)
}

type Sender interface {
	SendMessage(ctx context.Context, chatID int64, text string, kb *telegram.InlineKeyboardMarkup) error
	EditMessage(ctx context.Context, chatID, messageID int64, text string, kb *telegram.InlineKeyboardMarkup) error
	AnswerCallback(ctx context.Context, callbackID, text string) error
}

type Bot struct {
	Store  Store
	Sender Sender
	// Allowed membatasi siapa yang boleh memakai bot. Kosong = semua orang.
	Allowed map[int64]bool
	// Now bisa diganti di tes. Nil = time.Now.
	Now func() time.Time
}

// Reply adalah balasan untuk satu pesan, boleh dengan tombol.
type Reply struct {
	Text     string
	Keyboard *telegram.InlineKeyboardMarkup
}

func text(s string) Reply { return Reply{Text: s} }

// Commands ditampilkan sebagai menu "/" di Telegram.
var Commands = []telegram.BotCommand{
	{Command: "hariini", Description: "Ringkasan hari ini"},
	{Command: "mingguini", Description: "Ringkasan minggu ini"},
	{Command: "bulanini", Description: "Ringkasan bulan ini"},
	{Command: "laporan", Description: "Laporan minggu ini vs minggu lalu"},
	{Command: "budget", Description: "Lihat atau atur budget bulanan"},
	{Command: "saldo", Description: "Pemasukan dikurangi pengeluaran sejak awal"},
	{Command: "riwayat", Description: "10 catatan terakhir"},
	{Command: "batal", Description: "Hapus catatan terakhir"},
	{Command: "bantuan", Description: "Cara pakai"},
}

func (b *Bot) now() time.Time {
	if b.Now != nil {
		return b.Now().In(WIB)
	}
	return time.Now().In(WIB)
}

func (b *Bot) allowed(chatID int64) bool {
	return len(b.Allowed) == 0 || b.Allowed[chatID]
}

func (b *Bot) HandleUpdate(ctx context.Context, u telegram.Update) error {
	switch {
	case u.CallbackQuery != nil:
		return b.handleCallback(ctx, u.CallbackQuery)
	case u.Message != nil && strings.TrimSpace(u.Message.Text) != "":
		return b.handleMessage(ctx, u.UpdateID, u.Message)
	}
	return nil
}

func (b *Bot) handleMessage(ctx context.Context, updateID int64, m *telegram.Message) error {
	chatID := m.Chat.ID
	if !b.allowed(chatID) {
		return b.Sender.SendMessage(ctx, chatID,
			fmt.Sprintf("Maaf, bot ini pribadi. ID chat kamu: <code>%d</code>", chatID), nil)
	}

	reply, err := b.respond(ctx, updateID, chatID, strings.TrimSpace(m.Text))
	if errors.Is(err, ledger.ErrDuplicate) {
		return nil // sudah dibalas waktu kiriman pertama
	}
	if err != nil {
		log.Printf("update %d: %v", updateID, err)
		reply = text("Waduh, lagi ada gangguan. Coba kirim lagi sebentar ya.")
	}
	return b.Sender.SendMessage(ctx, chatID, reply.Text, reply.Keyboard)
}

func (b *Bot) respond(ctx context.Context, updateID, chatID int64, msg string) (Reply, error) {
	if !strings.HasPrefix(msg, "/") {
		return b.record(ctx, updateID, chatID, msg)
	}

	fields := strings.Fields(msg)
	var s string
	var err error
	switch command(fields[0]) {
	case "/start", "/bantuan", "/help":
		s = helpText
	case "/hariini":
		s, err = b.summary(ctx, chatID, Today)
	case "/mingguini":
		s, err = b.summary(ctx, chatID, ThisWeek)
	case "/bulanini":
		s, err = b.summary(ctx, chatID, ThisMonth)
	case "/laporan":
		s, err = b.weeklyReport(ctx, chatID)
	case "/budget":
		s, err = b.budget(ctx, chatID, fields[1:])
	case "/saldo":
		s, err = b.balance(ctx, chatID)
	case "/riwayat":
		s, err = b.history(ctx, chatID)
	case "/batal":
		s, err = b.undo(ctx, chatID)
	default:
		s = "Perintah itu belum ada. Ketik /bantuan buat lihat daftarnya."
	}
	return text(s), err
}

// command mengubah "/HariIni@dompetku_bot" menjadi "/hariini".
func command(word string) string {
	cmd, _, _ := strings.Cut(word, "@")
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

func (b *Bot) weeklyReport(ctx context.Context, chatID int64) (string, error) {
	from, to := ThisWeek.Range(b.now())
	cur, err := b.Store.Totals(ctx, chatID, from, to)
	if err != nil {
		return "", err
	}
	prev, err := b.Store.Totals(ctx, chatID, from.AddDate(0, 0, -7), from)
	if err != nil {
		return "", err
	}
	return formatWeekly(from, to, cur, prev), nil
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
