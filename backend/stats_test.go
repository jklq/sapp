package main_test

import (
	"math"
	"net/http"
	"testing"
	"time"

	"git.sr.ht/~relay/sapp-backend/testutil"
	"git.sr.ht/~relay/sapp-backend/types"
)

// TestGetLastMonthSpendingStats tests the GET /v1/stats/spending/last-month endpoint.
func TestGetLastMonthSpendingStats(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	groceriesID := testutil.GetCategoryID(t, env.DB, "Groceries")
	transportID := testutil.GetCategoryID(t, env.DB, "Transport")
	shoppingID := testutil.GetCategoryID(t, env.DB, "Shopping")

	now := time.Now().UTC()
	within30Days := now.AddDate(0, 0, -15)
	outside30Days := now.AddDate(0, 0, -45)

	// --- Setup Data ---
	// User Spendings (within 30 days)
	// 1. User paid 50, shared 50/50 -> User cost: 25 (Groceries)
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "Shared Groceries", false, nil, &within30Days)
	// 2. User paid 30, alone -> User cost: 30 (Transport)
	_ = testutil.InsertSpending(t, env.DB, env.UserID, nil, transportID, 30.0, "Alone Transport", false, nil, &within30Days)
	// 3. User paid 40, partner takes all -> User cost: 0 (Shopping) - Should not appear in results
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, shoppingID, 40.0, "Gift for Partner", true, nil, &within30Days)
	// 4. Partner paid 100, shared 50/50 -> User cost: 50 (Groceries)
	_ = testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, groceriesID, 100.0, "Partner Shared Groceries", false, nil, &within30Days)
	// 5. Partner paid 20, user takes all -> User cost: 20 (Transport)
	_ = testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, transportID, 20.0, "Gift for User", true, nil, &within30Days)

	// User Spendings (outside 30 days - should be ignored)
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 200.0, "Old Shared Groceries", false, nil, &outside30Days)
	_ = testutil.InsertSpending(t, env.DB, env.UserID, nil, transportID, 50.0, "Old Alone Transport", false, nil, &outside30Days)

	// Expected Totals (within 30 days):
	// Groceries: 25 (from #1) + 50 (from #4) = 75
	// Transport: 30 (from #2) + 20 (from #5) = 50
	// Shopping: 0 (from #3) - Should not be included

	// --- Test Case: Fetch Stats ---
	t.Run("FetchStats", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/stats/spending/last-month", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var resp []types.CategorySpendingStat // Use types.CategorySpendingStat
		testutil.DecodeJSONResponse(t, rr, &resp)

		if len(resp) != 2 {
			t.Fatalf("Expected 2 categories with spending, got %d", len(resp))
		}

		// Check order (descending by total amount)
		if resp[0].CategoryName != "Groceries" {
			t.Errorf("Expected first category to be 'Groceries', got '%s'", resp[0].CategoryName)
		}
		if resp[1].CategoryName != "Transport" {
			t.Errorf("Expected second category to be 'Transport', got '%s'", resp[1].CategoryName)
		}

		// Check amounts (use tolerance)
		if math.Abs(resp[0].TotalAmount-75.0) > 0.001 {
			t.Errorf("Expected Groceries total 75.0, got %f", resp[0].TotalAmount)
		}
		if math.Abs(resp[1].TotalAmount-50.0) > 0.001 {
			t.Errorf("Expected Transport total 50.0, got %f", resp[1].TotalAmount)
		}
	})

	// --- Test Case: No Recent Spendings ---
	t.Run("NoRecentSpendings", func(t *testing.T) {
		// Delete recent spendings to test empty case
		thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)
		_, err := env.DB.Exec("DELETE FROM spendings WHERE spending_date >= ?", thirtyDaysAgo.Format(time.RFC3339))
		if err != nil {
			t.Fatalf("Failed to delete recent spendings for test: %v", err)
		}

		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/stats/spending/last-month", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var resp []types.CategorySpendingStat // Use types.CategorySpendingStat
		testutil.DecodeJSONResponse(t, rr, &resp)

		if len(resp) != 0 {
			t.Errorf("Expected 0 categories with recent spending, got %d", len(resp))
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/stats/spending/last-month", "invalid-token", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token")
	})
}
