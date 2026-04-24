package gateway

import (
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Gateway is the MCP gateway that chains middleware components:
// AuditLog -> Inspector -> Scope -> RateLimit -> CircuitBreaker -> Forward (server handler)
type Gateway struct {
	Inspector      *Inspector
	Scope          *ScopeEnforcer
	RateLimiter    *RateLimiter
	CircuitBreaker *CircuitBreaker
	AuditLogger    *AuditLogger
	Config         *Config
	Logger         *slog.Logger
}

// New creates a new Gateway from config.
func New(cfg *Config, logger *slog.Logger) *Gateway {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	return &Gateway{
		Inspector:      NewInspector(logger),
		Scope:          NewScopeEnforcer(cfg.DefaultPolicy, cfg.Allowlist, cfg.Denylist, logger),
		RateLimiter:    NewRateLimiter(cfg.ToRateLimiterConfig(), logger),
		CircuitBreaker: NewCircuitBreaker(cfg.ToCircuitBreakerConfig(), logger),
		AuditLogger:    NewAuditLogger(logger),
		Config:         cfg,
		Logger:         logger,
	}
}

// Install adds the gateway middleware chain to an MCP server.
// Middleware execution order (outermost first):
// 1. Audit (logs everything)
// 2. Inspector (validates request)
// 3. Scope (checks allowlist/denylist)
// 4. RateLimit (token bucket)
// 5. CircuitBreaker (failure protection)
// 6. Server handler (actual tool execution)
//
// AddReceivingMiddleware applies left-to-right (first = outermost).
func (g *Gateway) Install(server *mcp.Server) {
	// First middleware in list executes first (outermost)
	server.AddReceivingMiddleware(
		g.AuditLogger.Middleware(),    // 1. outermost - logs everything
		g.Inspector.Middleware(),      // 2. validates request
		g.Scope.Middleware(),          // 3. checks permissions
		g.RateLimiter.Middleware(),    // 4. rate limiting
		g.CircuitBreaker.Middleware(), // 5. innermost - failure protection
	)

	g.Logger.Info("gateway.installed",
		"policy", g.Config.DefaultPolicy,
		"rate_limit", g.Config.RateLimits.DefaultRate,
		"circuit_threshold", g.Config.CircuitBreaker.FailureThreshold,
		"audit", g.Config.Audit.Enabled,
	)
}
