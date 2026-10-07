package bot

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ayndri/dompetku/ledger"
	"github.com/ayndri/dompetku/telegram"
)

// fakeStore meniru store.Store di memori untuk satu chat.
type fakeStore struct {
	txs     []ledger.Tx
	nextID  int64
	seen    map[int64]bool
	rules   ledger.Learned
	budgets map[string]int64
}

func (f *fakeStore) Add(_ context.Context, _, updateID int64, txs []ledger.Tx) ([]ledger.Tx, error) {
	if f.seen == nil {
		f.seen = map[int64]bool{}
	}
	if f.seen[updateID] {
		return nil, ledger.ErrDuplicate
	}
	f.seen[updateID] = true
	var saved []ledger.Tx
	for _, tx := range txs {
		f.nextID++
		tx.ID = f.nextID
		f.txs = append(f.txs, tx)
		saved = append(saved, tx)
	}
	return saved, nil
}

func (f *fakeStore) find(id int64) int {
	return slices.IndexFunc(f.txs, func(tx ledger.Tx) bool { return tx.ID == id })
}

func (f *fakeStore) GetTx(_ context.Context, _, id int64) (ledger.Tx, bool, error) {
	if i := f.find(id); i >= 0 {
		return f.txs[i], true, nil
	}
	return ledger.Tx{}, false, nil
}

