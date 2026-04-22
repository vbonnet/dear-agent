package progress

import (
	"errors"
	"fmt"
	"testing"
)

func TestModeSelection(t *testing.T) {
	t.Run("Total == 0 creates spinner", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Test"})
		if p.mode != ModeSpinner {
			t.Errorf("Expected ModeSpinner, got %v", p.mode)
		}
		if p.spinnerBackend == nil {
			t.Error("spinnerBackend should not be nil")
		}
		if p.barBackend != nil {
			t.Error("barBackend should be nil")
		}
	})

	t.Run("Total > 0 creates progress bar", func(t *testing.T) {
		p := New(Options{Total: 100, Label: "Test"})
		if p.mode != ModeProgressBar {
			t.Errorf("Expected ModeProgressBar, got %v", p.mode)
		}
		if p.barBackend == nil {
			t.Error("barBackend should not be nil")
		}
		if p.spinnerBackend != nil {
			t.Error("spinnerBackend should be nil")
		}
	})
}

func TestUpdatePhase(t *testing.T) {
	// Test phase formatting
	tests := []struct {
		current  int
		total    int
		name     string
		expected string
	}{
		{1, 11, "W0 - Project Framing", "Phase 1/11: W0 - Project Framing"},
		{3, 11, "D2 - Existing Solutions", "Phase 3/11: D2 - Existing Solutions"},
		{11, 11, "S11 - Retrospective", "Phase 11/11: S11 - Retrospective"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			message := fmt.Sprintf("Phase %d/%d: %s", tt.current, tt.total, tt.name)
			if message != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, message)
			}
		})
	}
}

func TestIdempotentOperations(t *testing.T) {
	t.Run("Multiple Start calls are safe", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Test"})
		p.Start()
		p.Start() // Should not panic or error
		if !p.started {
			t.Error("Expected started to be true")
		}
	})

	t.Run("Complete without Start is safe", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Test"})
		p.Complete("Done") // Should not panic or error
		if p.started {
			t.Error("Expected started to be false")
		}
	})

	t.Run("Multiple Complete calls are safe", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Test"})
		p.Start()
		p.Complete("Done")
		p.Complete("Done") // Should not panic or error
		if p.started {
			t.Error("Expected started to be false")
		}
	})
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Total != 0 {
		t.Errorf("Expected Total to be 0, got %d", opts.Total)
	}
	if opts.Label != "" {
		t.Errorf("Expected Label to be empty, got %q", opts.Label)
	}
	if !opts.ShowETA {
		t.Error("Expected ShowETA to be true")
	}
	if !opts.ShowPercent {
		t.Error("Expected ShowPercent to be true")
	}
}

func TestUpdateWithoutStart(t *testing.T) {
	t.Run("spinner mode update without start is no-op", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Test"})
		// Should not panic
		p.Update(0, "new message")
		if p.started {
			t.Error("Expected started to be false")
		}
	})

	t.Run("progress bar mode update without start is no-op", func(t *testing.T) {
		p := New(Options{Total: 10, Label: "Test"})
		// Should not panic
		p.Update(5, "new message")
		if p.started {
			t.Error("Expected started to be false")
		}
	})
}

func TestUpdateAfterStart(t *testing.T) {
	t.Run("spinner mode update after start", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Test"})
		p.Start()
		// Should not panic in non-TTY mode
		p.Update(0, "updated message")
		p.Update(0, "")
		p.Complete("done")
	})

	t.Run("progress bar mode update after start", func(t *testing.T) {
		p := New(Options{Total: 10, Label: "Test"})
		p.Start()
		// Should not panic in non-TTY mode
		p.Update(1, "step 1")
		p.Update(5, "step 5")
		p.Update(10, "")
		p.Complete("done")
	})
}

func TestUpdatePhaseMethod(t *testing.T) {
	t.Run("UpdatePhase calls Update with formatted message", func(t *testing.T) {
		p := New(Options{Total: 11, Label: "Wayfinder"})
		p.Start()
		// Should not panic; exercises the actual UpdatePhase method
		p.UpdatePhase(1, 11, "W0 - Project Framing")
		p.UpdatePhase(5, 11, "D4 - Architecture")
		p.UpdatePhase(11, 11, "S11 - Retrospective")
		p.Complete("All phases done")
	})

	t.Run("UpdatePhase without start is no-op", func(t *testing.T) {
		p := New(Options{Total: 11, Label: "Wayfinder"})
		// Should not panic
		p.UpdatePhase(1, 11, "W0")
	})
}

