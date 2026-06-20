package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/vroom/goplswatch"
)

func TestEmitReportClean(t *testing.T) {
	var buf bytes.Buffer
	s := goplswatch.Sample{Procs: []goplswatch.Proc{{PID: 1, PPID: 500, RSSKiB: 100 * 1024, Cmd: "gopls"}}, FDFraction: 0.2}
	emitReport(&buf, s, nil, nil, config{thresholds: goplswatch.DefaultThresholds})
	got := buf.String()
	if !strings.Contains(got, "Status: OK") {
		t.Errorf("clean report missing OK status:\n%s", got)
	}
}

func TestEmitReportAlarmWithRemediation(t *testing.T) {
	var buf bytes.Buffer
	s := goplswatch.Sample{
		Procs:      []goplswatch.Proc{{PID: 1, PPID: 1, RSSKiB: 3000 * 1024, Cmd: "gopls"}},
		FDFraction: 0.95,
	}
	alarms := goplswatch.Evaluate(s, goplswatch.DefaultThresholds)
	rem := &reapResult{OrphansFound: 1, Killed: 1, KilledPIDs: []int{1}}
	emitReport(&buf, s, alarms, rem, config{thresholds: goplswatch.DefaultThresholds})
	got := buf.String()
	if !strings.Contains(got, "Status: ALARM") {
		t.Errorf("alarm report missing ALARM status:\n%s", got)
	}
	if !strings.Contains(got, "reaped 1/1") {
		t.Errorf("alarm report missing remediation summary:\n%s", got)
	}
}

func TestEmitJSONShape(t *testing.T) {
	var buf bytes.Buffer
	s := goplswatch.Sample{
		Procs:      []goplswatch.Proc{{PID: 1, PPID: 1, RSSKiB: 3000 * 1024, Cmd: "gopls"}},
		FDFraction: 0.95,
	}
	cfg := config{thresholds: goplswatch.DefaultThresholds}
	alarms := goplswatch.Evaluate(s, cfg.thresholds)
	if err := emitJSON(&buf, s, alarms, nil, cfg); err != nil {
		t.Fatalf("emitJSON: %v", err)
	}
	var got struct {
		GoplsCount int  `json:"gopls_count"`
		OK         bool `json:"ok"`
		Alarms     []struct {
			Metric string `json:"metric"`
		} `json:"alarms"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, buf.String())
	}
	if got.OK {
		t.Errorf("ok should be false when alarms fired")
	}
	if got.GoplsCount != 1 {
		t.Errorf("gopls_count = %d, want 1", got.GoplsCount)
	}
	if len(got.Alarms) == 0 {
		t.Errorf("expected alarms in JSON output")
	}
}

func TestDefaultTrailPath(t *testing.T) {
	p := defaultTrailPath()
	if !strings.HasSuffix(p, ".agm/vroom/trail.jsonl") {
		t.Errorf("defaultTrailPath() = %q, want suffix .agm/vroom/trail.jsonl", p)
	}
}
