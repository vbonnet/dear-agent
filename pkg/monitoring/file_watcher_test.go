package monitoring

import (
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

func TestIsGitFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/repo/.git/HEAD", true},
		{"/repo/.git/objects/pack/abc", true},
		{"/repo/.git", true},
		{"/repo/src/main.go", false},
		{"/repo/.github/workflows/ci.yml", false},
		{"/.git/config", true},
		{"/repo/.gitignore", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsGitFile(tt.path)
			if got != tt.want {
				t.Errorf("IsGitFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsTempFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/repo/file.go~", true},
		{"/repo/.file.swp", true},
		{"/repo/data.tmp", true},
		{"/repo/.#lock", true},
		{"/repo/#autosave#", true},
		{"/repo/main.go", false},
		{"/repo/test_test.go", false},
		{"/repo/.env", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsTempFile(tt.path)
			if got != tt.want {
				t.Errorf("IsTempFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsIDEFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/repo/.vscode/settings.json", true},
		{"/repo/.idea/workspace.xml", true},
		{"/repo/__pycache__/mod.pyc", true},
		{"/repo/node_modules/lodash/index.js", true},
		{"/repo/src/main.go", false},
		{"/repo/vendor/lib.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsIDEFile(tt.path)
			if got != tt.want {
				t.Errorf("IsIDEFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestShouldFilter(t *testing.T) {
	fw := &FileWatcher{
		agentID:  "test",
		workDir:  "/tmp",
		eventBus: eventbus.NewBus(nil),
		filters: []FileFilter{
			IsGitFile,
			IsTempFile,
			IsIDEFile,
		},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"/repo/.git/HEAD", true},
		{"/repo/file.go~", true},
		{"/repo/.vscode/settings.json", true},
		{"/repo/node_modules/x/y.js", true},
		{"/repo/src/main.go", false},
		{"/repo/pkg/lib/util.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := fw.shouldFilter(tt.path)
			if got != tt.want {
				t.Errorf("shouldFilter(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestShouldFilter_CustomFilter(t *testing.T) {
	fw := &FileWatcher{
		agentID:  "test",
		workDir:  "/tmp",
		eventBus: eventbus.NewBus(nil),
		filters:  []FileFilter{},
	}

	// No filters: nothing filtered
	if fw.shouldFilter("/repo/.git/HEAD") {
		t.Error("expected no filtering with empty filter list")
	}

	// Add custom filter
	fw.AddFilter(func(path string) bool {
		return path == "/block/this"
	})

	if !fw.shouldFilter("/block/this") {
		t.Error("expected custom filter to block /block/this")
	}
	if fw.shouldFilter("/allow/this") {
		t.Error("expected custom filter to allow /allow/this")
	}
}

func TestHandleEvent_MapsOperations(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()

	fw := &FileWatcher{
		agentID:  "fw-agent",
		workDir:  "/tmp",
		eventBus: bus,
		filters:  []FileFilter{},
	}

	tests := []struct {
		name     string
		op       fsnotify.Op
		wantType string
	}{
		{"create", fsnotify.Create, EventFileCreated},
		{"write", fsnotify.Write, EventFileModified},
		{"remove", fsnotify.Remove, EventFileDeleted},
		{"rename", fsnotify.Rename, EventFileDeleted},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col.drain() // clear previous events
			fw.handleEvent(fakeEvent(tt.op, "/tmp/test.go"))
			events := col.drain()
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}
			if events[0].Type != tt.wantType {
				t.Errorf("expected event type %q, got %q", tt.wantType, events[0].Type)
			}
			if events[0].Data["agent_id"] != "fw-agent" {
				t.Errorf("expected agent_id=fw-agent, got %v", events[0].Data["agent_id"])
			}
			if events[0].Data["path"] != "/tmp/test.go" {
				t.Errorf("expected path=/tmp/test.go, got %v", events[0].Data["path"])
			}
		})
	}
}

func TestHandleEvent_ChmodIgnored(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()

	fw := &FileWatcher{
		agentID:  "fw-agent",
		workDir:  "/tmp",
		eventBus: bus,
		filters:  []FileFilter{},
	}

	fw.handleEvent(fakeEvent(fsnotify.Chmod, "/tmp/test.go"))
	events := col.drain()
	if len(events) != 0 {
		t.Errorf("expected chmod to be ignored, got %d events", len(events))
	}
}

func TestHandleEvent_FilteredFile(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()

	fw := &FileWatcher{
		agentID:  "fw-agent",
		workDir:  "/tmp",
		eventBus: bus,
		filters:  []FileFilter{IsGitFile},
	}

	fw.handleEvent(fakeEvent(fsnotify.Create, "/repo/.git/HEAD"))
	events := col.drain()
	if len(events) != 0 {
		t.Errorf("expected .git file to be filtered, got %d events", len(events))
	}
}

func fakeEvent(op fsnotify.Op, path string) fsnotify.Event {
	return fsnotify.Event{Name: path, Op: op}
}
