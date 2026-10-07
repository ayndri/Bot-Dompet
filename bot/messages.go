package bot

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/ayndri/dompetku/ledger"
)

const helpText = `👋 <b>Halo, aku Dompetku.</b>
Ketik aja kayak lagi chat, nanti aku catat.

<b>Pengeluaran</b>
<code>kopi 25rb</code>
<code>grab ke kantor 32k</code>
<code>makan siang 18.500</code>

<b>Pemasukan</b>
<code>gaji 5jt</code>
<code>+freelance desain 1,5jt</code>
Awalan <code>+</code> artinya uang masuk, <code>-</code> artinya uang keluar.

<b>Perintah</b>
/hariini · /mingguini · /bulanini — ringkasan
/saldo — total masuk dikurangi keluar
/riwayat — 10 catatan terakhir
/batal — hapus catatan terakhir`

const parseHint = `Hmm, nominalnya nggak ketemu 🤔
Coba tulis kayak gini: <code>kopi 25rb</code>, <code>bensin 30.000</code>, atau <code>+gaji 5jt</code>.`

func kindIcon(k ledger.Kind) string {
	if k == ledger.Income {
		return "💰"
	}
	return "💸"
}

func formatAdded(tx ledger.Tx) string {
	var s strings.Builder
	fmt.Fprintf(&s, "%s %s dicatat\n<b>%s</b> · %s",
		kindIcon(tx.Kind), tx.Kind.Label(), ledger.Rupiah(tx.Amount), html.EscapeString(tx.Category))
	if tx.Note != "" {
		fmt.Fprintf(&s, "\n<i>%s</i>", html.EscapeString(tx.Note))
	}
	s.WriteString("\n\nSalah? Ketik /batal")
	return s.String()
}

func formatLine(tx ledger.Tx) string {
	sign := "−"
	if tx.Kind == ledger.Income {
		sign = "+"
	}
	line := fmt.Sprintf("%s%s · %s", sign, ledger.Rupiah(tx.Amount), html.EscapeString(tx.Category))
	if tx.Note != "" {
		line += " · " + html.EscapeString(tx.Note)
	}
	return line
}

func formatHistory(txs []ledger.Tx) string {
	if len(txs) == 0 {
		return "Belum ada catatan. Coba kirim <code>kopi 25rb</code>."
	}
	var s strings.Builder
	s.WriteString("<b>🧾 Catatan terakhir</b>\n")
	for _, tx := range txs {
		t := tx.CreatedAt.In(WIB)
		fmt.Fprintf(&s, "\n%s <i>(%s %s)</i>", formatLine(tx), shortDate(t), t.Format("15:04"))
	}
	return s.String()
}

func formatSummary(p Period, from, to time.Time, totals []ledger.CategoryTotal) string {
	var s strings.Builder
	fmt.Fprintf(&s, "<b>📊 %s</b> · %s\n\n", p.Title(), rangeLabel(p, from, to))
	if len(totals) == 0 {
		s.WriteString("Belum ada catatan.")
		return s.String()
	}

	var in, out int64
	var inCats, outCats []ledger.CategoryTotal
	for _, t := range totals {
		if t.Kind == ledger.Income {
			in += t.Total
			inCats = append(inCats, t)
		} else {
			out += t.Total
			outCats = append(outCats, t)
		}
	}

	diff := ledger.Rupiah(in - out)
	if in > out {
		diff = "+" + diff
	}
	fmt.Fprintf(&s, "Pemasukan: <b>%s</b>\nPengeluaran: <b>%s</b>\nSelisih: <b>%s</b>\n",
		ledger.Rupiah(in), ledger.Rupiah(out), diff)

	writeCategories(&s, "Pengeluaran per kategori", outCats, out)
	writeCategories(&s, "Pemasukan per kategori", inCats, in)
	return strings.TrimRight(s.String(), "\n")
}

func writeCategories(s *strings.Builder, title string, cats []ledger.CategoryTotal, total int64) {
	if len(cats) == 0 {
		return
	}
	fmt.Fprintf(s, "\n<b>%s</b>\n", title)
	for _, c := range cats {
		pct := (c.Total*100 + total/2) / total
		fmt.Fprintf(s, "• %s: %s (%d%%)\n", html.EscapeString(c.Category), ledger.Rupiah(c.Total), pct)
	}
}
