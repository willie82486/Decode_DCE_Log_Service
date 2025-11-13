package main

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
	Token   string `json:"token"`
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

// ELF record for DB storage (used by adminElvesListHandler responses)
type ElfRecord struct {
	BuildID     string `json:"buildId"`
	ElfFileName string `json:"elfFileName"`
}


