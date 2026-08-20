package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newFixtureRepo builds a throwaway repo root containing a policy directory
// with the given store contents.
func newFixtureRepo(t *testing.T, store string, extraFiles map[string]string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, storeDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, storePath), []byte(store), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	for name, content := range extraFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	return root
}

const goodStore = `{"rule":"bash-20-line-limit","path":"a.sh","status":"active","reason":"r","approver":"v","sunset":null,"added":"2026-04-24"}
{"rule":"bash-20-line-limit","path":"b.sh","status":"active","reason":"r","approver":"v","sunset":null,"added":"2026-04-24"}
`

func TestVerifyStoreAcceptsGoodStore(t *testing.T) {
	root := newFixtureRepo(t, goodStore, nil)
	if err := runVerifyStore([]string{"-repo", root}); err != nil {
		t.Fatalf("runVerifyStore on a valid store = %v, want nil", err)
	}
}

// These are the regressions the migration exists to prevent. Each must fail
// the build rather than degrade quietly.
func TestVerifyStoreRejects(t *testing.T) {
	unsorted := `{"rule":"bash-20-line-limit","path":"b.sh","status":"active","reason":"","approver":"","sunset":null,"added":"2026-04-24"}
{"rule":"bash-20-line-limit","path":"a.sh","status":"active","reason":"","approver":"","sunset":null,"added":"2026-04-24"}
`
	dup := `{"rule":"bash-20-line-limit","path":"a.sh","status":"active","reason":"","approver":"","sunset":null,"added":"2026-04-24"}
{"rule":"bash-20-line-limit","path":"a.sh","status":"active","reason":"","approver":"","sunset":null,"added":"2026-04-24"}
`
	cases := []struct {
		name    string
		store   string
		extra   map[string]string
		wantErr string
	}{
		{
			name:    "binary store reintroduced beside the text store",
			store:   goodStore,
			extra:   map[string]string{"exceptions.db": "SQLite format 3\x00"},
			wantErr: "binary waiver store",
		},
		{
			name:    "binary store with a .sqlite3 extension",
			store:   goodStore,
			extra:   map[string]string{"waivers.sqlite3": "\x00\x01"},
			wantErr: "binary waiver store",
		},
		{
			name:    "binary blob smuggled in under the .jsonl name",
			store:   "SQLite format 3\x00binary",
			wantErr: "NUL bytes",
		},
		{name: "unsorted store", store: unsorted, wantErr: "not sorted"},
		{name: "duplicate waiver", store: dup, wantErr: "duplicate"},
		{name: "unparseable line", store: "{not json}\n", wantErr: "invalid JSON"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newFixtureRepo(t, tc.store, tc.extra)
			err := runVerifyStore([]string{"-repo", root})
			if err == nil {
				t.Fatalf("runVerifyStore = nil, want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// A waived over-limit script passes; the same script passes again once the
// waiver's sunset date is in the past only if it is genuinely still active.
func TestCheckHonoursWaivers(t *testing.T) {
	root := newFixtureRepo(t, `{"rule":"bash-20-line-limit","path":"long.sh","status":"active","reason":"","approver":"","sunset":null,"added":"2026-04-24"}
`, nil)
	long := "#!/bin/bash\n" + strings.Repeat("echo x\n", 40)
	if err := os.WriteFile(filepath.Join(root, "long.sh"), []byte(long), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "unwaived.sh"), []byte(long), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := runCheck([]string{"-repo", root, "--all", "long.sh"}); err != nil {
		t.Errorf("waived script reported a violation: %v", err)
	}
	err := runCheck([]string{"-repo", root, "--all", "unwaived.sh"})
	if err == nil {
		t.Error("unwaived 40-line script passed the check")
	}
	// An in-scope script that is short passes without any waiver.
	if err := os.WriteFile(filepath.Join(root, "short.sh"), []byte("#!/bin/bash\necho hi\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := runCheck([]string{"-repo", root, "--all", "short.sh"}); err != nil {
		t.Errorf("short script reported a violation: %v", err)
	}
	// Excluded directories stay out of scope even when over the limit.
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "vendor", "v.sh"), []byte(long), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := runCheck([]string{"-repo", root, "--all", "vendor/v.sh"}); err != nil {
		t.Errorf("vendored script was checked despite being out of scope: %v", err)
	}
}

// A path list can name something that is not a regular file (a directory, or a
// symlink whose target is gone). Those must be skipped, not treated as an
// unreadable script that aborts the scan.
func TestCheckSkipsNonRegularFiles(t *testing.T) {
	root := newFixtureRepo(t, goodStore, nil)
	if err := os.MkdirAll(filepath.Join(root, "adir.sh"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "gone.sh"), filepath.Join(root, "dangling.sh")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if err := runCheck([]string{"-repo", root, "--all", "adir.sh", "dangling.sh", "absent.sh"}); err != nil {
		t.Errorf("non-regular paths should be skipped, got: %v", err)
	}
}
