package category

import (
	"database/sql"
	"database/sql"
	"encoding/json"
	"errors" // Import errors
	"log/slog"
	"net/http"

	"git.sr.ht/~relay/sapp-backend/auth" // Import auth package
)

// APICategory represents a category returned by the API.
type APICategory struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

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

		categories := []APICategory{}
		for rows.Next() {
			var cat APICategory
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
		var payload struct {
			SharedStatus string  `json:"shared_status"` // Matches AICategorizationPayload in frontend/types.ts (should be 'alone' or 'shared')
			Amount       float64 `json:"amount"`
			Prompt       string  `json:"prompt"`

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			slog.Error("failed to decode AI categorization request body", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// 2. Validate payload
		// 'mix' is not directly supported by this simplified demo setup for submission, AI might still use it.
		if payload.Prompt == "" || payload.Amount <= 0 || (payload.SharedStatus != "alone" && payload.SharedStatus != "shared") {
			slog.Warn("invalid AI categorization payload received", "url", r.URL, "user_id", userID, "payload", payload)
			http.Error(w, "Bad Request: Missing prompt, invalid amount, or invalid shared_status (must be 'alone' or 'shared')", http.StatusBadRequest)
			return
		}

		// 3. Determine Buyer and SharedWith using authenticated user and partner logic
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

		var sharedWith *Person = nil
		if payload.SharedStatus == "shared" {
			partnerID, partnerOk := auth.GetPartnerUserID(userID)
			if !partnerOk {
				slog.Error("could not determine partner user ID for sharing (AI categorization)", "url", r.URL, "user_id", userID)
				http.Error(w, "Cannot share: Partner not configured for this user.", http.StatusBadRequest)
				return
			}

			var partnerName string
			partnerRow := db.QueryRow("SELECT first_name FROM users WHERE id = ?", partnerID)
			err := partnerRow.Scan(&partnerName)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					slog.Error("configured partner user ID not found in database (AI categorization)", "url", r.URL, "user_id", userID, "partner_id", partnerID)
					http.Error(w, "Internal Server Error: Partner configuration issue", http.StatusInternalServerError)
				} else {
					slog.Error("failed to query partner user (AI categorization)", "url", r.URL, "user_id", userID, "partner_id", partnerID, "err", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
			sharedWith = &Person{Id: partnerID, Name: partnerName}
		}

		// 4. Prepare parameters for the categorization job
		params := CategorizationParams{
			TotalAmount: payload.Amount,
			SharedMode:  payload.SharedStatus, // Pass the original status ('alone' or 'shared')
			Buyer:       buyer,              // Use authenticated buyer object
			SharedWith:  sharedWith,         // Use determined sharedWith object (or nil)
			Prompt:      payload.Prompt,
			// 'tries' is handled internally by ProcessCategorizationJob
		}

		// 5. Add the job to the pool
		jobID, err := pool.AddJob(params) // AddJob uses the pool's db connection
		if err != nil {
			slog.Error("failed to add AI categorization job to pool", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		slog.Info("AI categorization job added", "url", r.URL, "user_id", userID, "job_id", jobID, "amount", payload.Amount, "shared_status", payload.SharedStatus)

		// 6. Respond with 202 Accepted
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		// Optionally return the job ID
		json.NewEncoder(w).Encode(map[string]int64{"job_id": jobID})
	}
}