func (f *fakeStore) SetCategory(_ context.Context, _, id int64, category string) (ledger.Tx, bool, error) {
	i := f.find(id)
	if i < 0 {
		return ledger.Tx{}, false, nil
	}
	f.txs[i].Category = category
	return f.txs[i], true, nil
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

func (f *fakeStore) ActiveChats(_ context.Context, since time.Time) ([]int64, error) {
	for _, tx := range f.txs {
		if !tx.CreatedAt.Before(since) {
			return []int64{chatID}, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) LearnRule(_ context.Context, _ int64, kind ledger.Kind, keyword, category string) error {
	if f.rules == nil {
		f.rules = ledger.Learned{}
	}
	if f.rules[kind] == nil {
		f.rules[kind] = map[string]string{}
	}
	f.rules[kind][keyword] = category
	return nil
}

func (f *fakeStore) Rules(_ context.Context, _ int64) (ledger.Learned, error) {
	return f.rules, nil
}

func (f *fakeStore) SetBudget(_ context.Context, _ int64, b ledger.Budget) error {
	if f.budgets == nil {
		f.budgets = map[string]int64{}
	}
	f.budgets[b.Category] = b.Amount
	return nil
}

func (f *fakeStore) DeleteBudget(_ context.Context, _ int64, category string) (bool, error) {
	_, ok := f.budgets[category]
	delete(f.budgets, category)
	return ok, nil
}

func (f *fakeStore) Budgets(_ context.Context, _ int64) ([]ledger.Budget, error) {
	var out []ledger.Budget
	for c, a := range f.budgets {
		out = append(out, ledger.Budget{Category: c, Amount: a})
	}
	slices.SortFunc(out, func(a, b ledger.Budget) int { return strings.Compare(a.Category, b.Category) })
	return out, nil
}

type sentMessage struct {
	text string
	kb   *telegram.InlineKeyboardMarkup
}

type fakeSender struct {
	sent    []sentMessage
	edits   []sentMessage
	answers []string
}

func (f *fakeSender) SendMessage(_ context.Context, _ int64, text string, kb *telegram.InlineKeyboardMarkup) error {
	f.sent = append(f.sent, sentMessage{text, kb})
	return nil
}

func (f *fakeSender) EditMessage(_ context.Context, _, _ int64, text string, kb *telegram.InlineKeyboardMarkup) error {
	f.edits = append(f.edits, sentMessage{text, kb})
	return nil
}

func (f *fakeSender) AnswerCallback(_ context.Context, _ string, text string) error {
	f.answers = append(f.answers, text)
	return nil
}

const chatID = 42

// Rabu, 7 Oktober 2026 pukul 01.00 WIB (masih 6 Oktober kalau dihitung UTC).
var testNow = time.Date(2026, 10, 6, 18, 0, 0, 0, time.UTC)

func newTestBot() (*Bot, *fakeStore, *fakeSender) {
	st := &fakeStore{}
	snd := &fakeSender{}
	return &Bot{Store: st, Sender: snd, Now: func() time.Time { return testNow }}, st, snd
}

var nextUpdate int64

func send(t *testing.T, b *Bot, text string) {
	t.Helper()
	nextUpdate++
	sendUpdate(t, b, nextUpdate, text)
}

func sendUpdate(t *testing.T, b *Bot, updateID int64, text string) {
	t.Helper()
	u := telegram.Update{UpdateID: updateID, Message: &telegram.Message{Chat: telegram.Chat{ID: chatID}, Text: text}}
	if err := b.HandleUpdate(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

func press(t *testing.T, b *Bot, data string) {
	t.Helper()
	u := telegram.Update{CallbackQuery: &telegram.CallbackQuery{
		ID:      "cb",
		Message: &telegram.Message{MessageID: 99, Chat: telegram.Chat{ID: chatID}},
		Data:    data,
	}}
	if err := b.HandleUpdate(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

func last(s *fakeSender) string { return s.sent[len(s.sent)-1].text }

func lastEdit(s *fakeSender) string { return s.edits[len(s.edits)-1].text }

func assertContains(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("tidak memuat %q:\n%s", want, got)
		}
	}
}

func TestRecordAndSummarize(t *testing.T) {
	b, _, snd := newTestBot()

	send(t, b, "kopi 25rb")
	assertContains(t, last(snd), "Pengeluaran dicatat", "Rp25.000")
	if snd.sent[0].kb == nil {
		t.Error("balasan catat tidak punya tombol Ganti kategori")
	}
	send(t, b, "gaji 5jt")
	send(t, b, "makan siang 75rb")

	send(t, b, "/hariini")
	assertContains(t, last(snd),
		"7 Okt",
		"Pemasukan: <b>Rp5.000.000</b>",
		"Pengeluaran: <b>Rp100.000</b>",
		"Selisih: <b>+Rp4.900.000</b>",
		"• Makan: Rp75.000 (75%)",
		"• Jajan: Rp25.000 (25%)",
	)
}

func TestDuplicateUpdateIsIgnored(t *testing.T) {
	b, st, snd := newTestBot()
	sendUpdate(t, b, 7, "kopi 25rb")
	sendUpdate(t, b, 7, "kopi 25rb")
	if len(st.txs) != 1 || len(snd.sent) != 1 {
		t.Fatalf("ingin 1 transaksi & 1 balasan, dapat %d & %d", len(st.txs), len(snd.sent))
	}
}

func TestUndo(t *testing.T) {
	b, st, snd := newTestBot()
	send(t, b, "/batal")
	assertContains(t, last(snd), "Belum ada")
	send(t, b, "kopi 25rb")
	send(t, b, "/batal")
	if len(st.txs) != 0 {
		t.Errorf("batal gagal: %d catatan tersisa", len(st.txs))
	}
	assertContains(t, last(snd), "Dihapus")
}

func TestUnparseableMessageGetsHint(t *testing.T) {
	b, st, snd := newTestBot()
	send(t, b, "halo")
	if len(st.txs) != 0 {
		t.Error("pesan tanpa nominal ikut tercatat")
	}
	assertContains(t, last(snd), "nominalnya nggak ketemu")
}

func TestAllowlist(t *testing.T) {
	b, st, snd := newTestBot()
	b.Allowed = map[int64]bool{1: true}
	send(t, b, "kopi 25rb")
	if len(st.txs) != 0 {
		t.Error("chat asing bisa mencatat")
	}
	assertContains(t, last(snd), "bot ini pribadi")

	press(t, b, "cat:1")
	if len(snd.edits) != 0 || snd.answers[0] != "Bot ini pribadi." {
		t.Errorf("tombol dari chat asing tidak ditolak: edits %d, jawaban %q", len(snd.edits), snd.answers)
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

func TestBackdatedEntry(t *testing.T) {
	b, st, snd := newTestBot()

	send(t, b, "kemarin bakso 15rb")
	assertContains(t, last(snd), "📅 Kemarin, 6 Okt")
	if got := st.txs[0].CreatedAt.In(WIB); got.Day() != 6 || got.Hour() != 1 {
		t.Errorf("waktu catatan %v, ingin 6 Okt pukul 01.00 WIB", got)
	}

	send(t, b, "senin bensin 30rb")
	assertContains(t, last(snd), "📅 Senin, 5 Okt")

	send(t, b, "/hariini")
	assertContains(t, last(snd), "Belum ada catatan")
}

func TestFutureDateIsRejected(t *testing.T) {
	b, st, snd := newTestBot()
	send(t, b, "20/10 kopi 25rb")
	if len(st.txs) != 0 {
		t.Error("catatan bertanggal masa depan ikut tersimpan")
	}
	assertContains(t, last(snd), "belum lewat")
}

func TestMultiLine(t *testing.T) {
	b, st, snd := newTestBot()
	send(t, b, "kemarin\nkopi 25rb\nparkir 5rb\nhalo\n+bonus 100rb")

	if len(st.txs) != 3 {
		t.Fatalf("ingin 3 catatan, dapat %d", len(st.txs))
	}
	for _, tx := range st.txs {
		if tx.CreatedAt.In(WIB).Day() != 6 {
			t.Errorf("%q bertanggal %v, ingin kemarin", tx.Note, tx.CreatedAt)
		}
	}
	assertContains(t, last(snd),
		"3 catatan disimpan",
		"Total keluar: <b>Rp30.000</b>",
		"Total masuk: <b>Rp100.000</b>",
		"Yang nggak kecatat",
		"<code>halo</code>",
	)
}

func TestChangeCategoryIsLearned(t *testing.T) {
	b, st, snd := newTestBot()

	send(t, b, "laundry 30rb")
	if st.txs[0].Category != ledger.Uncategorized {
		t.Fatalf("kategori awal %q, ingin %q", st.txs[0].Category, ledger.Uncategorized)
	}

	press(t, b, "cat:1")
	if kb := snd.edits[0].kb; kb == nil || len(kb.InlineKeyboard) < 3 {
		t.Fatal("pilihan kategori tidak muncul")
	}

	tagihan := slices.Index(ledger.Categories(ledger.Expense), "Tagihan")
	press(t, b, fmt.Sprintf("set:1:%d", tagihan))
	if st.txs[0].Category != "Tagihan" {
		t.Errorf("kategori setelah diganti %q", st.txs[0].Category)
	}
	assertContains(t, lastEdit(snd), "lain kali <i>laundry</i> otomatis masuk Tagihan")
	assertContains(t, snd.answers[len(snd.answers)-1], "Tagihan")

	send(t, b, "laundry kiloan 20rb")
	if got := st.txs[1].Category; got != "Tagihan" {
		t.Errorf("aturan tidak dipakai lagi: kategori %q", got)
	}
}

func TestCallbackOnDeletedEntry(t *testing.T) {
	b, _, snd := newTestBot()
	send(t, b, "kopi 25rb")
	send(t, b, "/batal")
	press(t, b, "cat:1")
	assertContains(t, snd.answers[0], "udah dihapus")
}

func TestBudget(t *testing.T) {
	b, _, snd := newTestBot()

	send(t, b, "/budget jajan 100rb")
	assertContains(t, last(snd), "Budget <b>Jajan</b> diatur Rp100.000")

	send(t, b, "kopi 70rb")
	if strings.Contains(last(snd), "Budget") {
		t.Errorf("peringatan terlalu dini:\n%s", last(snd))
	}
	send(t, b, "kopi 15rb")
	assertContains(t, last(snd), "⚠️ Budget <b>Jajan</b> udah kepakai 85%", "Sisa Rp15.000")

	send(t, b, "kopi 10rb")
	if strings.Contains(last(snd), "Budget") {
		t.Errorf("peringatan 80%% muncul dua kali:\n%s", last(snd))
	}
	send(t, b, "kopi 10rb")
	assertContains(t, last(snd), "🚨 Budget <b>Jajan</b> bulan ini kelewatan Rp5.000")

	send(t, b, "/budget")
	assertContains(t, last(snd), "Jajan</b>: Rp105.000 / Rp100.000 (105%)", "lewat Rp5.000")

	send(t, b, "/budget kopi 10rb")
	assertContains(t, last(snd), "nggak ada")

	send(t, b, "/budget jajan hapus")
	assertContains(t, last(snd), "Budget Jajan dihapus")
}

func TestDailyReminder(t *testing.T) {
	b, st, snd := newTestBot()
	st.txs = append(st.txs, ledger.Tx{Entry: ledger.Entry{Kind: ledger.Expense, Amount: 1}, CreatedAt: testNow.AddDate(0, 0, -1)})

	n, err := b.RunDaily(context.Background())
	if n != 1 || err != nil {
		t.Fatalf("terkirim %d, err %v", n, err)
	}
	assertContains(t, last(snd), "Hari ini belum ada catatan")

	send(t, b, "kopi 25rb")
	if n, _ := b.RunDaily(context.Background()); n != 0 {
		t.Error("pengingat tetap terkirim padahal hari ini sudah mencatat")
	}

	b.Allowed = map[int64]bool{1: true}
	if n, _ := b.SendWeeklyReports(context.Background()); n != 0 {
		t.Error("chat di luar allowlist tetap dikirimi pesan")
	}
}

func TestSundaySendsWeeklyReport(t *testing.T) {
	b, _, snd := newTestBot()
	send(t, b, "kopi 25rb")
	b.Now = func() time.Time { return time.Date(2026, 10, 11, 14, 0, 0, 0, time.UTC) } // Minggu 21.00 WIB

	if n, err := b.RunDaily(context.Background()); n != 1 || err != nil {
		t.Fatalf("terkirim %d, err %v", n, err)
	}
	assertContains(t, last(snd), "Laporan mingguan")
}

// addAt menaruh transaksi langsung di waktu tertentu, misalnya minggu lalu.
func addAt(st *fakeStore, at time.Time, text string) {
	e, err := ledger.Parse(text)
	if err != nil {
		panic(err)
	}
	st.nextID++
	st.txs = append(st.txs, ledger.Tx{Entry: e, ID: st.nextID, CreatedAt: at})
}

func TestWeeklyReportComparesWithLastWeek(t *testing.T) {
	b, st, snd := newTestBot()
	lastWeek := testNow.AddDate(0, 0, -7)
	addAt(st, lastWeek, "makan 100rb")
	addAt(st, lastWeek, "kopi 20rb")
	addAt(st, testNow, "gaji 5jt")
	addAt(st, testNow, "makan 90rb")
	addAt(st, testNow, "kopi 60rb")

	send(t, b, "/laporan")
	assertContains(t, last(snd),
		"Laporan mingguan</b> · 5 Okt – 11 Okt",
		"Pengeluaran: <b>Rp150.000</b>",
		"Paling boros: <b>Makan</b> Rp90.000 (60% pengeluaran)",
		"Pengeluaran naik 25% dari minggu lalu (+Rp30.000)",
		"Naik paling banyak: <b>Jajan</b> +Rp40.000",
	)
}

func TestWeeklyReportWithoutLastWeek(t *testing.T) {
	b, st, snd := newTestBot()
	addAt(st, testNow, "kopi 25rb")
	send(t, b, "/laporan")
	assertContains(t, last(snd), "belum bisa dibandingkan")
}
