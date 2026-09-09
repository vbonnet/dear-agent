//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package main

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

const (
	replyBodyFIFOHelperEnv  = "GO_WANT_REPLY_BODY_FIFO_HELPER"
	replyBodyFIFOPathEnv    = "REPLY_BODY_FIFO_HELPER_PATH"
	replyBodyFIFOTestWindow = 10 * time.Second
)

func TestLoadReplyBodyRejectsReplacedFIFOWithoutBlocking(t *testing.T) {
	if os.Getenv(replyBodyFIFOHelperEnv) == "1" {
		path := os.Getenv(replyBodyFIFOPathEnv)
		if _, err := loadReplyBody(path, nil); err == nil || !strings.Contains(err.Error(), "regular file") {
			t.Fatalf("replaced FIFO error = %v, want regular-file refusal", err)
		}
		return
	}

	path := filepath.Join(t.TempDir(), "reply.md")
	if err := os.WriteFile(path, []byte("initial regular file\n"), 0o600); err != nil {
		t.Fatalf("write initial reply file: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove initial reply file: %v", err)
	}
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("replace reply file with FIFO: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), replyBodyFIFOTestWindow)
	defer cancel()
	// #nosec G204 -- re-executes this test binary with a fixed test selector.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLoadReplyBodyRejectsReplacedFIFOWithoutBlocking$")
	cmd.Env = append(os.Environ(), replyBodyFIFOHelperEnv+"=1", replyBodyFIFOPathEnv+"="+path)
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("opening replaced FIFO blocked past %s: %v", replyBodyFIFOTestWindow, ctx.Err())
	}
	if err != nil {
		t.Fatalf("replaced FIFO helper: %v\n%s", err, output)
	}
}

// The deadline-bound FIFO test proves the current behavior, while this source
// guard makes the former pathname-stat-then-open implementation a deterministic
// regression instead of relying on winning a filesystem race in a test.
func TestLoadReplyBodyUsesNonblockingOpenerWithoutPathStat(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}

	var loadReplyBody *ast.FuncDecl
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Name.Name == "loadReplyBody" {
			loadReplyBody = function
			break
		}
	}
	if loadReplyBody == nil {
		t.Fatal("main.go does not declare loadReplyBody")
	}

	nonblockingOpens := 0
	pathStats := 0
	ast.Inspect(loadReplyBody.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if identifier, ok := call.Fun.(*ast.Ident); ok && identifier.Name == "openReplyBodyFile" {
			nonblockingOpens++
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "Stat" {
			return true
		}
		if identifier, ok := selector.X.(*ast.Ident); ok && identifier.Name == "os" {
			pathStats++
		}
		return true
	})

	if pathStats != 0 {
		t.Fatalf("loadReplyBody contains %d pathname os.Stat call(s); validate only the opened descriptor", pathStats)
	}
	if nonblockingOpens != 1 {
		t.Fatalf("loadReplyBody calls openReplyBodyFile %d times, want exactly once", nonblockingOpens)
	}
}

func TestLoadReplyBodyRejectsNamedDeviceWithoutReading(t *testing.T) {
	if _, err := loadReplyBody("/dev/zero", nil); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("named device error = %v, want regular-file refusal", err)
	}
}
