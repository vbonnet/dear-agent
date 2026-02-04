package engram

import (
	"log"
	"os"
	"strconv"
	"time"
)

// EngramConfig holds configuration for Engram integration
type EngramConfig struct {
	BinaryPath     string        // Path to engram binary (AGM_ENGRAM_PATH)
	Limit          int           // Max results (AGM_ENGRAM_LIMIT, default 10)
	ScoreThreshold float64       // Min score (AGM_ENGRAM_SCORE_THRESHOLD, default 0.7)
	Timeout        time.Duration // Query timeout (AGM_ENGRAM_TIMEOUT, default 5s)
}

const (
	DefaultEngramLimit    = 10
	DefaultScoreThreshold = 0.7
	DefaultQueryTimeout   = 5 * time.Second
)

// LoadEngramConfig loads configuration from environment variables
func LoadEngramConfig() EngramConfig {
	cfg := EngramConfig{
		Limit:          DefaultEngramLimit,
		ScoreThreshold: DefaultScoreThreshold,
		Timeout:        DefaultQueryTimeout,
	}

	// Override from environment
	if path := os.Getenv("AGM_ENGRAM_PATH"); path != "" {
		cfg.BinaryPath = path
	}

	if limitStr := os.Getenv("AGM_ENGRAM_LIMIT"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			cfg.Limit = limit
		} else {
			log.Printf("Warning: Invalid AGM_ENGRAM_LIMIT=%q, using default %d", limitStr, cfg.Limit)
		}
	}

	if thresholdStr := os.Getenv("AGM_ENGRAM_SCORE_THRESHOLD"); thresholdStr != "" {
		if threshold, err := strconv.ParseFloat(thresholdStr, 64); err == nil && threshold >= 0 && threshold <= 1 {
			cfg.ScoreThreshold = threshold
		} else {
			log.Printf("Warning: Invalid AGM_ENGRAM_SCORE_THRESHOLD=%q, using default %.1f", thresholdStr, cfg.ScoreThreshold)
		}
	}

	if timeoutStr := os.Getenv("AGM_ENGRAM_TIMEOUT"); timeoutStr != "" {
		if timeout, err := strconv.Atoi(timeoutStr); err == nil && timeout > 0 {
			cfg.Timeout = time.Duration(timeout) * time.Second
		} else {
			log.Printf("Warning: Invalid AGM_ENGRAM_TIMEOUT=%q, using default %v", timeoutStr, cfg.Timeout)
		}
	}

	return cfg
}
