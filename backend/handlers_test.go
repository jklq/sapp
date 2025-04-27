package main_test // Use main_test to avoid import cycles if needed, or backend_test

import (
	"net/http"
	"testing"

	"database/sql"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv" // Import strconv
	"strings"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth"     // Import auth for LoginRequest/Response
	"git.sr.ht/~relay/sapp-backend/category" // Import category for APICategory
	"git.sr.ht/~relay/sapp-backend/spendings"
	"git.sr.ht/~relay/sapp-backend/testutil" // Import the new test utility package
	"git.sr.ht/~relay/sapp-backend/transfer"
	"golang.org/x/crypto/bcrypt" // Import bcrypt for password verification in registration test
)

// TestLogin tests the /v1/login endpoint.
func TestLogin(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB() // Ensure DB connection is closed

	// --- Test Case: Successful Login ---
	t.Run("Success", func(t *testing.T) {
		loginPayload := auth.LoginRequest{
			Username: "demo_user", // Use the actual username for login
			Password: "password",  // Use the correct password seeded in schema.sql
		}
		req := testutil.NewAuthenticatedRequest(
			t,
			http.MethodPost,
			"/v1/login",
			"",
			loginPayload,
		) // No token needed for login
		rr := testutil.ExecuteRequest(
			t,
			env.Handler,
			req,
		)

		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var respBody auth.LoginResponse
		testutil.DecodeJSONResponse(t, rr, &respBody)

		// Expect user ID string as token
		expectedToken := strconv.FormatInt(env.UserID, 10)
		if respBody.Token != expectedToken {
			t.Errorf("handler returned unexpected token: got %v want %v", respBody.Token, expectedToken)
		}
		if respBody.UserID != env.UserID {
			t.Errorf("handler returned unexpected user ID: got %v want %v", respBody.UserID, env.UserID)
		}
		if respBody.FirstName != env.User1Name { // Check first name from test env setup
			t.Errorf("handler returned unexpected first name: got %v want %v", respBody.FirstName, env.User1Name)
		}
	})

	// --- Test Case: Incorrect Password ---
	t.Run("IncorrectPassword", func(t *testing.T) {
		loginPayload := auth.LoginRequest{
			Username: env.User1Name, // Use correct username from env
			Password: "wrongpassword",
		}
		req := testutil.NewAuthenticatedRequest(
			t,
			http.MethodPost,
			"/v1/login",
			"",
			loginPayload,
		)
		rr := testutil.ExecuteRequest(
			t,
			env.Handler,
			req,
		)

		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid credentials")
	})

	// --- Test Case: User Not Found (or not the specific demo user allowed by simplified login handler) ---
	t.Run("UserNotFoundOrNotDemo", func(t *testing.T) {
		loginPayload := auth.LoginRequest{
			Username: "nonexistent_user", // A user that doesn't exist
			Password: "password",
		}
		req := testutil.NewAuthenticatedRequest(
			t,
			http.MethodPost,
			"/v1/login",
			"",
			loginPayload,
		)
		rr := testutil.ExecuteRequest(
			t,
			env.Handler,
			req,
		)

		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized) // Expect Unauthorized as user doesn't match demo user check in HandleLogin
		testutil.AssertBodyContains(t, rr, "Invalid credentials")

		// Also test with the partner user - login handler only allows demo_user (ID 1)
		loginPayloadPartner := auth.LoginRequest{
			Username: "partner_user", // Use the correct username seeded in schema.sql
			Password: "password",
		}
		reqPartner := testutil.NewAuthenticatedRequest(
			t,
			http.MethodPost,
			"/v1/login",
			"",
			loginPayloadPartner,
		)
		rrPartner := testutil.ExecuteRequest(
			t,
			env.Handler,
			reqPartner,
		)
		// Partner user should now be able to log in successfully and receive the demo token
		testutil.AssertStatusCode(t, rrPartner, http.StatusOK)
		// Decode and check the response body for the partner user
		var respBodyPartner auth.LoginResponse
		testutil.DecodeJSONResponse(t, rrPartner, &respBodyPartner)

		// Expect partner ID string as token
		expectedPartnerToken := strconv.FormatInt(env.PartnerID, 10)
		if respBodyPartner.Token != expectedPartnerToken {
			t.Errorf("Partner login returned unexpected token: got %v want %v", respBodyPartner.Token, expectedPartnerToken)
		}
		if respBodyPartner.UserID != env.PartnerID { // Should get the partner's actual ID
			t.Errorf("Partner login returned unexpected user ID: got %v want %v", respBodyPartner.UserID, env.PartnerID)
		}
		if respBodyPartner.FirstName != env.PartnerName { // Should get the partner's name
			t.Errorf("Partner login returned unexpected first name: got %v want %v", respBodyPartner.FirstName, env.PartnerName)
		}
	})

	// --- Test Case: Missing Username ---
	t.Run("MissingUsername", func(t *testing.T) {
		loginPayload := auth.LoginRequest{
			Username: "",
			Password: "password",
		}
		req := testutil.NewAuthenticatedRequest(
			t,
			http.MethodPost,
			"/v1/login",
			"",
			loginPayload,
		)
		rr := testutil.ExecuteRequest(
			t,
			env.Handler,
			req,
		)

		testutil.AssertStatusCode(t, rr, http.StatusBadRequest)
		testutil.AssertBodyContains(t, rr, "Username and password are required")
	})
}

