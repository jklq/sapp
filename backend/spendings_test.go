package main_test

import (
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"

	"git.sr.ht/~relay/sapp-backend/spendings" // Import spendings for handlers
	"git.sr.ht/~relay/sapp-backend/testutil" // Import the new test utility package
	"git.sr.ht/~relay/sapp-backend/types"    // Import shared types
)

// Helper function to create a pointer to a string (copied here as it's used in TestGetHistory)
// Consider moving to testutil if used more widely.
func Ptr(s string) *string {
	return &s
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

		// The response structure is now spendings.HistoryResponse which contains []spendings.FrontendHistoryListItem
		var resp spendings.HistoryResponse
		testutil.DecodeJSONResponse(t, rr, &resp)
		if len(resp.History) != 0 {
			t.Errorf("Expected 0 history items, got %d", len(resp.History))
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
	// Set start date 35 days ago to ensure the next monthly occurrence (approx T-5d) is before 'now'
	deposit2Date := time.Now().AddDate(0, 0, -35)
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

		var resp spendings.HistoryResponse // Use spendings.HistoryResponse
		testutil.DecodeJSONResponse(t, rr, &resp)

		// Check total count (2 jobs + 1 non-recurring deposit + 2 recurring deposit occurrences = 5 items)
		// Deposit 1: T-10d (non-recurring)
		// Deposit 2: T-35d (recurring monthly) -> Occurrences at T-35d, T-5d (approx, calculated from T-35d + 1 month)
		// Job 1: T-2h
		// Job 2: T-1h
		// Expected order (newest first): Job2(T-1h), Job1(T-2h), Deposit2(T-5d), Deposit1(T-10d), Deposit2(T-35d)
		// Total expected items = 5
		if len(resp.History) != 5 {
			t.Fatalf("Expected 5 history items, got %d", len(resp.History))
			// For debugging:
			// for i, item := range resp.History {
			// 	t.Logf("Item %d: Type=%s, Date=%s", i, item.Type, item.Date)
			// }
		}

		// --- Verify Item Order and Content (Spot Checks) ---

		// Item 0: Should be Job 2 (most recent)
		item0 := resp.History[0]
		if item0.Type != "spending_group" || item0.JobID == nil || *item0.JobID != job2ID {
			t.Errorf("Expected item 0 to be spending_group JobID %d, got Type=%s, JobID=%v", job2ID, item0.Type, item0.JobID)
		}
		if item0.Spendings == nil || len(item0.Spendings) != 1 || item0.Spendings[0].ID != spending2_1 {
			t.Errorf("Expected 1 spending item with ID %d in item 0, got %v", spending2_1, item0.Spendings)
		}
		expectedStatus2 := fmt.Sprintf("Paid by %s", env.PartnerName)
		if item0.Spendings[0].SharingStatus != expectedStatus2 {
			t.Errorf("Expected spending 2_1 status '%s', got '%s'", expectedStatus2, item0.Spendings[0].SharingStatus)
		}

		// Item 1: Should be Job 1
		item1 := resp.History[1]
		if item1.Type != "spending_group" || item1.JobID == nil || *item1.JobID != job1ID {
			t.Errorf("Expected item 1 to be spending_group JobID %d, got Type=%s, JobID=%v", job1ID, item1.Type, item1.JobID)
		}
		if item1.Spendings == nil || len(item1.Spendings) != 2 {
			t.Fatalf("Expected 2 spending items in item 1, got %d", len(item1.Spendings))
		}
		if item1.Spendings[0].ID != spending1_1 || item1.Spendings[1].ID != spending1_2 {
			t.Errorf("Expected spending IDs %d, %d in item 1, got %d, %d", spending1_1, spending1_2, item1.Spendings[0].ID, item1.Spendings[1].ID)
		}

		// Item 2: Should be Deposit 2 occurrence (recent)
		item2 := resp.History[2]
		if item2.Type != "deposit" || item2.ID == nil || *item2.ID != deposit2ID {
			t.Errorf("Expected item 2 to be deposit ID %d, got Type=%s, ID=%v", deposit2ID, item2.Type, item2.ID)
		}
		if item2.Description == nil || *item2.Description != "Pocket Money" {
			t.Errorf("Expected item 2 description 'Pocket Money', got %v", item2.Description)
		}
		if item2.IsRecurring == nil || !*item2.IsRecurring || item2.RecurrencePeriod == nil || *item2.RecurrencePeriod != "monthly" {
			t.Errorf("Expected item 2 to be recurring monthly, got recurring=%v, period=%v", item2.IsRecurring, item2.RecurrencePeriod)
		}
		// Check date is the calculated next occurrence after deposit2Date (T-35d)
		// Use the same logic as the history service's calculateNextDate
		expectedNextDate := deposit2Date.AddDate(0, 1, 0) // Add 1 month
		if item2.Date.Format("2006-01-02") != expectedNextDate.Format("2006-01-02") {
			t.Errorf("Expected item 2 date to be %s (1 month after %s), got %s",
				expectedNextDate.Format("2006-01-02"),
				deposit2Date.Format("2006-01-02"),
				item2.Date.Format("2006-01-02"))
		}

		// Item 3: Should be Deposit 1 (non-recurring)
		item3 := resp.History[3]
		if item3.Type != "deposit" || item3.ID == nil || *item3.ID != deposit1ID {
			t.Errorf("Expected item 3 to be deposit ID %d, got Type=%s, ID=%v", deposit1ID, item3.Type, item3.ID)
		}
		if item3.Description == nil || *item3.Description != "Salary May" {
			t.Errorf("Expected item 3 description 'Salary May', got %v", item3.Description)
		}
		if item3.IsRecurring == nil || *item3.IsRecurring {
			t.Error("Expected item 3 not to be recurring")
		}
		if item3.Date.Format("2006-01-02") != deposit1Date.Format("2006-01-02") {
			t.Errorf("Expected item 3 date %s, got %s", deposit1Date.Format("2006-01-02"), item3.Date.Format("2006-01-02"))
		}

		// Item 4: Should be Deposit 2 occurrence (original)
		item4 := resp.History[4]
		if item4.Type != "deposit" || item4.ID == nil || *item4.ID != deposit2ID {
			t.Errorf("Expected item 4 to be deposit ID %d, got Type=%s, ID=%v", deposit2ID, item4.Type, item4.ID)
		}
		if item4.Date.Format("2006-01-02") != deposit2Date.Format("2006-01-02") {
			t.Errorf("Expected item 4 date %s, got %s", deposit2Date.Format("2006-01-02"), item4.Date.Format("2006-01-02"))
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/history", "invalid-token", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token")
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
		payload        types.UpdateSpendingPayload // Use types.UpdateSpendingPayload
		expectedStatus int
		expectedBody   string                       // Substring to check in body for errors
		verifyFunc     func(t *testing.T, id int64) // Optional verification function
	}{
		{
			name:       "SuccessUpdateToAlone",
			spendingID: spendingIDShared,
			payload: types.UpdateSpendingPayload{ // Use types.UpdateSpendingPayload
				Description:   "Updated to Alone",
				CategoryName:  "Transport",
				SharingStatus: types.StatusAlone, // Use types constant
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
			payload: types.UpdateSpendingPayload{ // Use types.UpdateSpendingPayload
				Description:   "Updated to Shared",
				CategoryName:  "Groceries",
				SharingStatus: types.StatusShared, // Use types constant
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
			payload: types.UpdateSpendingPayload{ // Use types.UpdateSpendingPayload
				Description:   "Updated to PaidByPartner",
				CategoryName:  "Shopping",
				SharingStatus: types.StatusPaidByPartner, // Use types constant
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
			payload: types.UpdateSpendingPayload{ // Use types.UpdateSpendingPayload
				Description:   "Test",
				CategoryName:  "Groceries",
				SharingStatus: types.StatusAlone, // Use types constant
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   "Spending item not found",
		},
		{
			name:       "ErrorForbidden",
			spendingID: spendingIDPartners, // Belongs to partner
			payload: types.UpdateSpendingPayload{ // Use types.UpdateSpendingPayload
				Description:   "Attempt Forbidden Update",
				CategoryName:  "Groceries",
				SharingStatus: types.StatusAlone, // Use types constant
			},
			expectedStatus: http.StatusForbidden,
			expectedBody:   "Forbidden",
		},
		{
			name:       "ErrorInvalidCategory",
			spendingID: spendingIDShared,
			payload: types.UpdateSpendingPayload{ // Use types.UpdateSpendingPayload
				Description:   "Test",
				CategoryName:  "NonExistentCategory",
				SharingStatus: types.StatusAlone, // Use types constant
			},
			expectedStatus: http.StatusBadRequest, // Bad request because category is invalid input
			expectedBody:   "Category not found",
		},
		{
			name:       "ErrorInvalidStatus",
			spendingID: spendingIDShared,
			payload: types.UpdateSpendingPayload{ // Use types.UpdateSpendingPayload
				Description:   "Test",
				CategoryName:  "Groceries",
				SharingStatus: "invalid_status", // Keep as string for test case
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Invalid sharing status",
		},
		{
			name:       "ErrorMissingCategory",
			spendingID: spendingIDShared,
			payload: types.UpdateSpendingPayload{ // Use types.UpdateSpendingPayload
				Description:   "Test",
				CategoryName:  "", // Missing category
				SharingStatus: types.StatusAlone, // Use types constant
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
		payload := types.UpdateSpendingPayload{ // Use types.UpdateSpendingPayload
			Description:   "Unauthorized Update",
			CategoryName:  "Groceries",
			SharingStatus: types.StatusAlone, // Use types constant
		}
		req := testutil.NewAuthenticatedRequest(t, http.MethodPut, url, "invalid-token", payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token")
	})
}
