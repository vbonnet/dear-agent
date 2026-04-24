package stophook

import "testing"

func TestResult_ExitCode(t *testing.T) {
	r := &Result{HookName: "test"}
	if r.ExitCode() != 0 {
		t.Error("empty result should exit 0")
	}

	r.Pass("check1", "ok")
	if r.ExitCode() != 0 {
		t.Error("pass-only should exit 0")
	}

	r.Warn("check2", "warning", "fix it")
	if r.ExitCode() != 0 {
		t.Error("warn-only should exit 0")
	}

	r.Block("check3", "blocked", "fix it now")
	if r.ExitCode() != 2 {
		t.Error("block should exit 2")
	}
}

func TestResult_HasBlocking(t *testing.T) {
	r := &Result{HookName: "test"}
	if r.HasBlocking() {
		t.Error("empty result should not have blocking")
	}

	r.Warn("check", "warning", "")
	if r.HasBlocking() {
		t.Error("warn-only should not have blocking")
	}

	r.Block("check2", "blocked", "")
	if !r.HasBlocking() {
		t.Error("should have blocking")
	}
}
