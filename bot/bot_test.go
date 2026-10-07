package bot

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ayndri/dompetku/ledger"
	"github.com/ayndri/dompetku/telegram"
)

// fakeStore meniru store.Store di memori.
type fakeStore struct {
	txs  []ledger.Tx
	seen map[[2]int64]bool
	now  func() time.Time
}

func (f *fakeStore) Add(_ context.Context, chatID, updateID int64, e ledger.Entry) (ledger.Tx, error) {
	if f.seen == nil {
		f.seen = map[[2]int64]bool{}
	}
	key := [2]int64{chatID, updateID}
	if f.seen[key] {
		return ledger.Tx{}, ledger.ErrDuplicate
	}
	f.seen[key] = true
	tx := ledger.Tx{Entry: e, ID: int64(len(f.txs) + 1), CreatedAt: f.now()}
	f.txs = append(f.txs, tx)
	return tx, nil
}

func (f *fakeStore) DeleteLast(_ context.Context, _ int64) (ledger.Tx, bool, error) {
	if len(f.txs) == 0 {
		return ledger.Tx{}, false, nil
	}
	tx := f.txs[len(f.txs)-1]
	f.txs = f.txs[:len(f.txs)-1]
	return tx, true, nil
}

func (f *fakeStore) Totals(_ context.Context, _ int64, from, to time.Time) ([]ledger.CategoryTotal, error) {
	sums := map[ledger.CategoryTotal]int64{}
	for _, tx := range f.txs {
		if tx.CreatedAt.Before(from) || !tx.CreatedAt.Before(to) {
			continue
		}
		sums[ledger.CategoryTotal{Kind: tx.Kind, Category: tx.Category}] += tx.Amount
	}
	var out []ledger.CategoryTotal
	for k, v := range sums {
		k.Total = v
		out = append(out, k)
	}
	slices.SortFunc(out, func(a, b ledger.CategoryTotal) int { return int(b.Total - a.Total) })
	return out, nil
}

func (f *fakeStore) Recent(_ context.Context, _ int64, limit int) ([]ledger.Tx, error) {
	out := slices.Clone(f.txs)
	slices.Reverse(out)
	return out[:min(limit, len(out))], nil
}

func (f *fakeStore) Balance(_ context.Context, _ int64) (int64, error) {
	var v int64
	for _, tx := range f.txs {
		if tx.Kind == ledger.Income {
			v += tx.Amount
		} else {
			v -= tx.Amount
		}
	}
	return v, nil
}

type fakeSender struct{ sent []string }

func (f *fakeSender) SendMessage(_ context.Context, _ int64, text string) error {
	f.sent = append(f.sent, text)
	return nil
}

// Rabu, 7 Oktober 2026 pukul 01.00 WIB (masih 6 Oktober kalau dihitung UTC).
var testNow = time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC)

func newTestBot() (*Bot, *fakeStore, *fakeSender) {
	now := func() time.Time { return testNow }
	st := &fakeStore{now: now}
	snd := &fakeSender{}
	return &Bot{Store: st, Sender: snd, Now: now}, st, snd
}

func send(t *testing.T, b *Bot, updateID int64, text string) {
	t.Helper()
	u := telegram.Update{UpdateID: updateID, Message: &telegram.Message{Chat: telegram.Chat{ID: 42}, Text: text}}
	if err := b.HandleUpdate(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

func last(s *fakeSender) string { return s.sent[len(s.sent)-1] }

func TestRecordAndSummarize(t *testing.T) {
	b, _, snd := newTestBot()

	send(t, b, 1, "kopi 25rb")
	if got := last(snd); !strings.Contains(got, "Pengeluaran dicatat") || !strings.Contains(got, "Rp25.000") {
		t.Fatalf("balasan catat: %q", got)
	}
	send(t, b, 2, "gaji 5jt")
	send(t, b, 3, "makan siang 75rb")

	send(t, b, 4, "/hariini")
	got := last(snd)
	for _, want := range []string{
		"7 Okt",
		"Pemasukan: <b>Rp5.000.000</b>",
		"Pengeluaran: <b>Rp100.000</b>",
		"Selisih: <b>+Rp4.900.000</b>",
		"• Makan: Rp75.000 (75%)",
		"• Jajan: Rp25.000 (25%)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ringkasan tidak memuat %q:\n%s", want, got)
		}
	}
}

func TestDuplicateUpdateIsIgnored(t *testing.T) {
	b, st, snd := newTestBot()
	send(t, b, 7, "kopi 25rb")
	send(t, b, 7, "kopi 25rb")
	if len(st.txs) != 1 || len(snd.sent) != 1 {
		t.Fatalf("ingin 1 transaksi & 1 balasan, dapat %d & %d", len(st.txs), len(snd.sent))
	}
}

func TestUndo(t *testing.T) {
	b, st, snd := newTestBot()
	send(t, b, 1, "/batal")
	if !strings.Contains(last(snd), "Belum ada") {
		t.Errorf("batal saat kosong: %q", last(snd))
	}
	send(t, b, 2, "kopi 25rb")
	send(t, b, 3, "/batal")
	if len(st.txs) != 0 || !strings.Contains(last(snd), "Dihapus") {
		t.Errorf("batal gagal: %d tersisa, balasan %q", len(st.txs), last(snd))
	}
}

func TestUnparseableMessageGetsHint(t *testing.T) {
	b, st, snd := newTestBot()
	send(t, b, 1, "halo")
	if len(st.txs) != 0 || !strings.Contains(last(snd), "nominalnya nggak ketemu") {
		t.Errorf("pesan tanpa nominal: %q", last(snd))
	}
}

func TestAllowlist(t *testing.T) {
	b, st, snd := newTestBot()
	b.Allowed = map[int64]bool{1: true}
	send(t, b, 1, "kopi 25rb") // chat 42, tidak diizinkan
	if len(st.txs) != 0 || !strings.Contains(last(snd), "bot ini pribadi") {
		t.Errorf("chat asing tidak ditolak: %q", last(snd))
	}
}

func TestWeekRangeStartsMonday(t *testing.T) {
	from, to := ThisWeek.Range(testNow)
	if want := time.Date(2026, 10, 5, 0, 0, 0, 0, WIB); !from.Equal(want) {
		t.Errorf("awal minggu %v, ingin %v", from, want)
	}
	if want := time.Date(2026, 10, 12, 0, 0, 0, 0, WIB); !to.Equal(want) {
		t.Errorf("akhir minggu %v, ingin %v", to, want)
	}
}
