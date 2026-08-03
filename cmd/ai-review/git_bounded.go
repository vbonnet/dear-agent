package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const gitCommandTimeout = 10 * time.Second

var errGitOutputLimit = errors.New("git output exceeds the review limit")

// gitOutputBounded runs Git with inherited Git configuration disabled and a
// hard wall-clock/output limit. Overflow kills and waits for the child before
// returning, so an untrusted PR object cannot leave a producer blocked on a
// full pipe.
func gitOutputBounded(parent context.Context, limit int, args ...string) ([]byte, error) {
	if limit < 1 {
		return nil, errors.New("git output limit must be positive")
	}
	if len(args) == 0 {
		return nil, errors.New("git command is required")
	}
	ctx, cancel := context.WithTimeout(parent, gitCommandTimeout)
	defer cancel()
	fullArgs := append([]string{"--no-replace-objects", "-c", "core.hooksPath=" + os.DevNull}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...) // #nosec G702 -- executable is fixed and every revision/path argument is validated before this boundary.
	cmd.Env = cleanReviewGitEnv()
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("prepare git %s: %w", args[0], err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start git %s: %w", args[0], err)
	}
	out, readErr := io.ReadAll(io.LimitReader(stdout, int64(limit)+1))
	if readErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("read git %s: %w", args[0], readErr)
	}
	if len(out) > limit {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return out, errGitOutputLimit
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git %s timed out: %w", args[0], ctx.Err())
		}
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

