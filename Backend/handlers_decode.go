package main

import (
	"database/sql"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// ------------------------------------------------------------------------------------------------	
// ---------------- Handler Functions ----------------
// ------------------------------------------------------------------------------------------------

// extractBuildIDFromELF tries to read GNU Build ID from ELF notes via `readelf -n`.
// Fallback: use SHA1 of file bytes if GNU Build ID is not present or tool missing.
func extractBuildIDFromELF(elfPath string) (string, error) {
	out, err := exec.Command("readelf", "-n", elfPath).CombinedOutput()
	if err == nil {
		text := string(out)
		const key = "Build ID:"
		if idx := strings.Index(text, key); idx >= 0 {
			rest := text[idx+len(key):]
			lineEnd := strings.IndexAny(rest, "\r\n")
			if lineEnd >= 0 {
				rest = rest[:lineEnd]
			}
			buildID := strings.TrimSpace(rest)
			if buildID != "" {
				return buildID, nil
			}
		}
	}
	// Fallback: SHA1 of file bytes
	f, err := os.Open(elfPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractBuildIDFromEncodedLog runs: hexdump -C <log> | head -n 4
// Then collects bytes at offsets 0x20..0x33 (total 20 bytes) as lower-case 40-hex BuildID.
func extractBuildIDFromEncodedLog(encodedLogPath string) (string, error) {
	// Use sh -c to support the pipeline (alpine base image doesn't include bash)
	cmd := exec.Command("sh", "-c", fmt.Sprintf("hexdump -C %q | head -n 4", encodedLogPath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("hexdump failed: %v, output: %s", err, string(out))
	}
	text := strings.ToLower(string(out))
	// Capture 16 bytes from 0x20 line and first 4 bytes from 0x30 line
	re20 := regexp.MustCompile(`(?m)^00000020\s+([0-9a-f]{2}(?:\s+[0-9a-f]{2}){15})`)
	re30 := regexp.MustCompile(`(?m)^00000030\s+([0-9a-f]{2}(?:\s+[0-9a-f]{2}){3})`)
	m20 := re20.FindStringSubmatch(text)
	m30 := re30.FindStringSubmatch(text)
	if m20 == nil || m30 == nil {
		return "", fmt.Errorf("build id bytes not found in hexdump output")
	}
	hex20 := strings.ReplaceAll(m20[1], " ", "")
	hex30 := strings.ReplaceAll(m30[1], " ", "")
	buildID := hex20 + hex30
	if len(buildID) != 40 {
		return "", fmt.Errorf("unexpected build id length: %d", len(buildID))
	}
	return buildID, nil
}

// decodeHandlerMultipart handles multipart upload of dce-enc.log and returns dce-decoded.log as attachment
func decodeHandlerMultipart(w http.ResponseWriter, r *http.Request) {
	// Parse multipart with a reasonable max memory (files will be stored in temp files if larger)
	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB
		http.Error(w, "Invalid multipart form data.", http.StatusBadRequest)
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

	// Determine pushtag (optional; default to 'auto' when omitted)
	pushtag := r.FormValue("pushtag")
	if pushtag == "" {
		pushtag = "auto"
	}
	// Determine buildID: prefer provided value, otherwise extract from uploaded log
	buildID := r.FormValue("buildId")
	if buildID == "" {
		var err error
		buildID, err = extractBuildIDFromEncodedLog(encodedLogPath)
		if err != nil || buildID == "" {
			http.Error(w, "Failed to extract Build ID from uploaded log.", http.StatusBadRequest)
			return
		}
	}
	// Expose build id for easier debugging (client can read from headers on both success/failure)
	w.Header().Set("X-Build-Id", buildID)
	log.Printf("Decoded BuildID from log: %s", buildID)

	// Load ELF by buildId from DB
	elfFileName, elfBlob, err := getELFByBuildID(buildID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, fmt.Sprintf("ELF not found for buildId %s. Please upload/fetch the matching ELF in Admin.", buildID), http.StatusNotFound)
			return
		}
		log.Printf("DB error while fetching ELF by buildId %s: %v", buildID, err)
		http.Error(w, "Internal server error: database failure while fetching ELF.", http.StatusInternalServerError)
		return
	}
	elfPath := filepath.Join(workDir, elfFileName)
	if err := os.WriteFile(elfPath, elfBlob, 0644); err != nil {
		http.Error(w, "Internal server error: Cannot write ELF temp file.", http.StatusInternalServerError)
		return
	}

	// Execute nvlog_decoder decode
	decodedLogFile := filepath.Join(workDir, "dce-decoded.log")
	// Important: -e expects a valid file path or base path. Our stored filename may already
	// include suffixes like "__<pushtag>__<buildId>". Passing additional suffixes would break the path.
	// Therefore, pass the exact path we just wrote.
	elfParam := elfPath
	decoderCmd := exec.Command(
		"nvlog_decoder",
		"-d", "none",
		"-i", encodedLogPath,
		"-o", decodedLogFile,
		"-e", elfParam,
		"-f", "DCE",
	)
	if out, err := decoderCmd.CombinedOutput(); err != nil {
		log.Printf("nvlog_decoder failed (buildId=%s, pushtag=%s): %v\nCmdOut:\n%s", buildID, pushtag, err, string(out))
		http.Error(w, "Error: Log decoder tool failed to run or produced an error.", http.StatusInternalServerError)
		return
	}
	// Ensure output file exists and is non-empty; nvlog_decoder might exit 0 but not write output
	if fi, err := os.Stat(decodedLogFile); err != nil || fi.Size() == 0 {
		log.Printf("nvlog_decoder produced no output file (buildId=%s, pushtag=%s). Path=%s err=%v size=%d",
			buildID, pushtag, decodedLogFile, err, func() int64 { if err == nil { return fi.Size() }; return -1 }())
		http.Error(w, "Error: Decoder produced no output. Please verify input log and ELF mapping.", http.StatusInternalServerError)
		return
	}

	// Return dce-decoded.log as downloadable attachment
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", "attachment; filename=\"dce-decoded.log\"")
	w.Header().Set("X-ELF-File", filepath.Base(elfPath))
	http.ServeFile(w, r, decodedLogFile)
}


