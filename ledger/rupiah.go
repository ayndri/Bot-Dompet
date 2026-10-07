package ledger

import (
	"strconv"
	"strings"
)

// Rupiah memformat 1250000 menjadi "Rp1.250.000".
func Rupiah(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	s := strconv.FormatInt(v, 10)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(c)
	}
	return sign + "Rp" + b.String()
}
