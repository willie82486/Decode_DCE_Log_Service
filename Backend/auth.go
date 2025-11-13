package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"github.com/golang-jwt/jwt/v5"
)

// --- JWT Auth helpers ---
func parseAndValidateToken(tokenStr string) (jwt.MapClaims, error) {
	tok, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return getJWTSecret(), nil
	})
	if err != nil || !tok.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims")
	}
	return claims, nil
}

func withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if len(auth) < 8 || auth[:7] != "Bearer " {
			writeJSONError(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		tokenStr := auth[7:]
		if _, err := parseAndValidateToken(tokenStr); err != nil {
			writeJSONError(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if len(auth) < 8 || auth[:7] != "Bearer " {
			writeJSONError(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		tokenStr := auth[7:]
		claims, err := parseAndValidateToken(tokenStr)
		if err != nil {
			writeJSONError(w, "Unauthorized.", http.StatusUnauthorized)
			return
		}
		role, _ := claims["role"].(string)
		if role != "admin" {
			writeJSONError(w, "Forbidden.", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// loginHandler handles user authentication.
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, "Only POST requests are accepted", http.StatusMethodNotAllowed)
		return
	}

	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(LoginResponse{Message: "Invalid request format.", Success: false})
		return
	}

	var storedPassword, role string
	err := db.QueryRow("SELECT password, role FROM users WHERE username = ?", creds.Username).Scan(&storedPassword, &role)
	if err != nil || storedPassword != creds.Password {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(LoginResponse{Message: "Invalid username or password.", Success: false})
		return
	}

	// Issue JWT token
	claims := jwt.MapClaims{
		"sub":  creds.Username,
		"role": role,
		"iss":  "dce-fw-service",
		"exp":  jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
		"iat":  jwt.NewNumericDate(time.Now()),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(getJWTSecret())
	if err != nil {
		writeJSONError(w, "Failed to issue token.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Message: "Authentication successful.",
		Success: true,
		Role:    role,
		Token:   signed,
	})
}


