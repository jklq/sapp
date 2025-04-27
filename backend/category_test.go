package main_test

import (
	"database/sql"
	"io"
	"net/http"
	"strings"
	"testing"

	"git.sr.ht/~relay/sapp-backend/testutil"
	"git.sr.ht/~relay/sapp-backend/types"
)

// TestGetCategories tests the /v1/categories endpoint.
func TestGetCategories(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Test Case: Success ---
	t.Run("Success", func(t *testing.T) {
		// Use env.AuthToken which is now the user ID string
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/categories", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var categories []types.Category // Use types.Category
		testutil.DecodeJSONResponse(t, rr, &categories)

		// Check if default categories from schema.sql are present
		if len(categories) < 9 { // Check against the number seeded
			t.Errorf("expected at least 9 categories, got %d", len(categories))
		}

		// Check for a specific category
		foundGroceries := false
		for _, cat := range categories {
			if cat.Name == "Groceries" {
				foundGroceries = true
				break
			}
		}
		if !foundGroceries {
			t.Error("expected to find 'Groceries' category, but didn't")
		}
	})

	// --- Test Case: Unauthorized (Missing Token) ---
	t.Run("UnauthorizedMissingToken", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/categories", "", nil) // No token
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Authorization header required")
	})

	// --- Test Case: Unauthorized (Malformed Token) ---
	t.Run("UnauthorizedMalformedToken", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/categories", "this-is-not-a-jwt", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token") // Expect JWT parsing error message
	})
}

// TestAICategorize tests the /v1/categorize endpoint.
func TestAICategorize(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Test Case: Successful Submission (Not Pre-settled) ---
	t.Run("SuccessNotPreSettled", func(t *testing.T) {
		payload := map[string]interface{}{
			"amount":      123.45,
			"prompt":      "Groceries from Rema",
			"pre_settled": false, // Explicitly false
			// "transaction_date": "2024-05-20", // Optional: Add date if needed
		}
		// Use env.AuthToken which is now the user ID string
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/categorize", env.AuthToken, payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusAccepted)

		var respBody map[string]int64
		testutil.DecodeJSONResponse(t, rr, &respBody)

		jobID, ok := respBody["job_id"]
		if !ok || jobID <= 0 {
			t.Errorf("handler returned invalid job_id: got %v", respBody)
		}

		// Verify job exists in DB
		var dbPrompt string
		var dbBuyerID int64
		var dbPartnerID sql.NullInt64
		var dbPreSettled bool
		err := env.DB.QueryRow("SELECT prompt, buyer, shared_with, pre_settled FROM ai_categorization_jobs WHERE id = ?", jobID).Scan(&dbPrompt, &dbBuyerID, &dbPartnerID, &dbPreSettled)
		if err != nil {
			t.Fatalf("Failed to query created job: %v", err)
		}
		if dbPrompt != payload["prompt"] {
			t.Errorf("DB prompt mismatch: got %q, want %q", dbPrompt, payload["prompt"])
		}
		if dbBuyerID != env.UserID {
			t.Errorf("DB buyer ID mismatch: got %d, want %d", dbBuyerID, env.UserID)
		}
		if !dbPartnerID.Valid || dbPartnerID.Int64 != env.PartnerID {
			t.Errorf("DB partner ID mismatch: got %v, want %d", dbPartnerID, env.PartnerID)
		}
		if dbPreSettled != false {
			t.Errorf("DB pre_settled mismatch: got %v, want false", dbPreSettled)
		}
	})

	// --- Test Case: Successful Submission (Pre-settled with Date) ---
	t.Run("SuccessPreSettledWithDate", func(t *testing.T) {
		payload := map[string]interface{}{
			"amount":           50.00,
			"prompt":           "Pre-settled lunch",
			"pre_settled":      true, // Explicitly true
			"transaction_date": "2024-05-21", // Add a date
		}
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/categorize", env.AuthToken, payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusAccepted)

		var respBody map[string]int64
		testutil.DecodeJSONResponse(t, rr, &respBody)
		jobID, ok := respBody["job_id"]
		if !ok || jobID <= 0 {
			t.Errorf("handler returned invalid job_id: got %v", respBody)
		}

		// Verify job exists in DB and pre_settled is true
		var dbPreSettled bool
		err := env.DB.QueryRow("SELECT pre_settled FROM ai_categorization_jobs WHERE id = ?", jobID).Scan(&dbPreSettled)
		if err != nil {
			t.Fatalf("Failed to query created job for pre_settled flag: %v", err)
		}
		if dbPreSettled != true {
			t.Errorf("DB pre_settled mismatch: got %v, want true", dbPreSettled)
		}
		// Verify transaction_date was stored correctly
		var dbDate sql.NullTime
		err = env.DB.QueryRow("SELECT transaction_date FROM ai_categorization_jobs WHERE id = ?", jobID).Scan(&dbDate)
		if err != nil {
			t.Fatalf("Failed to query created job for transaction_date: %v", err)
		}
		if !dbDate.Valid || dbDate.Time.Format("2006-01-02") != "2024-05-21" {
			t.Errorf("DB transaction_date mismatch: got %v, want 2024-05-21", dbDate)
		}
		// Note: Verifying the actual spending item's settled_at requires the worker to run.
		// This test only verifies the job flag is set correctly.
		// A separate integration test involving the worker would be needed for full verification.
	})

	// --- Test Cases: Bad Requests ---
	t.Run("BadRequests", func(t *testing.T) {
		testCases := []struct {
			name           string
			payload        map[string]interface{}
			expectedStatus int
			expectedBody   string
		}{
			{
				name:           "MissingPrompt",
				payload:        map[string]interface{}{"amount": 100.0},
				expectedStatus: http.StatusBadRequest,
				expectedBody:   "Missing prompt or invalid amount",
			},
			{
				name:           "ZeroAmount",
				payload:        map[string]interface{}{"amount": 0.0, "prompt": "test"},
				expectedStatus: http.StatusBadRequest,
				expectedBody:   "Missing prompt or invalid amount",
			},
			{
				name:           "NegativeAmount",
				payload:        map[string]interface{}{"amount": -50.0, "prompt": "test"},
				expectedStatus: http.StatusBadRequest,
				expectedBody:   "Missing prompt or invalid amount",
			},
			{
				name:           "InvalidJSON",
				payload:        nil, // Will cause NewAuthenticatedRequest to send invalid JSON
				expectedStatus: http.StatusBadRequest,
				expectedBody:   "Invalid JSON",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				var req *http.Request
				if tc.name == "InvalidJSON" {
					// Create request with invalid body manually
					req = testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/categorize", env.AuthToken, nil) // body=nil is fine
					req.Body = io.NopCloser(strings.NewReader("{invalid json"))                                      // Set invalid body
					req.Header.Set("Content-Type", "application/json")                                               // Still need content type
				} else {
					// Use env.AuthToken which is now the user ID string
					req = testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/categorize", env.AuthToken, tc.payload)
				}
				rr := testutil.ExecuteRequest(t, env.Handler, req)
				testutil.AssertStatusCode(t, rr, tc.expectedStatus)
				testutil.AssertBodyContains(t, rr, tc.expectedBody)
			})
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		payload := map[string]interface{}{
			"amount": 100.0,
			"prompt": "test",
		}
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/categorize", "invalid-token", payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token")
	})
}
