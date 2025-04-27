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

// TestAddDeposit tests the POST /v1/deposits endpoint.
func TestAddDeposit(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Test Cases ---
	testCases := []struct {
		name           string
		payload        types.AddDepositPayload // Use types.AddDepositPayload
		expectedStatus int
		expectedBody   string                       // Substring for error messages or success message
		verifyFunc     func(t *testing.T, id int64) // Optional verification
	}{
		{
			name: "SuccessOneOff",
			payload: types.AddDepositPayload{ // Use types.AddDepositPayload
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
			payload: types.AddDepositPayload{ // Use types.AddDepositPayload
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
			payload: types.AddDepositPayload{ // Use types.AddDepositPayload
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
			payload: types.AddDepositPayload{ // Use types.AddDepositPayload
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
			payload: types.AddDepositPayload{ // Use types.AddDepositPayload
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
			payload: types.AddDepositPayload{ // Use types.AddDepositPayload
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
				var respBody types.AddDepositResponse // Use types.AddDepositResponse
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
		payload := types.AddDepositPayload{Amount: 100, Description: "Test", DepositDate: "2024-01-01"} // Use types.AddDepositPayload
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/deposits", "invalid-token", payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		payload := types.AddDepositPayload{Amount: 100, Description: "Test", DepositDate: "2024-01-01"} // Use types.AddDepositPayload
		req := testutil.NewAuthenticatedRequest(t, http.MethodPost, "/v1/deposits", "invalid-token", payload)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token")
	})
}

// TestGetDeposits tests the GET /v1/deposits endpoint.
func TestGetDeposits(t *testing.T) {
	env := testutil.SetupTestEnvironment(t)
	defer env.TearDownDB()

	// --- Test Case: Empty Deposits ---
	t.Run("EmptyDeposits", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/deposits", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var resp []types.Deposit // Use types.Deposit
		testutil.DecodeJSONResponse(t, rr, &resp)
		if len(resp) != 0 {
			t.Errorf("Expected 0 deposits, got %d", len(resp))
		}
	})

	// --- Setup Data ---
	depositDate1 := time.Now().AddDate(0, 0, -5)
	depositDate2 := time.Now().AddDate(0, 0, -15)
	deposit1ID := testutil.InsertDeposit(t, env.DB, env.UserID, 1500.0, "Bonus", depositDate1, false, nil)
	deposit2ID := testutil.InsertDeposit(t, env.DB, env.UserID, 200.0, "Refund", depositDate2, false, nil)
	// Insert deposit for partner (should not be fetched)
	_ = testutil.InsertDeposit(t, env.DB, env.PartnerID, 500.0, "Partner Deposit", depositDate1, false, nil)

	// --- Test Case: Fetch Deposits ---
	t.Run("FetchDeposits", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/deposits", env.AuthToken, nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusOK)

		var resp []types.Deposit // Use types.Deposit
		testutil.DecodeJSONResponse(t, rr, &resp)

		if len(resp) != 2 {
			t.Fatalf("Expected 2 deposits, got %d", len(resp))
		}

		// Check order (most recent first based on deposit_date)
		if resp[0].ID != deposit1ID {
			t.Errorf("Expected first deposit ID %d, got %d", deposit1ID, resp[0].ID)
		}
		if resp[1].ID != deposit2ID {
			t.Errorf("Expected second deposit ID %d, got %d", deposit2ID, resp[1].ID)
		}

		// Check content of one deposit
		if resp[0].Description != "Bonus" || math.Abs(resp[0].Amount-1500.0) > 0.001 {
			t.Errorf("Deposit 1 content mismatch: got %+v", resp[0])
		}
		// Check date (ignoring time part for simplicity)
		if resp[0].DepositDate.Format("2006-01-02") != depositDate1.Format("2006-01-02") {
			t.Errorf("Deposit 1 date mismatch: got %s, want %s", resp[0].DepositDate.Format("2006-01-02"), depositDate1.Format("2006-01-02"))
		}
	})

	// --- Test Case: Unauthorized ---
	t.Run("Unauthorized", func(t *testing.T) {
		req := testutil.NewAuthenticatedRequest(t, http.MethodGet, "/v1/deposits", "invalid-token", nil)
		rr := testutil.ExecuteRequest(t, env.Handler, req)
		testutil.AssertStatusCode(t, rr, http.StatusUnauthorized)
		testutil.AssertBodyContains(t, rr, "Invalid token")
	})
}
