package tmux

import "fmt"

// FakeTmuxClient simulates tmux behavior with internal state.
// Used for hermetic testing without real tmux sessions.
type FakeTmuxClient struct {
	Sessions map[string]*FakeSession
}

type FakeSession struct {
	Name    string
	Content string
	Alive   bool
}

func NewFakeTmuxClient() *FakeTmuxClient {
	return &FakeTmuxClient{Sessions: make(map[string]*FakeSession)}
}

func (f *FakeTmuxClient) CreateSession(name string) error {
	f.Sessions[name] = &FakeSession{Name: name, Content: "❯ ", Alive: true}
	return nil
}

func (f *FakeTmuxClient) KillSession(name string) error {
	if s, ok := f.Sessions[name]; ok {
		s.Alive = false
		return nil
	}
	return fmt.Errorf("session %s not found", name)
}

func (f *FakeTmuxClient) SendKeys(session, keys string) error {
	if s, ok := f.Sessions[session]; ok && s.Alive {
		s.Content += keys + "\n"
		return nil
	}
	return fmt.Errorf("session %s not found or dead", session)
}

func (f *FakeTmuxClient) CapturePane(session string) (string, error) {
	if s, ok := f.Sessions[session]; ok {
		return s.Content, nil
	}
	return "", fmt.Errorf("session %s not found", session)
}

func (f *FakeTmuxClient) ListSessions() ([]string, error) {
	var names []string
	for name, s := range f.Sessions {
		if s.Alive {
			names = append(names, name)
		}
	}
	return names, nil
}

func (f *FakeTmuxClient) IsSessionAlive(name string) (bool, error) {
	if s, ok := f.Sessions[name]; ok {
		return s.Alive, nil
	}
	return false, nil
}
