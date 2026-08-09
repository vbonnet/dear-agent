package agent

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

const detectorTestHome = "/detector-test-home"

type fakeDetectorInputs struct {
	env       map[string]string
	home      string
	homeErr   error
	paths     map[string]bool
	homeCalls int
	pathCalls []string
}

func newFakeDetectorInputs() *fakeDetectorInputs {
	return &fakeDetectorInputs{
		env:   make(map[string]string),
		home:  detectorTestHome,
		paths: make(map[string]bool),
	}
}

func (f *fakeDetectorInputs) inputs() detectorInputs {
	return detectorInputs{
		getenv: func(key string) string {
			return f.env[key]
		},
		userHomeDir: func() (string, error) {
			f.homeCalls++
			return f.home, f.homeErr
		},
		pathExists: func(path string) bool {
			f.pathCalls = append(f.pathCalls, path)
			return f.paths[path]
		},
	}
}

func TestDetectUsesExactPriority(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		env             map[string]string
		home            string
		homeErr         error
		paths           map[string]bool
		want            Agent
		wantNoHomeRead  bool
		wantNoPathReads bool
	}{
		{
			name: "claude value environment wins over every lower signal",
			env: map[string]string{
				"CLAUDECODE":        "1",
				"CURSOR":            "1",
				"CURSOR_SESSION_ID": "cursor-session",
				"WINDSURF":          "1",
				"AIDER_MODEL":       "model",
				"AIDER_ARCHITECT":   "1",
			},
			home:            detectorTestHome,
			paths:           allDetectorMarkerPaths(detectorTestHome),
			want:            AgentClaudeCode,
			wantNoHomeRead:  true,
			wantNoPathReads: true,
		},
		{
			name: "claude entrypoint environment wins over every lower signal",
			env: map[string]string{
				"CLAUDECODE":             "0",
				"CLAUDE_CODE_ENTRYPOINT": "/bin/claude",
				"CURSOR":                 "1",
				"WINDSURF":               "1",
				"AIDER_MODEL":            "model",
			},
			home:            detectorTestHome,
			paths:           allDetectorMarkerPaths(detectorTestHome),
			want:            AgentClaudeCode,
			wantNoHomeRead:  true,
			wantNoPathReads: true,
		},
		{
			name: "cursor value environment wins below non-triggering claude",
			env: map[string]string{
				"CLAUDECODE":  "0",
				"CURSOR":      "1",
				"WINDSURF":    "1",
				"AIDER_MODEL": "model",
			},
			home:            detectorTestHome,
			paths:           allDetectorMarkerPaths(detectorTestHome),
			want:            AgentCursor,
			wantNoHomeRead:  true,
			wantNoPathReads: true,
		},
		{
			name: "cursor session environment wins below non-triggering cursor value",
			env: map[string]string{
				"CURSOR":            "0",
				"CURSOR_SESSION_ID": "cursor-session",
				"WINDSURF":          "1",
				"AIDER_MODEL":       "model",
			},
			home:            detectorTestHome,
			paths:           allDetectorMarkerPaths(detectorTestHome),
			want:            AgentCursor,
			wantNoHomeRead:  true,
			wantNoPathReads: true,
		},
		{
			name: "windsurf environment wins below non-triggering cursor",
			env: map[string]string{
				"CURSOR":      "0",
				"WINDSURF":    "1",
				"AIDER_MODEL": "model",
			},
			home:            detectorTestHome,
			paths:           allDetectorMarkerPaths(detectorTestHome),
			want:            AgentWindsurf,
			wantNoHomeRead:  true,
			wantNoPathReads: true,
		},
		{
			name: "aider model environment precedes every filesystem signal",
			env: map[string]string{
				"WINDSURF":    "0",
				"AIDER_MODEL": "model",
			},
			home:            detectorTestHome,
			paths:           allDetectorMarkerPaths(detectorTestHome),
			want:            AgentAider,
			wantNoHomeRead:  true,
			wantNoPathReads: true,
		},
		{
			name: "aider architect environment is supported",
			env: map[string]string{
				"AIDER_ARCHITECT": "1",
			},
			home:            detectorTestHome,
			paths:           allDetectorMarkerPaths(detectorTestHome),
			want:            AgentAider,
			wantNoHomeRead:  true,
			wantNoPathReads: true,
		},
		{
			name: "non-triggering environment values fall through to unknown",
			env: map[string]string{
				"CLAUDECODE": "enabled",
				"CURSOR":     "enabled",
				"WINDSURF":   "enabled",
			},
			home:  detectorTestHome,
			paths: unrelatedDetectorPaths(),
			want:  AgentUnknown,
		},
		{
			name:            "home resolution error fails closed before cwd markers",
			home:            detectorTestHome,
			homeErr:         errors.New("home unavailable"),
			paths:           allDetectorMarkerPaths(detectorTestHome),
			want:            AgentUnknown,
			wantNoPathReads: true,
		},
		{
			name:  "home claude marker wins over every cwd marker",
			home:  detectorTestHome,
			paths: allDetectorMarkerPaths(detectorTestHome),
			want:  AgentClaudeCode,
		},
		{
			name: "cursor cwd marker wins over lower cwd markers",
			home: detectorTestHome,
			paths: detectorPaths(
				".cursorrules",
				".windsurfrules",
				".aider.conf.yml",
				".aiderignore",
			),
			want: AgentCursor,
		},
		{
			name: "windsurf cwd marker wins over aider cwd markers",
			home: detectorTestHome,
			paths: detectorPaths(
				".windsurfrules",
				".aider.conf.yml",
				".aiderignore",
			),
			want: AgentWindsurf,
		},
		{
			name:  "aider config cwd marker is supported",
			home:  detectorTestHome,
			paths: detectorPaths(".aider.conf.yml"),
			want:  AgentAider,
		},
		{
			name:  "aider ignore cwd marker is supported",
			home:  detectorTestHome,
			paths: detectorPaths(".aiderignore"),
			want:  AgentAider,
		},
		{
			name:  "unrelated home contents return unknown",
			home:  detectorTestHome,
			paths: unrelatedDetectorPaths(),
			want:  AgentUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeDetectorInputs()
			fake.env = tt.env
			fake.home = tt.home
			fake.homeErr = tt.homeErr
			fake.paths = tt.paths

			if got := newDetector(fake.inputs()).Detect(); got != tt.want {
				t.Fatalf("Detect() = %q, want %q", got, tt.want)
			}
			if tt.wantNoHomeRead && fake.homeCalls != 0 {
				t.Fatalf("Detect() resolved home %d time(s), want no home read", fake.homeCalls)
			}
			if tt.wantNoPathReads && len(fake.pathCalls) != 0 {
				t.Fatalf("Detect() inspected paths %v, want no path reads", fake.pathCalls)
			}
		})
	}
}

func TestDetectorCachesDetectedAndUnknownResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  func(*fakeDetectorInputs)
		mutate func(*fakeDetectorInputs)
		want   Agent
	}{
		{
			name: "detected agent remains cached",
			setup: func(fake *fakeDetectorInputs) {
				fake.env["CURSOR"] = "1"
			},
			mutate: func(fake *fakeDetectorInputs) {
				fake.env = map[string]string{"CLAUDECODE": "1"}
			},
			want: AgentCursor,
		},
		{
			name:  "unknown remains cached",
			setup: func(*fakeDetectorInputs) {},
			mutate: func(fake *fakeDetectorInputs) {
				fake.paths[filepath.Join(fake.home, ".claude")] = true
			},
			want: AgentUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeDetectorInputs()
			tt.setup(fake)
			detector := newDetector(fake.inputs())

			if got := detector.Detect(); got != tt.want {
				t.Fatalf("first Detect() = %q, want %q", got, tt.want)
			}
			tt.mutate(fake)
			if got := detector.Detect(); got != tt.want {
				t.Fatalf("cached Detect() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectorClearCacheRereadsCurrentInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		setup     func(*fakeDetectorInputs)
		firstWant Agent
		mutate    func(*fakeDetectorInputs)
		nextWant  Agent
	}{
		{
			name: "environment change",
			setup: func(fake *fakeDetectorInputs) {
				fake.env["CURSOR"] = "1"
			},
			firstWant: AgentCursor,
			mutate: func(fake *fakeDetectorInputs) {
				fake.env = map[string]string{"CLAUDECODE": "1"}
			},
			nextWant: AgentClaudeCode,
		},
		{
			name:      "filesystem change after cached unknown",
			setup:     func(*fakeDetectorInputs) {},
			firstWant: AgentUnknown,
			mutate: func(fake *fakeDetectorInputs) {
				fake.paths[".windsurfrules"] = true
			},
			nextWant: AgentWindsurf,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			fake := newFakeDetectorInputs()
			tt.setup(fake)
			detector := newDetector(fake.inputs())

			if got := detector.Detect(); got != tt.firstWant {
				t.Fatalf("first Detect() = %q, want %q", got, tt.firstWant)
			}
			tt.mutate(fake)
			detector.ClearCache()
			if got := detector.Detect(); got != tt.nextWant {
				t.Fatalf("Detect() after ClearCache() = %q, want %q", got, tt.nextWant)
			}
		})
	}
}

func TestDetectorInstancesReadInputsLazily(t *testing.T) {
	t.Parallel()

	fake := newFakeDetectorInputs()
	fake.env["CURSOR"] = "1"
	detectorOne := newDetector(fake.inputs())
	detectorTwo := newDetector(fake.inputs())

	if got := detectorOne.Detect(); got != AgentCursor {
		t.Fatalf("detectorOne.Detect() = %q, want %q", got, AgentCursor)
	}

	fake.env = map[string]string{"WINDSURF": "1"}
	if got := detectorOne.Detect(); got != AgentCursor {
		t.Fatalf("cached detectorOne.Detect() = %q, want %q", got, AgentCursor)
	}
	if got := detectorTwo.Detect(); got != AgentWindsurf {
		t.Fatalf("lazy detectorTwo.Detect() = %q, want %q", got, AgentWindsurf)
	}
}

func TestDetectorClearCacheOnFreshDetector(t *testing.T) {
	t.Parallel()

	fake := newFakeDetectorInputs()
	detector := newDetector(fake.inputs())
	detector.ClearCache()

	if got := detector.Detect(); got != AgentUnknown {
		t.Fatalf("Detect() = %q, want %q", got, AgentUnknown)
	}
}

func TestDetectorConcurrentDetectAndClearCache(t *testing.T) {
	t.Parallel()

	const iterations = 250
	tests := []struct {
		name   string
		inputs detectorInputs
		want   Agent
	}{
		{
			name: "detected agent",
			inputs: detectorInputs{
				getenv: func(key string) string {
					if key == "CURSOR" {
						return "1"
					}
					return ""
				},
				userHomeDir: func() (string, error) { return detectorTestHome, nil },
				pathExists:  func(string) bool { return false },
			},
			want: AgentCursor,
		},
		{
			name: "unknown agent",
			inputs: detectorInputs{
				getenv:      func(string) string { return "" },
				userHomeDir: func() (string, error) { return detectorTestHome, nil },
				pathExists:  func(string) bool { return false },
			},
			want: AgentUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			detector := newDetector(tt.inputs)
			if got := detector.Detect(); got != tt.want {
				t.Fatalf("warm Detect() = %q, want %q", got, tt.want)
			}

			start := make(chan struct{})
			results := make(chan Agent, iterations)
			var workers sync.WaitGroup
			workers.Add(2)

			go func() {
				defer workers.Done()
				<-start
				for range iterations {
					results <- detector.Detect()
				}
			}()
			go func() {
				defer workers.Done()
				<-start
				for range iterations {
					detector.ClearCache()
				}
			}()

			close(start)
			workers.Wait()
			close(results)

			for got := range results {
				if got != tt.want {
					t.Fatalf("concurrent Detect() = %q, want %q", got, tt.want)
				}
			}
			if got := detector.Detect(); got != tt.want {
				t.Fatalf("final Detect() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectorConcurrentCacheGenerations(t *testing.T) {
	t.Parallel()

	var claudeMarker atomic.Bool
	var evaluations atomic.Int64
	inputs := detectorInputs{
		getenv: func(string) string { return "" },
		userHomeDir: func() (string, error) {
			evaluations.Add(1)
			return detectorTestHome, nil
		},
		pathExists: func(path string) bool {
			return path == filepath.Join(detectorTestHome, ".claude") && claudeMarker.Load()
		},
	}
	detector := newDetector(inputs)

	if got := evaluations.Load(); got != 0 {
		t.Fatalf("evaluations after construction = %d, want 0", got)
	}
	assertConcurrentDetectionWave(t, detector, AgentUnknown)
	if got := evaluations.Load(); got != 1 {
		t.Fatalf("evaluations after first wave = %d, want 1", got)
	}

	claudeMarker.Store(true)
	assertConcurrentDetectionWave(t, detector, AgentUnknown)
	if got := evaluations.Load(); got != 1 {
		t.Fatalf("evaluations for cached wave = %d, want 1", got)
	}

	detector.ClearCache()
	assertConcurrentDetectionWave(t, detector, AgentClaudeCode)
	if got := evaluations.Load(); got != 2 {
		t.Fatalf("evaluations after clear = %d, want 2", got)
	}
	assertConcurrentDetectionWave(t, detector, AgentClaudeCode)
	if got := evaluations.Load(); got != 2 {
		t.Fatalf("evaluations for second cached wave = %d, want 2", got)
	}
}

func TestNewDetectorUsesCompleteSystemInputs(t *testing.T) {
	t.Parallel()

	detector := NewDetector()
	if detector == nil {
		t.Fatal("NewDetector() returned nil")
	}
	if detector.inputs == nil {
		t.Fatal("NewDetector() did not configure system inputs")
	}
	if detector.inputs.getenv == nil || detector.inputs.userHomeDir == nil || detector.inputs.pathExists == nil {
		t.Fatal("NewDetector() configured incomplete system inputs")
	}
}

func TestDetectorZeroValueRemainsUsable(t *testing.T) {
	var detector Detector
	detector.ClearCache()

	inputs := detector.activeInputs()
	if inputs.getenv == nil || inputs.userHomeDir == nil || inputs.pathExists == nil {
		t.Fatal("zero-value Detector resolved incomplete system inputs")
	}

	if got := detector.Detect(); !isSupportedAgent(got) {
		t.Fatalf("zero-value Detector.Detect() = %q, want a supported Agent value", got)
	}
}

func TestSystemDetectorInputsPathExists(t *testing.T) {
	t.Parallel()

	inputs := systemDetectorInputs()
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "marker")
	if err := os.WriteFile(filePath, []byte("marker"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "file", path: filePath, want: true},
		{name: "directory", path: tempDir, want: true},
		{name: "missing", path: filepath.Join(tempDir, "missing"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := inputs.pathExists(tt.path); got != tt.want {
				t.Fatalf("pathExists(%q) = %t, want %t", tt.path, got, tt.want)
			}
		})
	}
}

func allDetectorMarkerPaths(home string) map[string]bool {
	return detectorPaths(
		filepath.Join(home, ".claude"),
		".cursorrules",
		".windsurfrules",
		".aider.conf.yml",
		".aiderignore",
	)
}

func unrelatedDetectorPaths() map[string]bool {
	return detectorPaths(
		filepath.Join("/unrelated-operator-home", ".claude"),
		filepath.Join("/unrelated-project", ".cursorrules"),
	)
}

func detectorPaths(paths ...string) map[string]bool {
	result := make(map[string]bool, len(paths))
	for _, path := range paths {
		result[path] = true
	}
	return result
}

func isSupportedAgent(agent Agent) bool {
	switch agent {
	case AgentClaudeCode, AgentCursor, AgentWindsurf, AgentAider, AgentUnknown:
		return true
	default:
		return false
	}
}

func assertConcurrentDetectionWave(t *testing.T, detector *Detector, want Agent) {
	t.Helper()

	const calls = 64
	start := make(chan struct{})
	results := make(chan Agent, calls)
	var workers sync.WaitGroup
	workers.Add(calls)
	for range calls {
		go func() {
			defer workers.Done()
			<-start
			results <- detector.Detect()
		}()
	}

	close(start)
	workers.Wait()
	close(results)
	for got := range results {
		if got != want {
			t.Fatalf("concurrent Detect() = %q, want %q", got, want)
		}
	}
}
