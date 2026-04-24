package capacity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetector_ReadMeminfo(t *testing.T) {
	// Create a fake /proc/meminfo
	dir := t.TempDir()
	meminfo := filepath.Join(dir, "meminfo")
	content := `MemTotal:       32456780 kB
MemFree:         1234567 kB
MemAvailable:   16228390 kB
Buffers:          234567 kB
Cached:          8901234 kB
`
	if err := os.WriteFile(meminfo, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	d := &Detector{
		procMeminfo:     meminfo,
		cpuCountFunc:    func() int { return 8 },
		claudeCountFunc: func() (int, error) { return 3, nil },
	}

	info, err := d.Detect()
	if err != nil {
		t.Fatalf("Detect() error: %v", err)
	}

	// MemTotal: 32456780 kB = 32456780 * 1024 bytes
	expectedTotal := uint64(32456780) * 1024
	if info.TotalRAMBytes != expectedTotal {
		t.Errorf("TotalRAMBytes = %d, want %d", info.TotalRAMBytes, expectedTotal)
	}

	expectedAvail := uint64(16228390) * 1024
	if info.AvailableRAMBytes != expectedAvail {
		t.Errorf("AvailableRAMBytes = %d, want %d", info.AvailableRAMBytes, expectedAvail)
	}

	if info.NumCPUs != 8 {
		t.Errorf("NumCPUs = %d, want 8", info.NumCPUs)
	}
	if info.ClaudeProcesses != 3 {
		t.Errorf("ClaudeProcesses = %d, want 3", info.ClaudeProcesses)
	}
}

func TestDetector_MissingMemTotal(t *testing.T) {
	dir := t.TempDir()
	meminfo := filepath.Join(dir, "meminfo")
	content := `MemAvailable:   16228390 kB
`
	os.WriteFile(meminfo, []byte(content), 0644)

	d := &Detector{
		procMeminfo:     meminfo,
		cpuCountFunc:    func() int { return 4 },
		claudeCountFunc: func() (int, error) { return 0, nil },
	}

	_, err := d.Detect()
	if err == nil {
		t.Fatal("expected error for missing MemTotal")
	}
}

func TestCalculator_SmallMachine(t *testing.T) {
	// 8GB RAM, 4 CPUs, 2 Claude processes
	// Available: 5GB
	c := NewCalculator()
	info := SystemInfo{
		TotalRAMBytes:     8 * 1024 * 1024 * 1024,
		AvailableRAMBytes: 5 * 1024 * 1024 * 1024,
		NumCPUs:           4,
		ClaudeProcesses:   2,
	}

	result := c.Calculate(info)

	// RAM-based: (5GB - 4GB) / 800MB = 1.25 → 1
	if result.RAMBasedMax != 1 {
		t.Errorf("RAMBasedMax = %d, want 1", result.RAMBasedMax)
	}
	// CPU-based: 4 * 2 = 8
	if result.CPUBasedMax != 8 {
		t.Errorf("CPUBasedMax = %d, want 8", result.CPUBasedMax)
	}
	// Max = min(1, 8, 20) = 1
	if result.MaxSessions != 1 {
		t.Errorf("MaxSessions = %d, want 1", result.MaxSessions)
	}
	// Available = max(0, 1 - 2) = 0
	if result.AvailableSlots != 0 {
		t.Errorf("AvailableSlots = %d, want 0", result.AvailableSlots)
	}
	// RAM usage: 3GB used / 8GB total = 37.5% → GREEN
	if result.Zone != ZoneGreen {
		t.Errorf("Zone = %s, want GREEN", result.Zone)
	}
}

func TestCalculator_LargeMachine(t *testing.T) {
	// 64GB RAM, 16 CPUs, 5 Claude processes
	// Available: 40GB
	c := NewCalculator()
	info := SystemInfo{
		TotalRAMBytes:     64 * 1024 * 1024 * 1024,
		AvailableRAMBytes: 40 * 1024 * 1024 * 1024,
		NumCPUs:           16,
		ClaudeProcesses:   5,
	}

	result := c.Calculate(info)

	// RAM-based: (40GB - 4GB) / 800MB = 36*1024/800 = 46
	if result.RAMBasedMax != 46 {
		t.Errorf("RAMBasedMax = %d, want 46", result.RAMBasedMax)
	}
	// CPU-based: 16 * 2 = 32
	if result.CPUBasedMax != 32 {
		t.Errorf("CPUBasedMax = %d, want 32", result.CPUBasedMax)
	}
	// Max = min(45, 32, 20) = 20 (hard cap)
	if result.MaxSessions != 20 {
		t.Errorf("MaxSessions = %d, want 20 (hard cap)", result.MaxSessions)
	}
	// Available = 20 - 5 = 15
	if result.AvailableSlots != 15 {
		t.Errorf("AvailableSlots = %d, want 15", result.AvailableSlots)
	}
}

func TestCalculator_HighPressure(t *testing.T) {
	// 16GB RAM, 8 CPUs, 10 Claude processes
	// Available: 2GB (less than reserved)
	c := NewCalculator()
	info := SystemInfo{
		TotalRAMBytes:     16 * 1024 * 1024 * 1024,
		AvailableRAMBytes: 2 * 1024 * 1024 * 1024,
		NumCPUs:           8,
		ClaudeProcesses:   10,
	}

	result := c.Calculate(info)

	// RAM-based: 2GB < 4GB reserved → 0
	if result.RAMBasedMax != 0 {
		t.Errorf("RAMBasedMax = %d, want 0", result.RAMBasedMax)
	}
	// Max = 0
	if result.MaxSessions != 0 {
		t.Errorf("MaxSessions = %d, want 0", result.MaxSessions)
	}
	// RAM usage: 14GB / 16GB = 87.5% → RED
	if result.Zone != ZoneRed {
		t.Errorf("Zone = %s, want RED", result.Zone)
	}
}

func TestCalculator_CriticalZone(t *testing.T) {
	// 31GB RAM, nearly exhausted
	c := NewCalculator()
	info := SystemInfo{
		TotalRAMBytes:     31 * 1024 * 1024 * 1024,
		AvailableRAMBytes: 2 * 1024 * 1024 * 1024,
		NumCPUs:           8,
		ClaudeProcesses:   19,
	}

	result := c.Calculate(info)

	// RAM usage: 29GB / 31GB = 93.5% → CRITICAL
	if result.Zone != ZoneCritical {
		t.Errorf("Zone = %s, want CRITICAL", result.Zone)
	}
}

func TestCalculator_EnvOverride(t *testing.T) {
	os.Setenv("AGM_MAX_SESSIONS", "3")
	defer os.Unsetenv("AGM_MAX_SESSIONS")

	c := NewCalculator()
	info := SystemInfo{
		TotalRAMBytes:     64 * 1024 * 1024 * 1024,
		AvailableRAMBytes: 40 * 1024 * 1024 * 1024,
		NumCPUs:           16,
		ClaudeProcesses:   1,
	}

	result := c.Calculate(info)

	if result.MaxSessions != 3 {
		t.Errorf("MaxSessions = %d, want 3 (env override)", result.MaxSessions)
	}
	if result.EnvOverride != 3 {
		t.Errorf("EnvOverride = %d, want 3", result.EnvOverride)
	}
}

func TestZoneClassification(t *testing.T) {
	tests := []struct {
		percent float64
		want    Zone
	}{
		{0, ZoneGreen},
		{30, ZoneGreen},
		{59.9, ZoneGreen},
		{60, ZoneYellow},
		{75, ZoneYellow},
		{80, ZoneYellow},
		{80.1, ZoneRed},
		{89, ZoneRed},
		{90, ZoneRed},
		{90.1, ZoneCritical},
		{95, ZoneCritical},
		{100, ZoneCritical},
	}
	for _, tt := range tests {
		got := classifyZone(tt.percent)
		if got != tt.want {
			t.Errorf("classifyZone(%.1f) = %s, want %s", tt.percent, got, tt.want)
		}
	}
}

func TestSystemInfo_Helpers(t *testing.T) {
	info := SystemInfo{
		TotalRAMBytes:     32 * 1024 * 1024 * 1024, // 32 GB
		AvailableRAMBytes: 16 * 1024 * 1024 * 1024, // 16 GB
	}

	if g := info.TotalRAMGB(); g < 31.9 || g > 32.1 {
		t.Errorf("TotalRAMGB() = %f, want ~32", g)
	}
	if g := info.AvailableRAMGB(); g < 15.9 || g > 16.1 {
		t.Errorf("AvailableRAMGB() = %f, want ~16", g)
	}
	if u := info.UsedRAMBytes(); u != 16*1024*1024*1024 {
		t.Errorf("UsedRAMBytes() = %d, want 16GB", u)
	}
	if p := info.RAMUsagePercent(); p < 49.9 || p > 50.1 {
		t.Errorf("RAMUsagePercent() = %f, want ~50", p)
	}
}

func TestCalculator_MediumMachine(t *testing.T) {
	// 32GB RAM, 8 CPUs, 4 Claude processes
	// Available: 20GB
	c := NewCalculator()
	info := SystemInfo{
		TotalRAMBytes:     32 * 1024 * 1024 * 1024,
		AvailableRAMBytes: 20 * 1024 * 1024 * 1024,
		NumCPUs:           8,
		ClaudeProcesses:   4,
	}

	result := c.Calculate(info)

	// RAM-based: (20GB - 4GB) / 800MB = 20
	if result.RAMBasedMax != 20 {
		t.Errorf("RAMBasedMax = %d, want 20", result.RAMBasedMax)
	}
	// CPU-based: 8 * 2 = 16
	if result.CPUBasedMax != 16 {
		t.Errorf("CPUBasedMax = %d, want 16", result.CPUBasedMax)
	}
	// Max = min(20, 16, 20) = 16
	if result.MaxSessions != 16 {
		t.Errorf("MaxSessions = %d, want 16", result.MaxSessions)
	}
	// Available = 16 - 4 = 12
	if result.AvailableSlots != 12 {
		t.Errorf("AvailableSlots = %d, want 12", result.AvailableSlots)
	}
	// RAM usage: 12GB / 32GB = 37.5% → GREEN
	if result.Zone != ZoneGreen {
		t.Errorf("Zone = %s, want GREEN", result.Zone)
	}
}
