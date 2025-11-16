package main

import "net/http"

// ------------------------------------------------------------------------------------------------	
// ---------------- Handler Functions ----------------
// ------------------------------------------------------------------------------------------------

// healthzHandler is a lightweight health endpoint for liveness/readiness checks.
func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}


