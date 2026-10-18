package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Conformance: internal/deploy/SPEC.md DEP-29.
func TestNormalRegistryPairRepairsRuntimeInvalidLiveJobs(t *testing.T) {
	for _, command := range []string{"sync", "install"} {
		t.Run(command, func(t *testing.T) {
			repo := t.TempDir()
			home := t.TempDir()
			pulses := `{"pulses":[{"name":"next-tick","type":"file_mtime","path":"~/next","window":"1h"}]}`
			jobs := `{"jobs":[{"name":"next-job","pulse":"next-tick"}]}`
			mustWrite(t, filepath.Join(repo, "deploy/absence-alarm/pulses.json"), pulses)
			mustWrite(t, filepath.Join(repo, "custom/jobs.json"), jobs)
			mustWrite(t, filepath.Join(repo, "deploy/manifest.yaml"), `artifacts:
  - name: recovery-loop-jobs
    source: custom/jobs.json
    deployed: ~/.config/dear-agent/recovery-loop-jobs.json
    mode: "0644"
  - name: absence-alarm-pulses
    source: deploy/absence-alarm/pulses.json
    deployed: ~/.config/dear-agent/absence-alarm-pulses.json
    mode: "0644"
`)

			pulsePath := filepath.Join(home, ".config/dear-agent/absence-alarm-pulses.json")
			jobsPath := filepath.Join(home, ".config/dear-agent/recovery-loop-jobs.json")
			mustWrite(t, pulsePath,
				`{"pulses":[{"name":"stale-tick","type":"file_mtime","path":"~/stale","window":"1h"}]}`)
			mustWrite(t, jobsPath, `{"jobs":[`)

			code, out, errs := invoke(t, repo, home, command)
			if code != 0 {
				t.Fatalf("%s exit=%d stdout=%s stderr=%s", command, code, out, errs)
			}
			for path, want := range map[string]string{pulsePath: pulses, jobsPath: jobs} {
				got, err := os.ReadFile(path)
				if err != nil || string(got) != want {
					t.Fatalf("%s repaired %s: got=%s err=%v want=%s", command, path, got, err, want)
				}
			}
		})
	}
}
