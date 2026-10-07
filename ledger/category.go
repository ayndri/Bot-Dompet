package ledger

import (
	"slices"
	"strings"
	"unicode"
)

const Uncategorized = "Lainnya"

type rule struct {
	name  string
	words []string
}

// Urutan menentukan prioritas: "tiket kereta" kena Transport sebelum Hiburan.
var expenseRules = []rule{
	{"Makan", []string{"makan", "sarapan", "nasi", "bakso", "mie", "ayam", "warteg", "padang", "gofood", "grabfood", "shopeefood", "lunch", "dinner"}},
	{"Jajan", []string{"kopi", "jajan", "snack", "boba", "es", "teh", "cemilan", "camilan", "roti", "gorengan", "martabak", "seblak"}},
	{"Transport", []string{"grab", "gojek", "ojek", "ojol", "maxim", "bensin", "pertalite", "pertamax", "parkir", "tol", "kereta", "krl", "bus", "taksi", "pesawat"}},
	{"Tagihan", []string{"listrik", "pln", "pdam", "wifi", "internet", "pulsa", "kuota", "bpjs", "kos", "kost", "sewa", "cicilan", "langganan", "netflix", "spotify"}},
	{"Belanja", []string{"belanja", "shopee", "tokopedia", "tokped", "baju", "sepatu", "indomaret", "alfamart", "sabun", "sampo", "skincare"}},
	{"Hiburan", []string{"nonton", "bioskop", "game", "konser", "karaoke", "liburan", "tiket"}},
	{"Kesehatan", []string{"obat", "dokter", "apotek", "klinik", "vitamin"}},
	{"Sosial", []string{"sedekah", "infaq", "infak", "zakat", "kado", "kondangan", "sumbangan", "donasi"}},
}

var incomeRules = []rule{
	{"Gaji", []string{"gaji", "salary", "thr"}},
	{"Bonus", []string{"bonus", "insentif", "komisi"}},
	{"Freelance", []string{"freelance", "proyek", "project", "klien", "client"}},
	{"Penjualan", []string{"jual", "jualan"}},
	{"Investasi", []string{"dividen", "bunga", "reksadana", "saham"}},
	{"Hadiah", []string{"hadiah", "angpao", "kado"}},
}

// Kata yang cukup jelas menandakan uang masuk tanpa perlu awalan "+".
var incomeWords = []string{
	"gaji", "salary", "thr", "bonus", "insentif", "komisi", "freelance",
	"dividen", "cashback", "refund", "angpao", "pemasukan", "terima", "dapat", "dapet", "jual", "jualan",
}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func DetectKind(note string) Kind {
	for _, w := range words(note) {
		if slices.Contains(incomeWords, w) {
			return Income
		}
	}
	return Expense
}

func Categorize(kind Kind, note string) string {
	rules := expenseRules
	if kind == Income {
		rules = incomeRules
	}
	ws := words(note)
	for _, r := range rules {
		for _, w := range ws {
			if slices.Contains(r.words, w) {
				return r.name
			}
		}
	}
	return Uncategorized
}
