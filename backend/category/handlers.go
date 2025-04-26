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
