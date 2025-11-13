package main

import (
	"log"
	"net/http"
)

func main() {
	// Initialize database
	if err := initDB(); err != nil {
		log.Fatalf("DB init failed: %v", err)
	}

	mux := http.NewServeMux()
	// Register handlers
	mux.HandleFunc("/api/decode", withAuth(decodeHandlerMultipart))
	mux.HandleFunc("/api/login", loginHandler)
	mux.HandleFunc("/api/admin/users", withAdmin(adminUsersHandler))
	mux.HandleFunc("/api/admin/elves/by-url", withAdmin(adminElvesByURLHandler))
	mux.HandleFunc("/api/admin/elves/by-url/stream", withAdmin(adminElvesByURLStreamHandler))
	mux.HandleFunc("/api/admin/elves/upload", withAdmin(adminElvesUploadHandler))
	mux.HandleFunc("/api/admin/elves", withAdmin(adminElvesListHandler))
	mux.HandleFunc("/healthz", healthzHandler)

	// Use mux directly (CORS handled via Nginx same-origin)
	handler := mux
	log.Println("Starting server on :8080...")
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}