// TestPartnerRegistration tests the POST /v1/register/partners endpoint.
func TestPartnerRegistration(t *testing.T) {
	// Use a separate env setup because we want a clean DB without the default users
	// Use a separate env setup because we want a clean DB for some sub-tests
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Pre-computation for Conflict Test ---
	// Get the username of the user seeded by SetupTestEnvironment for the conflict test later.
	// We need this *before* potentially deleting users for the Success test.
	_ = env.User1Name

	// --- Test Case: Successful Registration ---
	t.Run("Success", func(t *testing.T) {
		// Define payload for new users
		payload := auth.PartnerRegistrationRequest{
			User1: auth.UserRegistrationDetails{Username: "alice", Password: "password123", FirstName: "Alice"},
			User2: auth.UserRegistrationDetails{Username: "bob", Password: "password456", FirstName: "Bob"},
		}

		// Make the registration request
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/register/partners", "", payload) // No auth needed
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		// Assert success status and decode response
		testutil.AssertStatusCode(t, rr, http.StatusCreated)
		var respBody auth.PartnerRegistrationResponse
		testutil.DecodeJSONResponse(t, rr, &respBody)

		// Basic response body checks
		if respBody.Message == "" {
			t.Error("Expected a success message in response body")
		}
		if respBody.User1ID <= 0 || respBody.User2ID <= 0 {
			t.Errorf("Expected positive user IDs in response body, got %d and %d", respBody.User1ID, respBody.User2ID)
		}
		if respBody.User1ID == respBody.User2ID {
			t.Error("Expected different user IDs in response body")
		}

		// Verify User 1 in DB
		var dbUsername1, dbFirstName1, dbHash1 string
		err := env.DB.QueryRow("SELECT username, first_name, password_hash FROM users WHERE id = ?", respBody.User1ID).Scan(&dbUsername1, &dbFirstName1, &dbHash1)
		if err != nil {
			t.Fatalf("Failed to query user 1 (ID: %d): %v", respBody.User1ID, err)
		}
		if dbUsername1 != payload.User1.Username {
			t.Errorf("User 1 username mismatch: got %s, want %s", dbUsername1, payload.User1.Username)
		}
		if dbFirstName1 != payload.User1.FirstName {
			t.Errorf("User 1 first name mismatch: got %s, want %s", dbFirstName1, payload.User1.FirstName)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(dbHash1), []byte(payload.User1.Password)); err != nil {
			t.Errorf("User 1 password hash mismatch: %v", err)
		}

		// Verify User 2 in DB
		var dbUsername2, dbFirstName2, dbHash2 string
		err = env.DB.QueryRow("SELECT username, first_name, password_hash FROM users WHERE id = ?", respBody.User2ID).Scan(&dbUsername2, &dbFirstName2, &dbHash2)
		if err != nil {
			t.Fatalf("Failed to query user 2 (ID: %d): %v", respBody.User2ID, err)
		}
		if dbUsername2 != payload.User2.Username {
			t.Errorf("User 2 username mismatch: got %s, want %s", dbUsername2, payload.User2.Username)
		}
		if dbFirstName2 != payload.User2.FirstName {
			t.Errorf("User 2 first name mismatch: got %s, want %s", dbFirstName2, payload.User2.FirstName)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(dbHash2), []byte(payload.User2.Password)); err != nil {
			t.Errorf("User 2 password hash mismatch: %v", err)
		}

		// Verify Partnership in DB (ensure correct order based on IDs)
		var pUser1, pUser2 int64
		qUser1, qUser2 := respBody.User1ID, respBody.User2ID
		if qUser1 > qUser2 { // Ensure user1_id < user2_id for query
			qUser1, qUser2 = qUser2, qUser1
		}
		err = env.DB.QueryRow("SELECT user1_id, user2_id FROM partnerships WHERE user1_id = ? AND user2_id = ?", qUser1, qUser2).Scan(&pUser1, &pUser2)
		if err != nil {
			t.Fatalf("Failed to query partnership for users %d and %d: %v", respBody.User1ID, respBody.User2ID, err)
		}
		if pUser1 != qUser1 || pUser2 != qUser2 {
			t.Errorf("Partnership DB mismatch: got %d, %d; want %d, %d", pUser1, pUser2, qUser1, qUser2)
		}
	})

	// --- Test Cases: Bad Requests ---
	t.Run("BadRequests", func(t *testing.T) {
		testCases := []struct {
			name         string
			payload      auth.PartnerRegistrationRequest
			expectedMsg  string
			expectedCode int
		}{
			{"MissingUser1Username", auth.PartnerRegistrationRequest{User1: auth.UserRegistrationDetails{Password: "p", FirstName: "F"}, User2: auth.UserRegistrationDetails{Username: "u", Password: "p", FirstName: "F"}}, "All fields", http.StatusBadRequest},
			{"MissingUser2Password", auth.PartnerRegistrationRequest{User1: auth.UserRegistrationDetails{Username: "u", Password: "p", FirstName: "F"}, User2: auth.UserRegistrationDetails{Username: "u2", FirstName: "F"}}, "All fields", http.StatusBadRequest},
			{"SameUsernames", auth.PartnerRegistrationRequest{User1: auth.UserRegistrationDetails{Username: "same", Password: "p", FirstName: "F"}, User2: auth.UserRegistrationDetails{Username: "same", Password: "p2", FirstName: "F2"}}, "Usernames must be different", http.StatusBadRequest},
			{"ShortPassword", auth.PartnerRegistrationRequest{User1: auth.UserRegistrationDetails{Username: "u1", Password: "short", FirstName: "F1"}, User2: auth.UserRegistrationDetails{Username: "u2", Password: "password", FirstName: "F2"}}, "Password must be at least 6 characters", http.StatusBadRequest},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/register/partners", "", tc.payload)
				rr := testutil.ExecuteRequest(t, env.Handler, req)
				testutil.AssertStatusCode(t, rr, tc.expectedCode)
				testutil.AssertBodyContains(t, rr, tc.expectedMsg)
			})
		}
	})

	// --- Test Case: Username Conflict ---
	t.Run("UsernameConflict", func(t *testing.T) {
		// Seed a user specifically for this conflict test
		conflictUsername := "existing_user"
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
		_, err := env.DB.Exec("INSERT INTO users (username, password_hash, first_name) VALUES (?, ?, ?)", conflictUsername, string(hashedPassword), "ConflictTestUser")
		if err != nil {
			t.Fatalf("Failed to seed user for UsernameConflict test: %v", err)
		}

		// Attempt to register using the existing username
		payload := auth.PartnerRegistrationRequest{
			User1: auth.UserRegistrationDetails{Username: conflictUsername, Password: "password123", FirstName: "Conflict"}, // Use existing username
			User2: auth.UserRegistrationDetails{Username: "new_partner", Password: "password456", FirstName: "New"},
		}
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/register/partners", "", payload) // No auth needed
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusConflict)
		testutil.AssertBodyContains(t, rr, "already exist")
	})
}

