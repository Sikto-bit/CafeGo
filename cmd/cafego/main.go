package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"cafego/internal/store"
	"cafego/internal/web"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	dbPath := flag.String("db", "", "Explicit SQLite path (default: data/cafego-local.db next to the executable)")
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	admin := flag.String("create-admin", "", "Create administrator login and exit; password is read from stdin")
	secure := flag.Bool("secure-cookies", false, "Use only with HTTPS")
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
	if *admin != "" {
		if len(*admin) < 3 || len(*admin) > 32 || strings.IndexFunc(*admin, func(r rune) bool {
			return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_')
		}) >= 0 {
			log.Fatal("Login must contain 3-32 ASCII letters, digits or underscore")
		}
		fmt.Fprintln(os.Stderr, "Enter a new password (10-72 bytes; terminal input is visible):")
		password, e := bufio.NewReader(os.Stdin).ReadString('\n')
		if e != nil && len(password) == 0 {
			log.Fatal(e)
		}
		password = strings.TrimRight(password, "\r\n")
		if len(password) < 10 || len(password) > 72 {
			log.Fatal("Password must contain 10-72 bytes")
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if e != nil {
			log.Fatal(e)
		}
		if _, e = db.Exec("INSERT INTO users(login,password_hash,role) VALUES(?,?,'admin')", *admin, string(hash)); e != nil {
			log.Fatal("Cannot create admin (login may already exist): ", e)
		}
		fmt.Println("Administrator created:", *admin)
		return
	}
	if err = store.Seed(db); err != nil {
		log.Fatal(err)
	}
	app := web.New(db, *secure)
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
