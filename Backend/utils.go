package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
)
// ------------------------------------------------------------------------------------------------	
// ---------------- Utility Functions ----------------
// ------------------------------------------------------------------------------------------------

// writeJSONError writes an error response in JSON format.
func writeJSONError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ResponsePayload{Message: message, Success: false})
}

func randomIDHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getJWTSecret() []byte {
	// For production, override via env JWT_SECRET
	return []byte(getenv("JWT_SECRET", "dev-secret"))
}


