package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"

	"github.com/golang-jwt/jwt/v5"

	"golang.org/x/crypto/bcrypt"

	"git.sr.ht/~relay/sapp-backend/types"
)

// JWT secret key - SHOULD be loaded securely from environment variables in production
var jwtSecretKey = []byte(os.Getenv("JWT_SECRET_KEY"))

// Constants for token expiration
const accessTokenDuration = 15 * time.Minute     // Short-lived access token (e.g., 15 minutes)
const refreshTokenDuration = 30 * 24 * time.Hour // Long-lived refresh token (e.g., 30 days)

// Default secret removed - must be set via environment variable.

type contextKey string

const userContextKey = contextKey("userID")

// LoginRequest moved to types package
// LoginResponse moved to types package

// AccessTokenClaims defines the structure for JWT access token claims
type AccessTokenClaims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

// RefreshTokenClaims defines the structure for JWT refresh token claims (can be simpler)
// We primarily rely on the database record for refresh token validity.
// The JWT itself mainly serves as a carrier for the user ID if needed during refresh,
// but we'll validate against the DB hash.
type RefreshTokenClaims struct {
	UserID int64 `json:"user_id"`
	jwt.RegisteredClaims
}

// PartnerRegistrationRequest moved to types package
// UserRegistrationDetails moved to types package
// PartnerRegistrationResponse moved to types package

// --- Token Generation ---

// generateAccessToken generates a short-lived JWT access token.
func generateAccessToken(userID int64, secret []byte) (string, error) {
	expirationTime := time.Now().Add(accessTokenDuration)
	claims := &AccessTokenClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "sapp-backend",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}

// generateRefreshToken generates a long-lived, secure random string for the refresh token.
// It does NOT generate a JWT for the refresh token itself, only the access token.
// The refresh token value stored in the DB is a hash of this generated string.
func generateRefreshTokenValue() (string, error) {
	b := make([]byte, 32) // Generate 32 random bytes
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil // Encode to URL-safe base64 string
}

// hashToken hashes a token string using SHA256.
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.URLEncoding.EncodeToString(hash[:]) // Store hash as base64 string
}

// storeRefreshToken stores the hashed refresh token in the database.
func storeRefreshToken(db *sql.DB, userID int64, tokenValue string) error {
	hashedToken := hashToken(tokenValue)
	expiresAt := time.Now().Add(refreshTokenDuration)

	// Consider removing old tokens for the user first for cleanliness
	_, err := db.Exec("DELETE FROM refresh_tokens WHERE user_id = ?", userID)
	if err != nil {
		slog.Error("Failed to delete old refresh tokens", "user_id", userID, "err", err)
		// Continue execution, but log the error
	}

	_, err = db.Exec(`
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES (?, ?, ?)
	`, userID, hashedToken, expiresAt)

	if err != nil {
		slog.Error("Failed to store refresh token", "user_id", userID, "err", err)
		return fmt.Errorf("could not store refresh token: %w", err)
	}
	slog.Debug("Refresh token stored successfully", "user_id", userID)
	return nil
}

// validateRefreshToken checks if the provided refresh token value is valid in the database.
// Returns the user ID if valid, otherwise returns 0 and an error.
func validateRefreshToken(db *sql.DB, tokenValue string) (int64, error) {
	hashedToken := hashToken(tokenValue)
	var userID int64
	var expiresAt time.Time

	err := db.QueryRow(`
		SELECT user_id, expires_at FROM refresh_tokens
		WHERE token_hash = ?
	`, hashedToken).Scan(&userID, &expiresAt)

	if err != nil {
		if err == sql.ErrNoRows {
			slog.Warn("Refresh token validation failed: token not found", "token_hash_prefix", hashedToken[:min(10, len(hashedToken))]) // Log prefix for debugging
			return 0, errors.New("invalid refresh token")
		}
		slog.Error("Database error validating refresh token", "err", err)
		return 0, fmt.Errorf("database error during refresh token validation: %w", err)
	}

	if time.Now().After(expiresAt) {
		slog.Warn("Refresh token validation failed: token expired", "user_id", userID, "expires_at", expiresAt)
		// Optionally delete the expired token
		_, delErr := db.Exec("DELETE FROM refresh_tokens WHERE token_hash = ?", hashedToken)
		if delErr != nil {
			slog.Error("Failed to delete expired refresh token", "user_id", userID, "err", delErr)
		}
		return 0, errors.New("refresh token expired")
	}

	slog.Debug("Refresh token validated successfully", "user_id", userID)
	return userID, nil
}

// --- HTTP Handlers ---

// HandleLogin creates a handler for user login
func HandleLogin(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.LoginRequest // Use types.LoginRequest
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

		// Password matches - Generate Tokens
		slog.Info("User logged in successfully, generating tokens", "username", req.Username, "userID", userID, "firstName", firstName)

		// Ensure JWT secret key is configured
		secret := []byte(os.Getenv("JWT_SECRET_KEY"))
		if len(secret) == 0 {
			slog.Error("CRITICAL: JWT_SECRET_KEY environment variable is not set. Cannot generate tokens.")
			http.Error(w, "Internal Server Error: Service configuration incomplete", http.StatusInternalServerError)
			return
		}

		accessToken, err := generateAccessToken(userID, secret)
		if err != nil {
			slog.Error("Failed to generate access token", "username", req.Username, "userID", userID, "err", err)
			http.Error(w, "Internal server error during login", http.StatusInternalServerError)
			return
		}

		refreshTokenValue, err := generateRefreshTokenValue()
		if err != nil {
			slog.Error("Failed to generate refresh token value", "username", req.Username, "userID", userID, "err", err)
			http.Error(w, "Internal server error during login", http.StatusInternalServerError)
			return
		}

		// Store the hashed refresh token in the database
		err = storeRefreshToken(db, userID, refreshTokenValue)
		if err != nil {
			// Error already logged in storeRefreshToken
			http.Error(w, "Internal server error during login", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.LoginResponse{ // Use types.LoginResponse
			AccessToken:  accessToken,
			RefreshToken: refreshTokenValue, // Return the raw refresh token value to the client
			UserID:       userID,
			FirstName:    firstName,
		})
	}
}

