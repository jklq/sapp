package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime" // Import runtime package

	"git.sr.ht/~relay/sapp-backend/auth" // Import auth package
	"git.sr.ht/~relay/sapp-backend/category"
	"git.sr.ht/~relay/sapp-backend/pay"
	"github.com/rs/cors"
	_ "modernc.org/sqlite"
)

// Logging middleware
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("Received request", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// Helper function to apply middleware
func applyMiddleware(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

func main() {
	// Setup logging
	logHandler := slog.NewTextHandler(os.Stderr, nil)
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	slog.Info("Starting sapp backend...")

	// Database connection
	dbPath := os.Getenv("DATABASE_PATH")
	if dbPath == "" {
		slog.Error("DATABASE_PATH environment variable not set")
		os.Exit(1)
	}
	db, err := sql.Open("sqlite", dbPath)

	if err != nil {
		slog.Error("failed to open database", "path", dbPath, "err", err)
		os.Exit(1)
	}
	defer db.Close()
	// Test connection
	if err := db.Ping(); err != nil {
		slog.Error("failed to ping database", "path", dbPath, "err", err)
		os.Exit(1)
	}
	slog.Info("Database connection successful", "path", dbPath)

	// --- AI Categorization Pool ---
	// Determine number of workers (e.g., based on CPU cores)
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1 // Ensure at least one worker
	}
	slog.Info("Initializing AI categorization pool", "workers", numWorkers)
	categorizationPool := category.NewCategorizingPool(db, numWorkers)

	// Start the pool workers in the background
	go categorizationPool.StartPool()
	slog.Info("AI categorization pool started")
	// --- End AI Categorization Pool ---

	// --- HTTP Server Setup ---
	mux := http.NewServeMux()

	// --- Public Routes ---
	mux.HandleFunc("POST /v1/login", auth.HandleLogin(db)) // Login is public

	// --- Protected Routes ---
	// Create handlers for protected routes
	payHandler := http.HandlerFunc(pay.HandlePayRoute(db))
	getCategoriesHandler := http.HandlerFunc(category.HandleGetCategories(db))
	categorizeHandler := http.HandlerFunc(category.HandleAICategorize(db, categorizationPool))

	// Apply AuthMiddleware to protected handlers
	mux.Handle("POST /v1/pay/{shared_status}/{amount}/{category}", applyMiddleware(payHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/categories", applyMiddleware(getCategoriesHandler, auth.AuthMiddleware))
	mux.Handle("POST /v1/categorize", applyMiddleware(categorizeHandler, auth.AuthMiddleware))

	// CORS handler - Apply CORS *after* routing but *before* auth potentially
	// Or apply CORS as the outermost layer if auth doesn't rely on headers modified by CORS
	corsHandler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"}, // Allow all origins for now, restrict in production
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type"}, // Ensure Authorization is allowed
	})
	// Apply CORS first, then logging, then the mux router
	handler := corsHandler.Handler(loggingMiddleware(mux))

	// Server configuration
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000" // Default port
		slog.Warn("PORT environment variable not set, using default", "port", port)
	}

	serverAddr := fmt.Sprintf(":%s", port)
	slog.Info("Starting HTTP server", "address", serverAddr)

	// Start the server
	err = http.ListenAndServe(serverAddr, handler)
	if err != nil {
		slog.Error("HTTP server failed", "err", err)
		os.Exit(1)
	}
}
