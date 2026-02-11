package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/discordwell/arkessro-evolved/internal/app"
)

func main() {
	var (
		addr     = flag.String("addr", "127.0.0.1:8080", "listen address")
		db       = flag.String("db", "./data/dev.db", "sqlite db path")
		rseed    = flag.Int64("seed", 0, "prediction seed (0 = deterministic)")
		basePath = flag.String("base-path", "", "optional URL base path, e.g. /evochain")
	)
	flag.Parse()

	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)

	application, err := app.New(app.Config{
		DBPath:          *db,
		PredictionSeed:  *rseed,
		Logger:          logger,
		StaticCacheBust: fmt.Sprintf("%d", time.Now().Unix()),
		BasePath:        *basePath,
	})
	if err != nil {
		logger.Fatalf("init: %v", err)
	}
	defer application.Close()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           application.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		logger.Printf("listening on http://%s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
