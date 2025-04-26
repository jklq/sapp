package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// Simple static token for demo purposes
const demoUserToken = "demo-user-auth-token"
const demoUserID = int64(1)
// const partnerUserID = int64(2) // REMOVED: Partner ID is now dynamic via partnerships table

type contextKey string

const userContextKey = contextKey("userID")

// LoginRequest defines the structure for the login request body
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse defines the structure for the login response body
type LoginResponse struct {
	Token string `json:"token"`
	UserID int64 `json:"user_id"`
	FirstName string `json:"first_name"`
}

// PartnerRegistrationRequest defines the structure for the partner registration request body
type PartnerRegistrationRequest struct {
	User1 UserRegistrationDetails `json:"user1"`
	User2 UserRegistrationDetails `json:"user2"`
}

// UserRegistrationDetails contains the details needed to register a single user
type UserRegistrationDetails struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	FirstName string `json:"first_name"`
}

// PartnerRegistrationResponse defines the structure for the partner registration response body
type PartnerRegistrationResponse struct {
	Message string `json:"message"`
	User1ID int64  `json:"user1_id"`
	User2ID int64  `json:"user2_id"`
}

// HandleLogin creates a handler for user login
func HandleLogin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		if req.Username == "" || req.Password == "" {
			http.Error(w, "Username and password are required", http.StatusBadRequest)
			return
		}

		var storedHash, firstName string
		var userID int64
		// Query only the specific demo user for this simplified setup
		err := db.QueryRow("SELECT id, password_hash, first_name FROM users WHERE username = ? AND id = ?", req.Username, demoUserID).Scan(&userID, &storedHash, &firstName)
		if err != nil {
			if err == sql.ErrNoRows {
				slog.Warn("Login attempt failed: user not found or not demo user", "username", req.Username)
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			} else {
				slog.Error("Database error during login", "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		slog.Info("Attempting password comparison", "username", req.Username, "userID", userID)
		// Compare the provided password with the stored hash
		err = bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password))
		if err != nil {
			// Password doesn't match
			slog.Warn("Login attempt failed: invalid password", "username", req.Username, "err", err) // Log the bcrypt error
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		// Password matches - return the static token for the demo user
		slog.Info("User logged in successfully", "username", req.Username, "userID", userID, "firstName", firstName)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{
			Token: demoUserToken,
			UserID: userID,
			FirstName: firstName,
		})
	}
}

// Middleware checks for the static demo token
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, "Authorization header format must be Bearer {token}", http.StatusUnauthorized)
			return
		}

		token := parts[1]
		if token != demoUserToken {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Add user ID to context for downstream handlers
		ctx := context.WithValue(r.Context(), userContextKey, demoUserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserIDFromContext retrieves the user ID stored in the request context.
// Returns 0 and false if the user ID is not found or not an int64.
func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userContextKey).(int64)
	return userID, ok
}

// Querier defines an interface with the QueryRow method, satisfied by *sql.DB and *sql.Tx.
type Querier interface {
	QueryRow(query string, args ...interface{}) *sql.Row
}

// GetPartnerUserID finds the partner ID for a given user ID by querying the partnerships table.
// It accepts a Querier interface, which can be either *sql.DB or *sql.Tx.
func GetPartnerUserID(q Querier, requestingUserID int64) (int64, bool) {
	var partnerID int64

	// Query based on the user being either user1_id or user2_id
	// The CHECK constraint ensures user1_id < user2_id, simplifying the query slightly
	// but we query both possibilities for robustness.
	query := `
		SELECT user2_id FROM partnerships WHERE user1_id = ?
		UNION
		SELECT user1_id FROM partnerships WHERE user2_id = ?
	`
	// Use the Querier interface 'q' to execute the query
	err := q.QueryRow(query, requestingUserID, requestingUserID).Scan(&partnerID)

	if err != nil {
		if err == sql.ErrNoRows {
			// No partner found for this user
			return 0, false
		}
		// Log other database errors
		slog.Error("Error querying partner ID", "user_id", requestingUserID, "err", err)
		return 0, false
	}

	return partnerID, true
}

