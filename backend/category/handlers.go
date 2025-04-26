package category

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
)

// APICategory represents a category returned by the API.
type APICategory struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// HandleGetCategories returns an http.HandlerFunc that fetches all categories.
func HandleGetCategories(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, name FROM categories ORDER BY name ASC")
		if err != nil {
			slog.Error("failed to query categories", "url", r.URL, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		categories := []APICategory{}
		for rows.Next() {
			var cat APICategory
			if err := rows.Scan(&cat.ID, &cat.Name); err != nil {
				slog.Error("failed to scan category row", "url", r.URL, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			categories = append(categories, cat)
		}

		if err := rows.Err(); err != nil {
			slog.Error("error iterating category rows", "url", r.URL, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(categories); err != nil {
			slog.Error("failed to encode categories to JSON", "url", r.URL, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}
}

// HandleAICategorize handles requests to submit a spending description for AI categorization.
func HandleAICategorize(db *sql.DB, pool CategorizingPoolStrategy) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Decode the JSON payload from the request body
		var payload struct {
			SharedStatus string  `json:"shared_status"` // Matches AICategorizationPayload in frontend/types.ts
			Amount       float64 `json:"amount"`
			Prompt       string  `json:"prompt"`
		}

		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			slog.Error("failed to decode AI categorization request body", "url", r.URL, "err", err)
			http.Error(w, "Bad Request: Invalid JSON", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// 2. Validate payload (basic validation)
		if payload.Prompt == "" || payload.Amount <= 0 || (payload.SharedStatus != "alone" && payload.SharedStatus != "shared" && payload.SharedStatus != "mix") {
			slog.Warn("invalid AI categorization payload received", "url", r.URL, "payload", payload)
			http.Error(w, "Bad Request: Missing or invalid fields", http.StatusBadRequest)
			return
		}

		// 3. Determine Buyer and SharedWith (FIXME: Hardcoded user IDs)
		// Similar to pay.go, we'll hardcode the buyer and potential shared partner for now.
		// This should be replaced with actual user authentication and selection logic.
		buyer := Person{Id: 1, Name: "UserOne"} // FIXME: Hardcoded buyer ID and Name
		var sharedWith *Person = nil

		if payload.SharedStatus != "alone" {
			// FIXME: Hardcoded shared_with user ID and Name.
			// In a real app, this might come from the request or user context.
			// Also, need to handle the case where the user doesn't exist.
			sharedWith = &Person{Id: 1, Name: "UserOne"} // FIXME: Using buyer ID for shared partner for now
			// If shared_status is 'mix' or 'shared', we need a valid shared partner.
			// If the hardcoded ID is invalid or doesn't exist, AddJob might fail later or behave unexpectedly.
			// A check against the DB here would be better.
		}

		// 4. Prepare parameters for the categorization job
		params := CategorizationParams{
			DB:          db, // Pass the DB connection
			TotalAmount: payload.Amount,
			SharedMode:  payload.SharedStatus,
			Buyer:       buyer,
			SharedWith:  sharedWith,
			Prompt:      payload.Prompt,
			// 'tries' is handled internally by ProcessCategorizationJob
		}

		// 5. Add the job to the pool
		jobID, err := pool.AddJob(params)
		if err != nil {
			slog.Error("failed to add AI categorization job to pool", "url", r.URL, "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		slog.Info("AI categorization job added", "url", r.URL, "job_id", jobID, "amount", payload.Amount, "shared_status", payload.SharedStatus)

		// 6. Respond with 202 Accepted
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		// Optionally return the job ID
		json.NewEncoder(w).Encode(map[string]int64{"job_id": jobID})
	}
}
