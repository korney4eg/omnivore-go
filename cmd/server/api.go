package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	apihandler "github.com/omnivore-app/omnivore/internal/api/handler"
	"github.com/omnivore-app/omnivore/internal/api/middleware"
	"github.com/omnivore-app/omnivore/internal/auth"
	"github.com/omnivore-app/omnivore/internal/config"
	"github.com/omnivore-app/omnivore/internal/db"
	"github.com/omnivore-app/omnivore/internal/graphql/generated"
	"github.com/omnivore-app/omnivore/internal/graphql/resolver"
	"github.com/omnivore-app/omnivore/internal/redisutil"
	"github.com/omnivore-app/omnivore/internal/repository"
	"github.com/omnivore-app/omnivore/internal/service"
	"github.com/omnivore-app/omnivore/internal/storage"
	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Run the Omnivore GraphQL API server",
	RunE:  runAPI,
}

func init() {
	Cmd.AddCommand(apiCmd)
}

func runAPI(cmd *cobra.Command, args []string) error {
	cfg := config.LoadAPI()

	// Validate required config
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL or PG_* variables required")
	}
	if cfg.RedisURL == "" {
		return fmt.Errorf("REDIS_URL required")
	}

	// Initialize database connection
	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	fmt.Printf("✓ Connected to PostgreSQL\n")

	// Initialize Redis connections
	redisDS, err := redisutil.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	defer redisDS.CacheClient.Close()
	if redisDS.MQClient != redisDS.CacheClient {
		defer redisDS.MQClient.Close()
	}

	fmt.Printf("✓ Connected to Redis\n")

	// Test database connectivity
	if err := database.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	// Test Redis connectivity
	if err := redisDS.CacheClient.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}

	// Initialize storage client. Config.BlobURL() defaults to MinIO-compatible
	// object storage for self-hosted setups when no explicit blob URL is set.
	var storageClient *storage.Client
	if cfg.BlobURL() != "" {
		storageClient, err = storage.New(context.Background(), cfg.BlobURL())
		if err != nil {
			fmt.Printf("⚠ Storage initialization failed (will skip HTML uploads): %v\n", err)
			storageClient = nil
		} else {
			defer storageClient.Close()
			fmt.Printf("✓ Connected to blob storage (%s)\n", cfg.BlobURL())
		}
	}

	// Create GraphQL server
	gqlResolver := resolver.NewResolver(db.GetGorm(), redisDS.MQClient, storageClient)
	gqlServer := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: gqlResolver}))

	// Initialize auth middleware
	jwtConfig, err := auth.DefaultJWTConfig()
	if err != nil {
		return fmt.Errorf("failed to initialize JWT config: %w", err)
	}
	fmt.Printf("✓ JWT authentication initialized\n")

	apiKeyConfig := &auth.APIKeyConfig{
		DB:          db.GetGorm(),
		RedisClient: redisDS.CacheClient,
		CacheTTL:    5 * time.Minute,
	}
	fmt.Printf("✓ API key authentication initialized\n")

	authMiddleware := middleware.NewAuthMiddleware(jwtConfig, apiKeyConfig)

	// Create shared repositories
	libraryItemRepo := repository.NewLibraryItemRepository(db.GetGorm())
	labelRepo := repository.NewLabelRepository(db.GetGorm())

	// Create REST handlers
	authHandler := apihandler.NewAuthHandler(db.GetGorm(), jwtConfig)
	shortcutsHandler := apihandler.NewShortcutsHandler(db.GetGorm())
	pageHandler := apihandler.NewPageHandler(
		service.NewSaveURLService(libraryItemRepo, redisDS.MQClient),
		service.NewSavePageService(libraryItemRepo, redisDS.MQClient, storageClient),
	)
	articleHandler := apihandler.NewArticleHandler(db.GetGorm(), libraryItemRepo, labelRepo)
	contentHandler := apihandler.NewContentHandler(db.GetGorm(), libraryItemRepo)
	serviceHandler := apihandler.NewServiceHandler()

	// Create HTTP router
	mux := http.NewServeMux()

	// Health check endpoints with DB/Redis connectivity check
	mux.HandleFunc("/health", healthCheckHandler(database, redisDS))
	mux.HandleFunc("/_ah/health", healthCheckHandler(database, redisDS))

	// Service endpoints
	mux.HandleFunc("/_ah/version", serviceHandler.Version)
	mux.HandleFunc("/_ah/warmup", serviceHandler.Warmup)

	// REST API endpoints
	// Auth endpoints
	mux.HandleFunc("/api/auth/login", authHandler.Login)
	mux.HandleFunc("/api/auth/email-login", authHandler.EmailLogin)
	mux.HandleFunc("/api/auth/signup", authHandler.Signup)
	mux.HandleFunc("/api/auth/logout", authHandler.Logout)
	mux.HandleFunc("/api/auth/me", authHandler.Me)
	mux.HandleFunc("/api/auth/verify", authHandler.Verify)
	mux.HandleFunc("/api/shortcuts", shortcutsHandler.Handle)

	// Page endpoints (browser extension)
	mux.HandleFunc("/api/page/save", pageHandler.Save)

	// Article endpoints (CRUD)
	mux.HandleFunc("/api/article/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			articleHandler.GetArticle(w, r)
		case http.MethodPut:
			articleHandler.UpdateArticle(w, r)
		case http.MethodDelete:
			articleHandler.DeleteArticle(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Content endpoints (HTML serving)
	mux.HandleFunc("/api/content/", contentHandler.GetContent)

	// GraphQL endpoint
	mux.Handle("/api/graphql", gqlServer)

	if cfg.EnableGraphQLPlayground {
		mux.Handle("/api/playground", playground.Handler("GraphQL Playground", "/api/graphql"))
		fmt.Printf("✓ GraphQL Playground enabled at /api/playground\n")
	}

	// Apply middleware chain
	var handler http.Handler = mux
	handler = middleware.Logger(handler)
	handler = middleware.Recovery(handler)
	handler = middleware.CORS(handler)
	handler = authMiddleware.Auth(handler)

	// Rate limiting for API endpoints only
	apiLimiter := middleware.NewRateLimiter(100, 200) // 100 req/s, burst 200
	handler = middleware.ApplyToPrefix("/api/", apiLimiter.Middleware, handler)

	addr := fmt.Sprintf(":%d", cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  640 * time.Second,
		WriteTimeout: 640 * time.Second,
		IdleTimeout:  630 * time.Second, // 10min + 30s buffer for load balancer keep-alive
	}

	// Start server in a goroutine
	go func() {
		fmt.Printf("🚀 API server listening on %s\n", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal for graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n🛑 Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "server shutdown error: %v\n", err)
		return err
	}

	fmt.Println("✓ Server stopped gracefully")
	return nil
}

// healthCheckHandler returns a handler that checks PostgreSQL and Redis connectivity.
func healthCheckHandler(database *sql.DB, redisDS *redisutil.RedisDataSource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Check PostgreSQL
		if err := database.PingContext(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "unhealthy",
				"error":  "database connection failed",
			})
			return
		}

		// Check Redis
		if err := redisDS.CacheClient.Ping(ctx).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "unhealthy",
				"error":  "redis connection failed",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}
