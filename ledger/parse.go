package ledger

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// MaxAmount menolak salah ketik seperti "kopi 2000000jt".
const MaxAmount int64 = 1_000_000_000_000

var ErrNoAmount = errors.New("nominal tidak ditemukan")

// Contoh yang cocok: 25000, 18.500, Rp25.000, 25rb, 32k, 1,5jt, 2juta.
var amountRe = regexp.MustCompile(`(?i)^(?:rp\.?)?(\d+(?:[.,]\d+)*)(rb|ribu|k|jt|juta)?$`)

// Parse membaca pesan santai seperti "kopi 25rb" atau "+freelance 1,5jt".
//
// Awalan "+" memaksa pemasukan, "-" memaksa pengeluaran. Tanpa awalan,
// jenisnya ditebak dari kata kunci (gaji, bonus, dst).
func Parse(text string) (Entry, error) {
	text = strings.TrimSpace(text)

	var kind Kind
	if rest, ok := strings.CutPrefix(text, "+"); ok {
		kind, text = Income, rest
	} else if rest, ok := strings.CutPrefix(text, "-"); ok {
		kind, text = Expense, rest
	}

	// Kalau ada beberapa angka ("2 kopi 50rb"), yang bersatuan menang.
	// Kalau sama-sama bersatuan atau sama-sama polos, yang terakhir menang.
	fields := strings.Fields(text)
	best, bestScaled := -1, false
	var amount int64
	for i, f := range fields {
		v, scaled, ok := parseAmount(f)
		if !ok || (bestScaled && !scaled) {
			continue
		}
		best, bestScaled, amount = i, scaled, v
	}
	if best < 0 {
		return Entry{}, ErrNoAmount
	}

	note := make([]string, 0, len(fields))
	for i, f := range fields {
		if i == best || strings.EqualFold(f, "rp") || strings.EqualFold(f, "rp.") {
			continue
		}
		note = append(note, f)
	}

	e := Entry{Kind: kind, Amount: amount, Note: strings.Join(note, " ")}
	if e.Kind == "" {
		e.Kind = DetectKind(e.Note)
	}
	e.Category = Categorize(e.Kind, e.Note)
	return e, nil
}

// parseAmount mengembalikan nilai, apakah token memakai satuan (rb/jt), dan ok.
func parseAmount(tok string) (int64, bool, bool) {
	m := amountRe.FindStringSubmatch(tok)
	if m == nil {
		return 0, false, false
	}
	num, suffix := m[1], strings.ToLower(m[2])

	var v int64
	var ok bool
	switch suffix {
	case "":
		v, ok = parsePlain(num)
	case "jt", "juta":
		v, ok = parseScaled(num, 1_000_000)
	default:
		v, ok = parseScaled(num, 1_000)
	}
	return v, suffix != "", ok && v > 0 && v <= MaxAmount
}

// parsePlain membaca angka tanpa satuan. Titik/koma dianggap pemisah ribuan,
// jadi setiap kelompok setelahnya wajib 3 digit: "18.500" boleh, "1.5" tidak.
func parsePlain(s string) (int64, bool) {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '.' || r == ',' })
	for _, p := range parts[1:] {
		if len(p) != 3 {
			return 0, false
		}
	}
	v, err := strconv.ParseInt(strings.Join(parts, ""), 10, 64)
	return v, err == nil
}

// parseScaled membaca angka bersatuan. Titik/koma dianggap desimal:
// "1,5" × 1jt = 1.500.000. Hasil yang tidak bulat ke rupiah ditolak.
func parseScaled(s string, mult int64) (int64, bool) {
	intPart, frac, hasFrac := strings.Cut(strings.ReplaceAll(s, ",", "."), ".")
	if strings.Contains(frac, ".") || len(frac) > 6 {
		return 0, false
	}
	i, err := strconv.ParseInt(intPart, 10, 64)
	if err != nil || i > MaxAmount/mult {
		return 0, false
	}
	v := i * mult
	if hasFrac {
		f, err := strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, false
		}
		pow := int64(1)
		for range len(frac) {
			pow *= 10
		}
		if (f*mult)%pow != 0 {
			return 0, false
		}
		v += f * mult / pow
	}
	return v, true
}

// ParseAmount membaca satu token nominal seperti "300rb" atau "1.500.000".
func ParseAmount(tok string) (int64, bool) {
	v, _, ok := parseAmount(tok)
	return v, ok
}