func cleanReviewGitEnv() []string {
	// Start from an allowlist instead of deleting only GIT_* variables: API
	// keys, credential-helper sockets, CI tokens, and provider credentials must
	// never be inherited by a subprocess that examines an untrusted Git object.
	env := make([]string, 0, 16)
	for _, name := range []string{"PATH", "TMPDIR", "TMP", "TEMP", "SystemRoot"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return append(env,
		"HOME=",
		"LANG=C",
		"LC_ALL=C",
		"TZ=UTC",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_NO_REPLACE_OBJECTS=1",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_LITERAL_PATHSPECS=1",
		"GIT_PROTOCOL_FROM_USER=0",
		"GIT_TERMINAL_PROMPT=0",
	)
}

// gitRegularTextBlobsBounded resolves literal paths at revision, admits only
// regular non-executable blobs, and returns their exact bounded object bytes.
// Missing and non-regular paths are omitted so the caller can attach the
// appropriate authenticated ownership context to its human-review reason.
func gitRegularTextBlobsBounded(parent context.Context, revision string, paths []string, perBlobLimit, totalLimit int) (map[string][]byte, error) {
	if !validObjectID(revision) {
		return nil, errors.New("invalid bounded tree revision")
	}
	requested := make(map[string]bool, len(paths))
	ordered := make([]string, 0, len(paths))
	for _, path := range paths {
		if !safeGitPath(path) {
			return nil, errors.New("unsafe bounded tree path")
		}
		if !requested[path] {
			requested[path] = true
			ordered = append(ordered, path)
		}
	}
	if len(ordered) == 0 {
		return map[string][]byte{}, nil
	}
	args := append([]string{"ls-tree", "-z", revision, "--"}, ordered...)
	out, err := gitOutputBounded(parent, maxGitMetadataBytes, args...)
	if err != nil {
		return nil, err
	}
	requests := make([]gitBlobRequest, 0, len(ordered))
	seen := make(map[string]bool, len(ordered))
	for _, raw := range bytesSplitNUL(out) {
		metadata, rawPath, ok := strings.Cut(string(raw), "\t")
		parts := strings.Fields(metadata)
		path := rawPath
		if !ok || len(parts) != 3 || !validObjectID(parts[2]) || !requested[path] || seen[path] {
			return nil, errors.New("git tree returned unauthenticated path metadata")
		}
		seen[path] = true
		if parts[0] == "100644" && parts[1] == "blob" {
			requests = append(requests, gitBlobRequest{Path: path, ObjectID: parts[2]})
		}
	}
	return gitTextBlobsBounded(parent, requests, perBlobLimit, totalLimit)
}

func bytesSplitNUL(out []byte) [][]byte {
	fields := bytes.Split(out, []byte{0})
	if len(fields) > 0 && len(fields[len(fields)-1]) == 0 {
		fields = fields[:len(fields)-1]
	}
	return fields
}

type gitBlobRequest struct {
	Path     string
	ObjectID string
}

// gitTextBlobsBounded reads raw committed blobs in one cat-file batch. Unlike
// git archive, cat-file does not apply a revision-controlled .gitattributes
// export-subst or export-ignore rule, so the reviewer sees the exact Git object
// bytes. Every request, response header, blob, aggregate, and subprocess is
// bounded before the returned map is trusted.
//
//nolint:gocyclo // The batch protocol validates each framing and resource bound in one auditable sequence.
func gitTextBlobsBounded(parent context.Context, requests []gitBlobRequest, perBlobLimit, totalLimit int) (map[string][]byte, error) {
	if perBlobLimit < 1 || totalLimit < 1 {
		return nil, errors.New("invalid bounded blob request")
	}
	if len(requests) == 0 {
		return map[string][]byte{}, nil
	}
	var input strings.Builder
	for _, request := range requests {
		if !safeGitPath(request.Path) || !validObjectID(request.ObjectID) {
			return nil, errors.New("unsafe bounded blob path")
		}
		if input.Len()+len(request.ObjectID)+1 > maxGitMetadataBytes {
			return nil, errors.New("bounded blob request exceeds the metadata limit")
		}
		input.WriteString(request.ObjectID)
		input.WriteByte('\n')
	}

	ctx, cancel := context.WithTimeout(parent, gitCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-replace-objects", "-c", "core.hooksPath="+os.DevNull, "cat-file", "--batch")
	cmd.Env = cleanReviewGitEnv()
	cmd.Stdin = strings.NewReader(input.String())
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("prepare git cat-file: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start git cat-file: %w", err)
	}
	fail := func(cause error) error {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return cause
	}

	reader := bufio.NewReaderSize(stdout, maxGitIdentityBytes)
	blobs := make(map[string][]byte, len(requests))
	total := 0
	for _, request := range requests {
		header, err := readBoundedLine(reader, maxGitIdentityBytes)
		if err != nil {
			return nil, fail(fmt.Errorf("read git cat-file header: %w", err))
		}
		fields := strings.Fields(strings.TrimSuffix(string(header), "\n"))
		if len(fields) != 3 || fields[0] != request.ObjectID || fields[1] != "blob" {
			return nil, fail(fmt.Errorf("git cat-file returned unauthenticated metadata for %s", request.Path))
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 || size > int64(perBlobLimit) || int64(total)+size > int64(totalLimit) {
			return nil, fail(fmt.Errorf("git blob exceeds the review limit (%s)", request.Path))
		}
		blob := make([]byte, int(size))
		if _, err := io.ReadFull(reader, blob); err != nil {
			return nil, fail(fmt.Errorf("read git blob %s: %w", request.Path, err))
		}
		separator, err := reader.ReadByte()
		if err != nil || separator != '\n' {
			return nil, fail(fmt.Errorf("git blob framing is invalid (%s)", request.Path))
		}
		if !validTextBlob(blob) {
			return nil, fail(fmt.Errorf("git blob is non-textual (%s)", request.Path))
		}
		blobs[request.Path] = blob
		total += len(blob)
	}
	if extra, err := reader.ReadByte(); err == nil {
		return nil, fail(fmt.Errorf("git cat-file returned unexpected output byte %q", extra))
	} else if !errors.Is(err, io.EOF) {
		return nil, fail(fmt.Errorf("finish git cat-file output: %w", err))
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("git cat-file timed out: %w", ctx.Err())
		}
		return nil, fmt.Errorf("git cat-file: %w", err)
	}
	if len(blobs) != len(requests) {
		return nil, errors.New("git cat-file did not authenticate every requested blob")
	}
	return blobs, nil
}

func readBoundedLine(reader *bufio.Reader, limit int) ([]byte, error) {
	line := make([]byte, 0, limit)
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(line)+len(fragment) > limit {
			return nil, errors.New("git response line exceeds the review limit")
		}
		line = append(line, fragment...)
		switch {
		case err == nil:
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return nil, err
		}
	}
}
