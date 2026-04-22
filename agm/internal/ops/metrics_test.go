package ops

import (
	"testing"
	"time"
)

func TestParseLoadAvg(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    LoadMetrics
	}{
		{
			name:    "normal load",
			content: "0.50 1.20 0.80 2/350 12345",
			want:    LoadMetrics{Load1: 0.5, Load5: 1.2, Load15: 0.8},
		},
		{
			name:    "high load",
			content: "15.30 12.10 8.50 5/400 99999",
			want:    LoadMetrics{Load1: 15.3, Load5: 12.1, Load15: 8.5},
		},
		{
			name:    "empty",
			content: "",
			want:    LoadMetrics{},
		},
		{
			name:    "too few fields",
			content: "0.50",
			want:    LoadMetrics{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLoadAvg(tt.content)
			if got != tt.want {
				t.Errorf("parseLoadAvg(%q) = %+v, want %+v", tt.content, got, tt.want)
			}
		})
	}
}

func TestParseMemInfo(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    MemoryMetrics
	}{
		{
			name: "typical meminfo",
			content: `MemTotal:       16384000 kB
MemFree:         2048000 kB
MemAvailable:    8192000 kB
Buffers:          512000 kB
Cached:          4096000 kB
SwapTotal:       2048000 kB
SwapFree:        2048000 kB`,
			want: MemoryMetrics{
				TotalMB:     16000,
				UsedMB:      8000,
				AvailableMB: 8000,
				UsedPercent: 50.0,
			},
		},
		{
			name: "high memory usage",
			content: `MemTotal:       8192000 kB
MemFree:          409600 kB
MemAvailable:     819200 kB`,
			want: MemoryMetrics{
				TotalMB:     8000,
				UsedMB:      7200,
				AvailableMB: 800,
				UsedPercent: 90.0,
			},
		},
		{
			name:    "empty",
			content: "",
			want:    MemoryMetrics{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseMemInfo(tt.content)
			if got != tt.want {
				t.Errorf("parseMemInfo() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGenerateAlerts_Load(t *testing.T) {
	res := ResourceMetrics{
		Load: LoadMetrics{Load1: 15.0, Load5: 10.0, Load15: 8.0},
	}
	alerts := generateAlerts(res, ThroughputMetrics{CommitsPerHour: 5}, nil, time.Now())

	found := false
	for _, a := range alerts {
		if a.Type == "load" {
			found = true
			if a.Level != "critical" {
				t.Errorf("expected critical level for load alert, got %q", a.Level)
			}
		}
	}
	if !found {
		t.Error("expected load alert when load > 12")
	}
}

func TestGenerateAlerts_NoAlerts(t *testing.T) {
	res := ResourceMetrics{
		Load:   LoadMetrics{Load1: 2.0, Load5: 1.5, Load15: 1.0},
		Memory: MemoryMetrics{UsedPercent: 50.0},
		Disk:   []DiskMetrics{{Mount: "/", UsedPercent: 40.0}},
	}
	alerts := generateAlerts(res, ThroughputMetrics{CommitsPerHour: 5}, nil, time.Now())

	if len(alerts) != 0 {
		t.Errorf("expected no alerts, got %d: %+v", len(alerts), alerts)
	}
}

func TestGenerateAlerts_Memory(t *testing.T) {
	res := ResourceMetrics{
		Memory: MemoryMetrics{UsedPercent: 85.0},
	}
	alerts := generateAlerts(res, ThroughputMetrics{CommitsPerHour: 1}, nil, time.Now())

	found := false
	for _, a := range alerts {
		if a.Type == "memory" {
			found = true
			if a.Level != "warning" {
				t.Errorf("expected warning level for memory alert, got %q", a.Level)
			}
		}
	}
	if !found {
		t.Error("expected memory alert when usage > 80%")
	}
}

func TestGenerateAlerts_Disk(t *testing.T) {
	res := ResourceMetrics{
		Disk: []DiskMetrics{
			{Mount: "/", UsedPercent: 90.0},
			{Mount: "/home", UsedPercent: 50.0},
		},
	}
	alerts := generateAlerts(res, ThroughputMetrics{CommitsPerHour: 1}, nil, time.Now())

	found := false
	for _, a := range alerts {
		if a.Type == "disk" {
			found = true
		}
	}
	if !found {
		t.Error("expected disk alert when usage > 85%")
	}
}

func TestGenerateAlerts_Throughput(t *testing.T) {
	res := ResourceMetrics{}
	alerts := generateAlerts(res, ThroughputMetrics{CommitsPerHour: 0}, nil, time.Now())

	found := false
	for _, a := range alerts {
		if a.Type == "throughput" {
			found = true
		}
	}
	if !found {
		t.Error("expected throughput alert when commits = 0")
	}
}

func TestCollectThroughputMetrics_WorkersLaunched(t *testing.T) {
	now := time.Now()
	sessions := []SessionSummary{
		{CreatedAt: now.Add(-30 * time.Minute).Format("2006-01-02T15:04:05Z")},
		{CreatedAt: now.Add(-90 * time.Minute).Format("2006-01-02T15:04:05Z")},
		{CreatedAt: now.Add(-10 * time.Minute).Format("2006-01-02T15:04:05Z")},
	}

	tp := collectThroughputMetrics(sessions, now, time.Hour)

	if tp.WorkersLaunched != 2 {
		t.Errorf("expected 2 workers launched in last hour, got %d", tp.WorkersLaunched)
	}
	if tp.WindowSeconds != 3600 {
		t.Errorf("expected window of 3600s, got %d", tp.WindowSeconds)
	}
}

func TestCollectCostMetrics(t *testing.T) {
	t.Run("only role:worker sessions counted", func(t *testing.T) {
		sessions := []SessionSummary{
			{Name: "worker-1", Status: "active", EstimatedCost: 10.0, Tags: []string{"role:worker"}},
			{Name: "worker-2", Status: "active", EstimatedCost: 20.0, Tags: []string{"role:worker"}},
			{Name: "archived-1", Status: "archived", EstimatedCost: 5.0, Tags: []string{"role:worker"}},
		}

		cm := collectCostMetrics(sessions)

		if cm.TotalSpend != 35.0 {
			t.Errorf("TotalSpend = %f, want 35.0", cm.TotalSpend)
		}
		if cm.WorkerCount != 2 {
			t.Errorf("WorkerCount = %d, want 2", cm.WorkerCount)
		}
		if cm.CostPerWorker != 17.5 {
			t.Errorf("CostPerWorker = %f, want 17.5", cm.CostPerWorker)
		}
		if cm.CommitCount != -1 {
			t.Errorf("CommitCount = %d, want -1 (sentinel)", cm.CommitCount)
		}
		if cm.CostPerCommit != -1 {
			t.Errorf("CostPerCommit = %f, want -1 (sentinel)", cm.CostPerCommit)
		}
	})

	t.Run("human sessions excluded from worker count", func(t *testing.T) {
		sessions := []SessionSummary{
			{Name: "human-debug", Status: "active", EstimatedCost: 10.0},
			{Name: "worker-1", Status: "active", EstimatedCost: 20.0, Tags: []string{"role:worker"}},
		}

		cm := collectCostMetrics(sessions)

		if cm.TotalSpend != 30.0 {
			t.Errorf("TotalSpend = %f, want 30.0", cm.TotalSpend)
		}
		if cm.WorkerCount != 1 {
			t.Errorf("WorkerCount = %d, want 1 (only role:worker)", cm.WorkerCount)
		}
	})

	t.Run("supervisor sessions excluded from worker count", func(t *testing.T) {
		sessions := []SessionSummary{
			{Name: "orchestrator", Status: "active", EstimatedCost: 5.0, Tags: []string{"role:orchestrator"}},
			{Name: "worker-1", Status: "active", EstimatedCost: 15.0, Tags: []string{"role:worker"}},
		}

		cm := collectCostMetrics(sessions)

		if cm.WorkerCount != 1 {
			t.Errorf("WorkerCount = %d, want 1 (only role:worker, not role:orchestrator)", cm.WorkerCount)
		}
	})

	t.Run("no sessions", func(t *testing.T) {
		cm := collectCostMetrics(nil)

		if cm.TotalSpend != 0 {
			t.Errorf("TotalSpend = %f, want 0", cm.TotalSpend)
		}
		if cm.CostPerWorker != 0 {
			t.Errorf("CostPerWorker = %f, want 0", cm.CostPerWorker)
		}
	})

	t.Run("all archived", func(t *testing.T) {
		sessions := []SessionSummary{
			{Name: "old-1", Status: "archived", EstimatedCost: 10.0, Tags: []string{"role:worker"}},
		}

		cm := collectCostMetrics(sessions)

		if cm.TotalSpend != 10.0 {
			t.Errorf("TotalSpend = %f, want 10.0", cm.TotalSpend)
		}
		if cm.WorkerCount != 0 {
			t.Errorf("WorkerCount = %d, want 0 (all archived)", cm.WorkerCount)
		}
		if cm.CostPerWorker != 0 {
			t.Errorf("CostPerWorker = %f, want 0 (no active workers)", cm.CostPerWorker)
		}
	})

	t.Run("zero cost sessions without role tag", func(t *testing.T) {
		sessions := []SessionSummary{
			{Name: "no-cost", Status: "active", EstimatedCost: 0},
			{Name: "no-cost-2", Status: "stopped", EstimatedCost: 0},
		}

		cm := collectCostMetrics(sessions)

		if cm.TotalSpend != 0 {
			t.Errorf("TotalSpend = %f, want 0", cm.TotalSpend)
		}
		if cm.WorkerCount != 0 {
			t.Errorf("WorkerCount = %d, want 0 (no role:worker tag)", cm.WorkerCount)
		}
		if cm.CostPerWorker != 0 {
			t.Errorf("CostPerWorker = %f, want 0", cm.CostPerWorker)
		}
	})
}

func TestHasWorkerTag(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want bool
	}{
		{"nil tags", nil, false},
		{"empty tags", []string{}, false},
		{"has role:worker", []string{"role:worker"}, true},
		{"has role:worker among others", []string{"cap:web-search", "role:worker"}, true},
		{"has other role", []string{"role:orchestrator"}, false},
		{"has no role tag", []string{"cap:claude-code"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasWorkerTag(tt.tags)
			if got != tt.want {
				t.Errorf("hasWorkerTag(%v) = %v, want %v", tt.tags, got, tt.want)
			}
		})
	}
}

func TestRoundTo1(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{1.0, 1.0},
		{1.15, 1.2},
		{1.14, 1.1},
		{99.99, 100.0},
		{0.0, 0.0},
	}
	for _, tt := range tests {
		got := roundTo1(tt.in)
		if got != tt.want {
			t.Errorf("roundTo1(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
