// Command ateliercopernic serves the Atelier Copernic website: the Vault, the
// Commission Dossier and the Ephemeris Ledger. Templates and static files are
// embedded, so the build is a single self-contained binary with no client-side
// JavaScript anywhere.
package main

import (
	"context"
	"embed"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

//go:embed templates static
var embedded embed.FS

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(log); err != nil {
		log.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	addr := os.Getenv("ADDR")
	if addr == "" {
		port := os.Getenv("PORT")
		if port == "" {
			port = "8080"
		}
		addr = ":" + port
	}

	assets, err := NewAssetStore(embedded)
	if err != nil {
		return err
	}
	renderer, err := NewRenderer(embedded, assets, log)
	if err != nil {
		return err
	}

	start := time.Now()
	catalog := NewCatalog()
	log.Info("catalogue ready",
		"editions", len(catalog.Editions),
		"ledger", len(catalog.Ledger),
		"trains_and_plates", time.Since(start).Round(time.Millisecond))

	app := &App{
		catalog:  catalog,
		dossiers: NewDossierStore(),
		render:   renderer,
		assets:   assets,
		log:      log,
		now:      time.Now,
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", addr, "url", "http://localhost"+addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
