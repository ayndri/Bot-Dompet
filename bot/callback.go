package bot

import (
	"context"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"

	"github.com/ayndri/dompetku/ledger"
	"github.com/ayndri/dompetku/telegram"
)

// Data tombol: "cat:<id>" buka pilihan kategori, "set:<id>:<urutan>" pilih
// kategori, "back:<id>" tutup pilihan. Telegram membatasi data 64 byte.
func changeCategoryButton(txID int64) *telegram.InlineKeyboardMarkup {
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "✏️ Ganti kategori", CallbackData: fmt.Sprintf("cat:%d", txID)},
	}}}
}

func categoryPicker(tx ledger.Tx) *telegram.InlineKeyboardMarkup {
	var rows [][]telegram.InlineKeyboardButton
	var row []telegram.InlineKeyboardButton
	for i, c := range ledger.Categories(tx.Kind) {
		label := c
		if c == tx.Category {
			label = "✓ " + c
		}
		row = append(row, telegram.InlineKeyboardButton{Text: label, CallbackData: fmt.Sprintf("set:%d:%d", tx.ID, i)})
		if len(row) == 3 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "« Kembali", CallbackData: fmt.Sprintf("back:%d", tx.ID)}})
	return &telegram.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func (b *Bot) handleCallback(ctx context.Context, q *telegram.CallbackQuery) error {
	if q.Message == nil {
		return b.Sender.AnswerCallback(ctx, q.ID, "")
	}
	chatID := q.Message.Chat.ID
	if !b.allowed(chatID) {
		return b.Sender.AnswerCallback(ctx, q.ID, "Bot ini pribadi.")
	}

	notice, err := b.callback(ctx, chatID, q.Message.MessageID, q.Data)
	if err != nil {
		log.Printf("tombol %q di chat %d: %v", q.Data, chatID, err)
		notice = "Lagi ada gangguan, coba lagi sebentar ya."
	}
	// Tombol harus selalu dijawab, kalau tidak ikon loading-nya berputar terus.
	return b.Sender.AnswerCallback(ctx, q.ID, notice)
}

func (b *Bot) callback(ctx context.Context, chatID, messageID int64, data string) (string, error) {
	action, rest, _ := strings.Cut(data, ":")
	idPart, arg, _ := strings.Cut(rest, ":")
	id, err := strconv.ParseInt(idPart, 10, 64)
	if err != nil {
		return "", nil // tombol dari versi lama atau data rusak; abaikan
	}

	tx, ok, err := b.Store.GetTx(ctx, chatID, id)
	if err != nil {
		return "", err
	}
	if !ok {
		return "Catatan ini udah dihapus.", b.Sender.EditMessage(ctx, chatID, messageID, "🗑️ <i>Catatan ini udah dihapus.</i>", nil)
	}
	today := startOfDay(b.now())

	switch action {
	case "cat":
		return "", b.Sender.EditMessage(ctx, chatID, messageID, formatAdded(tx, today)+"\n\nPilih kategori yang benar:", categoryPicker(tx))
	case "back":
		return "", b.Sender.EditMessage(ctx, chatID, messageID, formatAdded(tx, today), changeCategoryButton(tx.ID))
	case "set":
		cats := ledger.Categories(tx.Kind)
		i, err := strconv.Atoi(arg)
		if err != nil || i < 0 || i >= len(cats) {
			return "", nil
		}
		tx, ok, err = b.Store.SetCategory(ctx, chatID, id, cats[i])
		if err != nil || !ok {
			return "", err
		}

		msg := formatAdded(tx, today)
		if kw := ledger.Keyword(tx.Note); kw != "" {
			if err := b.Store.LearnRule(ctx, chatID, tx.Kind, kw, tx.Category); err != nil {
				return "", err
			}
			msg += fmt.Sprintf("\n\n✏️ Oke, lain kali <i>%s</i> otomatis masuk %s.",
				html.EscapeString(kw), html.EscapeString(tx.Category))
		}
		return "Kategori diganti ke " + tx.Category, b.Sender.EditMessage(ctx, chatID, messageID, msg, changeCategoryButton(tx.ID))
	}
	return "", nil
}
