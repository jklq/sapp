package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime" // Import runtime package

	"git.sr.ht/~relay/sapp-backend/category"
	"git.sr.ht/~relay/sapp-backend/pay"
	"github.com/rs/cors"
	_ "modernc.org/sqlite"
)

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

	// Register routes
	mux.HandleFunc("POST /v1/pay/{shared_status}/{amount}/{category}", pay.HandlePayRoute(db))
	mux.HandleFunc("GET /v1/categories", category.HandleGetCategories(db))
	// Register the new AI categorization route
	mux.HandleFunc("POST /v1/categorize", category.HandleAICategorize(db, categorizationPool)) // Pass pool

	// CORS handler
	handler := cors.Default().Handler(mux) // Use default CORS settings for now

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
