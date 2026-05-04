package tmux

import "fmt"

// FakeTmuxClient simulates tmux behavior with internal state.
// Used for hermetic testing without real tmux sessions.
type FakeTmuxClient struct {
	Sessions map[string]*FakeSession
}

// FakeSession is the in-memory state of a single fake tmux session.
type FakeSession struct {
	Name    string
	Content string
	Alive   bool
}

// NewFakeTmuxClient returns a FakeTmuxClient with no sessions.
func NewFakeTmuxClient() *FakeTmuxClient {
	return &FakeTmuxClient{Sessions: make(map[string]*FakeSession)}
}

// CreateSession adds a new alive fake session under the given name.
func (f *FakeTmuxClient) CreateSession(name string) error {
	f.Sessions[name] = &FakeSession{Name: name, Content: "❯ ", Alive: true}
	return nil
}

// KillSession marks the named session as dead.
func (f *FakeTmuxClient) KillSession(name string) error {
	if s, ok := f.Sessions[name]; ok {
		s.Alive = false
		return nil
	}
	return fmt.Errorf("session %s not found", name)
}

// SendKeys appends keys to the named session's pane content.
func (f *FakeTmuxClient) SendKeys(session, keys string) error {
	if s, ok := f.Sessions[session]; ok && s.Alive {
		s.Content += keys + "\n"
		return nil
	}
	return fmt.Errorf("session %s not found or dead", session)
}

// CapturePane returns the recorded pane content for session.
func (f *FakeTmuxClient) CapturePane(session string) (string, error) {
	if s, ok := f.Sessions[session]; ok {
		return s.Content, nil
	}
	return "", fmt.Errorf("session %s not found", session)
}

// ListSessions returns the names of all alive fake sessions.
func (f *FakeTmuxClient) ListSessions() ([]string, error) {
	var names []string
	for name, s := range f.Sessions {
		if s.Alive {
			names = append(names, name)
		}
	}
	return names, nil
}

// IsSessionAlive reports whether the named session exists and is alive.
func (f *FakeTmuxClient) IsSessionAlive(name string) (bool, error) {
	if s, ok := f.Sessions[name]; ok {
		return s.Alive, nil
	}
	return false, nil
}
