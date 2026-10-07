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

// Categories mengembalikan semua pilihan kategori untuk satu jenis transaksi,
// dipakai untuk tombol "Ganti kategori" dan perintah /budget.
func Categories(kind Kind) []string {
	rules := expenseRules
	if kind == Income {
		rules = incomeRules
	}
	names := make([]string, 0, len(rules)+1)
	for _, r := range rules {
		names = append(names, r.name)
	}
	return append(names, Uncategorized)
}

// FindCategory mencocokkan nama kategori tanpa peduli huruf besar-kecil.
func FindCategory(kind Kind, name string) (string, bool) {
	for _, c := range Categories(kind) {
		if strings.EqualFold(c, name) {
			return c, true
		}
	}
	return "", false
}

// Keyword menormalkan catatan menjadi kunci aturan yang dipelajari:
// huruf kecil, tanpa tanda baca, tanpa angka lepas ("2 Kopi!" jadi "kopi").
func Keyword(note string) string {
	var ws []string
	for _, w := range words(note) {
		if strings.Trim(w, "0123456789") != "" {
			ws = append(ws, w)
		}
	}
	return strings.Join(ws, " ")
}

// Learned berisi kategori yang diajarkan pengguna lewat tombol
// "Ganti kategori", dikelompokkan per jenis lalu per kata kunci.
type Learned map[Kind]map[string]string

// Lookup mencari kategori yang pernah diajarkan. Catatan yang sama persis
// menang; selain itu, aturan satu kata berlaku untuk catatan yang memuat
// kata itu ("laundry" juga mengenai "laundry kiloan").
func (l Learned) Lookup(kind Kind, note string) (string, bool) {
	rules := l[kind]
	if len(rules) == 0 {
		return "", false
	}
	kw := Keyword(note)
	if c, ok := rules[kw]; ok {
		return c, true
	}
	for _, w := range strings.Fields(kw) {
		if c, ok := rules[w]; ok {
			return c, true
		}
	}
	return "", false
}
