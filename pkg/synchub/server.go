package synchub

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ServerOptions configures the HTTP listener that exposes a Hub to
// remote surfaces. Defaults are conservative: localhost-only, token
// auth required.
type ServerOptions struct {
	// Listen is the address to bind to. Default "127.0.0.1:0" — the
	// kernel picks a port and writes it to TokenFile. Set to a Tailscale
	// interface address (e.g. "100.64.1.5:0") to expose over the
	// Tailnet. NEVER set to "0.0.0.0" — the constructor rejects it.
	Listen string

	// TokenFile is where the auth token is written, mode 0600. If empty,
	// defaults to ${AGM_SESSIONS_DIR}/${SessionID}/synchub.token, with
	// AGM_SESSIONS_DIR falling back to ~/.agm/sessions.
	TokenFile string

	// RateLimitRPS is the per-client request rate limit. 0 -> 50.
	RateLimitRPS int

	// ReadTimeout / WriteTimeout default to 5s / 5s — enough for the
	// largest plausible Q&A payload over Tailscale, short enough that a
	// slowloris attacker can't tie up workers.
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Server wraps a Hub as an HTTP service. All routes require the bearer
// token written to TokenFile at server start.
type Server struct {
	hub    *Hub
	opts   ServerOptions
	token  string
	srv    *http.Server
	ln     net.Listener
	closed chan struct{}

	rlMu sync.Mutex
	rl   map[string]*rateLimit

	// acqHandles maps fence -> server-side LockHandle. Per-server (not
	// package-global) so multiple Servers in one process don't share
	// state and a Server's Close empties the map. Findings: F-Sc1.
	acqMu      sync.Mutex
	acqHandles map[uint64]*LockHandle
}

// NewServer constructs the Server but does not start it. Call Start to
// begin serving.
func NewServer(hub *Hub, opts ServerOptions) (*Server, error) {
	if hub == nil {
		return nil, errors.New("synchub: hub required")
	}
	if opts.Listen == "" {
		opts.Listen = "127.0.0.1:0"
	}
	if err := rejectPublicBind(opts.Listen); err != nil {
		return nil, err
	}
	if opts.TokenFile == "" {
		sessDir := os.Getenv("AGM_SESSIONS_DIR")
		if sessDir == "" {
			home, _ := os.UserHomeDir()
			sessDir = filepath.Join(home, ".agm", "sessions")
		}
		opts.TokenFile = filepath.Join(sessDir, hub.opts.SessionID, "synchub.token")
	}
	if opts.RateLimitRPS == 0 {
		opts.RateLimitRPS = 50
	}
	if opts.ReadTimeout == 0 {
		opts.ReadTimeout = 5 * time.Second
	}
	if opts.WriteTimeout == 0 {
		opts.WriteTimeout = 5 * time.Second
	}

	token, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("synchub: token generation: %w", err)
	}
	return &Server{
		hub:        hub,
		opts:       opts,
		token:      token,
		closed:     make(chan struct{}),
		rl:         make(map[string]*rateLimit),
		acqHandles: make(map[uint64]*LockHandle),
	}, nil
}

// rejectPublicBind enforces the localhost/Tailnet-only rule. We reject
// the obvious ways to bind publicly; the operator can still subvert
// this by passing a public IP literal, but they have to mean it.
func rejectPublicBind(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("synchub: invalid Listen %q: %w", addr, err)
	}
	switch host {
	case "0.0.0.0", "::", "":
		return fmt.Errorf("synchub: Listen %q would bind all interfaces; use 127.0.0.1 or a Tailscale address", addr)
	}
	return nil
}

// Token returns the bearer token clients must present. Read once at
// startup; not rotated within a session.
func (s *Server) Token() string { return s.token }

// Addr returns the bound address (host:port). Valid after Start.
func (s *Server) Addr() string {
	if s.ln == nil {
		return ""
	}
	return s.ln.Addr().String()
}