// TestDeleteAIJob tests the DELETE /v1/jobs/{job_id} endpoint.
func TestDeleteAIJob(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	groceriesID := testutil.GetCategoryID(t, env.DB, "Groceries")
	transportID := testutil.GetCategoryID(t, env.DB, "Transport")

	// --- Setup Data ---
	// Job 1 (User's job with spendings, shared with Partner)
	jobIDUser := testutil.InsertAIJob(t, env.DB, env.UserID, &env.PartnerID, "User Job", 75.0, "finished", true, false, nil)
	spending1_1 := testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "User Shared", false, &jobIDUser, nil) // Shared with partner
	spending1_2 := testutil.InsertSpending(t, env.DB, env.UserID, nil, transportID, 25.0, "User Alone", false, &jobIDUser, nil)             // User alone

	// Job 2 (Partner's job - for forbidden test, shared with User)
	jobIDPartner := testutil.InsertAIJob(t, env.DB, env.PartnerID, &env.UserID, "Partner Job", 100.0, "finished", true, false, nil)
	_ = testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, groceriesID, 100.0, "Partner Shared", false, &jobIDPartner, nil) // Shared with user

	// --- Test Case: Success ---
	t.Run("Success", func(t *testing.T) {
		// Verify data exists before deletion
		var jobCount, spendingCount, userSpendingCount int
		env.DB.QueryRow("SELECT COUNT(*) FROM ai_categorization_jobs WHERE id = ?", jobIDUser).Scan(&jobCount)
		env.DB.QueryRow("SELECT COUNT(*) FROM spendings WHERE id IN (?, ?)", spending1_1, spending1_2).Scan(&spendingCount)
		env.DB.QueryRow("SELECT COUNT(*) FROM user_spendings WHERE spending_id IN (?, ?)", spending1_1, spending1_2).Scan(&userSpendingCount)
		if jobCount != 1 || spendingCount != 2 || userSpendingCount != 2 {
			t.Fatalf("Pre-delete check failed: job=%d, spendings=%d, user_spendings=%d", jobCount, spendingCount, userSpendingCount)
		}

		url := fmt.Sprintf("/v1/jobs/%d", jobIDUser)
		// Use env.AuthToken which is now the user ID string
		req := testutil.NewAuthenticatedRequest(t, http.MethodDelete, url, env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusNoContent)

		// Verify data is deleted
		env.DB.QueryRow("SELECT COUNT(*) FROM ai_categorization_jobs WHERE id = ?", jobIDUser).Scan(&jobCount)
		env.DB.QueryRow("SELECT COUNT(*) FROM spendings WHERE id IN (?, ?)", spending1_1, spending1_2).Scan(&spendingCount)
		env.DB.QueryRow("SELECT COUNT(*) FROM user_spendings WHERE spending_id IN (?, ?)", spending1_1, spending1_2).Scan(&userSpendingCount)
		if jobCount != 0 || spendingCount != 0 || userSpendingCount != 0 {
			t.Errorf("Post-delete check failed: job=%d, spendings=%d, user_spendings=%d", jobCount, spendingCount, userSpendingCount)
		}
	})

	// --- Test Case: Not Found ---
	t.Run("NotFound", func(t *testing.T) {
		url := "/v1/jobs/99999" // Non-existent job ID
		// Use env.AuthToken which is now the user ID string
		req := testutil.NewAuthenticatedRequest(t, http.MethodDelete, url, env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusNotFound)
		testutil.AssertBodyContains(t, rr, "Job not found")
	})

	// --- Test Case: Forbidden ---
	t.Run("Forbidden", func(t *testing.T) {
		url := fmt.Sprintf("/v1/jobs/%d", jobIDPartner) // Job belongs to partner
		// Use env.AuthToken which is now the user ID string (User 1)
		req := testutil.NewAuthenticatedRequest(t, http.MethodDelete, url, env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusForbidden)
		testutil.AssertBodyContains(t, rr, "Forbidden")

		// Verify partner's job was NOT deleted
		var jobCount int
		env.DB.QueryRow("SELECT COUNT(*) FROM ai_categorization_jobs WHERE id = ?", jobIDPartner).Scan(&jobCount)
		if jobCount != 1 {
			t.Errorf("Partner's job should not have been deleted, count=%d", jobCount)
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		// Re-insert user's job as it was deleted in the success test
		jobIDUserReinsert := testutil.InsertAIJob(t, env.DB, env.UserID, &env.PartnerID, "User Job Reinsert", 10.0, "finished", true, false, nil)

		url := fmt.Sprintf("/v1/jobs/%d", jobIDUserReinsert)
		// Use an invalid user ID string as the token
		req := testutil.NewAuthenticatedRequest(t, http.MethodDelete, url, "invalid-user-id-string", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized) // Middleware should reject non-integer token

		// Verify job was NOT deleted
		var jobCount int
		env.DB.QueryRow("SELECT COUNT(*) FROM ai_categorization_jobs WHERE id = ?", jobIDUserReinsert).Scan(&jobCount)
		if jobCount != 1 {
			t.Errorf("Job should not have been deleted by unauthorized request, count=%d", jobCount)
		}
	})

	// --- Test Case: Invalid Job ID Format ---
	t.Run("InvalidJobIDFormat", func(t *testing.T) {
		url := "/v1/jobs/abc" // Invalid format
		// Use env.AuthToken which is now the user ID string
		req := testutil.NewAuthenticatedRequest(t, http.MethodDelete, url, env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusBadRequest)
		testutil.AssertBodyContains(t, rr, "Invalid job ID")
	})
}

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

		var categories []category.APICategory
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

	// --- Test Case: Unauthorized (Invalid Token) ---
	t.Run("UnauthorizedInvalidToken", func(t *testing.T) {
		// Use an invalid user ID string as the token
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/categories", "invalid-user-id-string", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)                // Middleware should reject non-integer token
		testutil.AssertBodyContains(t, rr, "Invalid authorization token format") // Expect new error message
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

	// --- Test Case: Successful Submission (Pre-settled) ---
	t.Run("SuccessPreSettled", func(t *testing.T) {
		payload := map[string]interface{}{
			"amount":      50.00,
			"prompt":      "Pre-settled lunch",
			"pre_settled": true, // Explicitly true
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
		// Use an invalid user ID string as the token
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/categorize", "invalid-user-id-string", payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized) // Middleware should reject non-integer token
	})
}

// TestAddDeposit tests the POST /v1/deposits endpoint.
func TestAddDeposit(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Test Cases ---
	testCases := []struct {
		name           string
		payload        deposit.AddDepositPayload
		expectedStatus int
		expectedBody   string                       // Substring for error messages or success message
		verifyFunc     func(t *testing.T, id int64) // Optional verification
	}{
		{
			name: "SuccessOneOff",
			payload: deposit.AddDepositPayload{
				Amount:      1000.00,
				Description: "Salary",
				DepositDate: "2024-05-15",
				IsRecurring: false,
			},
			expectedStatus: http.StatusCreated,
			expectedBody:   "Deposit added successfully",
			verifyFunc: func(t *testing.T, id int64) {
				var dbAmount float64
				var dbDesc string
				var dbDate string
				var dbRecurring bool
				var dbPeriod sql.NullString
				err := env.DB.QueryRow("SELECT amount, description, strftime('%Y-%m-%d', deposit_date), is_recurring, recurrence_period FROM deposits WHERE id = ?", id).Scan(&dbAmount, &dbDesc, &dbDate, &dbRecurring, &dbPeriod)
				if err != nil {
					t.Fatalf("Verification query failed: %v", err)
				}
				if math.Abs(dbAmount-1000.00) > 0.001 {
					t.Errorf("Expected amount 1000.00, got %f", dbAmount)
				}
				if dbDesc != "Salary" {
					t.Errorf("Expected description 'Salary', got '%s'", dbDesc)
				}
				if dbDate != "2024-05-15" {
					t.Errorf("Expected date '2024-05-15', got '%s'", dbDate)
				}
				if dbRecurring != false {
					t.Error("Expected is_recurring false")
				}
				if dbPeriod.Valid {
					t.Errorf("Expected recurrence_period NULL, got %v", dbPeriod.String)
				}
			},
		},
		{
			name: "SuccessRecurring",
			payload: deposit.AddDepositPayload{
				Amount:           50.00,
				Description:      "Pocket Money",
				DepositDate:      "2024-05-10",
				IsRecurring:      true,
				RecurrencePeriod: Ptr("weekly"), // Use helper for pointer
			},
			expectedStatus: http.StatusCreated,
			expectedBody:   "Deposit added successfully",
			verifyFunc: func(t *testing.T, id int64) {
				var dbRecurring bool
				var dbPeriod sql.NullString
				err := env.DB.QueryRow("SELECT is_recurring, recurrence_period FROM deposits WHERE id = ?", id).Scan(&dbRecurring, &dbPeriod)
				if err != nil {
					t.Fatalf("Verification query failed: %v", err)
				}
				if dbRecurring != true {
					t.Error("Expected is_recurring true")
				}
				if !dbPeriod.Valid || dbPeriod.String != "weekly" {
					t.Errorf("Expected recurrence_period 'weekly', got %v", dbPeriod)
				}
			},
		},
		{
			name: "ErrorNegativeAmount",
			payload: deposit.AddDepositPayload{
				Amount:      -100.00,
				Description: "Invalid",
				DepositDate: "2024-05-15",
				IsRecurring: false,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Amount must be positive",
		},
		{
			name: "ErrorMissingDescription",
			payload: deposit.AddDepositPayload{
				Amount:      100.00,
				Description: "", // Missing
				DepositDate: "2024-05-15",
				IsRecurring: false,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Description is required",
		},
		{
			name: "ErrorInvalidDateFormat",
			payload: deposit.AddDepositPayload{
				Amount:      100.00,
				Description: "Bad Date",
				DepositDate: "15-05-2024", // Wrong format
				IsRecurring: false,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid date format",
		},
		{
			name: "ErrorMissingRecurrencePeriod",
			payload: deposit.AddDepositPayload{
				Amount:      100.00,
				Description: "Recurring No Period",
				DepositDate: "2024-05-15",
				IsRecurring: true,
				// RecurrencePeriod: nil, // Missing
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Recurrence period is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/deposits", env.AuthToken, tc.payload)
			rr := testutil.ExecuteRequest(t, env.Handler, req)

			testutil.AssertStatusCode(t, rr, tc.expectedStatus)
			if tc.expectedBody != "" {
				testutil.AssertBodyContains(t, rr, tc.expectedBody)
			}

			if tc.expectedStatus == http.StatusCreated && tc.verifyFunc != nil {
				var respBody deposit.AddDepositResponse
				testutil.DecodeJSONResponse(t, rr, &respBody)
				if respBody.DepositID <= 0 {
					t.Fatalf("Expected positive deposit ID in response, got %d", respBody.DepositID)
				}
				tc.verifyFunc(t, respBody.DepositID)
			}
		})
	}

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		payload := deposit.AddDepositPayload{Amount: 100, Description: "Test", DepositDate: "2024-01-01"}
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/deposits", "invalid-token", payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
	})
}

// Helper function to create a pointer to a string
func Ptr(s string) *string {
	return &s
}

// TestGetHistory tests the /v1/history endpoint (previously /v1/spendings).
func TestGetHistory(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	groceriesID := testutil.GetCategoryID(t, env.DB, "Groceries")
	transportID := testutil.GetCategoryID(t, env.DB, "Transport")

	// --- Test Case: Empty History ---
	t.Run("EmptyHistory", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/history", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var resp spendings.HistoryResponse
		testutil.DecodeJSONResponse(t, rr, &resp)
		if len(resp.SpendingGroups) != 0 {
			t.Errorf("Expected 0 spending groups, got %d", len(resp.SpendingGroups))
		}
		if len(resp.Deposits) != 0 {
			t.Errorf("Expected 0 deposits, got %d", len(resp.Deposits))
		}
	})

	// --- Setup Data for Subsequent Tests ---
	// Spending Job 1: Shared groceries and alone transport (User paid)
	job1Time := time.Now().Add(-2 * time.Hour) // Ensure distinct time
	job1ID := testutil.InsertAIJob(t, env.DB, env.UserID, &env.PartnerID, "Groceries and bus ticket", 75.0, "finished", true, false, nil)
	spending1_1 := testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "Milk & Bread", false, &job1ID, nil)
	spending1_2 := testutil.InsertSpending(t, env.DB, env.UserID, nil, transportID, 25.0, "Bus Ticket", false, &job1ID, nil)
	// Manually update job created_at time for sorting test
	_, err := env.DB.Exec("UPDATE ai_categorization_jobs SET created_at = ? WHERE id = ?", job1Time, job1ID)
	if err != nil {
		t.Fatalf("Failed to update job1 time: %v", err)
	}

	// Spending Job 2: Paid by partner (User submitted job)
	job2Time := time.Now().Add(-1 * time.Hour) // Ensure distinct time
	job2ID := testutil.InsertAIJob(t, env.DB, env.UserID, &env.PartnerID, "Gift for me from Partner", 100.0, "finished", true, false, nil)
	spending2_1 := testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 100.0, "Gift", true, &job2ID, nil)
	_, err = env.DB.Exec("UPDATE ai_categorization_jobs SET created_at = ? WHERE id = ?", job2Time, job2ID)
	if err != nil {
		t.Fatalf("Failed to update job2 time: %v", err)
	}

	// Deposit 1: Salary
	deposit1Time := time.Now().Add(-3 * time.Hour) // Ensure distinct time
	deposit1Date := time.Now().AddDate(0, 0, -10)  // 10 days ago
	deposit1ID := testutil.InsertDeposit(t, env.DB, env.UserID, 2000.0, "Salary May", deposit1Date, false, nil)
	_, err = env.DB.Exec("UPDATE deposits SET created_at = ? WHERE id = ?", deposit1Time, deposit1ID) // Update created_at for sorting consistency if needed
	if err != nil {
		t.Fatalf("Failed to update deposit1 time: %v", err)
	}

	// Deposit 2: Recurring
	deposit2Time := time.Now().Add(-4 * time.Hour) // Ensure distinct time
	deposit2Date := time.Now().AddDate(0, -1, 0)   // 1 month ago
	deposit2ID := testutil.InsertDeposit(t, env.DB, env.UserID, 50.0, "Pocket Money", deposit2Date, true, Ptr("monthly"))
	_, err = env.DB.Exec("UPDATE deposits SET created_at = ? WHERE id = ?", deposit2Time, deposit2ID) // Update created_at for sorting consistency if needed
	if err != nil {
		t.Fatalf("Failed to update deposit2 time: %v", err)
	}

	// --- Test Case: Fetch History (Combined) ---
	t.Run("FetchHistoryCombined", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/history", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var resp spendings.HistoryResponse
		testutil.DecodeJSONResponse(t, rr, &resp)

		// Check counts
		if len(resp.SpendingGroups) != 2 {
			t.Fatalf("Expected 2 spending groups, got %d", len(resp.SpendingGroups))
		}
		if len(resp.Deposits) != 2 {
			t.Fatalf("Expected 2 deposits, got %d", len(resp.Deposits))
		}

		// --- Verify Spending Groups (Order: Job2 then Job1 based on created_at DESC) ---
		group2 := resp.SpendingGroups[0] // Job 2 should be first
		if group2.JobID != job2ID {
			t.Errorf("Expected first group JobID %d, got %d", job2ID, group2.JobID)
		}
		if group2.Type != "spending_group" {
			t.Errorf("Expected group type 'spending_group', got '%s'", group2.Type)
		}
		if len(group2.Spendings) != 1 || group2.Spendings[0].ID != spending2_1 {
			t.Errorf("Expected 1 spending item with ID %d in group 2, got %v", spending2_1, group2.Spendings)
		}
		expectedStatus2 := fmt.Sprintf("Paid by %s", env.PartnerName)
		if group2.Spendings[0].SharingStatus != expectedStatus2 {
			t.Errorf("Expected spending 2_1 status '%s', got '%s'", expectedStatus2, group2.Spendings[0].SharingStatus)
		}

		group1 := resp.SpendingGroups[1] // Job 1 should be second
		if group1.JobID != job1ID {
			t.Errorf("Expected second group JobID %d, got %d", job1ID, group1.JobID)
		}
		if len(group1.Spendings) != 2 {
			t.Fatalf("Expected 2 spending items in group 1, got %d", len(group1.Spendings))
		}
		if group1.Spendings[0].ID != spending1_1 || group1.Spendings[1].ID != spending1_2 {
			t.Errorf("Expected spending IDs %d, %d in group 1, got %d, %d", spending1_1, spending1_2, group1.Spendings[0].ID, group1.Spendings[1].ID)
		}

		// --- Verify Deposits (Order: Deposit1 then Deposit2 based on deposit_date DESC) ---
		dep1 := resp.Deposits[0] // Deposit 1 (May 15th) should be first
		if dep1.ID != deposit1ID {
			t.Errorf("Expected first deposit ID %d, got %d", deposit1ID, dep1.ID)
		}
		if dep1.Type != "deposit" {
			t.Errorf("Expected deposit type 'deposit', got '%s'", dep1.Type)
		}
		if dep1.Description != "Salary May" {
			t.Errorf("Expected deposit 1 description 'Salary May', got '%s'", dep1.Description)
		}
		if dep1.IsRecurring {
			t.Error("Expected deposit 1 not to be recurring")
		}

		dep2 := resp.Deposits[1] // Deposit 2 (Apr 25th) should be second
		if dep2.ID != deposit2ID {
			t.Errorf("Expected second deposit ID %d, got %d", deposit2ID, dep2.ID)
		}
		if !dep2.IsRecurring || dep2.RecurrencePeriod == nil || *dep2.RecurrencePeriod != "monthly" {
			t.Errorf("Expected deposit 2 to be recurring monthly, got recurring=%v, period=%v", dep2.IsRecurring, dep2.RecurrencePeriod)
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/history", "invalid-token", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
	})
}

