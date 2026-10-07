package ledger

import (
	"errors"
	"testing"
	"time"
)

func TestSplitDate(t *testing.T) {
	wib := time.FixedZone("WIB", 7*60*60)
	today := time.Date(2026, 10, 7, 0, 0, 0, 0, wib) // Rabu
	day := func(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, wib) }

	tests := []struct {
		in       string
		wantDay  time.Time
		wantRest string
	}{
		{"kemarin bakso 15rb", day(6), "bakso 15rb"},
		{"Kmrn bakso 15rb", day(6), "bakso 15rb"},
		{"kemarin lusa parkir 5rb", day(5), "parkir 5rb"},
		{"senin bensin 30rb", day(5), "bensin 30rb"},
		{"rabu kopi 20rb", day(7), "kopi 20rb"}, // hari ini sendiri
		{"kamis kopi 20rb", day(1), "kopi 20rb"},
		{"5/10 parkir 5rb", day(5), "parkir 5rb"},
		{"kemarin", day(6), ""},
		{"kopi 25rb", time.Time{}, "kopi 25rb"},
		{"31/2 kopi 25rb", time.Time{}, "31/2 kopi 25rb"}, // bukan tanggal
	}
	for _, tt := range tests {
		got, rest, err := SplitDate(tt.in, today)
		if err != nil {
			t.Errorf("SplitDate(%q) error: %v", tt.in, err)
			continue
		}
		if !got.Equal(tt.wantDay) || rest != tt.wantRest {
			t.Errorf("SplitDate(%q) = %v, %q; ingin %v, %q", tt.in, got, rest, tt.wantDay, tt.wantRest)
		}
	}

	if _, _, err := SplitDate("20/10 kopi 25rb", today); !errors.Is(err, ErrFutureDate) {
		t.Errorf("tanggal masa depan: err %v, ingin ErrFutureDate", err)
	}
}

func TestLearnedLookup(t *testing.T) {
	l := Learned{Expense: {"laundry": "Tagihan", "beli buku": "Pendidikan"}}

	tests := []struct {
		note string
		want string
		ok   bool
	}{
		{"laundry", "Tagihan", true},
		{"Laundry kiloan!", "Tagihan", true}, // aturan satu kata ikut berlaku
		{"beli buku", "Pendidikan", true},
		{"beli pulpen", "", false}, // aturan dua kata hanya untuk catatan yang sama persis
	}
	for _, tt := range tests {
		got, ok := l.Lookup(Expense, tt.note)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Lookup(%q) = %q, %v; ingin %q, %v", tt.note, got, ok, tt.want, tt.ok)
		}
	}
	if _, ok := l.Lookup(Income, "laundry"); ok {
		t.Error("aturan pengeluaran ikut berlaku untuk pemasukan")
	}
}
