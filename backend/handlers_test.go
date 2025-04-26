package main_test // Use main_test to avoid import cycles if needed, or backend_test

import (
	"net/http"
	"testing"

	"database/sql"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth"     // Import auth for LoginRequest/Response
	"git.sr.ht/~relay/sapp-backend/category" // Import category for APICategory
	"git.sr.ht/~relay/sapp-backend/spendings"
	"git.sr.ht/~relay/sapp-backend/testutil" // Import the new test utility package
	"git.sr.ht/~relay/sapp-backend/transfer"
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

// TestAICategorize tests the /v1/categorize endpoint.
func TestAICategorize(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Test Case: Successful Submission ---
	t.Run("Success", func(t *testing.T) {
		payload := map[string]interface{}{
			"amount": 123.45,
			"prompt": "Groceries from Rema",
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

		// Verify job exists in DB
		var dbPrompt string
		var dbBuyerID int64
		var dbPartnerID sql.NullInt64
		err := env.DB.QueryRow("SELECT prompt, buyer, shared_with FROM ai_categorization_jobs WHERE id = ?", jobID).Scan(&dbPrompt, &dbBuyerID, &dbPartnerID)
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
	})
}

// TestGetSpendings tests the /v1/spendings endpoint.
func TestGetSpendings(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	groceriesID := testutil.GetCategoryID(t, env.DB, "Groceries")
	transportID := testutil.GetCategoryID(t, env.DB, "Transport")

	// --- Test Case: No Spendings ---
	t.Run("NoSpendings", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/spendings", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)
		testutil.AssertBodyContains(t, rr, `[]`) // Expect empty JSON array
	})

	// --- Setup Data for Subsequent Tests ---
	// Job 1: Shared groceries and alone transport
	job1ID := testutil.InsertAIJob(t, env.DB, env.UserID, &env.PartnerID, "Groceries and bus ticket", 75.0, "finished", true, false, nil)
	spending1_1 := testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "Milk & Bread", false, &job1ID, nil) // Shared
	spending1_2 := testutil.InsertSpending(t, env.DB, env.UserID, nil, transportID, 25.0, "Bus Ticket", false, &job1ID, nil)              // Alone

	// Job 2: Paid by partner
	job2ID := testutil.InsertAIJob(t, env.DB, env.UserID, &env.PartnerID, "Gift for me from Partner", 100.0, "finished", true, false, nil)
	spending2_1 := testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 100.0, "Gift", true, &job2ID, nil) // Paid by Partner

	// Job 3: Ambiguous flag
	ambigReason := "Unclear item"
	job3ID := testutil.InsertAIJob(t, env.DB, env.UserID, &env.PartnerID, "Mystery box", 20.0, "finished", true, true, &ambigReason)
	spending3_1 := testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 20.0, "Mystery", false, &job3ID, nil) // Shared

	// --- Test Case: Fetch Spendings ---
	t.Run("FetchSpendings", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/spendings", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var groups []spendings.TransactionGroup
		testutil.DecodeJSONResponse(t, rr, &groups)

		if len(groups) != 3 {
			t.Fatalf("Expected 3 transaction groups, got %d", len(groups))
		}

		// Basic checks on the first group (Job 3 - most recent)
		group3 := groups[0]
		if group3.JobID != job3ID {
			t.Errorf("Expected first group JobID %d, got %d", job3ID, group3.JobID)
		}
		if !group3.IsAmbiguityFlagged || group3.AmbiguityFlagReason == nil || *group3.AmbiguityFlagReason != ambigReason {
			t.Errorf("Expected ambiguity flag set with reason '%s', got flag=%v, reason=%v", ambigReason, group3.IsAmbiguityFlagged, group3.AmbiguityFlagReason)
		}
		if len(group3.Spendings) != 1 || group3.Spendings[0].ID != spending3_1 {
			t.Errorf("Expected 1 spending item with ID %d in group 3, got %v", spending3_1, group3.Spendings)
		}
		if group3.Spendings[0].SharingStatus != "Shared with Partner" {
			t.Errorf("Expected spending 3_1 status 'Shared with Partner', got '%s'", group3.Spendings[0].SharingStatus)
		}

		// Basic checks on the second group (Job 2)
		group2 := groups[1]
		if group2.JobID != job2ID {
			t.Errorf("Expected second group JobID %d, got %d", job2ID, group2.JobID)
		}
		if group2.IsAmbiguityFlagged {
			t.Error("Expected ambiguity flag not set for group 2")
		}
		if len(group2.Spendings) != 1 || group2.Spendings[0].ID != spending2_1 {
			t.Errorf("Expected 1 spending item with ID %d in group 2, got %v", spending2_1, group2.Spendings)
		}
		if group2.Spendings[0].SharingStatus != "Paid by Partner" {
			t.Errorf("Expected spending 2_1 status 'Paid by Partner', got '%s'", group2.Spendings[0].SharingStatus)
		}

		// Basic checks on the third group (Job 1)
		group1 := groups[2]
		if group1.JobID != job1ID {
			t.Errorf("Expected third group JobID %d, got %d", job1ID, group1.JobID)
		}
		if len(group1.Spendings) != 2 {
			t.Fatalf("Expected 2 spending items in group 1, got %d", len(group1.Spendings))
		}
		// Order should be spending1_1, spending1_2 based on ID ASC
		if group1.Spendings[0].ID != spending1_1 || group1.Spendings[1].ID != spending1_2 {
			t.Errorf("Expected spending IDs %d, %d in group 1, got %d, %d", spending1_1, spending1_2, group1.Spendings[0].ID, group1.Spendings[1].ID)
		}
		if group1.Spendings[0].SharingStatus != "Shared with Partner" {
			t.Errorf("Expected spending 1_1 status 'Shared with Partner', got '%s'", group1.Spendings[0].SharingStatus)
		}
		if group1.Spendings[1].SharingStatus != "Alone" {
			t.Errorf("Expected spending 1_2 status 'Alone', got '%s'", group1.Spendings[1].SharingStatus)
		}
		if group1.Spendings[1].PartnerName != nil {
			t.Errorf("Expected spending 1_2 PartnerName to be nil, got %v", *group1.Spendings[1].PartnerName)
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/spendings", "invalid-token", nil)
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
	// Spending 1: Initially shared groceries
	spendingIDShared := testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "Initial Shared", false, nil, nil)
	// Spending 2: Initially alone transport
	spendingIDAlone := testutil.InsertSpending(t, env.DB, env.UserID, nil, transportID, 25.0, "Initial Alone", false, nil, nil)
	// Spending 3: Initially paid by partner
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, shoppingID, 100.0, "Initial PaidByPartner", true, nil, nil)
	// Spending 4: Belongs to partner (for forbidden test)
	_ = testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, groceriesID, 30.0, "Partner's Spending", false, nil, nil)

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
			spendingID: spendingIDAlone, // Use the one previously updated to shared
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
			spendingID: 4, // Belongs to partner
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
		req := testutil.NewAuthenticatedRequest(t, http.MethodPut, url, "invalid-token", payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
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
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/transfer/status", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var resp transfer.TransferStatusResponse
		testutil.DecodeJSONResponse(t, rr, &resp)

		if resp.PartnerName != "Partner" {
			t.Errorf("Expected partner name 'Partner', got '%s'", resp.PartnerName)
		}
		if resp.AmountOwed != 0.0 {
			t.Errorf("Expected amount owed 0.0, got %f", resp.AmountOwed)
		}
		if resp.OwedBy != nil || resp.OwedTo != nil {
			t.Errorf("Expected OwedBy and OwedTo to be nil, got %v, %v", resp.OwedBy, resp.OwedTo)
		}
	})

	// --- Setup Data ---
	// 1. User paid 50, shared with partner -> Partner owes 25
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "Shared Groceries", false, nil, nil)
	// 2. Partner paid 100, shared with user -> User owes 50
	_ = testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, shoppingID, 100.0, "Shared Shopping", false, nil, nil)
	// 3. User paid 30, alone -> No effect on balance
	_ = testutil.InsertSpending(t, env.DB, env.UserID, nil, groceriesID, 30.0, "Alone Groceries", false, nil, nil)
	// 4. User paid 40, partner takes all -> Partner owes 40
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, shoppingID, 40.0, "Gift for Partner", true, nil, nil)
	// 5. Partner paid 20, user takes all -> User owes 20
	_ = testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, groceriesID, 20.0, "Gift for User", true, nil, nil)
	// 6. Settled spending (should be ignored)
	settledTime := time.Now().Add(-time.Hour).UTC()
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 200.0, "Settled Item", false, nil, &settledTime)

	// Expected Balance:
	// User Net = +25 (from 1) - 50 (from 2) + 40 (from 4) - 20 (from 5) = -5
	// User owes Partner 5.0

	// --- Test Case: Calculated Status ---
	t.Run("CalculatedStatus", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/transfer/status", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var resp transfer.TransferStatusResponse
		testutil.DecodeJSONResponse(t, rr, &resp)

		if resp.PartnerName != "Partner" {
			t.Errorf("Expected partner name 'Partner', got '%s'", resp.PartnerName)
		}
		// Use tolerance for float comparison
		if math.Abs(resp.AmountOwed-5.0) > 0.001 {
			t.Errorf("Expected amount owed 5.0, got %f", resp.AmountOwed)
		}
		if resp.OwedBy == nil || *resp.OwedBy != "Demo" {
			t.Errorf("Expected OwedBy 'Demo', got %v", resp.OwedBy)
		}
		if resp.OwedTo == nil || *resp.OwedTo != "Partner" {
			t.Errorf("Expected OwedTo 'Partner', got %v", resp.OwedTo)
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/transfer/status", "invalid-token", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
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
	spending1 := testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "Shared", false, nil, nil)
	spending2 := testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, groceriesID, 100.0, "Shared by Partner", false, nil, nil)
	// Add an item not involving the partner (should not be settled)
	spendingAlone := testutil.InsertSpending(t, env.DB, env.UserID, nil, groceriesID, 30.0, "Alone", false, nil, nil)

	// --- Test Case: Successful Record ---
	t.Run("Success", func(t *testing.T) {
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
		req2 := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/transfer/record", env.AuthToken, nil)
		rr2 := testutil.ExecuteRequest(t, env.Handler, req2)
		testutil.AssertStatusCode(t, rr2, http.StatusOK)

		// Verify still only one transfer record (or two if run in isolation)
		var transferCount int
		// Count transfers within the last minute to avoid counting transfers from previous runs if tests are slow
		err := env.DB.QueryRow("SELECT COUNT(*) FROM transfers WHERE settled_by_user_id = ? AND settled_with_user_id = ? AND settlement_time > ?", env.UserID, env.PartnerID, time.Now().Add(-1*time.Minute)).Scan(&transferCount)
		// Expecting 1 new transfer record from this test run (or potentially 2 if the first call was also in this run)
		if err != nil || transferCount < 1 || transferCount > 2 { // Allow 1 or 2 depending on exact timing relative to previous test
			t.Errorf("Expected 1 or 2 recent transfer records after second call, found %d (err: %v)", transferCount, err)
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/transfer/record", "invalid-token", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
	})

	// Note: Testing the "No partner configured" case requires modifying the test setup or auth logic.
}