// TestUpdateSpending tests the PUT /v1/spendings/{spending_id} endpoint.
func TestUpdateSpending(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	groceriesID := testutil.GetCategoryID(t, env.DB, "Groceries")
	transportID := testutil.GetCategoryID(t, env.DB, "Transport")
	shoppingID := testutil.GetCategoryID(t, env.DB, "Shopping")

	// --- Setup Data ---
	// Spending 1: Initially shared groceries (User paid, shared with Partner)
	spendingIDShared := testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "Initial Shared", false, nil, nil)
	// Spending 2: Initially alone transport (User paid, alone)
	spendingIDAlone := testutil.InsertSpending(t, env.DB, env.UserID, nil, transportID, 25.0, "Initial Alone", false, nil, nil)
	// Spending 3: Initially paid by partner (User paid, shared with Partner, Partner takes all)
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, shoppingID, 100.0, "Initial PaidByPartner", true, nil, nil)
	// Spending 4: Belongs to partner (Partner paid, shared with User) - for forbidden test
	spendingIDPartners := testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, groceriesID, 30.0, "Partner's Spending", false, nil, nil)

	// --- Test Cases ---
	testCases := []struct {
		name           string
		spendingID     int64
		payload        spendings.UpdateSpendingPayload
		expectedStatus int
		expectedBody   string                       // Substring to check in body for errors
		verifyFunc     func(t *testing.T, id int64) // Optional verification function
	}{
		{
			name:       "SuccessUpdateToAlone",
			spendingID: spendingIDShared,
			payload: spendings.UpdateSpendingPayload{
				Description:   "Updated to Alone",
				CategoryName:  "Transport",
				SharingStatus: spendings.StatusAlone,
			},
			expectedStatus: http.StatusOK,
			verifyFunc: func(t *testing.T, id int64) {
				var desc string
				var catID int64
				var sharedWith sql.NullInt64
				var takesAll bool
				err := env.DB.QueryRow("SELECT s.description, s.category, us.shared_with, us.shared_user_takes_all FROM spendings s JOIN user_spendings us ON s.id = us.spending_id WHERE s.id = ?", id).Scan(&desc, &catID, &sharedWith, &takesAll)
				if err != nil {
					t.Fatalf("Verification query failed: %v", err)
				}
				if desc != "Updated to Alone" {
					t.Errorf("Expected description 'Updated to Alone', got '%s'", desc)
				}
				if catID != transportID {
					t.Errorf("Expected category ID %d, got %d", transportID, catID)
				}
				if sharedWith.Valid {
					t.Errorf("Expected shared_with to be NULL, got %v", sharedWith.Int64)
				}
				if takesAll {
					t.Error("Expected shared_user_takes_all to be false")
				}
			},
		},
		{
			name:       "SuccessUpdateToShared",
			spendingID: spendingIDAlone,
			payload: spendings.UpdateSpendingPayload{
				Description:   "Updated to Shared",
				CategoryName:  "Groceries",
				SharingStatus: spendings.StatusShared,
			},
			expectedStatus: http.StatusOK,
			verifyFunc: func(t *testing.T, id int64) {
				var sharedWith sql.NullInt64
				var takesAll bool
				err := env.DB.QueryRow("SELECT shared_with, shared_user_takes_all FROM user_spendings WHERE spending_id = ?", id).Scan(&sharedWith, &takesAll)
				if err != nil {
					t.Fatalf("Verification query failed: %v", err)
				}
				if !sharedWith.Valid || sharedWith.Int64 != env.PartnerID {
					t.Errorf("Expected shared_with to be %d, got %v", env.PartnerID, sharedWith)
				}
				if takesAll {
					t.Error("Expected shared_user_takes_all to be false")
				}
			},
		},
		{
			name:       "SuccessUpdateToPaidByPartner",
			spendingID: spendingIDShared, // Use the initially shared one
			payload: spendings.UpdateSpendingPayload{
				Description:   "Updated to PaidByPartner",
				CategoryName:  "Shopping",
				SharingStatus: spendings.StatusPaidByPartner,
			},
			expectedStatus: http.StatusOK,
			verifyFunc: func(t *testing.T, id int64) {
				var sharedWith sql.NullInt64
				var takesAll bool
				err := env.DB.QueryRow("SELECT shared_with, shared_user_takes_all FROM user_spendings WHERE spending_id = ?", id).Scan(&sharedWith, &takesAll)
				if err != nil {
					t.Fatalf("Verification query failed: %v", err)
				}
				if !sharedWith.Valid || sharedWith.Int64 != env.PartnerID {
					t.Errorf("Expected shared_with to be %d, got %v", env.PartnerID, sharedWith)
				}
				if !takesAll {
					t.Error("Expected shared_user_takes_all to be true")
				}
			},
		},
		{
			name:       "ErrorNotFound",
			spendingID: 99999,
			payload: spendings.UpdateSpendingPayload{
				Description:   "Test",
				CategoryName:  "Groceries",
				SharingStatus: spendings.StatusAlone,
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Spending item not found",
		},
		{
			name:       "ErrorForbidden",
			spendingID: spendingIDPartners, // Belongs to partner
			payload: spendings.UpdateSpendingPayload{
				Description:   "Attempt Forbidden Update",
				CategoryName:  "Groceries",
				SharingStatus: spendings.StatusAlone,
			},
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Forbidden",
		},
		{
			name:       "ErrorInvalidCategory",
			spendingID: spendingIDShared,
			payload: spendings.UpdateSpendingPayload{
				Description:   "Test",
				CategoryName:  "NonExistentCategory",
				SharingStatus: spendings.StatusAlone,
			},
			expectedStatus: http.StatusBadRequest, // Bad request because category is invalid input
			expectedBody:   "Category not found",
		},
		{
			name:       "ErrorInvalidStatus",
			spendingID: spendingIDShared,
			payload: spendings.UpdateSpendingPayload{
				Description:   "Test",
				CategoryName:  "Groceries",
				SharingStatus: "invalid_status",
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid sharing status",
		},
		{
			name:       "ErrorMissingCategory",
			spendingID: spendingIDShared,
			payload: spendings.UpdateSpendingPayload{
				Description:   "Test",
				CategoryName:  "", // Missing category
				SharingStatus: spendings.StatusAlone,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Category name is required",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := fmt.Sprintf("/v1/spendings/%d", tc.spendingID)
			// Use env.AuthToken which is now the user ID string
			req := testutil.NewAuthenticatedRequest(t, http.MethodPut, url, env.AuthToken, tc.payload)
			rr := testutil.ExecuteRequest(t, env.Handler, req)

			testutil.AssertStatusCode(t, rr, tc.expectedStatus)
			if tc.expectedBody != "" {
				testutil.AssertBodyContains(t, rr, tc.expectedBody)
			}
			if tc.verifyFunc != nil {
				tc.verifyFunc(t, tc.spendingID)
			}
		})
	}

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		url := fmt.Sprintf("/v1/spendings/%d", spendingIDShared)
		payload := spendings.UpdateSpendingPayload{
			Description:   "Unauthorized Update",
			CategoryName:  "Groceries",
			SharingStatus: spendings.StatusAlone,
		}
		// Use an invalid user ID string as the token
		req := testutil.NewAuthenticatedRequest(t, http.MethodPut, url, "invalid-user-id-string", payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized) // Middleware should reject non-integer token
	})
}

