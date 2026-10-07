package ledger

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want Entry
	}{
		{"kopi 25rb", Entry{Expense, 25_000, "Jajan", "kopi"}},
		{"grab ke kantor 32k", Entry{Expense, 32_000, "Transport", "grab ke kantor"}},
		{"makan siang 18.500", Entry{Expense, 18_500, "Makan", "makan siang"}},
		{"Rp 25.000 parkir", Entry{Expense, 25_000, "Transport", "parkir"}},
		{"Rp1.250.000 kos", Entry{Expense, 1_250_000, "Tagihan", "kos"}},
		{"tiket kereta 150rb", Entry{Expense, 150_000, "Transport", "tiket kereta"}},
		{"2 kopi 50rb", Entry{Expense, 50_000, "Jajan", "2 kopi"}},
		{"25000", Entry{Expense, 25_000, Uncategorized, ""}},
		{"gaji 5jt", Entry{Income, 5_000_000, "Gaji", "gaji"}},
		{"+freelance desain 1,5jt", Entry{Income, 1_500_000, "Freelance", "freelance desain"}},
		{"+ dari ibu 200rb", Entry{Income, 200_000, Uncategorized, "dari ibu"}},
		{"-cashback 10rb", Entry{Expense, 10_000, Uncategorized, "cashback"}},
		{"bensin 2,5ribu", Entry{Expense, 2_500, "Transport", "bensin"}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Parse(%q)\n got  %+v\n want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{
		"kopi",
		"kopi 1.5",       // tanpa satuan, "5" bukan kelompok ribuan
		"kopi 0",         // nol bukan transaksi
		"kopi 1,2345rb",  // hasilnya bukan rupiah bulat
		"kopi 2000000jt", // di atas MaxAmount
		"kopi 1.2.3jt",   // dua pemisah desimal
	} {
		if got, err := Parse(in); !errors.Is(err, ErrNoAmount) {
			t.Errorf("Parse(%q) = %+v, %v; ingin ErrNoAmount", in, got, err)
		}
	}
}

func TestRupiah(t *testing.T) {
	tests := map[int64]string{
		0:         "Rp0",
		999:       "Rp999",
		1_000:     "Rp1.000",
		1_234_567: "Rp1.234.567",
		-25_000:   "-Rp25.000",
	}
	for in, want := range tests {
		if got := Rupiah(in); got != want {
			t.Errorf("Rupiah(%d) = %q, ingin %q", in, got, want)
		}
	}
}
