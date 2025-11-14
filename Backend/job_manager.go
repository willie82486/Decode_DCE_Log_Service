package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
	"context"
)

type ByURLJobStatus string

const (
	JobRunning ByURLJobStatus = "running"
	JobDone    ByURLJobStatus = "done"
	JobError   ByURLJobStatus = "error"
)

type ByURLEvent struct {
	Type       string `json:"type"`        // step|error|done|snapshot
	Message    string `json:"message"`     // step message or error
	StepIndex  int    `json:"stepIndex"`   // 0-based
	TotalSteps int    `json:"totalSteps"`  // constant for this flow
	BuildID    string `json:"buildId"`     // when done
	ElfName    string `json:"elfFileName"` // when done
}

// ByURLJob holds state and subscribers for a long-running fetch-by-url job
type ByURLJob struct {
	ID        string
	Pushtag   string
	URL       string

	ctx        context.Context
	cancelFunc context.CancelFunc

	Steps     []string
	StepIndex int
	Total     int
	Status    ByURLJobStatus
	ErrMsg    string

	BuildID string
	ElfName string

	StartedAt time.Time
	UpdatedAt time.Time

	subscribers map[chan ByURLEvent]struct{}
	mu          sync.Mutex
}

func (j *ByURLJob) snapshot() ByURLEvent {
	j.mu.Lock()
	defer j.mu.Unlock()
	msg, _ := json.Marshal(map[string]interface{}{
		"steps":      j.Steps,
		"status":     j.Status,
		"stepIndex":  j.StepIndex,
		"totalSteps": j.Total,
		"buildId":    j.BuildID,
		"elfName":    j.ElfName,
	})
	return ByURLEvent{
		Type:       "snapshot",
		Message:    string(msg),
		StepIndex:  j.StepIndex,
		TotalSteps: j.Total,
		BuildID:    j.BuildID,
		ElfName:    j.ElfName,
	}
}

func (j *ByURLJob) broadcast(ev ByURLEvent) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.UpdatedAt = time.Now()
	switch ev.Type {
	case "step":
		j.Steps = append(j.Steps, ev.Message)
		j.StepIndex = ev.StepIndex
	case "error":
		j.Status = JobError
		j.ErrMsg = ev.Message
	case "done":
		j.Status = JobDone
		j.BuildID = ev.BuildID
		j.ElfName = ev.ElfName
	}
	for ch := range j.subscribers {
		select {
		case ch <- ev:
		default:
			// drop if slow
		}
	}
}

func (j *ByURLJob) addSubscriber() chan ByURLEvent {
	ch := make(chan ByURLEvent, 64)
	j.mu.Lock()
	j.subscribers[ch] = struct{}{}
	// send catch-up steps
	stepIdx := 0
	for idx, msg := range j.Steps {
		stepIdx = idx
		ch <- ByURLEvent{Type: "step", Message: msg, StepIndex: idx, TotalSteps: j.Total}
	}
	// send current terminal state if any
	switch j.Status {
	case JobError:
		ch <- ByURLEvent{Type: "error", Message: j.ErrMsg, StepIndex: stepIdx, TotalSteps: j.Total}
	case JobDone:
		ch <- ByURLEvent{Type: "done", Message: "Completed.", StepIndex: j.StepIndex, TotalSteps: j.Total, BuildID: j.BuildID, ElfName: j.ElfName}
	}
	j.mu.Unlock()
	return ch
}

func (j *ByURLJob) removeSubscriber(ch chan ByURLEvent) {
	j.mu.Lock()
	if _, ok := j.subscribers[ch]; ok {
		delete(j.subscribers, ch)
	}
	j.mu.Unlock()
	close(ch)
}

// JobManager manages ByURL jobs keyed by ID and also by (pushtag,url) to dedupe
type JobManager struct {
	mu         sync.Mutex
	jobsByID   map[string]*ByURLJob
	keyToJobID map[string]string
}

func NewJobManager() *JobManager {
	return &JobManager{
		jobsByID:   make(map[string]*ByURLJob),
		keyToJobID: make(map[string]string),
	}
}

func jobKey(pushtag, url string) string {
	return fmt.Sprintf("%s||%s", pushtag, url)
}

func (m *JobManager) Get(jobID string) (*ByURLJob, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobsByID[jobID]
	return j, ok
}

func (m *JobManager) GetOrCreate(pushtag, url string) (*ByURLJob, bool) {
	key := jobKey(pushtag, url)
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.keyToJobID[key]; ok {
		if j, ok2 := m.jobsByID[id]; ok2 {
			return j, false // existing
		}
	}
	// create
	id, _ := randomIDHex(12)
	ctx, cancel := context.WithCancel(context.Background())
	job := &ByURLJob{
		ID:          id,
		Pushtag:     pushtag,
		URL:         url,
		ctx:         ctx,
		cancelFunc:  cancel,
		Steps:       []string{"Starting..."},
		StepIndex:   0,
		Total:       11, // keep in sync with performByURLJob steps
		Status:      JobRunning,
		StartedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		subscribers: make(map[chan ByURLEvent]struct{}),
	}
	m.jobsByID[id] = job
	m.keyToJobID[key] = id
	return job, true
}

func (m *JobManager) Remove(jobID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j, ok := m.jobsByID[jobID]; ok {
		delete(m.jobsByID, jobID)
		delete(m.keyToJobID, jobKey(j.Pushtag, j.URL))
	}
}

// global instance
var byURLJobs = NewJobManager()

// --- TTL Reaper ---
// Config via env (with sane defaults)
var (
	finishedTTL   = parseDurationEnv("BYURL_JOB_FINISHED_TTL", "30m")
	runningTTL    = parseDurationEnv("BYURL_JOB_RUNNING_TTL", "12h")
	reaperEvery   = parseDurationEnv("BYURL_JOB_REAPER_INTERVAL", "1m")
)

func parseDurationEnv(key, def string) time.Duration {
	v := getenv(key, def)
	d, err := time.ParseDuration(v)
	if err != nil {
		return func() time.Duration { d2, _ := time.ParseDuration(def); return d2 }()
	}
	return d
}

func startByURLJobReaper() {
	ticker := time.NewTicker(reaperEvery)
	go func() {
		for range ticker.C {
			now := time.Now()
			byURLJobs.mu.Lock()
			for id, job := range byURLJobs.jobsByID {
				// finished jobs: drop after finishedTTL
				if job.Status == JobDone || job.Status == JobError {
					if now.Sub(job.UpdatedAt) > finishedTTL {
						delete(byURLJobs.jobsByID, id)
						delete(byURLJobs.keyToJobID, jobKey(job.Pushtag, job.URL))
					}
					continue
				}
				// running jobs: if no update for too long, time out and remove (notify subscribers)
				if job.Status == JobRunning && now.Sub(job.UpdatedAt) > runningTTL {
					// best-effort notify
					job.broadcast(ByURLEvent{Type: "error", Message: "job timed out", StepIndex: job.StepIndex, TotalSteps: job.Total})
					delete(byURLJobs.jobsByID, id)
					delete(byURLJobs.keyToJobID, jobKey(job.Pushtag, job.URL))
				}
			}
			byURLJobs.mu.Unlock()
		}
	}()
}

