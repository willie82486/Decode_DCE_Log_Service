package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Handles IT admin user management.
func adminUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodPost {
		var newUserPayload NewAdminUser
		if err := json.NewDecoder(r.Body).Decode(&newUserPayload); err != nil {
			writeJSONError(w, "Invalid request format for adding user.", http.StatusBadRequest)
			return
		}
		if newUserPayload.Username == "" || newUserPayload.Password == "" || (newUserPayload.Role != "admin" && newUserPayload.Role != "user") {
			writeJSONError(w, "Username and password cannot be empty.", http.StatusBadRequest)
			return
		}
		newID, err := randomIDHex(12)
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
			u.Password = ""
			usersList = append(usersList, u)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": fmt.Sprintf("Fetched %d IT admin users.", len(usersList)),
			"users":   usersList,
		})
		return
	} else if r.Method == http.MethodDelete {
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
		json.NewEncoder(w).Encode(ResponsePayload{Message: "User deleted.", Success: true})
		return
	}
	writeJSONError(w, "Only GET, POST and DELETE requests are supported.", http.StatusMethodNotAllowed)
}

// POST JSON {pushtag, url} -> fetch archives, extract ELF, read BuildID, store to DB
func adminElvesByURLHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeJSONError(w, "Only POST is supported.", http.StatusMethodNotAllowed)
		return
	}
	var req PushtagMapping
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request body.", http.StatusBadRequest)
		return
	}
	if req.Pushtag == "" || req.URL == "" {
		writeJSONError(w, "Pushtag and URL cannot be empty.", http.StatusBadRequest)
		return
	}
	workDir, err := ioutil.TempDir("", "dce-elf-fetch-")
	if err != nil {
		writeJSONError(w, "Internal error: temp dir.", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(workDir)

	tbz2File := filepath.Join(workDir, "full_linux_for_tegra.tbz2")
	if err := exec.Command("curl", "-L", fmt.Sprintf("%s/full_linux_for_tegra.tbz2", req.URL), "-o", tbz2File).Run(); err != nil {
		writeJSONError(w, "Failed to download full_linux_for_tegra.tbz2.", http.StatusBadRequest)
		return
	}
	if err := exec.Command("tar", "-xjf", tbz2File, "-C", workDir).Run(); err != nil {
		writeJSONError(w, "Failed to extract main archive.", http.StatusInternalServerError)
		return
	}
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
		writeJSONError(w, "host_overlay_deployed.tbz2 not found.", http.StatusInternalServerError)
		return
	}
	hostOverlayDir := filepath.Join(workDir, "host_overlay")
	os.Mkdir(hostOverlayDir, 0755)
	if err := exec.Command("tar", "-xjf", bTbz2Path, "-C", hostOverlayDir).Run(); err != nil {
		writeJSONError(w, "Failed to extract host overlay archive.", http.StatusInternalServerError)
		return
	}
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
		writeJSONError(w, "ELF not found in overlay.", http.StatusInternalServerError)
		return
	}
	buildID, err := extractBuildIDFromELF(elfPath)
	if err != nil || buildID == "" {
		writeJSONError(w, "Failed to extract Build ID from ELF.", http.StatusInternalServerError)
		return
	}
	elfBytes, err := os.ReadFile(elfPath)
	if err != nil {
		writeJSONError(w, "Failed to read ELF.", http.StatusInternalServerError)
		return
	}
	rename := fmt.Sprintf("display-t234-dce-log.elf__%s__%s", req.Pushtag, buildID)
	if err := storeELF(buildID, rename, elfBytes); err != nil {
		writeJSONError(w, "Failed to store ELF.", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"message":     "ELF fetched and stored.",
		"buildId":     buildID,
		"elfFileName": rename,
	})
}

