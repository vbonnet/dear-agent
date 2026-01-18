package engram

import (
	"os"
	"testing"
	"time"
)

func TestLoadEngramConfig_Defaults(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("AGM_ENGRAM_PATH")
	os.Unsetenv("AGM_ENGRAM_LIMIT")
	os.Unsetenv("AGM_ENGRAM_SCORE_THRESHOLD")
	os.Unsetenv("AGM_ENGRAM_TIMEOUT")

	cfg := LoadEngramConfig()

	if cfg.BinaryPath != "" {
		t.Errorf("Expected empty BinaryPath, got %s", cfg.BinaryPath)
	}
	if cfg.Limit != DefaultEngramLimit {
		t.Errorf("Expected Limit=%d, got %d", DefaultEngramLimit, cfg.Limit)
	}
	if cfg.ScoreThreshold != DefaultScoreThreshold {
		t.Errorf("Expected ScoreThreshold=%.1f, got %.1f", DefaultScoreThreshold, cfg.ScoreThreshold)
	}
	if cfg.Timeout != DefaultQueryTimeout {
		t.Errorf("Expected Timeout=%v, got %v", DefaultQueryTimeout, cfg.Timeout)
	}
}

func TestLoadEngramConfig_EnvironmentVariables(t *testing.T) {
	os.Setenv("AGM_ENGRAM_PATH", "/custom/path/engram")
	os.Setenv("AGM_ENGRAM_LIMIT", "15")
	os.Setenv("AGM_ENGRAM_SCORE_THRESHOLD", "0.8")
	os.Setenv("AGM_ENGRAM_TIMEOUT", "10")
	defer func() {
		os.Unsetenv("AGM_ENGRAM_PATH")
		os.Unsetenv("AGM_ENGRAM_LIMIT")
		os.Unsetenv("AGM_ENGRAM_SCORE_THRESHOLD")
		os.Unsetenv("AGM_ENGRAM_TIMEOUT")
	}()

	cfg := LoadEngramConfig()

	if cfg.BinaryPath != "/custom/path/engram" {
		t.Errorf("Expected BinaryPath=/custom/path/engram, got %s", cfg.BinaryPath)
	}
	if cfg.Limit != 15 {
		t.Errorf("Expected Limit=15, got %d", cfg.Limit)
	}
	if cfg.ScoreThreshold != 0.8 {
		t.Errorf("Expected ScoreThreshold=0.8, got %.1f", cfg.ScoreThreshold)
	}
	if cfg.Timeout != 10*time.Second {
		t.Errorf("Expected Timeout=10s, got %v", cfg.Timeout)
	}
}

func TestLoadEngramConfig_InvalidValues(t *testing.T) {
	os.Setenv("AGM_ENGRAM_LIMIT", "invalid")
	os.Setenv("AGM_ENGRAM_SCORE_THRESHOLD", "2.0")
	os.Setenv("AGM_ENGRAM_TIMEOUT", "-5")
	defer func() {
		os.Unsetenv("AGM_ENGRAM_LIMIT")
		os.Unsetenv("AGM_ENGRAM_SCORE_THRESHOLD")
		os.Unsetenv("AGM_ENGRAM_TIMEOUT")
	}()

	cfg := LoadEngramConfig()

	// Should use defaults for invalid values
	if cfg.Limit != DefaultEngramLimit {
		t.Errorf("Expected default Limit=%d for invalid value, got %d", DefaultEngramLimit, cfg.Limit)
	}
	if cfg.ScoreThreshold != DefaultScoreThreshold {
		t.Errorf("Expected default ScoreThreshold=%.1f for invalid value, got %.1f", DefaultScoreThreshold, cfg.ScoreThreshold)
	}
	if cfg.Timeout != DefaultQueryTimeout {
		t.Errorf("Expected default Timeout=%v for invalid value, got %v", DefaultQueryTimeout, cfg.Timeout)
	}
}
