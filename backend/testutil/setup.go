package testutil

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth"
	"git.sr.ht/~relay/sapp-backend/category"
	"git.sr.ht/~relay/sapp-backend/pay"
	"git.sr.ht/~relay/sapp-backend/spendings"
	"git.sr.ht/~relay/sapp-backend/transfer"
	"github.com/rs/cors"
	_ "modernc.org/sqlite"
)

// MockModelAPI provides a mock implementation of the category.ModelAPI interface.
type MockModelAPI struct {
	// You can add fields here to control the mock's behavior, e.g., predefined responses.
	Response *category.ModelAPIResponse
	Error    error
	// Optional: Add a function to dynamically determine response based on prompt
	PromptFunc func(prompt string) (*category.ModelAPIResponse, error)
}

// Prompt implements the category.ModelAPI interface for the mock.
func (m *MockModelAPI) Prompt(prompt string) (*category.ModelAPIResponse, error) {
	slog.Debug("MockModelAPI: Prompt called", "prompt_substring", prompt[:min(100, len(prompt))]) // Log subset of prompt

	// Use dynamic function if provided
	if m.PromptFunc != nil {
		return m.PromptFunc(prompt)
	}

	// Otherwise, use predefined response/error
	if m.Error != nil {
		slog.Warn("MockModelAPI: Returning predefined error", "error", m.Error)
		return nil, m.Error
	}
	if m.Response != nil {
		slog.Info("MockModelAPI: Returning predefined response")
		return m.Response, nil
	}
	// Default response if nothing is set
	slog.Warn("MockModelAPI: Returning default empty response (no predefined response, error, or function set)")
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
		// Use slog.Debug for test request logging to avoid clutter unless debugging
		slog.Debug("Test Request Start", "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
		slog.Debug("Test Request End", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}

// runMigrations executes the schema SQL against the database.
func runMigrations(db *sql.DB, schemaPath string) error {
	slog.Info("Running migrations...", "schema", schemaPath)
	query, err := os.ReadFile(schemaPath)
	if err != nil {
		// Attempt to find schema relative to test file if not found directly
		// This helps when running 'go test ./...' from the root
		altPath := filepath.Join("..", schemaPath) // Go up one level from testutil
		slog.Warn("Schema not found at primary path, trying alternative", "primary", schemaPath, "alternative", altPath)
		query, err = os.ReadFile(altPath)
		if err != nil {
			return fmt.Errorf("error reading schema file at %s or %s: %w", schemaPath, altPath, err)
		}
		schemaPath = altPath // Use the found path for logging
		slog.Info("Found schema at alternative path", "schema", schemaPath)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("error beginning migration transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(string(query))
	if err != nil {
		return fmt.Errorf("error executing schema SQL from %s: %w", schemaPath, err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("error committing migration transaction: %w", err)
	}
	slog.Info("Migrations completed successfully.")
	return nil
}

// TestEnv holds the components needed for running tests.
type TestEnv struct {
	DB         *sql.DB
	Handler    http.Handler
	MockAPI    *MockModelAPI // Expose mock API for test-specific configuration
	AuthToken  string        // Store the valid auth token
	UserID     int64         // Store the primary test user ID
	PartnerID  int64         // Store the partner user ID
	TearDownDB func()        // Function to close the DB connection
}

// SetupTestEnvironment initializes an in-memory DB, runs migrations,
// sets up handlers with a mock API, and returns the handler and DB connection.
func SetupTestEnvironment(t *testing.T) *TestEnv {
	t.Helper() // Mark this as a test helper function

	// --- Setup Logging ---
	// Configure logging level based on environment variable or default to Info
	logLevel := slog.LevelInfo
	if os.Getenv("SAPP_LOG_LEVEL") == "DEBUG" {
		logLevel = slog.LevelDebug
	}
	logHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})
	logger := slog.New(logHandler)
	slog.SetDefault(logger)
	slog.Info("Setting up test environment...")

	// --- Setup Test Database ---
	dbPath := "file::memory:?cache=shared" // Use shared cache in-memory DB
	slog.Debug("Setting up in-memory SQLite database", "path", dbPath)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	// Keep the connection alive for the duration of the test run
	db.SetMaxIdleConns(1)
	db.SetMaxOpenConns(1)

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close() // Close before failing
		t.Fatalf("failed to ping in-memory database: %v", err)
	}
	slog.Debug("In-memory database connection successful")

	// --- Run Migrations ---
	// Adjust path relative to the 'backend' directory where tests usually run
	schemaPath := "cmd/migrate/schema.sql"
	if err := runMigrations(db, schemaPath); err != nil {
		db.Close() // Close before failing
		t.Fatalf("failed to run database migrations: %v", err)
	}

	// --- Setup Mock AI API ---
	slog.Debug("Setting up Mock Model API")
	mockAPI := &MockModelAPI{
		// Default mock response can be overridden in specific tests
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
		Error: nil,
	}

	// --- Initialize AI Categorization Pool with Mock API ---
	numWorkers := 1 // Use fewer workers for tests unless testing concurrency
	slog.Debug("Initializing AI categorization pool with mock API", "workers", numWorkers)
	categorizationPool := category.NewCategorizingPool(db, numWorkers, mockAPI)
	// Do NOT start the pool automatically in tests. Start it manually if a test needs background processing.
	// go categorizationPool.StartPool()

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

	// Apply AuthMiddleware to protected handlers
	mux.Handle("POST /v1/pay/{shared_status}/{amount}/{category}", applyMiddleware(payHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/categories", applyMiddleware(getCategoriesHandler, auth.AuthMiddleware))
	mux.Handle("POST /v1/categorize", applyMiddleware(categorizeHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/spendings", applyMiddleware(getSpendingsHandler, auth.AuthMiddleware))
	mux.Handle("PUT /v1/spendings/{spending_id}", applyMiddleware(updateSpendingHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/transfer/status", applyMiddleware(getTransferStatusHandler, auth.AuthMiddleware))
	mux.Handle("POST /v1/transfer/record", applyMiddleware(recordTransferHandler, auth.AuthMiddleware))

	// --- Apply Middleware (CORS, Logging) ---
	corsHandler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"}, // Keep permissive for testing
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
	})
	// Apply CORS first, then logging, then the mux router
	handler := corsHandler.Handler(loggingMiddleware(mux))

	slog.Info("Test environment setup complete.")

	// Get the hardcoded demo user token and IDs from auth package (consider making these constants public or providing getters)
	// Assuming auth.demoUserToken, auth.demoUserID, auth.partnerUserID are accessible or known.
	// If they are not exported, duplicate them here or refactor auth package.
	// Let's assume they are known for now:
	const demoUserToken = "demo-user-auth-token"
	const demoUserID = int64(1)
	const partnerUserID = int64(2)

	return &TestEnv{
		DB:         db,
		Handler:    handler,
		MockAPI:    mockAPI,
		AuthToken:  demoUserToken,
		UserID:     demoUserID,
		PartnerID:  partnerUserID,
		TearDownDB: func() { db.Close() },
	}
}

// Helper function to get category ID by name
func GetCategoryID(t *testing.T, db *sql.DB, categoryName string) int64 {
	t.Helper()
	var categoryID int64
	err := db.QueryRow("SELECT id FROM categories WHERE name = ?", categoryName).Scan(&categoryID)
	if err != nil {
		t.Fatalf("Failed to get category ID for '%s': %v", categoryName, err)
	}
	return categoryID
}

// Helper function to insert a spending item for testing
func InsertSpending(t *testing.T, db *sql.DB, buyerID int64, partnerID *int64, categoryID int64, amount float64, description string, sharedUserTakesAll bool, jobID *int64, settledAt *time.Time) int64 {
	t.Helper()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Failed to begin transaction for inserting spending: %v", err)
	}
	defer tx.Rollback()

	// Insert into spendings
	res, err := tx.Exec(`INSERT INTO spendings (amount, description, category, made_by) VALUES (?, ?, ?, ?)`,
		amount, description, categoryID, buyerID)
	if err != nil {
		t.Fatalf("Failed to insert into spendings table: %v", err)
	}
	spendingID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get last insert ID for spending: %v", err)
	}

	// Insert into user_spendings
	_, err = tx.Exec(`INSERT INTO user_spendings (spending_id, buyer, shared_with, shared_user_takes_all, settled_at) VALUES (?, ?, ?, ?, ?)`,
		spendingID, buyerID, partnerID, sharedUserTakesAll, settledAt)
	if err != nil {
		t.Fatalf("Failed to insert into user_spendings table: %v", err)
	}

	// Optionally link to AI job
	if jobID != nil {
		_, err = tx.Exec(`INSERT INTO ai_categorized_spendings (spending_id, job_id) VALUES (?, ?)`,
			spendingID, *jobID)
		if err != nil {
			t.Fatalf("Failed to insert into ai_categorized_spendings table: %v", err)
		}
	}

	if err = tx.Commit(); err != nil {
		t.Fatalf("Failed to commit transaction for inserting spending: %v", err)
	}

	return spendingID
}

// Helper function to insert an AI job for testing
func InsertAIJob(t *testing.T, db *sql.DB, buyerID int64, partnerID *int64, prompt string, totalAmount float64, status string, isFinished bool, isAmbiguous bool, ambiguityReason *string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO ai_categorization_jobs
		(buyer, shared_with, prompt, total_amount, status, is_finished, is_ambiguity_flagged, ambiguity_flag_reason, created_at, status_updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		buyerID, partnerID, prompt, totalAmount, status, isFinished, isAmbiguous, ambiguityReason, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		t.Fatalf("Failed to insert AI job: %v", err)
	}
	jobID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get last insert ID for AI job: %v", err)
	}
	return jobID
}

// Helper function to create a new request with JSON body and auth token.
func NewAuthenticatedRequest(t *testing.T, method, path, token string, body interface{}) *http.Request {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, path, bodyReader)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// Helper function to execute a request and return the recorder.
func ExecuteRequest(t *testing.T, handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// Helper function to assert response status code.
func AssertStatusCode(t *testing.T, rr *httptest.ResponseRecorder, expectedStatus int) {
	t.Helper()
	if status := rr.Code; status != expectedStatus {
		t.Errorf("handler returned wrong status code: got %v want %v", status, expectedStatus)
		t.Logf("Response body: %s", rr.Body.String()) // Log body on error
	}
}

// Helper function to assert response body contains specific strings.
func AssertBodyContains(t *testing.T, rr *httptest.ResponseRecorder, expectedSubstrings ...string) {
	t.Helper()
	body := rr.Body.String()
	for _, sub := range expectedSubstrings {
		if !bytes.Contains(rr.Body.Bytes(), []byte(sub)) {
			t.Errorf("handler response body does not contain expected string '%s'", sub)
			t.Logf("Response body: %s", body) // Log full body
		}
	}
}

// Helper function to decode JSON response body.
func DecodeJSONResponse(t *testing.T, rr *httptest.ResponseRecorder, target interface{}) {
	t.Helper()
	err := json.NewDecoder(rr.Body).Decode(target)
	if err != nil {
		t.Fatalf("Failed to decode JSON response body: %v\nBody: %s", err, rr.Body.String())
	}
}
