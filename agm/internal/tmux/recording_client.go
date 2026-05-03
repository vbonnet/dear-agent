package tmux

import (
	"encoding/json"
	"os"
	"time"
)

// RecordedEvent captures a TmuxClient method call for golden file testing.
type RecordedEvent struct {
	Timestamp time.Time `json:"ts"`
	Method    string    `json:"method"`
	Args      []string  `json:"args"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
}

// RecordingTmuxClient wraps another TmuxClient and records all calls.
type RecordingTmuxClient struct {
	Inner  TmuxClient
	Events []RecordedEvent
}

func NewRecordingTmuxClient(inner TmuxClient) *RecordingTmuxClient {
	return &RecordingTmuxClient{Inner: inner}
}

func (r *RecordingTmuxClient) record(method string, args []string, result string, err error) {
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	r.Events = append(r.Events, RecordedEvent{
		Timestamp: time.Now(),
		Method:    method,
		Args:      args,
		Result:    result,
		Error:     errStr,
	})
}

func (r *RecordingTmuxClient) CreateSession(name string) error {
	err := r.Inner.CreateSession(name)
	r.record("CreateSession", []string{name}, "", err)
	return err
}

func (r *RecordingTmuxClient) KillSession(name string) error {
	err := r.Inner.KillSession(name)
	r.record("KillSession", []string{name}, "", err)
	return err
}

func (r *RecordingTmuxClient) SendKeys(session, keys string) error {
	err := r.Inner.SendKeys(session, keys)
	r.record("SendKeys", []string{session, keys}, "", err)
	return err
}

func (r *RecordingTmuxClient) CapturePane(session string) (string, error) {
	result, err := r.Inner.CapturePane(session)
	r.record("CapturePane", []string{session}, result, err)
	return result, err
}

func (r *RecordingTmuxClient) ListSessions() ([]string, error) {
	result, err := r.Inner.ListSessions()
	r.record("ListSessions", nil, "", err)
	return result, err
}

func (r *RecordingTmuxClient) IsSessionAlive(name string) (bool, error) {
	alive, err := r.Inner.IsSessionAlive(name)
	r.record("IsSessionAlive", []string{name}, "", err)
	return alive, err
}

// SaveGoldenFile writes recorded events to a JSON file.
func (r *RecordingTmuxClient) SaveGoldenFile(path string) error {
	data, err := json.MarshalIndent(r.Events, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}