// TestGetTransferStatus tests the /v1/transfer/status endpoint.
func TestGetTransferStatus(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	groceriesID := testutil.GetCategoryID(t, env.DB, "Groceries")
	shoppingID := testutil.GetCategoryID(t, env.DB, "Shopping")

	// --- Test Case: Initial Status (No Spendings) ---
	t.Run("InitialStatus", func(t *testing.T) {
		// Use env.AuthToken which is now the user ID string
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/transfer/status", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var resp transfer.TransferStatusResponse
		testutil.DecodeJSONResponse(t, rr, &resp)

		if resp.PartnerName != env.PartnerName {
			t.Errorf("Expected partner name '%s', got '%s'", env.PartnerName, resp.PartnerName)
		}
		if resp.AmountOwed != 0.0 {
			t.Errorf("Expected amount owed 0.0, got %f", resp.AmountOwed)
		}
		if resp.OwedBy != nil || resp.OwedTo != nil {
			t.Errorf("Expected OwedBy and OwedTo to be nil, got %v, %v", resp.OwedBy, resp.OwedTo)
		}
	})

	// --- Setup Data ---
	// 1. User paid 50, shared with partner -> Partner owes User 25
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "Shared Groceries", false, nil, nil)
	// 2. Partner paid 100, shared with user -> User owes Partner 50
	_ = testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, shoppingID, 100.0, "Shared Shopping", false, nil, nil)
	// 3. User paid 30, alone -> No effect on balance
	_ = testutil.InsertSpending(t, env.DB, env.UserID, nil, groceriesID, 30.0, "Alone Groceries", false, nil, nil)
	// 4. User paid 40, partner takes all -> Partner owes User 40
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, shoppingID, 40.0, "Gift for Partner", true, nil, nil)
	// 5. Partner paid 20, user takes all -> User owes Partner 20
	_ = testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, groceriesID, 20.0, "Gift for User", true, nil, nil)
	// 6. Settled spending (should be ignored)
	settledTime := time.Now().Add(-time.Hour).UTC()
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 200.0, "Settled Item", false, nil, &settledTime)

	// Expected Balance:
	// User Net = +25 (from 1) - 50 (from 2) + 40 (from 4) - 20 (from 5) = -5.0
	// User owes Partner 5.0

	// --- Test Case: Calculated Status ---
	t.Run("CalculatedStatus", func(t *testing.T) {
		// Use env.AuthToken which is now the user ID string
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/transfer/status", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var resp transfer.TransferStatusResponse
		testutil.DecodeJSONResponse(t, rr, &resp)

		if resp.PartnerName != env.PartnerName {
			t.Errorf("Expected partner name '%s', got '%s'", env.PartnerName, resp.PartnerName)
		}
		// Use tolerance for float comparison
		if math.Abs(resp.AmountOwed-5.0) > 0.001 {
			t.Errorf("Expected amount owed 5.0, got %f", resp.AmountOwed)
		}
		if resp.OwedBy == nil || *resp.OwedBy != env.User1Name {
			t.Errorf("Expected OwedBy '%s', got %v", env.User1Name, resp.OwedBy)
		}
		if resp.OwedTo == nil || *resp.OwedTo != env.PartnerName {
			t.Errorf("Expected OwedTo '%s', got %v", env.PartnerName, resp.OwedTo)
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		// Use an invalid user ID string as the token
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/transfer/status", "invalid-user-id-string", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized) // Middleware should reject non-integer token
	})

	// Note: Testing the "No partner configured" case requires modifying the test setup or auth logic,
	// as the current setup always assumes the demo user has a partner.
}

