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

		var resp types.TransferStatusResponse // Use types.TransferStatusResponse
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

		var resp types.TransferStatusResponse // Use types.TransferStatusResponse
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
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/transfer/status", "invalid-token", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token")
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
		var resp types.TransferStatusResponse // Use types.TransferStatusResponse
		testutil.DecodeJSONResponse(t, rrStatus, &resp)
		if resp.AmountOwed != 0.0 || resp.OwedBy != nil || resp.OwedTo != nil {
			t.Errorf("Expected status to be settled after recording transfer, got %v", resp)
		}
	})

	// --- Test Case: Record Again (Idempotency Check) ---
	// Should succeed, create a new transfer record, but not update already settled items
	t.Run("RecordAgain", func(t *testing.T) {
		// Get initial transfer count
		var initialTransferCount int
		err := env.DB.QueryRow("SELECT COUNT(*) FROM transfers WHERE settled_by_user_id = ? AND settled_with_user_id = ?", env.UserID, env.PartnerID).Scan(&initialTransferCount)
		if err != nil {
			t.Fatalf("Failed to get initial transfer count: %v", err)
		}
		// Ensure the first call happened (from the Success test case)
		if initialTransferCount == 0 {
			t.Log("Warning: Initial transfer count was 0, running first record call.")
			req1 := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/transfer/record", env.AuthToken, nil)
			rr1 := testutil.ExecuteRequest(t, env.Handler, req1)
			testutil.AssertStatusCode(t, rr1, http.StatusOK)
			// Re-query count
			err = env.DB.QueryRow("SELECT COUNT(*) FROM transfers WHERE settled_by_user_id = ? AND settled_with_user_id = ?", env.UserID, env.PartnerID).Scan(&initialTransferCount)
			if err != nil || initialTransferCount == 0 {
				t.Fatalf("Failed to get transfer count after first call: %v (count: %d)", err, initialTransferCount)
			}
		}


		// Record again
		req2 := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/transfer/record", env.AuthToken, nil)
		rr2 := testutil.ExecuteRequest(t, env.Handler, req2)
		testutil.AssertStatusCode(t, rr2, http.StatusOK)

		// Verify transfer count increased by exactly one
		var finalTransferCount int
		err = env.DB.QueryRow("SELECT COUNT(*) FROM transfers WHERE settled_by_user_id = ? AND settled_with_user_id = ?", env.UserID, env.PartnerID).Scan(&finalTransferCount)
		if err != nil {
			t.Fatalf("Failed to get final transfer count: %v", err)
		}
		if finalTransferCount != initialTransferCount+1 {
			t.Errorf("Expected transfer count to increase by 1 (from %d to %d), but got %d", initialTransferCount, initialTransferCount+1, finalTransferCount)
		}

		// Optional: Verify that the settled_at timestamps of the spendings did not change after the second call
		// (Requires storing the initial settled_at times from the 'Success' test)
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/transfer/record", "invalid-token", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token")
	})

	// Note: Testing the "No partner configured" case requires modifying the test setup or auth logic.
}
