package main_test // Use main_test to avoid import cycles if needed, or backend_test

import (
	"net/http"
	"testing"

	"git.sr.ht/~relay/sapp-backend/auth" // Import auth for LoginRequest/Response
	"git.sr.ht/~relay/sapp-backend/category" // Import category for APICategory
	"git.sr.ht/~relay/sapp-backend/testutil" // Import the new test utility package
)

// TestLogin tests the /v1/login endpoint.
func TestLogin(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB() // Ensure DB connection is closed

	// --- Test Case: Successful Login ---
	t.Run("Success", func(t *testing.T) {
		loginPayload := auth.LoginRequest{
			Username: "demo_user",
			Password: "password", // Use the correct password seeded in schema.sql
		}
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/login", "", loginPayload) // No token needed for login
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var respBody auth.LoginResponse
		testutil.DecodeJSONResponse(t, rr, &respBody)

		if respBody.Token != env.AuthToken {
			t.Errorf("handler returned unexpected token: got %v want %v", respBody.Token, env.AuthToken)
		}
		if respBody.UserID != env.UserID {
			t.Errorf("handler returned unexpected user ID: got %v want %v", respBody.UserID, env.UserID)
		}
		if respBody.FirstName != "Demo" { // Check first name seeded in schema
			t.Errorf("handler returned unexpected first name: got %v want %v", respBody.FirstName, "Demo")
		}
	})

	// --- Test Case: Incorrect Password ---
	t.Run("IncorrectPassword", func(t *testing.T) {
		loginPayload := auth.LoginRequest{
			Username: "demo_user",
			Password: "wrongpassword",
		}
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/login", "", loginPayload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid credentials")
	})

	// --- Test Case: User Not Found ---
	t.Run("UserNotFound", func(t *testing.T) {
		loginPayload := auth.LoginRequest{
			Username: "nonexistent_user",
			Password: "password",
		}
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/login", "", loginPayload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized) // Expect Unauthorized as user doesn't match demo user check
		testutil.AssertBodyContains(t, rr, "Invalid credentials")
	})

	// --- Test Case: Missing Username ---
	t.Run("MissingUsername", func(t *testing.T) {
		loginPayload := auth.LoginRequest{
			Username: "",
			Password: "password",
		}
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/login", "", loginPayload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusBadRequest)
		testutil.AssertBodyContains(t, rr, "Username and password are required")
	})
}

// TestGetCategories tests the /v1/categories endpoint.
func TestGetCategories(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Test Case: Success ---
	t.Run("Success", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/categories", env.AuthToken, nil) // Use valid token
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
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/categories", "invalid-token", nil) // Invalid token
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token")
	})
}

// --- Add more test functions below for other endpoints ---
// func TestAICategorize(t *testing.T) { ... }
// func TestGetSpendings(t *testing.T) { ... }
// func TestUpdateSpending(t *testing.T) { ... }
// func TestGetTransferStatus(t *testing.T) { ... }
// func TestRecordTransfer(t *testing.T) { ... }
// func TestPay(t *testing.T) { ... }