// Start begins serving. Returns once the listener is bound; the HTTP
// loop runs in its own goroutine. Token is written to TokenFile before
// the listener accepts the first connection.
func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", s.opts.Listen)
	if err != nil {
		return fmt.Errorf("synchub: listen %s: %w", s.opts.Listen, err)
	}
	s.ln = ln

	if err := writeToken(s.opts.TokenFile, s.token, s.Addr()); err != nil {
		_ = ln.Close()
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/qa/ask", s.handle(s.handleAsk))
	mux.HandleFunc("/v1/qa/answer", s.handle(s.handleAnswer))
	mux.HandleFunc("/v1/qa/cancel", s.handle(s.handleCancel))
	mux.HandleFunc("/v1/qa/inspect", s.handle(s.handleInspect))
	mux.HandleFunc("/v1/lock/acquire", s.handle(s.handleAcquire))
	mux.HandleFunc("/v1/lock/release", s.handle(s.handleRelease))
	mux.HandleFunc("/v1/lock/inspect", s.handle(s.handleInspectLock))

	s.srv = &http.Server{
		Handler: mux,
		// F-Sc7: explicit keep-alive bounds. ReadHeaderTimeout is the
		// slowloris guard; IdleTimeout closes connections that go silent
		// so per-IP keep-alive doesn't pin server FDs forever.
		ReadTimeout:       s.opts.ReadTimeout,
		WriteTimeout:      s.opts.WriteTimeout,
		ReadHeaderTimeout: 2 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go func() {
		_ = s.srv.Serve(ln)
		close(s.closed)
	}()
	return nil
}

// Close shuts down the listener and removes the token file.
func (s *Server) Close(ctx context.Context) error {
	if s.srv == nil {
		return nil
	}
	err := s.srv.Shutdown(ctx)
	_ = os.Remove(s.opts.TokenFile)
	return err
}

// handle wraps a handler with auth + rate limit. Defense in depth: even
// over localhost, we treat every request as untrusted until the token
// checks out.
func (s *Server) handle(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.checkAuth(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !s.checkRate(r) {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		fn(w, r)
	}
}

func (s *Server) checkAuth(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	// Constant-time compare to avoid timing oracles. Token entropy is
	// already 256 bits; this is belt-and-braces for a localhost service.
	return subtle.ConstantTimeCompare([]byte(h[len(prefix):]), []byte(s.token)) == 1
}

type rateLimit struct {
	count int
	reset time.Time
}

// checkRate is per-client = per-remote-IP. For Tailnet that's the
// Tailscale IP, which is identity-rich for our threat model.
//
// WARNING (F-S2 from multi-persona review): if synchub is exposed
// through an HTTP reverse proxy, all clients appear to come from the
// proxy's IP and rate limiting collapses to a single bucket. Do not
// reverse-proxy synchub; expose it directly on the bound interface.
//
// The bucket map is GC'd opportunistically: each call, we evict
// entries whose reset window has been gone for >1 minute, which keeps
// the map bounded to roughly active-IPs-per-minute (F-Sc5).
func (s *Server) checkRate(r *http.Request) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	now := time.Now()
	s.rlMu.Lock()
	defer s.rlMu.Unlock()

	// Opportunistic GC: cheap (single iter) and bounded amortized.
	if len(s.rl) > 64 {
		for k, v := range s.rl {
			if now.Sub(v.reset) > time.Minute {
				delete(s.rl, k)
			}
		}
	}

	rl, ok := s.rl[host]
	if !ok || now.After(rl.reset) {
		s.rl[host] = &rateLimit{count: 1, reset: now.Add(time.Second)}
		return true
	}
	if rl.count >= s.opts.RateLimitRPS {
		return false
	}
	rl.count++
	return true
}

// -------- request/response shapes --------

type askReq struct {
	Prompt string `json:"prompt"`
}
type askResp struct {
	ID QuestionID `json:"id"`
}

type answerReq struct {
	ID      QuestionID `json:"id"`
	Surface Surface    `json:"surface"`
	Payload []byte     `json:"payload"`
}
type answerResp struct {
	Winner Surface   `json:"winner"`
	At     time.Time `json:"at"`
}

type cancelReq struct {
	ID QuestionID `json:"id"`
}

type inspectReq struct {
	ID QuestionID `json:"id"`
}

type acquireReq struct {
	Key      string        `json:"key"`
	Holder   string        `json:"holder"`
	Timeout  time.Duration `json:"timeout"`
	Deadline time.Duration `json:"deadline"`
}
type acquireResp struct {
	Fence    uint64    `json:"fence"`
	Acquired time.Time `json:"acquired"`
	Deadline time.Time `json:"deadline"`
}

type releaseReq struct {
	Key   string `json:"key"`
	Fence uint64 `json:"fence"`
}

type inspectLockReq struct {
	Key string `json:"key"`
}

// -------- handlers --------

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	var req askReq
	if !readJSON(w, r, &req) {
		return
	}
	id, err := s.hub.AskQuestion(r.Context(), req.Prompt)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, askResp{ID: id})
}

func (s *Server) handleAnswer(w http.ResponseWriter, r *http.Request) {
	var req answerReq
	if !readJSON(w, r, &req) {
		return
	}
	ans, err := s.hub.Answer(r.Context(), req.ID, req.Surface, req.Payload)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, answerResp{Winner: ans.Winner, At: ans.At})
}