// performByURLJob runs the by-URL flow in background and reports steps to job
func performByURLJob(job *ByURLJob) {
	step := 0
	emitStep := func(msg string) {
		job.broadcast(ByURLEvent{Type: "step", Message: msg, StepIndex: step, TotalSteps: job.Total})
		step++
	}
	emitErr := func(err error) {
		job.broadcast(ByURLEvent{Type: "error", Message: err.Error(), StepIndex: step, TotalSteps: job.Total})
	}
	emitDone := func(buildID, elfName string) {
		job.broadcast(ByURLEvent{Type: "done", Message: "Completed.", StepIndex: step, TotalSteps: job.Total, BuildID: buildID, ElfName: elfName})
	}

	emitStep("Creating temp workspace...")
	workDir, err := ioutil.TempDir("", "dce-elf-fetch-")
	if err != nil { emitErr(fmt.Errorf("temp dir: %w", err)); return }
	defer os.RemoveAll(workDir)

	emitStep("Downloading full_linux_for_tegra.tbz2...")
	tbz2File := filepath.Join(workDir, "full_linux_for_tegra.tbz2")
	if err := exec.CommandContext(job.ctx, "curl", "-L", fmt.Sprintf("%s/full_linux_for_tegra.tbz2", job.URL), "-o", tbz2File).Run(); err != nil {
		emitErr(fmt.Errorf("download failed: %w", err)); return
	}

	emitStep("Extracting main archive...")
	if err := exec.CommandContext(job.ctx, "tar", "-xjf", tbz2File, "-C", workDir).Run(); err != nil {
		emitErr(fmt.Errorf("extract main: %w", err)); return
	}

	emitStep("Locating host_overlay_deployed.tbz2...")
	bTbz2Path := ""
	filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil { return nil }
		if info.Name() == "host_overlay_deployed.tbz2" { bTbz2Path = path; return filepath.SkipDir }
		return nil
	})
	if bTbz2Path == "" { emitErr(fmt.Errorf("host_overlay_deployed.tbz2 not found")); return }

	emitStep("Extracting host overlay...")
	hostOverlayDir := filepath.Join(workDir, "host_overlay")
	os.Mkdir(hostOverlayDir, 0755)
	if err := exec.CommandContext(job.ctx, "tar", "-xjf", bTbz2Path, "-C", hostOverlayDir).Run(); err != nil {
		emitErr(fmt.Errorf("extract overlay: %w", err)); return
	}

	emitStep("Searching for display-t234-dce-log.elf...")
	elfPath := ""
	filepath.Walk(hostOverlayDir, func(path string, info os.FileInfo, err error) error {
		if err != nil { return nil }
		if info.Name() == "display-t234-dce-log.elf" { elfPath = path; return filepath.SkipDir }
		return nil
	})
	if elfPath == "" { emitErr(fmt.Errorf("ELF not found in overlay")); return }

	emitStep("Extracting Build ID from ELF...")
	buildID, err := extractBuildIDFromELF(elfPath)
	if err != nil || buildID == "" { emitErr(fmt.Errorf("extract build id failed")); return }

	emitStep("Reading ELF bytes...")
	elfBytes, err := os.ReadFile(elfPath)
	if err != nil { emitErr(fmt.Errorf("read elf: %w", err)); return }
	elfName := fmt.Sprintf("display-t234-dce-log.elf__%s__%s", job.Pushtag, buildID)

	emitStep("Storing ELF to database...")
	if err := storeELF(buildID, elfName, elfBytes); err != nil { emitErr(fmt.Errorf("store elf: %w", err)); return }

	emitStep("Completed.")
	emitDone(buildID, elfName)
}

// cancel a running by-URL job
func adminElvesByURLCancelHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeJSONError(w, "Only POST is supported.", http.StatusMethodNotAllowed)
		return
	}
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		writeJSONError(w, "Missing jobId.", http.StatusBadRequest)
		return
	}
	job, ok := byURLJobs.Get(jobID)
	if !ok {
		writeJSONError(w, "job not found.", http.StatusNotFound)
		return
	}
	if job.Status != JobRunning {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "job not running"})
		return
	}
	// cancel context; goroutine will emit error and exit
	job.cancelFunc()
	job.broadcast(ByURLEvent{Type: "error", Message: "cancelled by user", StepIndex: job.StepIndex, TotalSteps: job.Total})
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "cancel requested"})
}

