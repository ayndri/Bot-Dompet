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

<b>Lupa nyatat?</b>
<code>kemarin bakso 15rb</code>
<code>senin bensin 30rb</code>
<code>5/10 parkir 5rb</code>

<b>Banyak sekaligus</b>, satu baris satu catatan:
<code>kopi 25rb
parkir 5rb
makan 30rb</code>

Kategorinya salah? Tekan <b>✏️ Ganti kategori</b>. Aku bakal ingat buat lain kali.

<b>Perintah</b>
/hariini · /mingguini · /bulanini — ringkasan
/laporan — minggu ini dibanding minggu lalu
/budget — batas pengeluaran bulanan per kategori
/saldo — total masuk dikurangi keluar
/riwayat — 10 catatan terakhir
/batal — hapus catatan terakhir

Tiap jam 9 malam aku ngingetin kalau hari itu belum ada catatan, dan tiap Minggu malam aku kirim laporan mingguan.`

const parseHint = `Hmm, nominalnya nggak ketemu 🤔
Coba tulis kayak gini: <code>kopi 25rb</code>, <code>bensin 30.000</code>, atau <code>+gaji 5jt</code>.`

func kindIcon(k ledger.Kind) string {
	if k == ledger.Income {
		return "💰"
	}
	return "💸"
}

var weekdayNames = [...]string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}

// dayLabel mengembalikan "" untuk hari ini, selain itu misalnya
// "Kemarin, 6 Okt" atau "Senin, 5 Okt".
func dayLabel(at, today time.Time) string {
	day := startOfDay(at)
	switch {
	case day.Equal(today):
		return ""
	case day.Equal(today.AddDate(0, 0, -1)):
		return "Kemarin, " + shortDate(day)
	default:
		return weekdayNames[day.Weekday()] + ", " + shortDate(day)
	}
}

func formatAdded(tx ledger.Tx, today time.Time) string {
	var s strings.Builder
	fmt.Fprintf(&s, "%s %s dicatat\n<b>%s</b> · %s",
		kindIcon(tx.Kind), tx.Kind.Label(), ledger.Rupiah(tx.Amount), html.EscapeString(tx.Category))
	if tx.Note != "" {
		fmt.Fprintf(&s, "\n<i>%s</i>", html.EscapeString(tx.Note))
	}
	if d := dayLabel(tx.CreatedAt, today); d != "" {
		fmt.Fprintf(&s, "\n📅 %s", d)
	}
	s.WriteString("\n\nSalah nominal? Ketik /batal")
	return s.String()
}

func formatAddedMany(saved []ledger.Tx, failed []string, today time.Time) string {
	var s strings.Builder
	fmt.Fprintf(&s, "✅ <b>%d catatan disimpan</b>\n", len(saved))
	var in, out int64
	for _, tx := range saved {
		line := formatLine(tx)
		if d := dayLabel(tx.CreatedAt, today); d != "" {
			line += " <i>(" + d + ")</i>"
		}
		s.WriteString("\n" + line)
		if tx.Kind == ledger.Income {
			in += tx.Amount
		} else {
			out += tx.Amount
		}
	}
	s.WriteString("\n")
	if out > 0 {
		fmt.Fprintf(&s, "\nTotal keluar: <b>%s</b>", ledger.Rupiah(out))
	}
	if in > 0 {
		fmt.Fprintf(&s, "\nTotal masuk: <b>%s</b>", ledger.Rupiah(in))
	}
	if len(failed) > 0 {
		s.WriteString("\n\nYang nggak kecatat:\n• " + strings.Join(failed, "\n• "))
	}
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
		fmt.Fprintf(s, "• %s: %s (%d%%)\n", html.EscapeString(c.Category), ledger.Rupiah(c.Total), percent(c.Total, total))
	}
}

func formatWeekly(from, to time.Time, cur, prev []ledger.CategoryTotal) string {
	var s strings.Builder
	fmt.Fprintf(&s, "<b>🗓️ Laporan mingguan</b> · %s\n\n", rangeLabel(ThisWeek, from, to))

	in, out, outCats := splitTotals(cur)
	_, prevOut, prevCats := splitTotals(prev)
	if in == 0 && out == 0 {
		s.WriteString("Minggu ini belum ada catatan. Yuk mulai catat lagi, cukup ketik <code>kopi 25rb</code>.")
		return s.String()
	}

	diff := ledger.Rupiah(in - out)
	if in > out {
		diff = "+" + diff
	}
	fmt.Fprintf(&s, "Pemasukan: <b>%s</b>\nPengeluaran: <b>%s</b>\nSelisih: <b>%s</b>\n",
		ledger.Rupiah(in), ledger.Rupiah(out), diff)

	if out == 0 {
		return s.String() + "\nNggak ada pengeluaran sama sekali minggu ini 👏"
	}

	// Totals sudah terurut dari yang terbesar, jadi elemen pertama paling boros.
	top := outCats[0]
	fmt.Fprintf(&s, "\n🔥 Paling boros: <b>%s</b> %s (%d%% pengeluaran)\n",
		html.EscapeString(top.Category), ledger.Rupiah(top.Total), percent(top.Total, out))

	if prevOut == 0 {
		s.WriteString("Minggu lalu belum ada pengeluaran, jadi belum bisa dibandingkan.")
		return s.String()
	}

	change := out - prevOut
	switch {
	case change > 0:
		fmt.Fprintf(&s, "📈 Pengeluaran naik %d%% dari minggu lalu (+%s)\n", percent(change, prevOut), ledger.Rupiah(change))
	case change < 0:
		fmt.Fprintf(&s, "📉 Pengeluaran turun %d%% dari minggu lalu (−%s)\n", percent(-change, prevOut), ledger.Rupiah(-change))
	default:
		s.WriteString("Pengeluaran sama persis dengan minggu lalu.\n")
	}

	if cat, up := biggestIncrease(outCats, prevCats); up > 0 {
		fmt.Fprintf(&s, "⬆️ Naik paling banyak: <b>%s</b> +%s\n", html.EscapeString(cat), ledger.Rupiah(up))
	}
	return strings.TrimRight(s.String(), "\n")
}

// splitTotals memisahkan total pemasukan, total pengeluaran, dan daftar
// kategori pengeluaran (urutan dari input dipertahankan).
func splitTotals(totals []ledger.CategoryTotal) (in, out int64, outCats []ledger.CategoryTotal) {
	for _, t := range totals {
		if t.Kind == ledger.Income {
			in += t.Total
		} else {
			out += t.Total
			outCats = append(outCats, t)
		}
	}
	return in, out, outCats
}

func biggestIncrease(cur, prev []ledger.CategoryTotal) (string, int64) {
	before := map[string]int64{}
	for _, c := range prev {
		before[c.Category] = c.Total
	}
	var name string
	var best int64
	for _, c := range cur {
		if up := c.Total - before[c.Category]; up > best {
			name, best = c.Category, up
		}
	}
	return name, best
}

func percent(part, whole int64) int64 {
	return (part*100 + whole/2) / whole
}
