package bot

import (
	"fmt"
	"time"
)

// WIB tidak punya daylight saving, jadi zona tetap sudah cukup dan tidak
// bergantung pada database zona waktu di server.
var WIB = time.FixedZone("WIB", 7*60*60)

type Period int

const (
	Today Period = iota
	ThisWeek
	ThisMonth
)

// Range mengembalikan rentang [from, to) dalam WIB. Minggu dimulai Senin.
func (p Period) Range(now time.Time) (from, to time.Time) {
	now = now.In(WIB)
	y, m, d := now.Date()
	day := time.Date(y, m, d, 0, 0, 0, 0, WIB)

	switch p {
	case ThisWeek:
		sinceMonday := (int(day.Weekday()) + 6) % 7
		from = day.AddDate(0, 0, -sinceMonday)
		return from, from.AddDate(0, 0, 7)
	case ThisMonth:
		from = time.Date(y, m, 1, 0, 0, 0, 0, WIB)
		return from, from.AddDate(0, 1, 0)
	default:
		return day, day.AddDate(0, 0, 1)
	}
}

func (p Period) Title() string {
	switch p {
	case ThisWeek:
		return "Minggu ini"
	case ThisMonth:
		return "Bulan ini"
	default:
		return "Hari ini"
	}
}

var monthsShort = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
var monthsLong = [...]string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

func shortDate(t time.Time) string {
	return fmt.Sprintf("%d %s", t.Day(), monthsShort[t.Month()-1])
}

func rangeLabel(p Period, from, to time.Time) string {
	switch p {
	case ThisWeek:
		return shortDate(from) + " – " + shortDate(to.AddDate(0, 0, -1))
	case ThisMonth:
		return fmt.Sprintf("%s %d", monthsLong[from.Month()-1], from.Year())
	default:
		return shortDate(from)
	}
}
