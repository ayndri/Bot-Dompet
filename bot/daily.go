package bot

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const reminderText = `🔔 Hari ini belum ada catatan.
Ada pengeluaran yang kelewat? Ketik aja, misalnya <code>makan 20rb</code>.
Kalau memang nggak ada, abaikan pesan ini 😊`

// RunDaily dipanggil cron tiap malam. Hari Minggu mengirim laporan mingguan;
// hari lain mengingatkan chat yang belum mencatat apa pun hari ini.
// Satu jadwal untuk dua tugas, karena paket gratis Vercel membatasi cron.
func (b *Bot) RunDaily(ctx context.Context) (int, error) {
	if b.now().Weekday() == time.Sunday {
		return b.SendWeeklyReports(ctx)
	}
	return b.SendReminders(ctx)
}

// SendWeeklyReports mengirim laporan mingguan ke setiap chat aktif.
func (b *Bot) SendWeeklyReports(ctx context.Context) (int, error) {
	return b.broadcast(ctx, func(chatID int64) (string, error) {
		return b.weeklyReport(ctx, chatID)
	})
}

// SendReminders mengingatkan chat aktif yang belum mencatat apa pun hari ini.
func (b *Bot) SendReminders(ctx context.Context) (int, error) {
	from, to := Today.Range(b.now())
	return b.broadcast(ctx, func(chatID int64) (string, error) {
		totals, err := b.Store.Totals(ctx, chatID, from, to)
		if err != nil || len(totals) > 0 {
			return "", err
		}
		return reminderText, nil
	})
}

// broadcast menjalankan compose untuk setiap chat yang mencatat sesuatu dalam
// dua minggu terakhir, lalu mengirim hasilnya kalau tidak kosong. Satu chat
// gagal tidak menghentikan yang lain.
func (b *Bot) broadcast(ctx context.Context, compose func(chatID int64) (string, error)) (int, error) {
	from, _ := ThisWeek.Range(b.now())
	chats, err := b.Store.ActiveChats(ctx, from.AddDate(0, 0, -7))
	if err != nil {
		return 0, err
	}

	sent := 0
	var errs []error
	for _, chatID := range chats {
		if !b.allowed(chatID) {
			continue
		}
		msg, err := compose(chatID)
		if err == nil && msg != "" {
			err = b.Sender.SendMessage(ctx, chatID, msg, nil)
			if err == nil {
				sent++
			}
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("chat %d: %w", chatID, err))
		}
	}
	return sent, errors.Join(errs...)
}
