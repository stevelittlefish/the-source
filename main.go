package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/stevelittlefish/the-source/internal/catalog"
	"github.com/stevelittlefish/the-source/internal/config"
	"github.com/stevelittlefish/the-source/internal/server"
)

func main() {
	path := flag.String("config", "source.dev.toml", "TOML configuration file")
	flag.Parse()
	c, err := config.Load(*path)
	if err != nil {
		log.Fatal(err)
	}
	books, err := catalog.LoadFile(c.CatalogPath)
	if err != nil {
		log.Fatal(err)
	}
	app, err := server.New(books, c.BooksDir)
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	srv := &http.Server{Addr: c.ServerAddr, Handler: app, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Printf("shutdown: %v", err)
			srv.Close()
		}
		close(done)
	}()
	log.Printf("THE SOURCE — books on tap, plumbing included. Listening on %s", c.ServerAddr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-done
}
