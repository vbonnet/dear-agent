// Copyright 2026 dear-agent contributors. See LICENSE.

package a2a

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2asrv"
)

// DefaultInvokePath is the URL path the A2A JSON-RPC handler is mounted
// at if ServerConfig.InvokePath is empty.
const DefaultInvokePath = "/invoke"

// ServerConfig groups the dependencies needed to construct a Server.
type ServerConfig struct {
	// Card is the self-description served at the well-known path. The
	// URL and ProtocolVersion fields are populated automatically once
	// the listener has bound; callers may leave them empty.
	Card *a2a.AgentCard

	// Handler processes each task.
	Handler SessionHandler

	// Addr is passed to net.Listen("tcp", Addr). Use ":0" to bind an
	// arbitrary free port (test-friendly).
	Addr string

	// InvokePath is the URL path the JSON-RPC handler is mounted at.
	// Defaults to DefaultInvokePath.
	InvokePath string

	// HTTPServer is optional. When nil, a default *http.Server is built.
	// Callers that want custom timeouts or TLS configuration may supply
	// their own and the Server will only assign Handler/Addr on it.
	HTTPServer *http.Server
}

// Server is an HTTP A2A endpoint over a SessionHandler. It serves both
// the JSON-RPC invocation URL and the well-known Agent Card path.
type Server struct {
	cfg      ServerConfig
	listener net.Listener
	http     *http.Server
	exec     *sessionExecutor

	mu      sync.Mutex
	started bool
	serveCh chan error
}

// NewServer validates the configuration and binds the listener. The
// server does not begin serving until Start is called.
func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.Handler == nil {
		return nil, errors.New("a2a: ServerConfig.Handler is required")
	}
	if cfg.Card == nil {
		return nil, errors.New("a2a: ServerConfig.Card is required")
	}
	if cfg.InvokePath == "" {
		cfg.InvokePath = DefaultInvokePath
	}
	if cfg.Addr == "" {
		cfg.Addr = ":0"
	}

	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return nil, fmt.Errorf("a2a: listen %s: %w", cfg.Addr, err)
	}

	exec := &sessionExecutor{handler: cfg.Handler}
	requestHandler := a2asrv.NewHandler(exec)

	// Resolve the public URL now so the Agent Card carries it.
	card := *cfg.Card
	if card.URL == "" {
		card.URL = fmt.Sprintf("http://%s%s", listener.Addr().String(), cfg.InvokePath)
	}
	if card.ProtocolVersion == "" {
		card.ProtocolVersion = string(a2a.Version)
	}
	if card.PreferredTransport == "" {
		card.PreferredTransport = a2a.TransportProtocolJSONRPC
	}

	mux := http.NewServeMux()
	mux.Handle(cfg.InvokePath, a2asrv.NewJSONRPCHandler(requestHandler))
	mux.Handle(a2asrv.WellKnownAgentCardPath, a2asrv.NewStaticAgentCardHandler(&card))

	srv := cfg.HTTPServer
	if srv == nil {
		srv = &http.Server{
			ReadHeaderTimeout: 5 * time.Second,
		}
	}
	srv.Handler = mux

	return &Server{
		cfg:      cfg,
		listener: listener,
		http:     srv,
		exec:     exec,
		serveCh:  make(chan error, 1),
	}, nil
}

// Start begins serving HTTP requests on a background goroutine. The
// server keeps running until Shutdown is called or the listener fails.
func (s *Server) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return errors.New("a2a: server already started")
	}
	s.started = true
	s.mu.Unlock()

	go func() {
		err := s.http.Serve(s.listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		s.serveCh <- err
		close(s.serveCh)
	}()
	return nil
}

// Addr returns the resolved listen address (host:port). Valid after
// NewServer returns; in particular when ":0" was requested, the port is
// already known here.
func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

// URL returns the A2A JSON-RPC invocation URL clients post to.
func (s *Server) URL() string {
	return fmt.Sprintf("http://%s%s", s.Addr(), s.cfg.InvokePath)
}

// AgentCardURL returns the URL of the well-known Agent Card document.
func (s *Server) AgentCardURL() string {
	return fmt.Sprintf("http://%s%s", s.Addr(), a2asrv.WellKnownAgentCardPath)
}

// CardBaseURL returns the base URL (no path) that an Agent Card
// resolver can be pointed at — i.e. http://host:port. The resolver
// appends the well-known path itself.
func (s *Server) CardBaseURL() string {
	return fmt.Sprintf("http://%s", s.Addr())
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if err := s.http.Shutdown(ctx); err != nil {
		return err
	}
	select {
	case err := <-s.serveCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
