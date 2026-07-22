# Hash package architecture

<!-- Last audited at: 2026-07-18 -->

`pkg/hash` is a small file-digest boundary. It owns path normalization and one
stable SHA-256 representation so callers do not repeat either policy.

## Interface

```go
func ExpandPath(path string) (string, error)
func CalculateFileHash(path string) (string, error)
```

`ExpandPath` accepts ordinary relative or absolute paths, `~`, and `~/...`.
It resolves ordinary paths with `filepath.Abs`, expands the current user's home
with `os.UserHomeDir`, and rejects `~user` syntax.

`CalculateFileHash` runs this flow:

```text
caller path
  -> ExpandPath
  -> os.Open
  -> io.Copy into sha256.New
  -> "sha256:" + lowercase hexadecimal digest
```

The file is streamed through the standard-library hasher. The package holds no
mutable process state and does not cache file contents or digests.

## Invariants and failure boundary

- SHA-256 is the only supported algorithm and `sha256:<64 hex characters>` is
  the only successful result format.
- Hash input is the exact file byte stream; names and metadata are excluded.
- Path expansion, open, and read failures return wrapped errors. A failed call
  never returns a partial digest.
- Directory hashing, digest comparison, `~user` expansion, and algorithm
  negotiation remain outside this package.

## Change guidance

Keep this module narrow. Add a new interface only when a real caller needs a
different input boundary or digest contract; do not add policy for directory
walking, persistence, or verification here. Format changes require updating
all stored-digest consumers and are therefore compatibility changes.

## Verification

```sh
go test -race ./pkg/hash -count=1
```

`hash_test.go` covers known digests, path forms, and I/O failures.
`property_test.go` covers determinism, content sensitivity, canonical format,
and arbitrary binary input.
