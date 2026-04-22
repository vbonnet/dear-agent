package tmux

// TmuxClient abstracts tmux operations for hermetic testing.
// Production code uses RealTmuxClient. Tests use FakeTmuxClient.
type TmuxClient interface {
	CreateSession(name string) error
	KillSession(name string) error
	SendKeys(session, keys string) error
	CapturePane(session string) (string, error)
	ListSessions() ([]string, error)
	IsSessionAlive(name string) (bool, error)
}