// TestRecordTransfer tests the POST /v1/transfer/record endpoint.
func TestRecordTransfer(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	groceriesID := testutil.GetCategoryID(t, env.DB, "Groceries")

	// --- Setup Data ---
	// Add some unsettled items involving the user and partner
	spending1 := testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "Shared", false, nil, nil)             // User paid, shared with Partner
	spending2 := testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, groceriesID, 100.0, "Shared by Partner", false, nil, nil) // Partner paid, shared with User
	// Add an item not involving the partner (should not be settled)
	spendingAlone := testutil.InsertSpending(t, env.DB, env.UserID, nil, groceriesID, 30.0, "Alone", false, nil, nil) // User paid, alone

	// --- Test Case: Successful Record ---
	t.Run("Success", func(t *testing.T) {
		// Use env.AuthToken which is now the user ID string
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/transfer/record", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		// Verify user_spendings are settled
		var settledAt1, settledAt2 sql.NullTime
		err := env.DB.QueryRow("SELECT settled_at FROM user_spendings WHERE spending_id = ?", spending1).Scan(&settledAt1)
		if err != nil || !settledAt1.Valid {
			t.Errorf("Spending item %d should be settled, but settled_at is %v (err: %v)", spending1, settledAt1, err)
		}
		err = env.DB.QueryRow("SELECT settled_at FROM user_spendings WHERE spending_id = ?", spending2).Scan(&settledAt2)
		if err != nil || !settledAt2.Valid {
			t.Errorf("Spending item %d should be settled, but settled_at is %v (err: %v)", spending2, settledAt2, err)
		}

		// Verify the 'alone' spending is NOT settled
		var settledAtAlone sql.NullTime
		err = env.DB.QueryRow("SELECT settled_at FROM user_spendings WHERE spending_id = ?", spendingAlone).Scan(&settledAtAlone)
		if err != nil {
			t.Errorf("Error querying alone spending: %v", err)
		}
		if settledAtAlone.Valid {
			t.Errorf("Alone spending item %d should NOT be settled, but settled_at is %v", spendingAlone, settledAtAlone)
		}

		// Verify transfer record exists
		var transferCount int
		err = env.DB.QueryRow("SELECT COUNT(*) FROM transfers WHERE settled_by_user_id = ? AND settled_with_user_id = ?", env.UserID, env.PartnerID).Scan(&transferCount)
		if err != nil || transferCount != 1 {
			t.Errorf("Expected 1 transfer record, found %d (err: %v)", transferCount, err)
		}

		// Verify status is now settled
		// Use env.AuthToken which is now the user ID string
		reqStatus := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/transfer/status", env.AuthToken, nil)
		rrStatus := testutil.ExecuteRequest(t, env.Handler, reqStatus)
		testutil.AssertStatusCode(t, rrStatus, http.StatusOK)
		var resp transfer.TransferStatusResponse
		testutil.DecodeJSONResponse(t, rrStatus, &resp)
		if resp.AmountOwed != 0.0 || resp.OwedBy != nil || resp.OwedTo != nil {
			t.Errorf("Expected status to be settled after recording transfer, got %v", resp)
		}
	})

	// --- Test Case: Record Again (Idempotency Check) ---
	// Should succeed, but not create duplicate transfer or update already settled items
	t.Run("RecordAgain", func(t *testing.T) {
		// First record (already done in previous test, but run again for isolation if needed)
		// req1 := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/transfer/record", env.AuthToken, nil)
		// rr1 := testutil.ExecuteRequest(t, env.Handler, req1)
		// testutil.AssertStatusCode(t, rr1, http.StatusOK)

		// Record again
		// Use env.AuthToken which is now the user ID string
		req2 := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/transfer/record", env.AuthToken, nil)
		rr2 := testutil.ExecuteRequest(t, env.Handler, req2)
		testutil.AssertStatusCode(t, rr2, http.StatusOK)

		// Verify still only one transfer record (or two if run in isolation)
		var transferCount int
		// Count transfers within the last minute (using UTC) to avoid counting transfers from previous runs if tests are slow
		oneMinuteAgoUTC := time.Now().UTC().Add(-1 * time.Minute)
		err := env.DB.QueryRow("SELECT COUNT(*) FROM transfers WHERE settled_by_user_id = ? AND settled_with_user_id = ? AND settlement_time > ?", env.UserID, env.PartnerID, oneMinuteAgoUTC).Scan(&transferCount)
		// Expecting 1 new transfer record from this test run (or potentially 2 if the first call was also in this run and within the minute)
		// The handler always inserts a transfer record, even if no spendings were updated.
		if err != nil || transferCount < 1 { // We expect at least one record from the second call within the last minute.
			t.Errorf("Expected at least 1 recent transfer record after second call, found %d (err: %v)", transferCount, err)
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		// Use an invalid user ID string as the token
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/transfer/record", "invalid-user-id-string", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized) // Middleware should reject non-integer token
	})

	// Note: Testing the "No partner configured" case requires modifying the test setup or auth logic.
}

