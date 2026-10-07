package bot

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"strings"
	"time"

	"github.com/ayndri/dompetku/ledger"
)

// record mencatat pesan biasa. Satu pesan boleh berisi beberapa baris;
// tiap baris satu catatan. Baris yang isinya cuma tanggal ("kemarin")
// berlaku untuk baris-baris di bawahnya.
func (b *Bot) record(ctx context.Context, updateID, chatID int64, msg string) (Reply, error) {
	now := b.now()
	today := startOfDay(now)

	learned, err := b.Store.Rules(ctx, chatID)
	if err != nil {
		return Reply{}, err
	}

	var txs []ledger.Tx
	var failed []string
	futureDate := false
	var blockDay time.Time // dari baris yang isinya cuma tanggal
	lines := nonEmptyLines(msg)
	for _, line := range lines {
		day, rest, err := ledger.SplitDate(line, today)
		if err != nil {
			futureDate = true
			failed = append(failed, fmt.Sprintf("<code>%s</code>: %s", html.EscapeString(line), err))
			continue
		}
		if !day.IsZero() && strings.TrimSpace(rest) == "" {
			blockDay = day
			continue
		}
		if day.IsZero() {
			day = blockDay
		}

		e, err := ledger.Parse(rest)
		if err != nil {
			failed = append(failed, fmt.Sprintf("<code>%s</code>: nominalnya nggak ketemu", html.EscapeString(line)))
			continue
		}
		if c, ok := learned.Lookup(e.Kind, e.Note); ok {
			e.Category = c
		}

		// Catatan bertanggal mundur memakai jam yang sama dengan sekarang,
		// supaya urutannya tetap masuk akal di hari itu.
		at := now
		if !day.IsZero() {
			at = day.Add(now.Sub(today))
		}
		txs = append(txs, ledger.Tx{Entry: e, CreatedAt: at})
	}

	if len(txs) == 0 {
		if len(failed) == 0 || (len(lines) == 1 && !futureDate) {
			return text(parseHint), nil
		}
		return text("Belum ada yang kecatat:\n• " + strings.Join(failed, "\n• ") + "\n\n" + parseHint), nil
	}

	saved, err := b.Store.Add(ctx, chatID, updateID, txs)
	if err != nil {
		return Reply{}, err
	}

	var reply Reply
	if len(saved) == 1 && len(failed) == 0 {
		reply = Reply{Text: formatAdded(saved[0], today), Keyboard: changeCategoryButton(saved[0].ID)}
	} else {
		reply = text(formatAddedMany(saved, failed, today))
	}

	// Peringatan budget hanya tambahan; kalau gagal, catatan tetap tersimpan.
	warnings, err := b.budgetWarnings(ctx, chatID, saved)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("cek budget chat %d: %v", chatID, err)
	}
	if len(warnings) > 0 {
		reply.Text += "\n\n" + strings.Join(warnings, "\n")
	}
	return reply, nil
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.In(WIB).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, WIB)
}