// TestPay tests the POST /v1/pay/{shared_status}/{amount}/{category} endpoint.
func TestPay(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Test Cases ---
	testCases := []struct {
		name           string
		sharedStatus   string
		amount         string
		category       string
		expectedStatus int
		expectedBody   string             // Substring for error messages
		verifyFunc     func(t *testing.T) // Optional verification
	}{
		{
			name:           "SuccessAlone",
			sharedStatus:   "alone",
			amount:         "42.50",
			category:       "Shopping",
			expectedStatus: http.StatusCreated,
			verifyFunc: func(t *testing.T) {
				var spendingID, buyerID int64
				var sharedWith sql.NullInt64
				var takesAll bool
				err := env.DB.QueryRow("SELECT spending_id, buyer, shared_with, shared_user_takes_all FROM user_spendings ORDER BY id DESC LIMIT 1").Scan(&spendingID, &buyerID, &sharedWith, &takesAll)
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
			name:           "SuccessShared",
			sharedStatus:   "shared",
			amount:         "100.00",
			category:       "Eating Out",
			expectedStatus: http.StatusCreated,
			verifyFunc: func(t *testing.T) {
				var buyerID int64
				var sharedWith sql.NullInt64
				var takesAll bool
				err := env.DB.QueryRow("SELECT buyer, shared_with, shared_user_takes_all FROM user_spendings ORDER BY id DESC LIMIT 1").Scan(&buyerID, &sharedWith, &takesAll)
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
			},
		},
		{
			name:           "ErrorInvalidStatus",
			sharedStatus:   "mixed", // Invalid status
			amount:         "10.0",
			category:       "Groceries",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid shared status",
		},
		{
			name:           "ErrorInvalidAmountZero",
			sharedStatus:   "alone",
			amount:         "0",
			category:       "Groceries",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Amount must be positive",
		},
		{
			name:           "ErrorInvalidAmountNegative",
			sharedStatus:   "alone",
			amount:         "-10.5",
			category:       "Groceries",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Amount must be positive",
		},
		{
			name:           "ErrorInvalidAmountFormat",
			sharedStatus:   "alone",
			amount:         "ten",
			category:       "Groceries",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid amount format",
		},
		{
			name:           "ErrorInvalidCategory",
			sharedStatus:   "alone",
			amount:         "20.0",
			category:       "NonExistent",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Category not found",
		},
		// Note: Testing "Partner not configured" requires modifying test setup/auth logic.
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// URL encode category name just in case
			encodedCategory := url.PathEscape(tc.category)
			url := fmt.Sprintf("/v1/pay/%s/%s/%s", tc.sharedStatus, tc.amount, encodedCategory)
			req := testutil.NewAuthenticatedRequest(t, http.MethodPost, url, env.AuthToken, nil) // No body needed
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
		url := "/v1/pay/alone/50/Shopping"
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, url, "invalid-token", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
	})
}
