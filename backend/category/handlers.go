package category

import (
	"database/sql"

	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"git.sr.ht/~relay/sapp-backend/auth"
	"git.sr.ht/~relay/sapp-backend/types"
)

// APICategory moved to types package

// HandleGetCategories returns an http.HandlerFunc that fetches all categories (protected).
func HandleGetCategories(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get authenticated user ID from context (even though not directly used in query, ensures user is logged in)
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for getting categories", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError)
			return
		}

		rows, err := db.Query("SELECT id, name FROM categories ORDER BY name ASC")
		if err != nil {
			slog.Error("failed to query categories", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		categories := []types.Category{} // Use types.Category
		for rows.Next() {
			var cat types.Category // Use types.Category
			// Scan only ID and Name, as APICategory doesn't include ai_notes
			if err := rows.Scan(&cat.ID, &cat.Name); err != nil {
				slog.Error("failed to scan category row", "url", r.URL, "user_id", userID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			categories = append(categories, cat)
		}
		// Check for errors from iterating over rows.
		if err := rows.Err(); err != nil {
			slog.Error("error iterating category rows", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(categories); err != nil {
			slog.Error("failed to encode categories to JSON", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

// HandleAICategorize handles requests to submit a spending description for AI categorization (protected).
func HandleAICategorize(db *sql.DB, pool CategorizingPoolStrategy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get authenticated user ID from context
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context for AI categorization", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError) // Should not happen
			return
		}

		// 1. Decode the JSON payload from the request body
		// Use the dedicated type from the types package
		var payload types.AICategorizationPayload

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			slog.Error("failed to decode AI categorization request body", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// --- Parse Optional Spending Date ---
		var spendingDate *time.Time // Pointer to handle optional date
		if payload.SpendingDate != nil && *payload.SpendingDate != "" {
			parsedDate, err := time.Parse("2006-01-02", *payload.SpendingDate)
			if err != nil {
				slog.Warn("invalid spending_date format received for AI categorization, ignoring", "url", r.URL, "user_id", userID, "date_string", *payload.SpendingDate, "err", err)
				// Don't fail, just proceed without a specific date (pool will use job creation time)
			} else {
				// Use the start of the day in UTC
				dateUTC := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, time.UTC)
				spendingDate = &dateUTC // Assign the address of the parsed date
				slog.Debug("Using provided spending_date for AI job", "user_id", userID, "date", *spendingDate)
			}
		} else {
			slog.Debug("spending_date not provided for AI job, will use job creation time", "user_id", userID)
		}
		// --- End Parse Spending Date ---

		// 2. Validate payload
		// shared_status validation is removed
		if payload.Prompt == "" || payload.Amount <= 0 {
			slog.Warn("invalid AI categorization payload received", "url", r.URL, "user_id", userID, "payload", payload)
			http.Error(w, "Bad Request: Missing prompt or invalid amount", http.StatusBadRequest)
			return
		}

		// 3. Determine Buyer and potentially SharedWith using authenticated user and partner logic
		// We always fetch the partner now, if one exists, and let the AI decide based on the prompt.
		var buyer Person
		buyerRow := db.QueryRow("SELECT first_name FROM users WHERE id = ?", userID)
		err := buyerRow.Scan(&buyer.Name)
		if err != nil {
			// Handle case where authenticated user ID somehow doesn't exist
			if errors.Is(err, sql.ErrNoRows) {
				slog.Error("authenticated user not found in database", "url", r.URL, "user_id", userID)
				http.Error(w, "Internal Server Error: User configuration issue", http.StatusInternalServerError)
			} else {
				slog.Error("failed to query buyer user", "url", r.URL, "user_id", userID, "err", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
		buyer.Id = userID

		// Always try to find the partner using the new DB query method.
		var sharedWith *Person = nil
		partnerID, partnerOk := auth.GetPartnerUserID(db, userID) // Pass db connection
		if partnerOk {
			// Partner relationship exists, fetch partner details
			var partnerName string
			partnerRow := db.QueryRow("SELECT first_name FROM users WHERE id = ?", partnerID)
			err := partnerRow.Scan(&partnerName)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					// Log error but don't fail the request, maybe partner was deleted? AI can proceed without partner name.
					slog.Error("configured partner user ID not found in database (AI categorization)", "url", r.URL, "user_id", userID, "partner_id", partnerID)
					// sharedWith remains nil
				} else {
					// Log DB error but don't fail the request, proceed without partner name.
					slog.Error("failed to query partner user (AI categorization)", "url", r.URL, "user_id", userID, "partner_id", partnerID, "err", err)
					// sharedWith remains nil
				}
			} else {
				// Partner found, set the sharedWith object
				sharedWith = &Person{Id: partnerID, Name: partnerName}
				slog.Info("Partner found for AI categorization", "user_id", userID, "partner_id", partnerID, "partner_name", partnerName)
			}
		} else {
			slog.Info("No partner configured for user (AI categorization)", "user_id", userID)
		}

		// 4. Prepare parameters for the categorization job
		// SharedMode is removed
		params := CategorizationParams{
			TotalAmount: payload.Amount,
			Buyer:       buyer,      // Use authenticated buyer object
			SharedWith:  sharedWith, // Use determined sharedWith object (or nil)
			Prompt:      payload.Prompt,
			PreSettled:  payload.PreSettled, // Pass the pre-settled flag
			// 'tries' is handled internally by ProcessCategorizationJob
		}

		// 5. Add the job to the pool, passing the parsed spendingDate
		jobID, err := pool.AddJob(params, spendingDate)
		if err != nil {
			slog.Error("failed to add AI categorization job to pool", "url", r.URL, "user_id", userID, "pre_settled", payload.PreSettled, "spending_date", spendingDate, "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		slog.Info("AI categorization job added", "url", r.URL, "user_id", userID, "job_id", jobID, "amount", payload.Amount, "pre_settled", payload.PreSettled)

		// 6. Respond with 202 Accepted
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		// Optionally return the job ID
		json.NewEncoder(w).Encode(map[string]int64{"job_id": jobID})
	}
}
