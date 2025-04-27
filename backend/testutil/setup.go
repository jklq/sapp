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
	"git.sr.ht/~relay/sapp-backend/deposit"
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
	DB          *sql.DB
	Handler     http.Handler
	MockAPI     *MockModelAPI // Expose mock API for test-specific configuration
	AuthToken   string        // Store the auth token (user ID string) for User 1
	UserID      int64         // Store the primary test user ID (User 1)
	User1Name   string        // Store User 1's first name
	PartnerID   int64         // Store the partner user ID (User 2)
	PartnerName string        // Store User 2's first name
	TearDownDB  func()        // Function to close the DB connection
}

// SetupTestEnvironment initializes an in-memory DB, runs migrations,
// creates two partnered users (User 1 and User 2),
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

	// Allow a couple of connections for potential concurrency during tests
	db.SetMaxIdleConns(2)
	db.SetMaxOpenConns(2)

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
	mux.HandleFunc("POST /v1/register/partners", auth.HandlePartnerRegistration(db)) // Register partner registration handler

	// --- Protected Routes ---
	payHandler := http.HandlerFunc(pay.HandlePayRoute(db))
	getCategoriesHandler := http.HandlerFunc(category.HandleGetCategories(db))
	// Pass pointer to categorizationPool to satisfy the interface
	categorizeHandler := http.HandlerFunc(category.HandleAICategorize(db, &categorizationPool)) // Use pool with mock API
	getHistoryHandler := http.HandlerFunc(spendings.HandleGetHistory(db))                      // Use spendings handler
	updateSpendingHandler := http.HandlerFunc(spendings.HandleUpdateSpending(db))
	getTransferStatusHandler := http.HandlerFunc(transfer.HandleGetTransferStatus(db))
	recordTransferHandler := http.HandlerFunc(transfer.HandleRecordTransfer(db))
	deleteAIJobHandler := http.HandlerFunc(spendings.HandleDeleteAIJob(db))
	addDepositHandler := http.HandlerFunc(deposit.HandleAddDeposit(db))
	getDepositsHandler := http.HandlerFunc(deposit.HandleGetDeposits(db))

	// Apply AuthMiddleware to protected handlers
	mux.Handle("POST /v1/pay", applyMiddleware(payHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/categories", applyMiddleware(getCategoriesHandler, auth.AuthMiddleware))
	mux.Handle("POST /v1/categorize", applyMiddleware(categorizeHandler, auth.AuthMiddleware))
	mux.Handle("GET /v1/history", applyMiddleware(getHistoryHandler, auth.AuthMiddleware)) // Updated route
	mux.Handle("PUT /v1/spendings/{spending_id}", applyMiddleware(updateSpendingHandler, auth.AuthMiddleware))
	mux.Handle("DELETE /v1/jobs/{job_id}", applyMiddleware(deleteAIJobHandler, auth.AuthMiddleware)) // Register delete job route
	mux.Handle("GET /v1/transfer/status", applyMiddleware(getTransferStatusHandler, auth.AuthMiddleware))
	mux.Handle("POST /v1/transfer/record", applyMiddleware(recordTransferHandler, auth.AuthMiddleware))
	mux.Handle("POST /v1/deposits", applyMiddleware(addDepositHandler, auth.AuthMiddleware)) // Register add deposit route
	mux.Handle("GET /v1/deposits", applyMiddleware(getDepositsHandler, auth.AuthMiddleware)) // Register get deposits route

	// --- Apply Middleware (CORS, Logging) ---
	corsHandler := cors.New(cors.Options{
		AllowedOrigins: []string{"*"}, // Keep permissive for testing
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
	})
	// Apply CORS first, then logging, then the mux router
	handler := corsHandler.Handler(loggingMiddleware(mux))

	slog.Info("Test environment setup complete.")

	// --- Retrieve Seeded User/Partner Info ---
	// The schema now seeds user 1 ('Demo') and user 2 ('Partner') and links them.
	// We retrieve their IDs and names to populate the TestEnv.
	var userID int64
	var userName string
	err = db.QueryRow("SELECT id, first_name FROM users WHERE username = 'demo_user'").Scan(&userID, &userName)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to retrieve seeded demo user info: %v", err)
	}

	var partnerID int64
	var partnerName string
	// Use the new GetPartnerUserID function to find the partner
	partnerID, partnerFound := auth.GetPartnerUserID(db, userID)
	if !partnerFound {
		db.Close()
		t.Fatalf("Failed to find partner for seeded demo user (ID: %d) using GetPartnerUserID", userID)
	}
	// Get partner's name
	err = db.QueryRow("SELECT first_name FROM users WHERE id = ?", partnerID).Scan(&partnerName)
	if err != nil {
		db.Close()
		t.Fatalf("Failed to retrieve partner user's name (ID: %d): %v", partnerID, err)
	}

	// --- Generate JWT Token for User 1 ---
	userTokenString, err := auth.GenerateTestJWT(userID) // Use helper to generate token
	if err != nil {
		db.Close()
		t.Fatalf("Failed to generate JWT for test user: %v", err)
	}
	slog.Debug("Generated JWT for test user", "user_id", userID)
	// --- End JWT Generation ---

	return &TestEnv{
		DB:          db,
		Handler:     handler,
		MockAPI:     mockAPI,
		AuthToken:   userTokenString, // User 1's ID string as token
		UserID:      userID,          // User 1 ID
		User1Name:   userName,        // User 1 Name
		PartnerID:   partnerID,       // User 2 ID
		PartnerName: partnerName,     // User 2 Name
		TearDownDB:  func() { db.Close() },
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
// partnerID parameter now represents the ID of the user being shared *with*, if any.
func InsertSpending(t *testing.T, db *sql.DB, buyerID int64, sharedWithID *int64, categoryID int64, amount float64, description string, sharedUserTakesAll bool, jobID *int64, settledAt *time.Time) int64 {
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
		spendingID, buyerID, sharedWithID, sharedUserTakesAll, settledAt)
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
// partnerID parameter now represents the ID of the user being shared *with*, if any.
func InsertAIJob(t *testing.T, db *sql.DB, buyerID int64, sharedWithID *int64, prompt string, totalAmount float64, status string, isFinished bool, isAmbiguous bool, ambiguityReason *string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO ai_categorization_jobs
		(buyer, shared_with, prompt, total_amount, status, is_finished, is_ambiguity_flagged, ambiguity_flag_reason, created_at, status_updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		buyerID, sharedWithID, prompt, totalAmount, status, isFinished, isAmbiguous, ambiguityReason, time.Now().UTC(), time.Now().UTC())
	if err != nil {
		t.Fatalf("Failed to insert AI job: %v", err)
	}
	jobID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get last insert ID for AI job: %v", err)
	}
	return jobID
}

// Helper function to insert a deposit item for testing
func InsertDeposit(t *testing.T, db *sql.DB, userID int64, amount float64, description string, depositDate time.Time, isRecurring bool, recurrencePeriod *string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO deposits (user_id, amount, description, deposit_date, is_recurring, recurrence_period, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, amount, description, depositDate.Format("2006-01-02 15:04:05"), isRecurring, recurrencePeriod, time.Now().UTC())
	if err != nil {
		t.Fatalf("Failed to insert deposit: %v", err)
	}
	depositID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get last insert ID for deposit: %v", err)
	}
	return depositID
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
		// Set Authorization header in Bearer format
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