// clear a job (only if not running)
func adminElvesByURLClearHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeJSONError(w, "Only POST is supported.", http.StatusMethodNotAllowed)
		return
	}
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		writeJSONError(w, "Missing jobId.", http.StatusBadRequest)
		return
	}
	job, ok := byURLJobs.Get(jobID)
	if !ok {
		// already cleared
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "job not found; treated as cleared"})
		return
	}
	if job.Status == JobRunning {
		writeJSONError(w, "cannot clear a running job; cancel first", http.StatusBadRequest)
		return
	}
	byURLJobs.Remove(jobID)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "job cleared"})
}
// POST {pushtag,url} -> start or reuse job; returns jobId
func adminElvesByURLStartHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeJSONError(w, "Only POST is supported.", http.StatusMethodNotAllowed)
		return
	}
	var req PushtagMapping
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, "Invalid request body.", http.StatusBadRequest)
		return
	}
	if req.Pushtag == "" || req.URL == "" {
		writeJSONError(w, "Pushtag and URL cannot be empty.", http.StatusBadRequest)
		return
	}
	job, created := byURLJobs.GetOrCreate(req.Pushtag, req.URL)
	if created {
		go performByURLJob(job)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"jobId":   job.ID,
		"created": created,
	})
}

// GET ?jobId=... -> current snapshot of a job
func adminElvesByURLStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		writeJSONError(w, "Only GET is supported.", http.StatusMethodNotAllowed)
		return
	}
	jobID := r.URL.Query().Get("jobId")
	if jobID == "" {
		writeJSONError(w, "Missing jobId.", http.StatusBadRequest)
		return
	}
	job, ok := byURLJobs.Get(jobID)
	if !ok {
		writeJSONError(w, "job not found.", http.StatusNotFound)
		return
	}
	snap := job.snapshot()
	var payload map[string]interface{}
	_ = json.Unmarshal([]byte(snap.Message), &payload)
	payload["success"] = true
	payload["jobId"] = job.ID
	json.NewEncoder(w).Encode(payload)
}

// SSE streaming of progress for fetch-by-url (by jobId or by pushtag+url)
func adminElvesByURLStreamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, "Only GET is supported.", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, "Streaming unsupported.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	jobID := r.URL.Query().Get("jobId")
	var job *ByURLJob
	if jobID != "" {
		var ok bool
		job, ok = byURLJobs.Get(jobID)
		if !ok {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", "job not found")
			flusher.Flush()
			return
		}
	} else {
		// backward-compat: accept pushtag & url to create or reuse job
		pushtag := r.URL.Query().Get("pushtag")
		srcURL := r.URL.Query().Get("url")
		if pushtag == "" || srcURL == "" {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", "Missing jobId or (pushtag,url)")
			flusher.Flush()
			return
		}
		var created bool
		job, created = byURLJobs.GetOrCreate(pushtag, srcURL)
		if created {
			go performByURLJob(job)
		}
	}

	// subscribe and stream events
	ch := job.addSubscriber()
	defer job.removeSubscriber(ch)

	// initial snapshot sent implicitly by addSubscriber via catch-up; also send meta jobId
	meta := map[string]string{"jobId": job.ID}
	b, _ := json.Marshal(meta)
	fmt.Fprintf(w, "event: meta\ndata: %s\n\n", string(b))
	flusher.Flush()

	notify := w.(http.CloseNotifier).CloseNotify()
	for {
		select {
		case ev := <-ch:
			switch ev.Type {
			case "step":
				fmt.Fprintf(w, "event: step\ndata: %s\n\n", ev.Message)
			case "error":
				fmt.Fprintf(w, "event: error\ndata: %s\n\n", ev.Message)
			case "done":
				payload := map[string]string{"buildId": ev.BuildID, "elfFileName": ev.ElfName}
				b, _ := json.Marshal(payload)
				fmt.Fprintf(w, "event: done\ndata: %s\n\n", string(b))
			case "snapshot":
				// not used here
			}
			flusher.Flush()
		case <-notify:
			return
		}
	}
}

