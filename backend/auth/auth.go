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
const partnerUserID = int64(2) // Hardcoded partner ID for the demo user

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

		// Compare the provided password with the stored hash
		err = bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password))
		if err != nil {
			// Password doesn't match
			slog.Warn("Login attempt failed: invalid password", "username", req.Username)
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
			return
		}

		// Password matches - return the static token for the demo user
		slog.Info("User logged in successfully", "username", req.Username, "userID", userID)
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

// GetPartnerUserID returns the hardcoded partner ID for the demo user.
// In a real application, this would involve more complex logic.
func GetPartnerUserID(requestingUserID int64) (int64, bool) {
	if requestingUserID == demoUserID {
		return partnerUserID, true
	}
	// In this demo, only the demo user has a defined partner.
	return 0, false
}
