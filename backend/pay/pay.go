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
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var shared_with *int

		switch r.PathValue("shared_status") {
		case "alone":
			shared_with = nil
		case "shared":
			row := tx.QueryRow("SELECT id FROM users WHERE id = ? LIMIT 1", 1) //FIXME

			err = row.Scan(shared_with)
			if err != nil {
				slog.Error("querying shared_with user failed", "url", r.URL, "err", err)
				w.WriteHeader(http.StatusInternalServerError)
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
