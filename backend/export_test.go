package main_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"git.sr.ht/~relay/sapp-backend/testutil"
	"git.sr.ht/~relay/sapp-backend/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExportAllData tests the GET /v1/export/all endpoint.
func TestExportAllData(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Setup Additional Test Data ---
	groceriesID := testutil.GetCategoryID(t, env.DB, "Groceries")
	transportID := testutil.GetCategoryID(t, env.DB, "Transport")
	shoppingID := testutil.GetCategoryID(t, env.DB, "Shopping")

	// AI Job 1 (User buys, shared)
	job1Date := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	job1ID := testutil.InsertAIJob(t, env.DB, env.UserID, &env.PartnerID, "Groceries and bus ticket", 75.0, "finished", true, false, nil)
	_, err := env.DB.Exec("UPDATE ai_categorization_jobs SET transaction_date = ?, created_at = ? WHERE id = ?", job1Date, job1Date, job1ID)
	require.NoError(t, err)
	_ = testutil.InsertSpending(t, env.DB, env.UserID, &env.PartnerID, groceriesID, 50.0, "Milk & Bread", false, &job1ID, nil) // Assign to _
	_ = testutil.InsertSpending(t, env.DB, env.UserID, nil, transportID, 25.0, "Bus Ticket", false, &job1ID, nil)             // Assign to _

	// AI Job 2 (Partner buys, user takes all)
	job2Date := time.Date(2024, 5, 5, 11, 0, 0, 0, time.UTC)
	job2ID := testutil.InsertAIJob(t, env.DB, env.PartnerID, &env.UserID, "Gift for User", 100.0, "finished", true, false, nil)
	_, err = env.DB.Exec("UPDATE ai_categorization_jobs SET transaction_date = ?, created_at = ? WHERE id = ?", job2Date, job2Date, job2ID)
	require.NoError(t, err)
	_ = testutil.InsertSpending(t, env.DB, env.PartnerID, &env.UserID, shoppingID, 100.0, "Gift", true, &job2ID, nil) // User takes all, Assign to _

	// Manual Spending (User buys, alone, settled)
	manualDate := time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC)
	settledTime := time.Now().Add(-time.Hour).UTC()
	manualSpendingID := testutil.InsertSpending(t, env.DB, env.UserID, nil, shoppingID, 30.0, "Manual Alone Settled", false, nil, &settledTime)
	_, err = env.DB.Exec("UPDATE spendings SET spending_date = ? WHERE id = ?", manualDate, manualSpendingID)
	require.NoError(t, err)

	// Deposit 1 (User, recurring)
	deposit1Date := time.Date(2024, 4, 15, 0, 0, 0, 0, time.UTC)
	deposit1EndDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
	deposit1ID := testutil.InsertDeposit(t, env.DB, env.UserID, 2000.0, "Salary", deposit1Date, true, testutil.Ptr("monthly")) // Use testutil.Ptr
	_, err = env.DB.Exec("UPDATE deposits SET end_date = ? WHERE id = ?", deposit1EndDate, deposit1ID)
	require.NoError(t, err)

	// Deposit 2 (Partner, one-off)
	deposit2Date := time.Date(2024, 5, 20, 0, 0, 0, 0, time.UTC)
	_ = testutil.InsertDeposit(t, env.DB, env.PartnerID, 50.0, "Partner Bonus", deposit2Date, false, nil)

	// Transfer (User settled with Partner)
	transferTime := time.Now().Add(-2 * time.Hour).UTC()
	_, err = env.DB.Exec("INSERT INTO transfers (settled_by_user_id, settled_with_user_id, settlement_time) VALUES (?, ?, ?)", env.UserID, env.PartnerID, transferTime)
	require.NoError(t, err)

	// --- Execute Export Request ---
	req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/export/all", env.AuthToken, nil)
	rr := testutil.ExecuteRequest(t, env.Handler, req)

	// --- Assertions ---
	testutil.AssertStatusCode(t, rr, http.StatusOK)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
	assert.Contains(t, rr.Header().Get("Content-Disposition"), "attachment; filename=sapp_export_")

	// Decode the response body
	var exportData types.FullExport
	err = json.Unmarshal(rr.Body.Bytes(), &exportData)
	require.NoError(t, err, "Failed to unmarshal export JSON")

	// Verify ExportedAt timestamp is recent
	assert.WithinDuration(t, time.Now().UTC(), exportData.ExportedAt, 10*time.Second)

	// Verify User and Partner details
	assert.Equal(t, env.User1Name, exportData.User.FirstName)
	assert.Equal(t, "demo_user", exportData.User.Username) // Assuming username from seed
	assert.Equal(t, env.PartnerName, exportData.Partner.FirstName)
	assert.Equal(t, "partner_user", exportData.Partner.Username) // Assuming username from seed

	// Verify Categories (check a few)
	assert.GreaterOrEqual(t, len(exportData.Categories), 9, "Expected at least 9 categories")
	foundGroceries := false
	for _, cat := range exportData.Categories {
		if cat.Name == "Groceries" {
			foundGroceries = true
			assert.NotNil(t, cat.AINotes, "Expected AI notes for Groceries")
			break
		}
	}
	assert.True(t, foundGroceries, "Did not find Groceries category in export")

	// Verify AI Jobs
	require.Len(t, exportData.AIJobs, 2, "Expected 2 AI jobs")
	// Sort jobs by date to make assertions easier (assuming export order isn't guaranteed)
	if exportData.AIJobs[0].TransactionDate.Before(exportData.AIJobs[1].TransactionDate) {
		exportData.AIJobs[0], exportData.AIJobs[1] = exportData.AIJobs[1], exportData.AIJobs[0]
	}
	// Job 2 (Partner bought)
	assert.Equal(t, "Gift for User", exportData.AIJobs[0].Prompt)
	assert.Equal(t, 100.0, exportData.AIJobs[0].TotalAmount)
	assert.Equal(t, job2Date, exportData.AIJobs[0].TransactionDate)
	assert.False(t, exportData.AIJobs[0].PreSettled)
	assert.Equal(t, "partner_user", exportData.AIJobs[0].BuyerUsername)
	require.Len(t, exportData.AIJobs[0].Spendings, 1)
	assert.Equal(t, "Shopping", exportData.AIJobs[0].Spendings[0].CategoryName)
	assert.Equal(t, 100.0, exportData.AIJobs[0].Spendings[0].Amount)
	assert.Equal(t, "Gift", exportData.AIJobs[0].Spendings[0].Description)
	assert.Equal(t, "PaidByPartner", exportData.AIJobs[0].Spendings[0].ApportionMode) // User takes all -> PaidByPartner

	// Job 1 (User bought)
	assert.Equal(t, "Groceries and bus ticket", exportData.AIJobs[1].Prompt)
	assert.Equal(t, 75.0, exportData.AIJobs[1].TotalAmount)
	assert.Equal(t, job1Date, exportData.AIJobs[1].TransactionDate)
	assert.False(t, exportData.AIJobs[1].PreSettled)
	assert.Equal(t, "demo_user", exportData.AIJobs[1].BuyerUsername)
	require.Len(t, exportData.AIJobs[1].Spendings, 2)
	// Sort spendings within job if order isn't guaranteed (e.g., by amount)
	if exportData.AIJobs[1].Spendings[0].Amount < exportData.AIJobs[1].Spendings[1].Amount {
		exportData.AIJobs[1].Spendings[0], exportData.AIJobs[1].Spendings[1] = exportData.AIJobs[1].Spendings[1], exportData.AIJobs[1].Spendings[0]
	}
	assert.Equal(t, "Groceries", exportData.AIJobs[1].Spendings[0].CategoryName)
	assert.Equal(t, 50.0, exportData.AIJobs[1].Spendings[0].Amount)
	assert.Equal(t, "Milk & Bread", exportData.AIJobs[1].Spendings[0].Description)
	assert.Equal(t, "Shared", exportData.AIJobs[1].Spendings[0].ApportionMode) // Shared with partner
	assert.Equal(t, "Transport", exportData.AIJobs[1].Spendings[1].CategoryName)
	assert.Equal(t, 25.0, exportData.AIJobs[1].Spendings[1].Amount)
	assert.Equal(t, "Bus Ticket", exportData.AIJobs[1].Spendings[1].Description)
	assert.Equal(t, "Alone", exportData.AIJobs[1].Spendings[1].ApportionMode) // User alone

	// Verify Manual Spendings
	require.Len(t, exportData.ManualSpendings, 1, "Expected 1 manual spending")
	ms := exportData.ManualSpendings[0]
	assert.Equal(t, 30.0, ms.Amount)
	assert.Equal(t, "Manual Alone Settled", ms.Description)
	assert.Equal(t, "Shopping", ms.CategoryName)
	assert.Equal(t, manualDate, ms.SpendingDate)
	assert.Equal(t, "demo_user", ms.BuyerUsername)
	assert.Equal(t, "Alone", ms.SharedStatus)
	require.NotNil(t, ms.SettledAt, "Manual spending should be settled")
	assert.WithinDuration(t, settledTime, *ms.SettledAt, time.Second)

	// Verify Deposits
	require.Len(t, exportData.Deposits, 2, "Expected 2 deposits")
	// Sort deposits by date
	if exportData.Deposits[0].DepositDate.Before(exportData.Deposits[1].DepositDate) {
		exportData.Deposits[0], exportData.Deposits[1] = exportData.Deposits[1], exportData.Deposits[0]
	}
	// Deposit 2 (Partner)
	assert.Equal(t, "Partner Bonus", exportData.Deposits[0].Description)
	assert.Equal(t, 50.0, exportData.Deposits[0].Amount)
	assert.Equal(t, deposit2Date, exportData.Deposits[0].DepositDate)
	assert.False(t, exportData.Deposits[0].IsRecurring)
	assert.Nil(t, exportData.Deposits[0].RecurrencePeriod)
	assert.Nil(t, exportData.Deposits[0].EndDate)
	assert.Equal(t, "partner_user", exportData.Deposits[0].OwnerUsername)
	// Deposit 1 (User)
	assert.Equal(t, "Salary", exportData.Deposits[1].Description)
	assert.Equal(t, 2000.0, exportData.Deposits[1].Amount)
	assert.Equal(t, deposit1Date, exportData.Deposits[1].DepositDate)
	assert.True(t, exportData.Deposits[1].IsRecurring)
	require.NotNil(t, exportData.Deposits[1].RecurrencePeriod)
	assert.Equal(t, "monthly", *exportData.Deposits[1].RecurrencePeriod)
	require.NotNil(t, exportData.Deposits[1].EndDate)
	assert.Equal(t, deposit1EndDate, *exportData.Deposits[1].EndDate)
	assert.Equal(t, "demo_user", exportData.Deposits[1].OwnerUsername)

	// Verify Transfers
	require.Len(t, exportData.Transfers, 1, "Expected 1 transfer")
	tr := exportData.Transfers[0]
	assert.WithinDuration(t, transferTime, tr.SettlementTime, time.Second)
	assert.Equal(t, "demo_user", tr.SettledByUsername)

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		reqUnauth := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/export/all", "invalid-token", nil)
		rrUnauth := testutil.ExecuteRequest(t, env.Handler, reqUnauth)
		testutil.AssertStatusCode(t, rrUnauth, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rrUnauth, "Invalid token")
	})
}
