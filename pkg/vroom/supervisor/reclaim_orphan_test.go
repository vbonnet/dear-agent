package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestOrphanReclaimer_Reclaim(t *testing.T) {
	tests := []struct {
		name       string
		targets    []string
		stdout     string
		runErr     error
		wantResult ReclaimResult
		wantErr    bool
		wantArgs   []string // expected args passed to the binary
	}{
		{
			name:       "clean pass, nothing to kill",
			stdout:     `{"targets":["gopls"],"orphans_found":0,"killed":0,"failed":0}`,
			wantResult: ReclaimResult{OrphansFound: 0, OrphansKilled: 0, OrphansFailed: 0},
			wantArgs:   []string{"session", "reap-orphans", "--json"},
		},
		{
			name:       "kills three orphans",
			stdout:     `{"targets":["gopls","agm-mcp-server"],"orphans_found":3,"killed":3,"failed":0,"killed_pids":[1,2,3]}`,
			wantResult: ReclaimResult{OrphansFound: 3, OrphansKilled: 3, OrphansFailed: 0},
			wantArgs:   []string{"session", "reap-orphans", "--json"},
		},
		{
			name:       "explicit targets are forwarded",
			targets:    []string{"gopls"},
			stdout:     `{"targets":["gopls"],"orphans_found":1,"killed":1,"failed":0}`,
			wantResult: ReclaimResult{OrphansFound: 1, OrphansKilled: 1},
			wantArgs:   []string{"session", "reap-orphans", "--json", "--targets", "gopls"},
		},
		{
			name:       "partial failure still returns counts (non-zero exit, valid JSON)",
			stdout:     `{"orphans_found":2,"killed":1,"failed":1,"killed_pids":[1],"failed_pids":[2]}`,
			runErr:     errors.New("exit status 1"),
			wantResult: ReclaimResult{OrphansFound: 2, OrphansKilled: 1, OrphansFailed: 1},
		},
		{
			name:    "unparseable output with run error is an error",
			stdout:  "command not found",
			runErr:  errors.New("exec: \"agm\": executable file not found"),
			wantErr: true,
		},
		{
			name:    "garbage JSON without run error is an error",
			stdout:  "not json at all",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotArgs []string
			r := &OrphanReclaimer{
				Targets: tt.targets,
				run: func(_ context.Context, name string, args ...string) ([]byte, error) {
					if name != "agm" {
						t.Errorf("binary = %q, want agm", name)
					}
					gotArgs = args
					return []byte(tt.stdout), tt.runErr
				},
			}

			got, err := r.Reclaim(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (result %+v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantResult {
				t.Errorf("result = %+v, want %+v", got, tt.wantResult)
			}
			if tt.wantArgs != nil && strings.Join(gotArgs, " ") != strings.Join(tt.wantArgs, " ") {
				t.Errorf("args = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}

func TestOrphanReclaimer_AGMBinDefault(t *testing.T) {
	r := &OrphanReclaimer{
		AGMBin: "/custom/agm",
		run: func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name != "/custom/agm" {
				t.Errorf("binary = %q, want /custom/agm", name)
			}
			return []byte(`{"orphans_found":0,"killed":0,"failed":0}`), nil
		},
	}
	if _, err := r.Reclaim(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOrphanReclaimer_RespectsTimeout(t *testing.T) {
	r := &OrphanReclaimer{
		Timeout: 50 * time.Millisecond,
		run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("expected a deadline on the context")
			}
			if d := time.Until(deadline); d <= 0 || d > 50*time.Millisecond {
				t.Errorf("deadline window = %v, want (0, 50ms]", d)
			}
			return []byte(`{"orphans_found":0,"killed":0,"failed":0}`), nil
		},
	}
	if _, err := r.Reclaim(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestOrphanReclaimer_ImplementsInterface is a compile-time assertion that
// OrphanReclaimer satisfies ResourceReclaimer (the seam Overseer.WithReclaimer
// expects).
func TestOrphanReclaimer_ImplementsInterface(t *testing.T) {
	var _ ResourceReclaimer = (*OrphanReclaimer)(nil)
}
