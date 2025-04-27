package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"

	"git.sr.ht/~relay/sapp-backend/auth"
	"git.sr.ht/~relay/sapp-backend/category"
	"git.sr.ht/~relay/sapp-backend/deposit"
	"git.sr.ht/~relay/sapp-backend/pay"
	"git.sr.ht/~relay/sapp-backend/spendings"
	"git.sr.ht/~relay/sapp-backend/transfer"
	"github.com/joho/godotenv"
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
	// Load .env file. Ignore error if it doesn't exist (e.g., in production where env vars are set directly)
	_ = godotenv.Load() // Load environment variables from .env file

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

	// --- Create the real ModelAPI implementation ---
	openRouterAPIKey := os.Getenv("OPENROUTER_KEY")
	// Note: Consider adding more robust configuration for model name
	openRouterModel := "deepseek/deepseek-chat"
	if openRouterAPIKey == "" {
		slog.Warn("OPENROUTER_KEY environment variable not set. AI categorization will likely fail.")
		// Depending on requirements, you might want to os.Exit(1) here
		// or allow the app to run with a non-functional AI component.
	}
	modelAPI := category.NewOpenRouterAPI(openRouterAPIKey, openRouterModel)
	// --- End ModelAPI creation ---

	// Pass the ModelAPI implementation to the pool
	categorizationPool := category.NewCategorizingPool(db, numWorkers, modelAPI)

	// Start the pool workers in the background
	go categorizationPool.StartPool()
	slog.Info("AI categorization pool started")
	// --- End AI Categorization Pool ---

	// --- HTTP Server Setup ---
	mux := http.NewServeMux()

	// --- Public Routes ---
	mux.HandleFunc("POST /v1/login", auth.HandleLogin(db))                           // Login is public
	mux.HandleFunc("POST /v1/register/partners", auth.HandlePartnerRegistration(db)) // Partner registration is public

	// --- Protected Routes ---
	// Create handlers for protected routes
	payHandler := http.HandlerFunc(pay.HandlePayRoute(db))
	getCategoriesHandler := http.HandlerFunc(category.HandleGetCategories(db))
	categorizeHandler := http.HandlerFunc(category.HandleAICategorize(db, &categorizationPool)) // Pass pointer to pool
	getHistoryHandler := http.HandlerFunc(spendings.HandleGetHistory(db)) // Use spendings.HandleGetHistory which internally uses history service
	updateSpendingHandler := http.HandlerFunc(spendings.HandleUpdateSpending(db))
	getTransferStatusHandler := http.HandlerFunc(transfer.HandleGetTransferStatus(db)) // Create handler for transfer status
	recordTransferHandler := http.HandlerFunc(transfer.HandleRecordTransfer(db))       // Create handler for recording transfer
	deleteAIJobHandler := http.HandlerFunc(spendings.HandleDeleteAIJob(db))            // Create handler for deleting AI job
	// Deposit Handlers
	addDepositHandler := http.HandlerFunc(deposit.HandleAddDeposit(db))         // Create handler for adding deposit
	getDepositsHandler := http.HandlerFunc(deposit.HandleGetDeposits(db))       // Create handler for getting deposit templates
	getDepositByIDHandler := http.HandlerFunc(deposit.HandleGetDepositByID(db)) // Create handler for getting single deposit template
	updateDepositHandler := http.HandlerFunc(deposit.HandleUpdateDeposit(db))   // Create handler for updating deposit template
	deleteDepositHandler := http.HandlerFunc(deposit.HandleDeleteDeposit(db))   // Create handler for deleting deposit template

	// Apply AuthMiddleware to protected handlers
	mux.Handle("POST /v1/pay", applyMiddleware(payHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/categories", applyMiddleware(getCategoriesHandler, auth.AuthMiddleware))
	mux.Handle("POST /v1/categorize", applyMiddleware(categorizeHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/history", applyMiddleware(getHistoryHandler, auth.AuthMiddleware)) // Updated route and handler
	mux.Handle("PUT /v1/spendings/{spending_id}", applyMiddleware(updateSpendingHandler, auth.AuthMiddleware))
	mux.Handle("DELETE /v1/jobs/{job_id}", applyMiddleware(deleteAIJobHandler, auth.AuthMiddleware))
	// Transfer Routes
	mux.Handle("GET /v1/transfer/status", applyMiddleware(getTransferStatusHandler, auth.AuthMiddleware))
	mux.Handle("POST /v1/transfer/record", applyMiddleware(recordTransferHandler, auth.AuthMiddleware))
	// Deposit Routes
	mux.Handle("POST /v1/deposits", applyMiddleware(addDepositHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/deposits", applyMiddleware(getDepositsHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/deposits/{deposit_id}", applyMiddleware(getDepositByIDHandler, auth.AuthMiddleware))
	mux.Handle("PUT /v1/deposits/{deposit_id}", applyMiddleware(updateDepositHandler, auth.AuthMiddleware))
	mux.Handle("DELETE /v1/deposits/{deposit_id}", applyMiddleware(deleteDepositHandler, auth.AuthMiddleware))

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
