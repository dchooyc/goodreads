package crawl

import (
	"errors"
	"io/fs"
	"sync"
	"time"
)

// TargetState is the per-target checkpoint record.
type TargetState struct {
	ExpectedGoodreadsWorkID string   `json:"expected_goodreads_work_id"`
	Status                  string   `json:"status"`
	AttemptedURLs           []string `json:"attempted_urls"`
	LastError               string   `json:"last_error,omitempty"`
	UpdatedAt               string   `json:"updated_at"`
}

type stateFile struct {
	Targets map[string]TargetState `json:"targets"`
}

// State is a checkpoint store that survives interruption. Every update is
// flushed to disk atomically (temp file + rename) so a killed run can resume
// without re-fetching completed targets.
type State struct {
	path string

	mu      sync.Mutex
	targets map[string]TargetState
}

// LoadState reads an existing checkpoint file, or starts empty if the file
// does not exist yet.
func LoadState(path string) (*State, error) {
	s := &State{path: path, targets: make(map[string]TargetState)}

	var f stateFile
	err := ReadJSONFile(path, &f)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return s, nil
		}
		return nil, err
	}
	if f.Targets != nil {
		s.targets = f.Targets
	}
	return s, nil
}

// Get returns the recorded state for a work ID.
func (s *State) Get(workID string) (TargetState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ts, ok := s.targets[workID]
	return ts, ok
}

// IsDone reports whether a target already reached a terminal status and can
// be skipped on resume. HTTP failures and blocks are retryable, so they are
// not terminal.
func (s *State) IsDone(workID string) bool {
	ts, ok := s.Get(workID)
	if !ok {
		return false
	}
	switch ts.Status {
	case StatusRecovered, StatusKeptFromPrevious, StatusBelowThreshold, StatusManualReview, StatusParseFailed, StatusUpdated:
		return true
	}
	return false
}

// Update records the outcome for a target and immediately checkpoints to
// disk.
func (s *State) Update(workID, status string, attemptedURLs []string, lastErr string) error {
	s.mu.Lock()
	s.targets[workID] = TargetState{
		ExpectedGoodreadsWorkID: workID,
		Status:                  status,
		AttemptedURLs:           attemptedURLs,
		LastError:               lastErr,
		UpdatedAt:               time.Now().UTC().Format(time.RFC3339),
	}
	snapshot := stateFile{Targets: make(map[string]TargetState, len(s.targets))}
	for k, v := range s.targets {
		snapshot.Targets[k] = v
	}
	s.mu.Unlock()

	return WriteJSONFileAtomic(s.path, snapshot)
}

// Counts returns how many targets are recorded per status.
func (s *State) Counts() map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := make(map[string]int)
	for _, ts := range s.targets {
		counts[ts.Status]++
	}
	return counts
}