// HandlePartnerRegistration creates a handler for registering two users as partners.
func HandlePartnerRegistration(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req PartnerRegistrationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		// --- Input Validation ---
		u1 := req.User1
		u2 := req.User2

		if u1.Username == "" || u1.Password == "" || u1.FirstName == "" ||
			u2.Username == "" || u2.Password == "" || u2.FirstName == "" {
			http.Error(w, "All fields (username, password, first name) are required for both users", http.StatusBadRequest)
			return
		}

		if u1.Username == u2.Username {
			http.Error(w, "Usernames must be different", http.StatusBadRequest)
			return
		}

		// Basic password length check (example)
		if len(u1.Password) < 6 || len(u2.Password) < 6 {
			http.Error(w, "Password must be at least 6 characters long", http.StatusBadRequest)
			return
		}

		// --- Password Hashing ---
		hashedPassword1, err := bcrypt.GenerateFromPassword([]byte(u1.Password), bcrypt.DefaultCost)
		if err != nil {
			slog.Error("Failed to hash password for user 1", "username", u1.Username, "err", err)
			http.Error(w, "Internal server error during registration", http.StatusInternalServerError)
			return
		}
		hashedPassword2, err := bcrypt.GenerateFromPassword([]byte(u2.Password), bcrypt.DefaultCost)
		if err != nil {
			slog.Error("Failed to hash password for user 2", "username", u2.Username, "err", err)
			http.Error(w, "Internal server error during registration", http.StatusInternalServerError)
			return
		}

		// --- Database Transaction ---
		tx, err := db.Begin()
		if err != nil {
			slog.Error("Failed to begin transaction for partner registration", "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer tx.Rollback() // Rollback on any error

		// Check username uniqueness before inserting
		var count int
		err = tx.QueryRow("SELECT COUNT(*) FROM users WHERE username = ? OR username = ?", u1.Username, u2.Username).Scan(&count)
		if err != nil {
			slog.Error("Failed to check username uniqueness", "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if count > 0 {
			http.Error(w, "One or both usernames already exist", http.StatusConflict) // 409 Conflict
			return
		}

		// Insert User 1
		res1, err := tx.Exec("INSERT INTO users (username, password_hash, first_name) VALUES (?, ?, ?)",
			u1.Username, string(hashedPassword1), u1.FirstName)
		if err != nil {
			slog.Error("Failed to insert user 1", "username", u1.Username, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		user1ID, err := res1.LastInsertId()
		if err != nil {
			slog.Error("Failed to get last insert ID for user 1", "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Insert User 2
		res2, err := tx.Exec("INSERT INTO users (username, password_hash, first_name) VALUES (?, ?, ?)",
			u2.Username, string(hashedPassword2), u2.FirstName)
		if err != nil {
			slog.Error("Failed to insert user 2", "username", u2.Username, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		user2ID, err := res2.LastInsertId()
		if err != nil {
			slog.Error("Failed to get last insert ID for user 2", "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Insert Partnership (ensure user1_id < user2_id for the CHECK constraint)
		var partner1, partner2 int64
		if user1ID < user2ID {
			partner1 = user1ID
			partner2 = user2ID
		} else {
			partner1 = user2ID
			partner2 = user1ID
		}
		_, err = tx.Exec("INSERT INTO partnerships (user1_id, user2_id) VALUES (?, ?)", partner1, partner2)
		if err != nil {
			slog.Error("Failed to insert partnership", "user1", partner1, "user2", partner2, "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Commit Transaction
		if err = tx.Commit(); err != nil {
			slog.Error("Failed to commit transaction for partner registration", "err", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// --- Success Response ---
		slog.Info("Partner registration successful", "user1", u1.Username, "user2", u2.Username, "user1_id", user1ID, "user2_id", user2ID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(PartnerRegistrationResponse{
			Message: "Users registered and partnered successfully",
			User1ID: user1ID,
			User2ID: user2ID,
		})
	}
}
