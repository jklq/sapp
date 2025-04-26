package pay

import (
	"database/sql"
	"errors" // Import errors package
	"log/slog"
	"net/http"
	"strconv"

	"git.sr.ht/~relay/sapp-backend/auth" // Import auth package
)

func HandlePayRoute(db *sql.DB) http.HandlerFunc { // Return http.HandlerFunc directly
	return func(w http.ResponseWriter, r *http.Request) {
		// Get authenticated user ID from context
		userID, ok := auth.GetUserIDFromContext(r.Context())
		if !ok {
			slog.Error("failed to get user ID from context", "url", r.URL)
			http.Error(w, "Authentication error", http.StatusInternalServerError) // Should not happen if middleware is correct
			return
		}

		tx, err := db.Begin()
		if err != nil {
			slog.Error("failed to begin transaction", "url", r.URL, "user_id", userID, "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		defer tx.Rollback() // Defer rollback in case of errors before commit

		var shared_with_id *int64 // Use *int64 for nullable foreign key

		sharedStatus := r.PathValue("shared_status")
		switch sharedStatus {
		case "alone":
			shared_with_id = nil
		case "shared":
			// Get the partner ID using the new function that queries the database
			partnerID, partnerOk := auth.GetPartnerUserID(tx, userID) // Pass transaction tx
			if !partnerOk {
				// Check if GetPartnerUserID logged the error already
				// slog.Error("could not determine partner user ID for sharing", "url", r.URL, "user_id", userID)
				http.Error(w, "Cannot share: Partner not found or not configured for this user.", http.StatusBadRequest)
				return
			}
			// Check if partner exists in DB (optional, but good practice)
			var exists int
			err := tx.QueryRow("SELECT 1 FROM users WHERE id = ?", partnerID).Scan(&exists)
			if err != nil {
				if err == sql.ErrNoRows {
					slog.Error("configured partner user ID not found in database", "url", r.URL, "user_id", userID, "partner_id", partnerID)
					http.Error(w, "Internal server error: Partner configuration issue", http.StatusInternalServerError)
				} else {
					slog.Error("failed to query partner user", "url", r.URL, "user_id", userID, "partner_id", partnerID, "err", err)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
				return
			}
			shared_with_id = &partnerID // Assign the partner ID
		default:
			slog.Warn("invalid shared status received", "url", r.URL, "user_id", userID, "status", sharedStatus)
			http.Error(w, "Invalid shared status.", http.StatusBadRequest)
			return
		}

		amountStr := r.PathValue("amount")
		amount, err := strconv.ParseFloat(amountStr, 64)
		if err != nil {
			slog.Error("parsing amount float failed", "url", r.URL, "user_id", userID, "amount_str", amountStr, "err", err)
			http.Error(w, "Invalid amount format", http.StatusBadRequest)
			return
		}
		if amount <= 0 {
			slog.Warn("non-positive amount received", "url", r.URL, "user_id", userID, "amount", amount)
			http.Error(w, "Amount must be positive", http.StatusBadRequest)
			return
		}


		var category_id int64 // Category ID is integer
		categoryName := r.PathValue("category")
		row := tx.QueryRow("SELECT id FROM categories WHERE name = ? LIMIT 1", categoryName)
		err = row.Scan(&category_id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				slog.Warn("category not found", "url", r.URL, "user_id", userID, "category_name", categoryName)
				http.Error(w, "Category not found.", http.StatusBadRequest)
				return
			}
			slog.Error("querying category failed", "url", r.URL, "user_id", userID, "category_name", categoryName, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Insert into spendings table (assuming manual pay creates a single 'spending')
		// Note: The original code inserted into a 'transactions' table which doesn't exist in the schema.
		// Adapting to insert into 'spendings' and 'user_spendings' like the AI worker does.
		spendingDesc := "Manual Entry" // Or potentially get description from frontend if added later
		res, err := tx.Exec(`INSERT INTO spendings (amount, description, category, made_by)
		VALUES (?,?,?,?)`, amount, spendingDesc, category_id, userID)
		if err != nil {
			slog.Error("inserting spending failed", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		spendingID, err := res.LastInsertId()
		if err != nil {
			slog.Error("failed to get last insert ID for spending", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Insert into user_spendings
		// For manual pay, shared_user_takes_all is always false.
		_, err = tx.Exec(`INSERT INTO user_spendings (spending_id, buyer, shared_with, shared_user_takes_all)
		VALUES (?,?,?,?)`, spendingID, userID, shared_with_id, false)
		if err != nil {
			slog.Error("inserting user_spending failed", "url", r.URL, "user_id", userID, "spending_id", spendingID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}


		// Commit the transaction
		if err = tx.Commit(); err != nil {
			slog.Error("commiting transaction failed", "url", r.URL, "user_id", userID, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}
