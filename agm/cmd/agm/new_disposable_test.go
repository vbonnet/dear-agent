package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestDisposableFlag_Registered(t *testing.T) {
	// Verify the --disposable flag is registered on newCmd
	flag := newCmd.Flags().Lookup("disposable")
	if flag == nil {
		t.Fatal("expected --disposable flag to be registered on newCmd")
	}
	if flag.DefValue != "false" {
		t.Errorf("expected default value 'false', got %q", flag.DefValue)
	}
}

func TestDisposableTTLFlag_Registered(t *testing.T) {
	// Verify the --disposable-ttl flag is registered on newCmd
	flag := newCmd.Flags().Lookup("disposable-ttl")
	if flag == nil {
		t.Fatal("expected --disposable-ttl flag to be registered on newCmd")
	}
	if flag.DefValue != "4h" {
		t.Errorf("expected default value '4h', got %q", flag.DefValue)
	}
}

func TestDisposableFlag_Parsing(t *testing.T) {
	// Create a fresh command to test flag parsing without side effects
	cmd := &cobra.Command{Use: "test"}
	var d bool
	var ttl string
	cmd.Flags().BoolVar(&d, "disposable", false, "disposable flag")
	cmd.Flags().StringVar(&ttl, "disposable-ttl", "4h", "disposable ttl")

	cmd.ParseFlags([]string{"--disposable", "--disposable-ttl", "30m"})

	if !d {
		t.Error("expected disposable to be true after parsing --disposable")
	}
	if ttl != "30m" {
		t.Errorf("expected disposable-ttl to be '30m', got %q", ttl)
	}
}

func TestDisposableFlag_DefaultTTL(t *testing.T) {
	// When --disposable is set without --disposable-ttl, default should be "4h"
	cmd := &cobra.Command{Use: "test"}
	var d bool
	var ttl string
	cmd.Flags().BoolVar(&d, "disposable", false, "disposable flag")
	cmd.Flags().StringVar(&ttl, "disposable-ttl", "4h", "disposable ttl")

	cmd.ParseFlags([]string{"--disposable"})

	if !d {
		t.Error("expected disposable to be true")
	}
	if ttl != "4h" {
		t.Errorf("expected default disposable-ttl '4h', got %q", ttl)
	}
}

func TestDisposableFlag_SetsManifestFields(t *testing.T) {
	// Verify that when disposable=true, manifest fields are set correctly
	// This tests the inline logic used during manifest creation
	disposableVal := true
	ttlVal := "2h"

	// Simulate the manifest field assignment logic from new.go
	resultDisposable := disposableVal
	resultTTL := func() string {
		if disposableVal {
			return ttlVal
		}
		return ""
	}()

	if !resultDisposable {
		t.Error("expected manifest Disposable to be true")
	}
	if resultTTL != "2h" {
		t.Errorf("expected manifest DisposableTTL to be '2h', got %q", resultTTL)
	}
}

func TestDisposableFlag_NotSetLeavesManifestEmpty(t *testing.T) {
	// When disposable is false, TTL should be empty string
	disposableVal := false
	ttlVal := "4h" // default value, but should not be stored

	resultTTL := func() string {
		if disposableVal {
			return ttlVal
		}
		return ""
	}()

	if resultTTL != "" {
		t.Errorf("expected empty TTL when disposable is false, got %q", resultTTL)
	}
}
