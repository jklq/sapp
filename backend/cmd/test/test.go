package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth"
	"git.sr.ht/~relay/sapp-backend/category"
	"git.sr.ht/~relay/sapp-backend/pay"
	"git.sr.ht/~relay/sapp-backend/spendings"
	"git.sr.ht/~relay/sapp-backend/transfer"
	_ "modernc.org/sqlite"
)

// MockModelAPI provides a mock implementation of the category.ModelAPI interface.
type MockModelAPI struct {
	// You can add fields here to control the mock's behavior, e.g., predefined responses.
	Response *category.ModelAPIResponse
	Error    error
}

// Prompt implements the category.ModelAPI interface for the mock.
func (m *MockModelAPI) Prompt(prompt string) (*category.ModelAPIResponse, error) {
	slog.Info("MockModelAPI: Prompt called", "prompt_substring", prompt[:min(100, len(prompt))]) // Log subset of prompt
	if m.Error != nil {
		slog.Warn("MockModelAPI: Returning predefined error", "error", m.Error)
		return nil, m.Error
	}
	if m.Response != nil {
		slog.Info("MockModelAPI: Returning predefined response")
		return m.Response, nil
	}
	// Default response if nothing is set
	slog.Warn("MockModelAPI: Returning default empty response (no predefined response or error set)")
	return &category.ModelAPIResponse{
		Choices: []category.Choice{
			{
				Message: category.Message{
					Content: `{"ambiguity_flag": "mock default response", "spendings":[]}`, // Default valid JSON
				},
			},
		},
	}, nil
}

// Helper function to apply middleware (copied from sapp/main.go)
func applyMiddleware(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}

