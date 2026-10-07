// Package handler adalah fungsi serverless Vercel di /api/webhook.
// Telegram memanggil URL ini setiap ada pesan masuk ke bot.
package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/ayndri/dompetku/app"
	"github.com/ayndri/dompetku/telegram"
)

// Disiapkan sekali per instance, lalu dipakai ulang selama instance masih
// hangat, supaya tidak membuka koneksi database di setiap pesan.
var (
	initOnce sync.Once
	svc      *app.App
	secret   string
	initErr  error
)

func setup() {
	cfg, err := app.ConfigFromEnv()
	if err != nil {
		initErr = err
		return
	}
	if cfg.WebhookSecret == "" {
		initErr = errors.New("TELEGRAM_WEBHOOK_SECRET belum diisi")
		return
	}
	secret = cfg.WebhookSecret
	svc, initErr = app.New(context.Background(), cfg)
}

func Handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	initOnce.Do(setup)
	if initErr != nil {
		log.Printf("init: %v", initErr)
		http.Error(w, "server belum siap", http.StatusInternalServerError)
		return
	}

	// Tanpa cek ini, siapa pun yang tahu URL-nya bisa menulis ke database.
	got := r.Header.Get("X-Telegram-Bot-Api-Secret-Token")
	if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var u telegram.Update
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&u); err != nil {
		// Tetap 200: kalau dibalas error, Telegram akan terus mengirim ulang.
		log.Printf("update tidak terbaca: %v", err)
		w.WriteHeader(http.StatusOK)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 9*time.Second)
	defer cancel()
	if err := svc.Bot.HandleUpdate(ctx, u); err != nil {
		log.Printf("update %d: %v", u.UpdateID, err)
	}
	w.WriteHeader(http.StatusOK)
}
