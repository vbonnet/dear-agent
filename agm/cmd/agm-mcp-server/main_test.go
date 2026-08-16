package main

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pkgversion "github.com/vbonnet/dear-agent/pkg/version"
)

func TestMCPInitializeReportsSharedBuildVersion(t *testing.T) {
	const sentinelVersion = "agm-mcp-provider-test"
	originalVersion := pkgversion.Version
	pkgversion.Version = sentinelVersion
	t.Cleanup(func() { pkgversion.Version = originalVersion })

	server := newMCPServer()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	t.Cleanup(func() {
		if err := serverSession.Close(); err != nil {
			t.Logf("close server session: %v", err)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "agm-mcp-version-test", Version: "test"}, nil)
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() {
		if err := clientSession.Close(); err != nil {
			t.Logf("close client session: %v", err)
		}
	})

	result := clientSession.InitializeResult()
	if result == nil {
		t.Fatal("InitializeResult is nil")
	}
	if result.ServerInfo == nil {
		t.Fatal("InitializeResult.ServerInfo is nil")
	}
	if result.ServerInfo.Name != "agm" {
		t.Errorf("ServerInfo.Name = %q, want agm", result.ServerInfo.Name)
	}
	if result.ServerInfo.Version != sentinelVersion {
		t.Errorf("ServerInfo.Version = %q, want shared build version %q", result.ServerInfo.Version, sentinelVersion)
	}
	if result.ProtocolVersion == "" {
		t.Error("ProtocolVersion is empty; want an independently SDK-negotiated value")
	}
	if result.ProtocolVersion == sentinelVersion {
		t.Errorf("ProtocolVersion = artifact version %q; want an independently SDK-negotiated value", sentinelVersion)
	}
}
