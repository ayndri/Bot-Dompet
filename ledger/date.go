package ledger

import (
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

var ErrFutureDate = errors.New("tanggalnya belum lewat")

var weekdays = map[string]time.Weekday{
	"minggu": time.Sunday,
	"senin":  time.Monday,
	"selasa": time.Tuesday,
	"rabu":   time.Wednesday,
	"kamis":  time.Thursday,
	"jumat":  time.Friday,
	"sabtu":  time.Saturday,
}

var yesterdayWords = []string{"kemarin", "kemaren", "kmrn", "kmarin"}

var dayMonthRe = regexp.MustCompile(`^(\d{1,2})/(\d{1,2})$`)

// SplitDate memisahkan penanda tanggal di awal teks: "kemarin", "kemarin
// lusa", nama hari ("senin" = Senin terakhir, termasuk hari ini), atau
// "5/10" (tanggal/bulan tahun ini).
//
// today adalah tengah malam hari ini di zona waktu pengguna. Kalau tidak ada
// penanda tanggal, day bernilai nol dan rest sama dengan text.
func SplitDate(text string, today time.Time) (day time.Time, rest string, err error) {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return time.Time{}, text, nil
	}
	first := strings.ToLower(fields[0])
	restFrom := func(n int) string { return strings.Join(fields[n:], " ") }

	if slices.Contains(yesterdayWords, first) {
		if len(fields) > 1 && strings.EqualFold(fields[1], "lusa") {
			return today.AddDate(0, 0, -2), restFrom(2), nil
		}
		return today.AddDate(0, 0, -1), restFrom(1), nil
	}
	if wd, ok := weekdays[first]; ok {
		back := (int(today.Weekday()) - int(wd) + 7) % 7
		return today.AddDate(0, 0, -back), restFrom(1), nil
	}
	if m := dayMonthRe.FindStringSubmatch(first); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		day = time.Date(today.Year(), time.Month(mo), d, 0, 0, 0, 0, today.Location())
		// time.Date menormalkan 31/2 jadi 3/3; anggap saja bukan tanggal.
		if day.Day() != d || int(day.Month()) != mo {
			return time.Time{}, text, nil
		}
		if day.After(today) {
			return time.Time{}, text, ErrFutureDate
		}
		return day, restFrom(1), nil
	}
	return time.Time{}, text, nil
}
