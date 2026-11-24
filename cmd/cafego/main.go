package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"cafego/internal/store"
	"cafego/internal/web"
)

func main() {
	dbPath := flag.String("db", "", "Explicit SQLite path (default: data/cafego-local.db next to the executable)")
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	flag.Parse()
	executable, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	resolved, err := databasePath(executable, *dbPath)
	if err != nil {
		log.Fatal(err)
	}
	*dbPath = resolved
	log.Printf("Database: %s", *dbPath)
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0700); err != nil {
		log.Fatal(err)
	}
	db, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err = store.Seed(db); err != nil {
		log.Fatal(err)
	}
	app := web.New(db)
	server := &http.Server{Addr: *addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	log.Printf("CafeGo: http://%s", *addr)
	if err = server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