func TestFail(t *testing.T) {
	t.Run("Fail on spinner mode", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Test"})
		p.Start()
		p.Fail(errors.New("something went wrong"))
		if p.started {
			t.Error("Expected started to be false after Fail")
		}
	})

	t.Run("Fail on progress bar mode", func(t *testing.T) {
		p := New(Options{Total: 10, Label: "Test"})
		p.Start()
		p.Fail(errors.New("something went wrong"))
		if p.started {
			t.Error("Expected started to be false after Fail")
		}
	})

	t.Run("Fail without Start is no-op", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Test"})
		p.Fail(errors.New("err"))
		if p.started {
			t.Error("Expected started to be false")
		}
	})

	t.Run("Multiple Fail calls are safe", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Test"})
		p.Start()
		p.Fail(errors.New("err"))
		p.Fail(errors.New("err")) // second call is no-op
		if p.started {
			t.Error("Expected started to be false")
		}
	})

	t.Run("Fail with nil error", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Test"})
		p.Start()
		// nil error should not panic
		p.Fail(nil)
		if p.started {
			t.Error("Expected started to be false after Fail")
		}
	})
}

func TestCompleteProgressBar(t *testing.T) {
	t.Run("Complete on progress bar mode", func(t *testing.T) {
		p := New(Options{Total: 5, Label: "Test"})
		p.Start()
		p.Update(3, "halfway")
		p.Complete("finished")
		if p.started {
			t.Error("Expected started to be false after Complete")
		}
	})
}

func TestNewWithVariousOptions(t *testing.T) {
	t.Run("empty label spinner", func(t *testing.T) {
		p := New(Options{Total: 0, Label: ""})
		if p.mode != ModeSpinner {
			t.Errorf("Expected ModeSpinner, got %v", p.mode)
		}
		if p.opts.Label != "" {
			t.Errorf("Expected empty label, got %q", p.opts.Label)
		}
	})

	t.Run("options with ShowETA and ShowPercent", func(t *testing.T) {
		p := New(Options{Total: 50, Label: "Processing", ShowETA: true, ShowPercent: true})
		if p.mode != ModeProgressBar {
			t.Errorf("Expected ModeProgressBar, got %v", p.mode)
		}
		if p.opts.Total != 50 {
			t.Errorf("Expected Total 50, got %d", p.opts.Total)
		}
		if !p.opts.ShowETA {
			t.Error("Expected ShowETA to be true")
		}
		if !p.opts.ShowPercent {
			t.Error("Expected ShowPercent to be true")
		}
	})

	t.Run("options with ShowETA false", func(t *testing.T) {
		p := New(Options{Total: 10, Label: "Test", ShowETA: false, ShowPercent: false})
		if p.opts.ShowETA {
			t.Error("Expected ShowETA to be false")
		}
		if p.opts.ShowPercent {
			t.Error("Expected ShowPercent to be false")
		}
	})

	t.Run("Total == 1 creates progress bar", func(t *testing.T) {
		p := New(Options{Total: 1, Label: "Single step"})
		if p.mode != ModeProgressBar {
			t.Errorf("Expected ModeProgressBar, got %v", p.mode)
		}
	})
}

func TestModeConstants(t *testing.T) {
	if ModeSpinner != 0 {
		t.Errorf("Expected ModeSpinner == 0, got %d", ModeSpinner)
	}
	if ModeProgressBar != 1 {
		t.Errorf("Expected ModeProgressBar == 1, got %d", ModeProgressBar)
	}
}

func TestSpinnerBackend(t *testing.T) {
	t.Run("newSpinnerBackend with label", func(t *testing.T) {
		sb := newSpinnerBackend(Options{Label: "Loading"})
		if sb.message != "Loading" {
			t.Errorf("Expected message 'Loading', got %q", sb.message)
		}
		// In non-TTY, spinner field should be nil
		if !IsTTY() && sb.spinner != nil {
			t.Error("Expected spinner to be nil in non-TTY mode")
		}
	})

	t.Run("newSpinnerBackend with empty label", func(t *testing.T) {
		sb := newSpinnerBackend(Options{Label: ""})
		if sb.message != "" {
			t.Errorf("Expected empty message, got %q", sb.message)
		}
	})

	t.Run("spinner Start in non-TTY", func(t *testing.T) {
		sb := newSpinnerBackend(Options{Label: "Test"})
		// Should not panic
		sb.Start()
	})

	t.Run("spinner Update in non-TTY", func(t *testing.T) {
		sb := newSpinnerBackend(Options{Label: "Test"})
		sb.Start()
		sb.Update("new message")
		if sb.message != "new message" {
			t.Errorf("Expected message 'new message', got %q", sb.message)
		}
	})

	t.Run("spinner Update with empty string preserves old message", func(t *testing.T) {
		sb := newSpinnerBackend(Options{Label: "original"})
		sb.Update("")
		if sb.message != "original" {
			t.Errorf("Expected message 'original', got %q", sb.message)
		}
	})

	t.Run("spinner Stop in non-TTY", func(t *testing.T) {
		sb := newSpinnerBackend(Options{Label: "Test"})
		sb.Start()
		// Should not panic
		sb.Stop()
	})
}

