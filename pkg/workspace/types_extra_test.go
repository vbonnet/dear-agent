package workspace

import "testing"

func TestDetectionMethod_String(t *testing.T) {
	tests := []struct {
		m    DetectionMethod
		want string
	}{
		{MethodFlag, "flag"},
		{MethodEnvVar, "env_var"},
		{MethodAutoDetect, "auto_detect"},
		{MethodDefault, "default"},
		{MethodInteractive, "interactive"},
		{MethodError, "error"},
		{DetectionMethod(99), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.m.String(); got != tt.want {
				t.Errorf("DetectionMethod(%d).String() = %q, want %q", tt.m, got, tt.want)
			}
		})
	}
}

func TestSettingsResolver_LevelSetters(t *testing.T) {
	r := NewSettingsResolver(nil, nil)

	// Level 4: component global is consulted when nothing higher is set.
	r.SetComponentGlobal(map[string]interface{}{"log_level": "global"})
	if got := r.ResolveSetting("log_level", "fallback"); got != "global" {
		t.Errorf("after SetComponentGlobal: got %v, want %q", got, "global")
	}

	// Level 5: component overrides beat global.
	r.SetComponentOverrides(map[string]interface{}{"log_level": "override"})
	if got := r.ResolveSetting("log_level", "fallback"); got != "override" {
		t.Errorf("after SetComponentOverrides: got %v, want %q", got, "override")
	}

	// Level 7: CLI flags beat everything below.
	r.SetCLIFlags(map[string]interface{}{"log_level": "cli"})
	if got := r.ResolveSetting("log_level", "fallback"); got != "cli" {
		t.Errorf("after SetCLIFlags: got %v, want %q", got, "cli")
	}
}
