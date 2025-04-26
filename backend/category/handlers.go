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

		// 3. Determine Buyer and SharedWith (FIXME: Hardcoded user IDs - using ID 1 for both)
		// This section now validates the hardcoded IDs against the database.
		var buyer Person
		buyerID := int64(1) // FIXME: Hardcoded buyer ID
		buyerRow := db.QueryRow("SELECT first_name FROM users WHERE id = ?", buyerID)
		err := buyerRow.Scan(&buyer.Name)
		if err != nil {
			if err == sql.ErrNoRows {
				slog.Error("buyer user not found in database", "url", r.URL, "buyer_id", buyerID)
				http.Error(w, "Internal Server Error: Buyer user configuration issue", http.StatusInternalServerError)
			} else {
				slog.Error("failed to query buyer user", "url", r.URL, "buyer_id", buyerID, "err", err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}
		buyer.Id = buyerID

		var sharedWith *Person = nil
		if payload.SharedStatus != "alone" {
			sharedWithID := int64(1) // FIXME: Hardcoded shared_with ID
			// Ensure sharedWithID is different from buyerID if necessary for your logic,
			// currently they are the same (1).
			var sharedWithName string
			sharedRow := db.QueryRow("SELECT first_name FROM users WHERE id = ?", sharedWithID)
			err := sharedRow.Scan(&sharedWithName)
			if err != nil {
				if err == sql.ErrNoRows {
					slog.Error("shared_with user not found in database", "url", r.URL, "shared_with_id", sharedWithID)
					http.Error(w, "Internal Server Error: Shared user configuration issue", http.StatusInternalServerError)
				} else {
					slog.Error("failed to query shared_with user", "url", r.URL, "shared_with_id", sharedWithID, "err", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
				return
			}
			sharedWith = &Person{Id: sharedWithID, Name: sharedWithName}
		}

		// 4. Prepare parameters for the categorization job
		params := CategorizationParams{
			// DB is no longer needed here as AddJob uses the pool's db connection
			TotalAmount: payload.Amount,
			SharedMode:  payload.SharedStatus,
			Buyer:       buyer, // Use validated buyer object
			SharedWith:  sharedWith, // Use validated sharedWith object (or nil)
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
