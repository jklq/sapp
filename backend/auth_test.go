package main_test

import (
	"database/sql"
	"net/http"
	"os"
	"testing"

	"git.sr.ht/~relay/sapp-backend/auth"     // Import auth for handlers and JWT
	"git.sr.ht/~relay/sapp-backend/testutil" // Import the new test utility package
	"git.sr.ht/~relay/sapp-backend/types"    // Import shared types
	"github.com/golang-jwt/jwt/v5"           // Import JWT for token validation
	"golang.org/x/crypto/bcrypt"             // Import bcrypt for password verification in registration test
)

// TestLogin tests the /v1/login endpoint.
func TestLogin(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB() // Ensure DB connection is closed

	// --- Test Case: Successful Login ---
	t.Run("Success", func(t *testing.T) {
		loginPayload := types.LoginRequest{ // Use types.LoginRequest
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

		var respBody types.LoginResponse // Use types.LoginResponse
		testutil.DecodeJSONResponse(t, rr, &respBody)

		// Validate the JWT token
		claims := &auth.Claims{}
		_, err := jwt.ParseWithClaims(respBody.Token, claims, func(token *jwt.Token) (interface{}, error) {
			// Use the same secret retrieval logic as in auth middleware (or a test helper)
			secret := []byte(os.Getenv("JWT_SECRET_KEY"))
			if len(secret) == 0 {
				secret = []byte("a-secure-secret-key-for-dev-only-replace-in-prod") // Default from auth.go
			}
			return secret, nil
		})
		if err != nil {
			t.Fatalf("Failed to parse or validate JWT token: %v", err)
		}
		if claims.UserID != env.UserID {
			t.Errorf("JWT claims contain unexpected user ID: got %v want %v", claims.UserID, env.UserID)
		}
		// Check other response fields
		if respBody.UserID != env.UserID {
			t.Errorf("handler returned unexpected user ID in response body: got %v want %v", respBody.UserID, env.UserID)
		}
		if respBody.FirstName != env.User1Name { // Check first name from test env setup
			t.Errorf("handler returned unexpected first name: got %v want %v", respBody.FirstName, env.User1Name)
		}
	})

	// --- Test Case: Incorrect Password ---
	t.Run("IncorrectPassword", func(t *testing.T) {
		loginPayload := types.LoginRequest{ // Use types.LoginRequest
			Username: "demo_user", // Use correct username from env (demo_user is seeded)
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

	// --- Test Case: User Not Found ---
	t.Run("UserNotFound", func(t *testing.T) {
		loginPayload := types.LoginRequest{ // Use types.LoginRequest
			Username: "nonexistent_user",
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

		// Also test with the partner user
		loginPayloadPartner := types.LoginRequest{ // Use types.LoginRequest
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
		// Partner user should be able to log in successfully
		testutil.AssertStatusCode(t, rrPartner, http.StatusOK)
		// Decode and check the response body for the partner user
		var respBodyPartner types.LoginResponse // Use types.LoginResponse
		testutil.DecodeJSONResponse(t, rrPartner, &respBodyPartner)

		// Validate the JWT token for the partner
		claimsPartner := &auth.Claims{}
		_, err := jwt.ParseWithClaims(respBodyPartner.Token, claimsPartner, func(token *jwt.Token) (interface{}, error) {
			secret := []byte(os.Getenv("JWT_SECRET_KEY"))
			if len(secret) == 0 {
				secret = []byte("a-secure-secret-key-for-dev-only-replace-in-prod")
			}
			return secret, nil
		})
		if err != nil {
			t.Fatalf("Failed to parse or validate partner JWT token: %v", err)
		}
		if claimsPartner.UserID != env.PartnerID {
			t.Errorf("Partner JWT claims contain unexpected user ID: got %v want %v", claimsPartner.UserID, env.PartnerID)
		}
		// Check other response fields
		if respBodyPartner.UserID != env.PartnerID {
			t.Errorf("Partner login returned unexpected user ID in response body: got %v want %v", respBodyPartner.UserID, env.PartnerID)
		}
		if respBodyPartner.FirstName != env.PartnerName {
			t.Errorf("Partner login returned unexpected first name: got %v want %v", respBodyPartner.FirstName, env.PartnerName)
		}
	})

	// --- Test Case: Missing Username ---
	t.Run("MissingUsername", func(t *testing.T) {
		loginPayload := types.LoginRequest{ // Use types.LoginRequest
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
		payload := types.PartnerRegistrationRequest{ // Use types.PartnerRegistrationRequest
			User1: types.UserRegistrationDetails{Username: "alice", Password: "password123", FirstName: "Alice"}, // Use types.UserRegistrationDetails
			User2: types.UserRegistrationDetails{Username: "bob", Password: "password456", FirstName: "Bob"},     // Use types.UserRegistrationDetails
		}

		// Make the registration request
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/register/partners", "", payload) // No auth needed
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		// Assert success status and decode response
		testutil.AssertStatusCode(t, rr, http.StatusCreated)
		var respBody types.PartnerRegistrationResponse // Use types.PartnerRegistrationResponse
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
			payload      types.PartnerRegistrationRequest // Use types.PartnerRegistrationRequest
			expectedMsg  string
			expectedCode int
		}{
			{"MissingUser1Username", types.PartnerRegistrationRequest{User1: types.UserRegistrationDetails{Password: "p", FirstName: "F"}, User2: types.UserRegistrationDetails{Username: "u", Password: "p", FirstName: "F"}}, "All fields", http.StatusBadRequest},
			{"MissingUser2Password", types.PartnerRegistrationRequest{User1: types.UserRegistrationDetails{Username: "u", Password: "p", FirstName: "F"}, User2: types.UserRegistrationDetails{Username: "u2", FirstName: "F"}}, "All fields", http.StatusBadRequest},
			{"SameUsernames", types.PartnerRegistrationRequest{User1: types.UserRegistrationDetails{Username: "same", Password: "p", FirstName: "F"}, User2: types.UserRegistrationDetails{Username: "same", Password: "p2", FirstName: "F2"}}, "Usernames must be different", http.StatusBadRequest},
			{"ShortPassword", types.PartnerRegistrationRequest{User1: types.UserRegistrationDetails{Username: "u1", Password: "short", FirstName: "F1"}, User2: types.UserRegistrationDetails{Username: "u2", Password: "password", FirstName: "F2"}}, "Password must be at least 6 characters", http.StatusBadRequest},
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
		payload := types.PartnerRegistrationRequest{ // Use types.PartnerRegistrationRequest
			User1: types.UserRegistrationDetails{Username: conflictUsername, Password: "password123", FirstName: "Conflict"}, // Use existing username
			User2: types.UserRegistrationDetails{Username: "new_partner", Password: "password456", FirstName: "New"},
		}
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/register/partners", "", payload) // No auth needed
		rr := testutil.ExecuteRequest(t, env.Handler, req)

		testutil.AssertStatusCode(t, rr, http.StatusConflict)
		testutil.AssertBodyContains(t, rr, "already exist")
	})
}
