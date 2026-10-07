// Package app merakit Bot dari variabel lingkungan. Dipakai bersama oleh
// webhook Vercel dan perintah di cmd/.
package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ayndri/dompetku/bot"
	"github.com/ayndri/dompetku/store"
	"github.com/ayndri/dompetku/telegram"
)

type Config struct {
	BotToken       string
	WebhookSecret  string
	DatabaseURL    string
	AllowedChatIDs map[int64]bool
}

func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BotToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		WebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
	}
	if cfg.BotToken == "" {
		return cfg, errors.New("TELEGRAM_BOT_TOKEN belum diisi")
	}
	if cfg.DatabaseURL == "" {
		return cfg, errors.New("DATABASE_URL belum diisi")
	}
	ids, err := parseChatIDs(os.Getenv("ALLOWED_CHAT_IDS"))
	if err != nil {
		return cfg, err
	}
	cfg.AllowedChatIDs = ids
	return cfg, nil
}

func parseChatIDs(s string) (map[int64]bool, error) {
	ids := map[int64]bool{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("ALLOWED_CHAT_IDS berisi %q, bukan angka", part)
		}
		ids[id] = true
	}
	return ids, nil
}

type App struct {
	Bot      *bot.Bot
	Telegram *telegram.Client
	Store    *store.Store
}

func New(ctx context.Context, cfg Config) (*App, error) {
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	tg := telegram.New(cfg.BotToken)
	return &App{
		Bot:      &bot.Bot{Store: st, Sender: tg, Allowed: cfg.AllowedChatIDs},
		Telegram: tg,
		Store:    st,
	}, nil
}

func (a *App) Close() { a.Store.Close() }

// LoadDotEnv mengisi variabel dari file .env untuk development lokal.
// Variabel yang sudah ada tidak ditimpa. File yang tidak ada diabaikan.
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
	return sc.Err()
}
