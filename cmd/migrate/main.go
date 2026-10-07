// Perintah migrate membuat tabel di database. Aman dijalankan berulang.
//
//	go run ./cmd/migrate
package main

import (
	"context"
	"log"
	"os"

	"github.com/ayndri/dompetku/app"
	"github.com/ayndri/dompetku/store"
)

func main() {
	if err := app.LoadDotEnv(".env"); err != nil {
		log.Fatal(err)
	}
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL belum diisi")
	}

	ctx := context.Background()
	st, err := store.Open(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	if err := st.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
	log.Println("Tabel siap.")
}
