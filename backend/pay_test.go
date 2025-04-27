package main_test

import (
	"database/sql"
	"math"
	"net/http"
	"testing"
	"time"

	"git.sr.ht/~relay/sapp-backend/testutil" // Import the new test utility package
	"git.sr.ht/~relay/sapp-backend/types"    // Import shared types
)

// TestPay tests the POST /v1/pay endpoint.
func TestPay(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// Get category IDs needed for verification
	shoppingCatID := testutil.GetCategoryID(t, env.DB, "Shopping")
	eatingOutCatID := testutil.GetCategoryID(t, env.DB, "Eating Out")
	_ = testutil.GetCategoryID(t, env.DB, "Groceries")

	// --- Test Cases ---
	testCases := []struct {
		name           string
		payload        types.PayPayload // Use types.PayPayload
		expectedStatus int
		expectedBody   string             // Substring for error messages
		verifyFunc     func(t *testing.T) // Optional verification
	}{
		{
			name: "SuccessAloneNotPreSettled",
			payload: types.PayPayload{ // Use types.PayPayload
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
				err = env.DB.QueryRow("SELECT amount, category FROM spendings WHERE id = ?", spendingID).Scan(&sAmount, &sCatID)
				if err != nil {
					t.Fatalf("Verification spending query failed: %v", err)
				}
				if math.Abs(sAmount-42.50) > 0.001 {
					t.Errorf("Expected amount 42.50, got %f", sAmount)
				}
				if sCatID != shoppingCatID {
					t.Errorf("Expected category ID %d (Shopping), got %d", shoppingCatID, sCatID)
				}
			},
		},
		{
			name: "SuccessSharedPreSettled",
			payload: types.PayPayload{ // Use types.PayPayload
				SharedStatus: "shared",
				Amount:       100.00,
				Category:     "Eating Out",
				PreSettled:   true, // Mark as pre-settled
			},
			expectedStatus: http.StatusCreated,
			verifyFunc: func(t *testing.T) {
				var spendingID, buyerID int64 // Need spendingID too
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
				// Verify spending details
				var sAmount float64
				var sCatID int64
				err = env.DB.QueryRow("SELECT amount, category FROM spendings WHERE id = ?", spendingID).Scan(&sAmount, &sCatID)
				if err != nil {
					t.Fatalf("Verification spending query failed: %v", err)
				}
				if math.Abs(sAmount-100.00) > 0.001 {
					t.Errorf("Expected amount 100.00, got %f", sAmount)
				}
				if sCatID != eatingOutCatID {
					t.Errorf("Expected category ID %d (Eating Out), got %d", eatingOutCatID, sCatID)
				}
			},
		},
		{
			name: "ErrorInvalidStatus",
			payload: types.PayPayload{ // Use types.PayPayload
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
			payload: types.PayPayload{ // Use types.PayPayload
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
			payload: types.PayPayload{ // Use types.PayPayload
				SharedStatus: "alone",
				Amount:       -10.5,
				Category:     "Groceries",
				PreSettled:   false,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Amount must be positive",
		},
		{
			name: "ErrorInvalidCategory",
			payload: types.PayPayload{ // Use types.PayPayload
				SharedStatus: "alone",
				Amount:       20.0,
				Category:     "NonExistent",
				PreSettled:   false,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Category not found",
		},
		{
			name: "ErrorMissingCategory", // Test missing category name in payload
			payload: types.PayPayload{ // Use types.PayPayload
				SharedStatus: "alone",
				Amount:       25.0,
				Category:     "", // Empty category name
				PreSettled:   false,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Category not found", // Backend treats empty string as category not found
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
		payload := types.PayPayload{SharedStatus: "alone", Amount: 50, Category: "Shopping", PreSettled: false} // Use types.PayPayload
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, url, "invalid-token", payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token")
	})
}
