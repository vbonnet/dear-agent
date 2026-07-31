package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOperatorGrantInstallArgsBindProbeToActualInstaller(t *testing.T) {
	const path = "/etc/dear-agent/override-grants/admin.json"
	for _, tt := range []struct {
		name           string
		nonInteractive bool
		want           []string
	}{
		{
			name:           "probe",
			nonInteractive: true,
			want: []string{
				"-k",
				"-n",
				"/bin/sh",
				"-c",
				unixOperatorGrantInstallScript,
				"dear-agent-override-grant-installer",
				path,
			},
		},
		{
			name: "prompt",
			want: []string{
				"-k",
				"/bin/sh",
				"-c",
				unixOperatorGrantInstallScript,
				"dear-agent-override-grant-installer",
				path,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := operatorGrantInstallArgs(path, tt.nonInteractive)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("operatorGrantInstallArgs() = %q, want %q", got, tt.want)
			}
			if strings.Contains(strings.Join(got, " "), "-v") {
				t.Fatalf("fresh authentication args use sudo validation pseudocommand: %q", got)
			}
		})
	}
}

func TestOperatorGrantInstallProbeDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grant.json")
	cmd := exec.Command(
		unixOperatorGrantInstaller,
		"-c",
		unixOperatorGrantInstallScript,
		"dear-agent-override-grant-installer",
		path,
	)
	cmd.Stdin = strings.NewReader(unixOperatorGrantProbeInput)
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != unixOperatorGrantProbeExitCode {
		t.Fatalf("probe error = %v, want exit %d", err, unixOperatorGrantProbeExitCode)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe created grant path: stat error = %v", err)
	}
}

func TestOperatorGrantInstallerWritesOnlyInstallPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "grant.json")
	const payload = "{\"approved\":true}\n"
	cmd := exec.Command(
		unixOperatorGrantInstaller,
		"-c",
		unixOperatorGrantInstallScript,
		"dear-agent-override-grant-installer",
		path,
	)
	cmd.Stdin = strings.NewReader(unixOperatorGrantInstallInput + payload)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("installer error = %v: %s", err, output)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read installed grant: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("installed grant = %q, want %q", got, payload)
	}
}

func TestOperatorGrantInstallerRefusesSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(dir, "victim")
	path := filepath.Join(dir, "grant.json")
	if err := os.WriteFile(victim, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(
		unixOperatorGrantInstaller,
		"-c",
		unixOperatorGrantInstallScript,
		"dear-agent-override-grant-installer",
		path,
	)
	cmd.Stdin = strings.NewReader(unixOperatorGrantInstallInput + "{\"approved\":true}\n")
	if err := cmd.Run(); err == nil {
		t.Fatal("installer accepted a symlinked grant target")
	}
	got, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "keep" {
		t.Fatalf("symlink target changed to %q", got)
	}
}
