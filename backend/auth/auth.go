package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os" // Import os to read environment variables
	"strings"
	"time" // Import time for token expiration

	"github.com/golang-jwt/jwt/v5" // Import JWT library
	"golang.org/x/crypto/bcrypt"
)

// JWT secret key - SHOULD be loaded securely from environment variables in production
// For development, we can use a default, but warn if it's not set.
var jwtSecretKey = []byte(os.Getenv("JWT_SECRET_KEY"))

const defaultJwtSecret = "a-secure-secret-key-for-dev-only-replace-in-prod" // CHANGE THIS IN PRODUCTION

type contextKey string

const userContextKey = contextKey("userID")

// LoginRequest defines the structure for the login request body
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse defines the structure for the login response body
type LoginResponse struct {
	Token     string `json:"token"`
	UserID    int64  `json:"user_id"`
	FirstName string `json:"first_name"`
}

// Claims defines the structure for JWT claims
type Claims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
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

// generateJWT generates a new JWT for a given user ID.
func generateJWT(userID int64) (string, error) {
	if len(jwtSecretKey) == 0 {
		slog.Warn("JWT_SECRET_KEY environment variable not set, using insecure default key for development.")
		jwtSecretKey = []byte(defaultJwtSecret)
		// In a real production scenario, you might want to return an error here
		// return "", errors.New("JWT secret key is not configured")
	}

	// Set token claims (e.g., expiration time)
	expirationTime := time.Now().Add(24 * time.Hour) // Token valid for 24 hours
	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "sapp-backend", // Optional: identify the issuer
		},
	}

	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token with the secret key
	tokenString, err := token.SignedString(jwtSecretKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	return tokenString, nil
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
		// Query user by username only
		err := db.QueryRow("SELECT id, password_hash, first_name FROM users WHERE username = ?", req.Username).Scan(&userID, &storedHash, &firstName)
		if err != nil {
			if err == sql.ErrNoRows {
				slog.Warn("Login attempt failed: user not found", "username", req.Username)
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

		// Password matches - Generate JWT
		slog.Info("User logged in successfully, generating JWT", "username", req.Username, "userID", userID, "firstName", firstName)
		tokenString, err := generateJWT(userID)
		if err != nil {
			slog.Error("Failed to generate JWT", "username", req.Username, "userID", userID, "err", err)
			http.Error(w, "Internal server error during login", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(LoginResponse{
			Token:     tokenString, // Return the generated JWT
			UserID:    userID,
			FirstName: firstName,
		})
	}
}

// AuthMiddleware validates the JWT token from the Authorization header.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if JWT secret is configured (only needs to check once, but doing it here is simple)
		if len(jwtSecretKey) == 0 {
			slog.Warn("JWT_SECRET_KEY environment variable not set, using insecure default key for development.")
			jwtSecretKey = []byte(defaultJwtSecret)
			// Allow proceeding in dev, but log warning. In prod, you might want to fail here.
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Expecting "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2) // Split into exactly 2 parts
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			slog.Warn("Invalid Authorization header format", "url", r.URL, "header", authHeader)
			http.Error(w, "Authorization header format must be Bearer {token}", http.StatusUnauthorized)
			return
		}
		tokenString := parts[1]

		// Parse and validate the token
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// Check the signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtSecretKey, nil
		})

		if err != nil {
			if errors.Is(err, jwt.ErrTokenExpired) {
				slog.Warn("JWT validation failed: token expired", "url", r.URL)
				http.Error(w, "Token expired", http.StatusUnauthorized)
			} else {
				slog.Warn("JWT validation failed", "url", r.URL, "err", err)
				http.Error(w, "Invalid token", http.StatusUnauthorized)
			}
			return
		}

		if !token.Valid {
			slog.Warn("JWT validation failed: token invalid", "url", r.URL)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Token is valid, extract user ID and add to context
		userID := claims.UserID
		if userID <= 0 {
			// Should not happen if token generation is correct, but check defensively
			slog.Error("Invalid user ID found in valid JWT claims", "url", r.URL, "userID", userID)
			http.Error(w, "Invalid token claims", http.StatusUnauthorized)
			return
		}

		// Optional: Verify user ID exists in DB? Can add overhead but increases security.
		// var exists int
		// err = db.QueryRow("SELECT 1 FROM users WHERE id = ?", userID).Scan(&exists) // Need DB access here
		// if err != nil { ... handle error or sql.ErrNoRows ... http.StatusUnauthorized }

		slog.Debug("AuthMiddleware: User identified via JWT", "userID", userID)
		ctx := context.WithValue(r.Context(), userContextKey, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserIDFromContext retrieves the user ID stored in the request context.
// Returns 0 and false if the user ID is not found or not an int64.
func GetUserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(userContextKey).(int64)
	return userID, ok
}

// GenerateTestJWT is a helper function specifically for tests to generate a token.
// It should NOT be used in production code.
func GenerateTestJWT(userID int64) (string, error) {
	// Use the same logic as generateJWT, potentially with shorter expiry for tests if desired
	return generateJWT(userID)
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