// adminElvesUploadHandler: POST multipart form with field 'elf'
func adminElvesUploadHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		writeJSONError(w, "Only POST is supported.", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		writeJSONError(w, "Invalid multipart form.", http.StatusBadRequest)
		return
	}
	f, header, err := r.FormFile("elf")
	if err != nil {
		writeJSONError(w, "Missing 'elf' file field.", http.StatusBadRequest)
		return
	}
	defer f.Close()

	workDir, err := ioutil.TempDir("", "dce-elf-upload-")
	if err != nil {
		writeJSONError(w, "Internal error: temp dir.", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(workDir)

	tmpElf := filepath.Join(workDir, header.Filename)
	out, err := os.Create(tmpElf)
	if err != nil {
		writeJSONError(w, "Internal error: cannot write temp file.", http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(out, f); err != nil {
		out.Close()
		writeJSONError(w, "Internal error: cannot save ELF.", http.StatusInternalServerError)
		return
	}
	out.Close()

	buildID, err := extractBuildIDFromELF(tmpElf)
	if err != nil || buildID == "" {
		writeJSONError(w, "Failed to extract Build ID from uploaded ELF.", http.StatusInternalServerError)
		return
	}
	elfBytes, err := os.ReadFile(tmpElf)
	if err != nil {
		writeJSONError(w, "Failed to read uploaded ELF.", http.StatusInternalServerError)
		return
	}
	elfFileName := header.Filename
	// Preserve original filename if it already matches: display-t234-dce-log.elf__<pushtag>__<40-hex>
	isHex40 := func(s string) bool {
		if len(s) != 40 {
		 return false
		}
		for _, ch := range s {
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			 return false
			}
		}
		return true
	}
	if strings.HasPrefix(elfFileName, "display-t234-dce-log.elf__") {
		parts := strings.Split(elfFileName, "__")
		// expect at least ["display-t234-dce-log.elf", "<pushtag>", "<40hex>"]
		if len(parts) >= 3 && isHex40(parts[len(parts)-1]) {
			// keep as-is (do not append extracted buildID)
		} else {
			// fallback to normalized filename with extracted buildID
			elfFileName = fmt.Sprintf("display-t234-dce-log.elf__%s", buildID)
		}
	} else {
		// no recognizable prefix -> fallback to normalized filename with extracted buildID
		elfFileName = fmt.Sprintf("display-t234-dce-log.elf__%s", buildID)
	}
	if err := storeELF(buildID, elfFileName, elfBytes); err != nil {
		writeJSONError(w, "Failed to store uploaded ELF.", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"message":     "ELF uploaded and stored.",
		"buildId":     buildID,
		"elfFileName": elfFileName,
	})
}

// GET: list all build_id + elf name; DELETE: delete by buildId
func adminElvesListHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query("SELECT build_id, elf_filename FROM build_elves ORDER BY created_at DESC")
		if err != nil {
			writeJSONError(w, "Failed to fetch ELF records.", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		list := make([]ElfRecord, 0, 64)
		for rows.Next() {
			var e ElfRecord
			if err := rows.Scan(&e.BuildID, &e.ElfFileName); err != nil {
				writeJSONError(w, "Failed to read ELF records.", http.StatusInternalServerError)
				return
			}
			list = append(list, e)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "elves": list})
		return
	case http.MethodDelete:
		buildID := r.URL.Query().Get("buildId")
		if buildID == "" {
			writeJSONError(w, "Missing buildId.", http.StatusBadRequest)
			return
		}
		res, err := db.Exec("DELETE FROM build_elves WHERE build_id = ?", buildID)
		if err != nil {
			writeJSONError(w, "Failed to delete ELF record.", http.StatusInternalServerError)
			return
		}
		aff, _ := res.RowsAffected()
		if aff == 0 {
			writeJSONError(w, "ELF record not found.", http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "ELF record deleted."})
		return
	default:
		writeJSONError(w, "Only GET and DELETE are supported.", http.StatusMethodNotAllowed)
		return
	}
}


