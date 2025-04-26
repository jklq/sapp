package pay

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"
)

func HandlePayRoute(db *sql.DB) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		tx, err := db.Begin()
		if err != nil {
			slog.Error("failed to begin transaction", "url", r.URL, "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Defer rollback in case of errors before commit
		defer tx.Rollback()

		var shared_with *int

		switch r.PathValue("shared_status") {
		case "alone":
			shared_with = nil
		case "shared":
			// Declare a variable to hold the scanned ID
			var sharedWithID int
			// Query the user ID (FIXME: Still hardcoded to user 1)
			row := tx.QueryRow("SELECT id FROM users WHERE id = ? LIMIT 1", 1) //FIXME
			// Scan the result into the address of sharedWithID
			err = row.Scan(&sharedWithID)
			if err != nil {
				slog.Error("querying shared_with user failed", "url", r.URL, "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			// Now that we have a valid ID, assign its address to the pointer
			shared_with = &sharedWithID
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid shared status."))
				return
			}
		default:
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid shared status."))
			return
		}

		amount, err := strconv.ParseFloat(r.PathValue("amount"), 64)
		if err != nil {
			slog.Error("parsing amount float failed", "url", r.URL, "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var category_id string
		row := tx.QueryRow("SELECT id FROM categories WHERE name = ? LIMIT 1", r.PathValue("category"))

		err = row.Scan(&category_id)

		if err != nil {
			if err == sql.ErrNoRows {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte("Category not found."))
				return
			}
			slog.Error("querying category failed", "url", r.URL, "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		//FIXME: hardcoded made_by user id 1
		_, err = tx.Exec("INSERT INTO transactions (amount,made_by,shared_with,category) VALUES (?, ?, ?, ?)",
			-amount,
			1,
			shared_with,
			category_id,
		)

		if err != nil {
			slog.Error("inserting transaction failed", "url", r.URL, "err", err)

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err = tx.Commit(); err != nil {
			slog.Error("commiting transaction failed", "url", r.URL, "err", err)

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}
