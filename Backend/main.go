package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	// "time"
	_ "github.com/go-sql-driver/mysql"
)

// --- Struct Definitions ---

// ResponsePayload defines the decoded result or general response sent to the frontend.
type ResponsePayload struct {
	DecodedLog string `json:"decodedLog"`
	ElfFileName string `json:"elfFileName"`
	Message    string `json:"message"`
	Success    bool   `json:"success"`
}

// LoginResponse defines the response for a successful login, including role information.
type LoginResponse struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
	Role    string `json:"role"` // "admin" or "user"
}

// NewAdminUser defines the request body when an admin adds a user.
type NewAdminUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"` // "admin" or "user"
}

// User defines the IT admin user data structure.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Password string `json:"password"` // Note: plaintext for simplicity (consider hashing in production)
	Role     string `json:"role"`
}

// Pushtag mapping request
type PushtagMapping struct {
	Pushtag string `json:"pushtag"`
	URL     string `json:"url"`
}

// --- Database ---
var db *sql.DB

// --- Utility Functions ---

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

func initDB() error {
	dsn := getenv("MYSQL_DSN", "dce_user:dce_pass@tcp(mariadb:3306)/dce_logs?parseTime=true")
	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	// Reasonable pool settings
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(0)

	// Verify connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	// Create tables if not exist
	usersSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(64) PRIMARY KEY,
		username VARCHAR(255) NOT NULL UNIQUE,
		password VARCHAR(255) NOT NULL,
		role VARCHAR(32) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	if _, err := db.Exec(usersSQL); err != nil {
		return fmt.Errorf("create users: %w", err)
	}
	pushtagSQL := `
	CREATE TABLE IF NOT EXISTS pushtag_urls (
		pushtag VARCHAR(255) PRIMARY KEY,
		url TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	if _, err := db.Exec(pushtagSQL); err != nil {
		return fmt.Errorf("create pushtag_urls: %w", err)
	}

	// Seed default admin user if not exists
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", "nvidia").Scan(&count); err != nil {
		return fmt.Errorf("seed admin check: %w", err)
	}
	if count == 0 {
		newID, err := randomIDHex(12)
		if err != nil {
			return fmt.Errorf("seed admin id: %w", err)
		}
		if _, err := db.Exec("INSERT INTO users (id, username, password, role) VALUES (?, ?, ?, ?)",
			newID, "nvidia", "nvidia", "admin"); err != nil {
			return fmt.Errorf("seed admin insert: %w", err)
		}
		log.Printf("Seeded default admin user 'nvidia' with role 'admin'.")
	}
	return nil
}

// --- API Handlers ---

// healthzHandler is a lightweight health endpoint for liveness/readiness checks.
func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func getRemoteDirByPushtag(pushtag string) (string, error) {
	var url string
	err := db.QueryRow("SELECT url FROM pushtag_urls WHERE pushtag = ?", pushtag).Scan(&url)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("pushtag not found")
	}
	if err != nil {
		return "", err
	}
	return url, nil
}

// decodeHandlerMultipart handles multipart upload of dce-enc.log and returns dce-decoded.log as attachment
func decodeHandlerMultipart(w http.ResponseWriter, r *http.Request) {
	// Parse multipart with a reasonable max memory (files will be stored in temp files if larger)
	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB
		http.Error(w, "Invalid multipart form data.", http.StatusBadRequest)
		return
	}

	pushtag := r.FormValue("pushtag")
	buildID := r.FormValue("buildId")
	if pushtag == "" || buildID == "" {
		http.Error(w, "Missing required fields: pushtag, buildId.", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing uploaded file field 'file'.", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create temporary working directory
	workDir, err := ioutil.TempDir("", "dce-decode-")
	if err != nil {
		log.Printf("Error creating temp dir: %v", err)
		http.Error(w, "Internal server error: Cannot create temp directory.", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(workDir)

	// Save uploaded file to workDir as dce-enc.log
	encodedLogPath := filepath.Join(workDir, "dce-enc.log")
	out, err := os.Create(encodedLogPath)
	if err != nil {
		http.Error(w, "Internal server error: Cannot create temp file.", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		http.Error(w, "Internal server error: Cannot write uploaded file.", http.StatusInternalServerError)
		return
	}
	out.Close()
	log.Printf("Received upload file %s -> %s", header.Filename, encodedLogPath)

	// Resolve remote directory via DB mapping
	remoteDir, err := getRemoteDirByPushtag(pushtag)
	if err != nil {
		http.Error(w, "Error: Pushtag mapping not found.", http.StatusNotFound)
		return
	}

	// 1) Download full_linux_for_tegra.tbz2
	tbz2File := filepath.Join(workDir, "full_linux_for_tegra.tbz2")
	curlCmd := exec.Command("curl", "-L", fmt.Sprintf("%s/full_linux_for_tegra.tbz2", remoteDir), "-o", tbz2File)
	if err := curlCmd.Run(); err != nil {
		log.Printf("Curl download failed for %s: %v", pushtag, err)
		http.Error(w, "Error: Failed to download full_linux_for_tegra.tbz2. Check Pushtag.", http.StatusInternalServerError)
		return
	}

	// 2) Extract full_linux_for_tegra.tbz2
	tarCmd := exec.Command("tar", "-xjf", tbz2File, "-C", workDir)
	if err := tarCmd.Run(); err != nil {
		log.Printf("Error extracting main tbz2: %v", err)
		http.Error(w, "Error: Failed to extract main archive.", http.StatusInternalServerError)
		return
	}

	// 3) Find and extract host_overlay_deployed.tbz2
	bTbz2Path := ""
	filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Name() == "host_overlay_deployed.tbz2" {
			bTbz2Path = path
			return filepath.SkipDir
		}
		return nil
	})
	if bTbz2Path == "" {
		http.Error(w, "Error: host_overlay_deployed.tbz2 not found after first extraction.", http.StatusInternalServerError)
		return
	}

	// 4) Extract host_overlay_deployed.tbz2
	hostOverlayDir := filepath.Join(workDir, "host_overlay")
	os.Mkdir(hostOverlayDir, 0755)
	tarOverlayCmd := exec.Command("tar", "-xjf", bTbz2Path, "-C", hostOverlayDir)
	if err := tarOverlayCmd.Run(); err != nil {
		log.Printf("Error extracting host_overlay: %v", err)
		http.Error(w, "Error: Failed to extract host overlay archive.", http.StatusInternalServerError)
		return
	}

	// 5) Find ELF file
	elfPath := ""
	filepath.Walk(hostOverlayDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Name() == "display-t234-dce-log.elf" {
			elfPath = path
			return filepath.SkipDir
		}
		return nil
	})
	if elfPath == "" {
		http.Error(w, "Error: Target ELF file not found.", http.StatusInternalServerError)
		return
	}

	// 6) Execute nvlog_decoder decode
	decodedLogFile := filepath.Join(workDir, "dce-decoded.log")
	elfParam := fmt.Sprintf("%s__%s__%s", elfPath, pushtag, buildID)
	decoderCmd := exec.Command(
		"nvlog_decoder",
		"-i", encodedLogPath,
		"-o", decodedLogFile,
		"-e", elfParam,
		"-f", "DCE",
	)
	if err := decoderCmd.Run(); err != nil {
		log.Printf("nvlog_decoder execution failed: %v", err)
		http.Error(w, "Error: Log decoder tool failed to run or produced an error.", http.StatusInternalServerError)
		return
	}

	// 7) Return dce-decoded.log as downloadable attachment
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dce-decoded.log\"")
	w.Header().Set("X-ELF-File", filepath.Base(elfPath))
	http.ServeFile(w, r, decodedLogFile)
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
	if err == sql.ErrNoRows || (err == nil && storedPassword != creds.Password) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(LoginResponse{Message: "Invalid username or password.", Success: false})
		return
	}
	if err != nil {
		writeJSONError(w, "Internal server error.", http.StatusInternalServerError)
		return
	}

	// Login successful
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Message: "Authentication successful.",
		Success: true,
		Role:    role,
	})
}

// adminUsersHandler handles IT admin user management.
func adminUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost {
		// --- POST: Add an IT admin user ---
		var newUserPayload NewAdminUser
		if err := json.NewDecoder(r.Body).Decode(&newUserPayload); err != nil {
			writeJSONError(w, "Invalid request format for adding user.", http.StatusBadRequest)
			return
		}

		if newUserPayload.Username == "" || newUserPayload.Password == "" || (newUserPayload.Role != "admin" && newUserPayload.Role != "user") {
			writeJSONError(w, "Username and password cannot be empty.", http.StatusBadRequest)
			return
		}

		newID, err := randomIDHex(12) // 24 hex chars
		if err != nil {
			writeJSONError(w, "Internal server error: id generation.", http.StatusInternalServerError)
			return
		}
		_, err = db.Exec("INSERT INTO users (id, username, password, role) VALUES (?, ?, ?, ?)",
			newID, newUserPayload.Username, newUserPayload.Password, newUserPayload.Role)
		if err != nil {
			writeJSONError(w, fmt.Sprintf("Failed to add user: %v", err), http.StatusConflict)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(ResponsePayload{
			Message: fmt.Sprintf("IT admin user '%s' added successfully.", newUserPayload.Username),
			Success: true,
		})
		return

	} else if r.Method == http.MethodGet {
		// --- GET: List all IT admin users ---
		rows, err := db.Query(`
			SELECT id, username, password, role
			FROM users
			ORDER BY CASE WHEN role='admin' THEN 0 ELSE 1 END, username ASC`)
		if err != nil {
			writeJSONError(w, "Failed to fetch users.", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		usersList := make([]User, 0, 32)
		for rows.Next() {
			var u User
			if err := rows.Scan(&u.ID, &u.Username, &u.Password, &u.Role); err != nil {
				writeJSONError(w, "Failed to read users.", http.StatusInternalServerError)
				return
			}
			// Do not expose password in response
			u.Password = ""
			usersList = append(usersList, u)
		}

		w.WriteHeader(http.StatusOK)
		// Response shape: {"success": true, "users": [...]}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Fetched %d IT admin users.", len(usersList)),
			"users": usersList,
		})
		return
	} else if r.Method == http.MethodDelete {
		// --- DELETE: Remove a user by id (query param id) ---
		id := r.URL.Query().Get("id")
		if id == "" {
			writeJSONError(w, "Missing user id.", http.StatusBadRequest)
			return
		}
		res, err := db.Exec("DELETE FROM users WHERE id = ?", id)
		if err != nil {
			writeJSONError(w, "Failed to delete user.", http.StatusInternalServerError)
			return
		}
		aff, _ := res.RowsAffected()
		if aff == 0 {
			writeJSONError(w, "User not found.", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(ResponsePayload{
			Message: "User deleted.",
			Success: true,
		})
		return
	}

	// Default: method not supported
	w.WriteHeader(http.StatusMethodNotAllowed)
	json.NewEncoder(w).Encode(ResponsePayload{Message: "Only GET, POST and DELETE requests are supported.", Success: false})
}

// adminPushtagsHandler handles pushtag<->url mapping management.
func adminPushtagsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method == http.MethodPost {
		var req PushtagMapping
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, "Invalid request format for pushtag mapping.", http.StatusBadRequest)
			return
		}
		if req.Pushtag == "" || req.URL == "" {
			writeJSONError(w, "Pushtag and URL cannot be empty.", http.StatusBadRequest)
			return
		}
		_, err := db.Exec(`
			INSERT INTO pushtag_urls (pushtag, url) VALUES (?, ?)
			ON DUPLICATE KEY UPDATE url = VALUES(url)`, req.Pushtag, req.URL)
		if err != nil {
			writeJSONError(w, "Failed to upsert pushtag mapping.", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(ResponsePayload{
			Message: "Pushtag mapping saved.",
			Success: true,
		})
		return
	} else if r.Method == http.MethodGet {
		rows, err := db.Query("SELECT pushtag, url FROM pushtag_urls ORDER BY pushtag ASC")
		if err != nil {
			writeJSONError(w, "Failed to fetch pushtags.", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var mappings []PushtagMapping
		for rows.Next() {
			var m PushtagMapping
			if err := rows.Scan(&m.Pushtag, &m.URL); err != nil {
				writeJSONError(w, "Failed to read pushtags.", http.StatusInternalServerError)
				return
			}
			mappings = append(mappings, m)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"message":  fmt.Sprintf("Fetched %d pushtags.", len(mappings)),
			"pushtags": mappings,
		})
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
	json.NewEncoder(w).Encode(ResponsePayload{Message: "Only GET and POST requests are supported.", Success: false})
}

// main is the program entry point.
func main() {
	// Initialize database
	if err := initDB(); err != nil {
		log.Fatalf("DB init failed: %v", err)
	}

	mux := http.NewServeMux()
	
	// 1) Register handlers
	mux.HandleFunc("/api/decode", decodeHandlerMultipart)
	mux.HandleFunc("/api/login", loginHandler)
	mux.HandleFunc("/api/admin/users", adminUsersHandler)
	mux.HandleFunc("/api/admin/pushtags", adminPushtagsHandler)
	mux.HandleFunc("/healthz", healthzHandler)
	
	// 2) Use mux directly (CORS handled via Nginx same-origin)
	handler := mux
	
	log.Println("Starting server on :8080...")
	
	// 3) Start the server
	if err := http.ListenAndServe(":8080", handler); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}