// Logging middleware (copied from sapp/main.go)
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("Request handled", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

// runMigrations executes the schema SQL against the database.
func runMigrations(db *sql.DB, schemaPath string) error {
	slog.Info("Running migrations...", "schema", schemaPath)
	query, err := os.ReadFile(schemaPath)
	if err != nil {
		return fmt.Errorf("error reading schema file %s: %w", schemaPath, err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("error beginning migration transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(string(query))
	if err != nil {
		return fmt.Errorf("error executing schema SQL: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("error committing migration transaction: %w", err)
	}
	slog.Info("Migrations completed successfully.")
	return nil
}

func main() {
	// --- Setup Logging ---
	logHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}) // Use Debug level for tests
	logger := slog.New(logHandler)
	slog.SetDefault(logger)
	slog.Info("Starting test environment setup...")

	// --- Setup Test Database ---
	// Use in-memory SQLite database for testing
	dbPath := "file::memory:?cache=shared" // Use shared cache in-memory DB
	slog.Info("Setting up in-memory SQLite database", "path", dbPath)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		slog.Error("failed to open in-memory database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	// Keep the connection alive for the duration of the test run
	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(1)

	// Test connection
	if err := db.Ping(); err != nil {
		slog.Error("failed to ping in-memory database", "err", err)
		os.Exit(1)
	}
	slog.Info("In-memory database connection successful")

	// --- Run Migrations ---
	// Assume schema.sql is relative to the project root or adjust path as needed.
	// This might require running the test binary from the project root.
	schemaPath := "cmd/migrate/schema.sql"
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		slog.Error("schema.sql not found", "path", schemaPath, "error", err)
		slog.Error("Ensure the test executable is run from the project root directory containing the 'backend' folder.")
		os.Exit(1)
	}

	if err := runMigrations(db, schemaPath); err != nil {
		slog.Error("failed to run database migrations", "err", err)
		os.Exit(1)
	}

	// --- Setup Mock AI API ---
	slog.Info("Setting up Mock Model API")
	// Configure the mock response as needed for your tests
	mockAPI := &MockModelAPI{
		// Example: Predefine a successful response
		Response: &category.ModelAPIResponse{
			Choices: []category.Choice{
				{
					Message: category.Message{
						Content: `{
							"ambiguity_flag": "",
							"spendings": [
								{"apportion_mode": "shared", "category": "Groceries", "amount": 50.0, "description": "Milk & Bread"},
								{"apportion_mode": "alone", "category": "Entertainment", "amount": 25.0, "description": "Cinema Ticket"}
							]
						}`,
					},
				},
			},
		},
		Error: nil, // Set to an error to simulate API failure
	}

	// --- Initialize AI Categorization Pool with Mock API ---
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}
	slog.Info("Initializing AI categorization pool with mock API", "workers", numWorkers)
	categorizationPool := category.NewCategorizingPool(db, numWorkers, mockAPI)
	// Note: We don't start the pool workers in this test setup unless needed for specific tests.
	// go categorizationPool.StartPool() // Uncomment if background processing is part of the test

	// --- HTTP Server Setup (Handlers Only) ---
	mux := http.NewServeMux()

	// --- Public Routes ---
	mux.HandleFunc("POST /v1/login", auth.HandleLogin(db))

	// --- Protected Routes ---
	payHandler := http.HandlerFunc(pay.HandlePayRoute(db))
	getCategoriesHandler := http.HandlerFunc(category.HandleGetCategories(db))
	categorizeHandler := http.HandlerFunc(category.HandleAICategorize(db, categorizationPool)) // Use pool with mock API
	getSpendingsHandler := http.HandlerFunc(spendings.HandleGetSpendings(db))
	updateSpendingHandler := http.HandlerFunc(spendings.HandleUpdateSpending(db))
	getTransferStatusHandler := http.HandlerFunc(transfer.HandleGetTransferStatus(db))
	recordTransferHandler := http.HandlerFunc(transfer.HandleRecordTransfer(db))
	deleteAIJobHandler := http.HandlerFunc(spendings.HandleDeleteAIJob(db)) // Add handler for delete job
	addDepositHandler := http.HandlerFunc(deposit.HandleAddDeposit(db))     // Add handler for adding deposit
	getDepositsHandler := http.HandlerFunc(deposit.HandleGetDeposits(db))   // Add handler for getting deposits

	// Apply AuthMiddleware to protected handlers
	mux.Handle("POST /v1/pay", applyMiddleware(payHandler, auth.AuthMiddleware)) // Correct path for pay handler
	mux.Handle("GET /v1/categories", applyMiddleware(getCategoriesHandler, auth.AuthMiddleware))
	mux.Handle("POST /v1/categorize", applyMiddleware(categorizeHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/history", applyMiddleware(getHistoryHandler, auth.AuthMiddleware)) // Updated route
	mux.Handle("PUT /v1/spendings/{spending_id}", applyMiddleware(updateSpendingHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/transfer/status", applyMiddleware(getTransferStatusHandler, auth.AuthMiddleware))
	mux.Handle("POST /v1/transfer/record", applyMiddleware(recordTransferHandler, auth.AuthMiddleware))

	// --- Apply Middleware (CORS, Logging) ---
	//corsHandler := cors.New(cors.Options{
	//	AllowedOrigins: []string{"*"}, // Keep permissive for testing
	//	AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	//	AllowedHeaders: []string{"Authorization", "Content-Type"},
	//	})
	//	handler := corsHandler.Handler(loggingMiddleware(mux))

	// --- Test Execution Placeholder ---
	slog.Info("Test environment setup complete. HTTP handler is configured.")
	slog.Info("You can now use the 'handler' variable with net/http/httptest to run tests.")

	// Example: Placeholder for where test execution would start
	// runIntegrationTests(handler, db) // Pass handler and db to test functions

	// Keep the main function running if needed, e.g., if pool workers were started.
	// For a simple setup verification, we can just exit.
	slog.Info("Exiting test setup.")
}

// runIntegrationTests is a placeholder for where you'd call your actual test functions.
// func runIntegrationTests(handler http.Handler, db *sql.DB) {
// 	slog.Info("Running integration tests...")
// 	// Example using httptest:
// 	// server := httptest.NewServer(handler)
// 	// defer server.Close()
// 	//
// 	// // Make requests to server.URL
// 	// req, _ := http.NewRequest("GET", server.URL+"/v1/categories", nil)
// 	// // Add auth header etc.
// 	// resp, err := http.DefaultClient.Do(req)
// 	// // Assertions on resp and err...
//
// 	// Or test handlers directly:
// 	// rr := httptest.NewRecorder()
// 	// req, _ := http.NewRequest("GET", "/v1/categories", nil)
// 	// // Add context, auth etc. to req
// 	// handler.ServeHTTP(rr, req)
// 	// // Assertions on rr.Code, rr.Body, etc.
//
// 	slog.Info("Integration tests finished.")
// }