func (s *Server) handleCancel(w http.ResponseWriter, r *http.Request) {
	var req cancelReq
	if !readJSON(w, r, &req) {
		return
	}
	if err := s.hub.Cancel(r.Context(), req.ID); err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, struct{}{})
}

func (s *Server) handleInspect(w http.ResponseWriter, r *http.Request) {
	var req inspectReq
	if !readJSON(w, r, &req) {
		return
	}
	snap, err := s.hub.Inspect(req.ID)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, snap)
}

func (s *Server) handleAcquire(w http.ResponseWriter, r *http.Request) {
	var req acquireReq
	if !readJSON(w, r, &req) {
		return
	}
	hdl, err := s.hub.Acquire(r.Context(), req.Key, req.Holder, AcquireOptions{
		Timeout:  req.Timeout,
		Deadline: req.Deadline,
	})
	if err != nil {
		httpErr(w, err)
		return
	}
	s.acqMu.Lock()
	s.acqHandles[hdl.Fence()] = hdl
	s.acqMu.Unlock()
	writeJSON(w, acquireResp{
		Fence:    hdl.Fence(),
		Acquired: hdl.acquired,
		Deadline: hdl.Deadline(),
	})
}

func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request) {
	var req releaseReq
	if !readJSON(w, r, &req) {
		return
	}
	s.acqMu.Lock()
	hdl, ok := s.acqHandles[req.Fence]
	if ok {
		delete(s.acqHandles, req.Fence)
	}
	s.acqMu.Unlock()
	if !ok {
		// Already released or never held by us. Match the Release()
		// no-op semantics: 200 OK with empty body.
		writeJSON(w, struct{}{})
		return
	}
	_ = hdl.Release()
	writeJSON(w, struct{}{})
}

func (s *Server) handleInspectLock(w http.ResponseWriter, r *http.Request) {
	var req inspectLockReq
	if !readJSON(w, r, &req) {
		return
	}
	snap, ok := s.hub.InspectLock(req.Key)
	if !ok {
		http.Error(w, "not held", http.StatusNotFound)
		return
	}
	writeJSON(w, snap)
}

// -------- helpers --------

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// httpErr maps synchub sentinels to HTTP statuses. The body carries
// both `code` (machine-readable category) and `details` (typed metadata
// extracted from typed errors). Discord/Desktop surfaces can reformat
// without parsing strings (F-U3 from multi-persona review).
func httpErr(w http.ResponseWriter, err error) {
	type body struct {
		Code    string         `json:"code"`
		Error   string         `json:"error"`
		Details map[string]any `json:"details,omitempty"`
	}
	code := "internal"
	status := http.StatusInternalServerError
	details := map[string]any{}

	var ce *ClosedError
	var ee *ExpiredError
	switch {
	case errors.As(err, &ce):
		code, status = "closed", http.StatusConflict
		details["winner"] = string(ce.Winner)
		details["at_unix_ms"] = ce.At.UnixMilli()
		details["question_id"] = string(ce.QuestionID)
	case errors.As(err, &ee):
		code, status = "expired", http.StatusGone
		details["at_unix_ms"] = ee.At.UnixMilli()
		details["question_id"] = string(ee.QuestionID)
	case errors.Is(err, ErrNotFound):
		code, status = "not_found", http.StatusNotFound
	case errors.Is(err, ErrClosed):
		code, status = "closed", http.StatusConflict
	case errors.Is(err, ErrExpired):
		code, status = "expired", http.StatusGone
	case errors.Is(err, ErrAlreadyHeld):
		code, status = "already_held", http.StatusConflict
	case errors.Is(err, ErrLockTimeout):
		code, status = "lock_timeout", http.StatusRequestTimeout
	case errors.Is(err, ErrLockClosed):
		code, status = "lock_closed", http.StatusConflict
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := body{Code: code, Error: err.Error()}
	if len(details) > 0 {
		resp.Details = details
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func generateToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

// writeToken writes "{addr}\n{token}\n" so the client can discover both
// in one file read. Mode 0600 so only the session owner can read it.
//
// Uses O_CREATE|O_EXCL to avoid a TOCTOU race on the path: if a hostile
// local process pre-created a symlink at `path` between MkdirAll and
// WriteFile, the open would follow it and the token would land
// wherever the attacker wanted. O_EXCL refuses to open an existing
// path (including a dangling symlink), so we unlink-then-exclusively-
// create. Findings: F-S1 in the multi-persona review.
func writeToken(path, token, addr string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("synchub: mkdir token dir: %w", err)
	}
	// Remove any stale file first; the exclusive create below will then
	// fail closed if something races us to repopulate it.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("synchub: clear token file: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("synchub: exclusively create token: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(addr + "\n" + token + "\n"); err != nil {
		return fmt.Errorf("synchub: write token: %w", err)
	}
	return nil
}