// AuthMiddleware validates the JWT token from the Authorization header.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ensure JWT secret key is configured before attempting validation
		secret := []byte(os.Getenv("JWT_SECRET_KEY"))
		if len(secret) == 0 {
			slog.Error("CRITICAL: JWT_SECRET_KEY environment variable is not set. Cannot validate token.", "url", r.URL)
			http.Error(w, "Internal Server Error: Service configuration incomplete", http.StatusInternalServerError)
			return
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
		claims := &AccessTokenClaims{} // Use AccessTokenClaims here
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// Check the signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return secret, nil // Use the fetched secret
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
		// Ensure we are parsing AccessTokenClaims
		accessTokenClaims, ok := token.Claims.(*AccessTokenClaims)
		if !ok || !token.Valid {
			slog.Warn("JWT validation failed: invalid claims type or token invalid", "url", r.URL)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		userID := accessTokenClaims.UserID
		if userID <= 0 {
			// Should not happen if token generation is correct, but check defensively
			slog.Error("Invalid user ID found in valid access token claims", "url", r.URL, "userID", userID)
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
	// Use the same logic as generateAccessToken, potentially with different expiry for tests if desired
	secret := []byte(os.Getenv("JWT_SECRET_KEY"))
	if len(secret) == 0 {
		return "", errors.New("JWT_SECRET_KEY not set for test JWT generation")
	}
	return generateAccessToken(userID, secret)
}

// HandleRefresh handles requests to refresh an access token using a refresh token.
func HandleRefresh(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RefreshTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		if req.RefreshToken == "" {
			http.Error(w, "Refresh token is required", http.StatusBadRequest)
			return
		}

		// Validate the refresh token against the database
		userID, err := validateRefreshToken(db, req.RefreshToken)
		if err != nil {
			// Errors logged in validateRefreshToken
			http.Error(w, err.Error(), http.StatusUnauthorized) // Return specific error (invalid/expired)
			return
		}

		// Refresh token is valid, generate a new access token
		secret := []byte(os.Getenv("JWT_SECRET_KEY"))
		if len(secret) == 0 {
			slog.Error("CRITICAL: JWT_SECRET_KEY environment variable is not set. Cannot generate access token during refresh.")
			http.Error(w, "Internal Server Error: Service configuration incomplete", http.StatusInternalServerError)
			return
		}

		newAccessToken, err := generateAccessToken(userID, secret)
		if err != nil {
			slog.Error("Failed to generate new access token during refresh", "user_id", userID, "err", err)
			http.Error(w, "Internal server error during token refresh", http.StatusInternalServerError)
			return
		}

		// --- Optional: Refresh Token Rotation ---
		// If implementing rotation, generate a new refresh token value here,
		// store it (deleting the old one), and return it in the response.
		// For simplicity, we are not rotating refresh tokens in this example.
		// --- End Optional Rotation ---

		slog.Info("Access token refreshed successfully", "user_id", userID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.RefreshTokenResponse{
			AccessToken: newAccessToken,
			// RefreshToken: newRefreshTokenValue, // Include if rotating
		})
	}
}

// HandleVerify handles requests to verify the current access token and return basic user info.
// It relies on AuthMiddleware to validate the token first.
func HandleVerify(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// AuthMiddleware should have already run and put the user ID in the context.
		userID, ok := GetUserIDFromContext(r.Context())
		if !ok {
			// This should not happen if AuthMiddleware is working correctly
			slog.Error("User ID not found in context after AuthMiddleware in HandleVerify", "url", r.URL)
			http.Error(w, "Authentication context error", http.StatusInternalServerError)
			return
		}

		// Fetch user's first name (or any other needed info)
		var firstName string
		err := db.QueryRow("SELECT first_name FROM users WHERE id = ?", userID).Scan(&firstName)
		if err != nil {
			if err == sql.ErrNoRows {
				slog.Error("User ID from valid token not found in database", "user_id", userID)
				http.Error(w, "User not found", http.StatusUnauthorized) // Treat as unauthorized
			} else {
				slog.Error("Database error fetching user info for verification", "user_id", userID, "err", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}

		slog.Debug("Token verified successfully, returning user info", "user_id", userID)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(types.VerifyResponse{
			UserID:    userID,
			FirstName: firstName,
		})
	}
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
		var req types.PartnerRegistrationRequest // Use types.PartnerRegistrationRequest
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
		json.NewEncoder(w).Encode(types.PartnerRegistrationResponse{ // Use types.PartnerRegistrationResponse
			Message: "Users registered and partnered successfully",
			User1ID: user1ID,
			User2ID: user2ID,
		})
	}
}
