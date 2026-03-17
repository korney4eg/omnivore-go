package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/queueprocessor"
	"github.com/omnivore-app/omnivore/internal/redisutil"
)

var queueProcessorCmd = &cobra.Command{
	Use:   "queue-processor",
	Short: "Start the backend queue processor",
	RunE:  runQueueProcessor,
}

func runQueueProcessor(cmd *cobra.Command, args []string) error {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect Redis.
	redisDS, err := redisutil.New(cfg)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	defer redisDS.Shutdown()

	// Connect PostgreSQL.
	dbPool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer dbPool.Close()

	// Start queue worker.
	worker := queueprocessor.NewWorker(ctx, cfg, redisDS, dbPool)
	worker.Start()

	// HTTP server for health and lifecycle endpoints.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_ah/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("GET /lifecycle/prestop", func(w http.ResponseWriter, r *http.Request) {
		log.Println("[queue-processor] /lifecycle/prestop: initiating graceful shutdown")
		cancel()
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "shutting down")
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	// Graceful shutdown on OS signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigCh:
			log.Printf("[queue-processor] received signal %s, shutting down", sig)
			cancel()
		case <-ctx.Done():
		}
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("[queue-processor] HTTP shutdown error: %v", err)
		}
	}()

	log.Printf("[queue-processor] HTTP server listening on :%d", cfg.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("http server: %w", err)
	}

	// Wait for in-flight jobs to finish.
	worker.Wait()
	log.Println("[queue-processor] shutdown complete")
	return nil
}
