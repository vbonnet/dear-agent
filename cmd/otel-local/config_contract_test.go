package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	yaml "gopkg.in/yaml.v3"
)

// jaegerConfigPath resolves deploy/jaeger/config.yaml from this package's
// directory. The contract below is about the deployed artifact's bytes, so the
// test reads the real file rather than a fixture.
func jaegerConfigPath() string {
	return filepath.Join("..", "..", "deploy", "jaeger", "config.yaml")
}

func loadJaegerConfig(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(jaegerConfigPath())
	if err != nil {
		t.Fatalf("reading jaeger config: %v", err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parsing jaeger config: %v", err)
	}
	return cfg
}

// dig walks a nested YAML map by key, returning the value and whether the whole
// path existed.
func dig(root map[string]any, path ...string) (any, bool) {
	var cur any = root
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[k]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// isLoopback reports whether an endpoint's host half is a loopback literal.
// A bare port (":4317") is NOT loopback: it binds every interface, which is the
// exact defect this contract exists to prevent.
func isLoopback(endpoint string) bool {
	host := endpoint
	if i := strings.LastIndex(endpoint, ":"); i >= 0 {
		host = endpoint[:i]
	}
	host = strings.Trim(host, "[]")
	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}

// TestJaegerReceiversBindLoopback covers JAEGER-CFG-01. The collector runs as an
// always-on launch agent on a laptop that joins untrusted networks and
// authenticates nothing, so a non-loopback receiver lets any peer on the
// current network inject spans or exhaust the bounded memstore.
func TestJaegerReceiversBindLoopback(t *testing.T) {
	cfg := loadJaegerConfig(t)
	protocols, ok := dig(cfg, "receivers", "otlp", "protocols")
	if !ok {
		t.Fatal("receivers.otlp.protocols missing from deploy/jaeger/config.yaml")
	}
	m, ok := protocols.(map[string]any)
	if !ok || len(m) == 0 {
		t.Fatalf("receivers.otlp.protocols is not a non-empty map: %#v", protocols)
	}
	for name, v := range m {
		proto, ok := v.(map[string]any)
		if !ok {
			t.Errorf("receiver %q: not a map: %#v", name, v)
			continue
		}
		endpoint, _ := proto["endpoint"].(string)
		if endpoint == "" {
			t.Errorf("receiver %q: no explicit endpoint; the default binds all interfaces", name)
			continue
		}
		if !isLoopback(endpoint) {
			t.Errorf("receiver %q binds %q, want a loopback address (JAEGER-CFG-01)", name, endpoint)
		}
	}
}

// TestJaegerQueryBindsLoopback covers JAEGER-CFG-02. Leaving jaeger_query at its
// default exposes the UI and query API on every interface, so any peer can
// enumerate services and read locally collected trace contents.
func TestJaegerQueryBindsLoopback(t *testing.T) {
	cfg := loadJaegerConfig(t)
	query, ok := dig(cfg, "extensions", "jaeger_query")
	if !ok {
		t.Fatal("extensions.jaeger_query missing from deploy/jaeger/config.yaml")
	}
	qm, ok := query.(map[string]any)
	if !ok {
		t.Fatalf("extensions.jaeger_query is not a map: %#v", query)
	}
	for _, transport := range []string{"http", "grpc"} {
		v, ok := qm[transport]
		if !ok {
			t.Errorf("jaeger_query.%s: absent; the default binds all interfaces (JAEGER-CFG-02)", transport)
			continue
		}
		tm, ok := v.(map[string]any)
		if !ok {
			t.Errorf("jaeger_query.%s: not a map: %#v", transport, v)
			continue
		}
		endpoint, _ := tm["endpoint"].(string)
		if endpoint == "" {
			t.Errorf("jaeger_query.%s: no explicit endpoint (JAEGER-CFG-02)", transport)
			continue
		}
		if !isLoopback(endpoint) {
			t.Errorf("jaeger_query.%s binds %q, want a loopback address (JAEGER-CFG-02)", transport, endpoint)
		}
	}
}

// TestJaegerStorageIsBounded covers JAEGER-CFG-03: an unbounded producer must
// not be able to exhaust host memory through the in-memory backend.
func TestJaegerStorageIsBounded(t *testing.T) {
	cfg := loadJaegerConfig(t)
	v, ok := dig(cfg, "extensions", "jaeger_storage", "backends", "memstore", "memory", "max_traces")
	if !ok {
		t.Fatal("memstore has no declared max_traces (JAEGER-CFG-03)")
	}
	n, ok := v.(int)
	if !ok || n <= 0 {
		t.Fatalf("memstore max_traces = %#v, want a positive integer (JAEGER-CFG-03)", v)
	}
}

// TestJaegerArtifactsAreDeployManaged covers JAEGER-CFG-04. Both the collector
// config and the launch agent plist must be registered in the deploy manifest,
// or `dear-deploy status` cannot report drift and the running collector can
// diverge from source unnoticed.
func TestJaegerArtifactsAreDeployManaged(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "deploy", "manifest.yaml"))
	if err != nil {
		t.Fatalf("reading deploy manifest: %v", err)
	}
	var manifest struct {
		Artifacts []struct {
			Name   string `yaml:"name"`
			Source string `yaml:"source"`
		} `yaml:"artifacts"`
	}
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parsing deploy manifest: %v", err)
	}
	sources := make(map[string]bool, len(manifest.Artifacts))
	for _, a := range manifest.Artifacts {
		sources[a.Source] = true
	}
	for _, want := range []string{
		"deploy/jaeger/config.yaml",
		"deploy/launchd/com.jaegertracing.jaeger.plist",
	} {
		if !sources[want] {
			t.Errorf("%s is not registered in deploy/manifest.yaml (JAEGER-CFG-04)", want)
		}
	}
}