func TestProgressBarBackend(t *testing.T) {
	t.Run("newProgressBarBackend with label", func(t *testing.T) {
		pb := newProgressBarBackend(Options{Total: 10, Label: "Processing"})
		if pb.total != 10 {
			t.Errorf("Expected total 10, got %d", pb.total)
		}
		// In non-TTY, bar field should be nil
		if !IsTTY() && pb.bar != nil {
			t.Error("Expected bar to be nil in non-TTY mode")
		}
	})

	t.Run("newProgressBarBackend with empty label", func(t *testing.T) {
		pb := newProgressBarBackend(Options{Total: 5, Label: ""})
		if pb.total != 5 {
			t.Errorf("Expected total 5, got %d", pb.total)
		}
	})

	t.Run("progressbar Start in non-TTY", func(t *testing.T) {
		pb := newProgressBarBackend(Options{Total: 10, Label: "Test"})
		// Should not panic
		pb.Start()
	})

	t.Run("progressbar Update in non-TTY with message", func(t *testing.T) {
		pb := newProgressBarBackend(Options{Total: 10, Label: "Test"})
		pb.Start()
		// Should print percentage in non-TTY mode
		pb.Update(5, "halfway")
	})

	t.Run("progressbar Update in non-TTY without message", func(t *testing.T) {
		pb := newProgressBarBackend(Options{Total: 10, Label: "Test"})
		pb.Start()
		pb.Update(5, "")
	})

	t.Run("progressbar Update with zero total", func(t *testing.T) {
		pb := newProgressBarBackend(Options{Total: 0, Label: "Test"})
		pb.Start()
		// total == 0, so percentage branch is skipped
		pb.Update(0, "message")
	})

	t.Run("progressbar Stop in non-TTY", func(t *testing.T) {
		pb := newProgressBarBackend(Options{Total: 10, Label: "Test"})
		pb.Start()
		// Should not panic
		pb.Stop()
	})
}

func TestTTYFunctions(t *testing.T) {
	t.Run("IsTTY returns bool", func(t *testing.T) {
		// In test environment this will be false, but function should not panic
		result := IsTTY()
		// Just verify it returns without error
		_ = result
	})

	t.Run("GetTerminalWidth in non-TTY", func(t *testing.T) {
		if !IsTTY() {
			width := GetTerminalWidth()
			if width != 0 {
				t.Errorf("Expected 0 in non-TTY mode, got %d", width)
			}
		}
	})
}

func TestFullLifecycleSpinner(t *testing.T) {
	p := New(Options{Total: 0, Label: "Downloading"})
	p.Start()
	p.Update(0, "50%")
	p.Update(0, "75%")
	p.Update(0, "100%")
	p.Complete("Download complete")
	if p.started {
		t.Error("Expected started to be false after Complete")
	}
}

func TestFullLifecycleProgressBar(t *testing.T) {
	p := New(Options{Total: 5, Label: "Installing"})
	p.Start()
	for i := 1; i <= 5; i++ {
		p.Update(i, fmt.Sprintf("Step %d", i))
	}
	p.Complete("Installation complete")
	if p.started {
		t.Error("Expected started to be false after Complete")
	}
}

func TestFullLifecycleWithFail(t *testing.T) {
	t.Run("spinner lifecycle with fail", func(t *testing.T) {
		p := New(Options{Total: 0, Label: "Working"})
		p.Start()
		p.Update(0, "trying...")
		p.Fail(errors.New("network timeout"))
		if p.started {
			t.Error("Expected started to be false after Fail")
		}
	})

	t.Run("progress bar lifecycle with fail", func(t *testing.T) {
		p := New(Options{Total: 10, Label: "Working"})
		p.Start()
		p.Update(3, "step 3")
		p.Fail(errors.New("disk full"))
		if p.started {
			t.Error("Expected started to be false after Fail")
		}
	})
}

func TestStartIdempotentProgressBar(t *testing.T) {
	p := New(Options{Total: 10, Label: "Test"})
	p.Start()
	p.Start() // second call should be no-op
	if !p.started {
		t.Error("Expected started to be true")
	}
	p.Complete("done")
}

func TestCompleteAndFailIdempotentProgressBar(t *testing.T) {
	t.Run("Complete without Start on progress bar", func(t *testing.T) {
		p := New(Options{Total: 10, Label: "Test"})
		p.Complete("done") // no-op
		if p.started {
			t.Error("Expected started to be false")
		}
	})

	t.Run("Fail without Start on progress bar", func(t *testing.T) {
		p := New(Options{Total: 10, Label: "Test"})
		p.Fail(errors.New("err")) // no-op
		if p.started {
			t.Error("Expected started to be false")
		}
	})
}