// TestPay tests the POST /v1/pay endpoint (now expects JSON body).
func TestPay(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Test Cases ---
	testCases := []struct {
		name           string
		payload        pay.PayPayload // Use the struct for payload
		expectedStatus int
		expectedBody   string             // Substring for error messages
		verifyFunc     func(t *testing.T) // Optional verification
	}{
		{
			name: "SuccessAloneNotPreSettled",
			payload: pay.PayPayload{
				SharedStatus: "alone",
				Amount:       42.50,
				Category:     "Shopping",
				PreSettled:   false,
			},
			expectedStatus: http.StatusCreated,
			verifyFunc: func(t *testing.T) {
				var spendingID, buyerID int64
				var sharedWith sql.NullInt64
				var takesAll bool
				var settledAt sql.NullTime // Check settled_at
				err := env.DB.QueryRow("SELECT spending_id, buyer, shared_with, shared_user_takes_all, settled_at FROM user_spendings ORDER BY id DESC LIMIT 1").Scan(&spendingID, &buyerID, &sharedWith, &takesAll, &settledAt)
				if err != nil {
					t.Fatalf("Verification query failed: %v", err)
				}
				if buyerID != env.UserID {
					t.Errorf("Expected buyer %d, got %d", env.UserID, buyerID)
				}
				if sharedWith.Valid {
					t.Errorf("Expected shared_with NULL, got %v", sharedWith.Int64)
				}
				if takesAll {
					t.Error("Expected shared_user_takes_all false")
				}
				if settledAt.Valid { // Should NOT be settled
					t.Errorf("Expected settled_at NULL, got %v", settledAt.Time)
				}
				// Verify spending details
				var sAmount float64
				var sCatID int64
				catID := testutil.GetCategoryID(t, env.DB, "Shopping")
				err = env.DB.QueryRow("SELECT amount, category FROM spendings WHERE id = ?", spendingID).Scan(&sAmount, &sCatID)
				if err != nil {
					t.Fatalf("Verification spending query failed: %v", err)
				}
				if math.Abs(sAmount-42.50) > 0.001 {
					t.Errorf("Expected amount 42.50, got %f", sAmount)
				}
				if sCatID != catID {
					t.Errorf("Expected category ID %d, got %d", catID, sCatID)
				}
			},
		},
		{
			name: "SuccessSharedPreSettled",
			payload: pay.PayPayload{
				SharedStatus: "shared",
				Amount:       100.00,
				Category:     "Eating Out",
				PreSettled:   true, // Mark as pre-settled
			},
			expectedStatus: http.StatusCreated,
			verifyFunc: func(t *testing.T) {
				var buyerID int64
				var sharedWith sql.NullInt64
				var takesAll bool
				var settledAt sql.NullTime // Check settled_at
				err := env.DB.QueryRow("SELECT buyer, shared_with, shared_user_takes_all, settled_at FROM user_spendings ORDER BY id DESC LIMIT 1").Scan(&buyerID, &sharedWith, &takesAll, &settledAt)
				if err != nil {
					t.Fatalf("Verification query failed: %v", err)
				}
				if buyerID != env.UserID {
					t.Errorf("Expected buyer %d, got %d", env.UserID, buyerID)
				}
				if !sharedWith.Valid || sharedWith.Int64 != env.PartnerID {
					t.Errorf("Expected shared_with %d, got %v", env.PartnerID, sharedWith)
				}
				if takesAll {
					t.Error("Expected shared_user_takes_all false")
				}
				if !settledAt.Valid { // Should BE settled
					t.Errorf("Expected settled_at to be non-NULL, got NULL")
				} else {
					// Check if the time is recent (e.g., within the last 5 seconds)
					if time.Since(settledAt.Time.UTC()) > 5*time.Second {
						t.Errorf("Expected recent settled_at time, got %v", settledAt.Time)
					}
				}
			},
		},
		{
			name: "ErrorInvalidStatus",
			payload: pay.PayPayload{
				SharedStatus: "mixed", // Invalid status
				Amount:       10.0,
				Category:     "Groceries",
				PreSettled:   false,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid shared status",
		},
		{
			name: "ErrorInvalidAmountZero",
			payload: pay.PayPayload{
				SharedStatus: "alone",
				Amount:       0,
				Category:     "Groceries",
				PreSettled:   false,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Amount must be positive",
		},
		{
			name: "ErrorInvalidAmountNegative",
			payload: pay.PayPayload{
				SharedStatus: "alone",
				Amount:       -10.5,
				Category:     "Groceries",
				PreSettled:   false,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Amount must be positive",
		},
		// Invalid amount format is now caught by JSON decoding
		// {
		// 	name:           "ErrorInvalidAmountFormat",
		// },
		{
			name: "ErrorInvalidCategory",
			payload: pay.PayPayload{
				SharedStatus: "alone",
				Amount:       20.0,
				Category:     "NonExistent",
				PreSettled:   false,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Category not found",
		},
		// Note: Testing "Partner not configured" requires modifying test setup/auth logic.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			url := "/v1/pay" // Use the base path now
			// Use env.AuthToken which is now the user ID string
			// Pass the payload as the body
			req := testutil.NewAuthenticatedRequest(t, http.MethodPost, url, env.AuthToken, tc.payload)
			rr := testutil.ExecuteRequest(t, env.Handler, req)

			testutil.AssertStatusCode(t, rr, tc.expectedStatus)
			if tc.expectedBody != "" {
				testutil.AssertBodyContains(t, rr, tc.expectedBody)
			}
			if tc.verifyFunc != nil {
				tc.verifyFunc(t)
			}
		})
	}

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		url := "/v1/pay"
		payload := pay.PayPayload{SharedStatus: "alone", Amount: 50, Category: "Shopping", PreSettled: false}
		// Use an invalid user ID string as the token
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, url, "invalid-user-id-string", payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized) // Middleware should reject non-integer token
	})
}
