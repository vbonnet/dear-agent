// Package diskledger attributes disk growth to the agent run that caused it.
//
// The host loses disk on essentially every agent run: sandboxes, worktrees,
// per-run Go build caches and append-only logs are all created by a run and
// only sometimes reclaimed when it ends. Free-space thresholds cannot tell you
// WHICH run leaked, and they only fire once the disk is already nearly full --
// by then the evidence of who did it is long gone.
//
// A ledger inverts that. Each run opens an entry naming the paths it owns and
// what they measured at the time, and closes it when the run ends. An entry
// that is closed with bytes still on disk, or never closed at all while its
// owning session is dead, is a leak with a name attached.
//
// Measurement is block-aware and inode-deduplicated on purpose. The four
// directory walkers this package replaces (gclog.DirSize,
// sandbox.calculateDirSize, sandbox.calculateDiskUsage,
// disk-watchdog.dirBytes) all sum apparent file sizes, which double-counts
// hardlinks and badly overstates APFS clonefile sandboxes -- so the bytes they
// report "reclaimed" are not the bytes the filesystem actually got back.
package diskledger

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// Usage is one measurement of a path.
//
// AllocatedBytes and ApparentBytes are both reported because they answer
// different questions and disagree in the cases that matter here. Apparent is
// the sum of file lengths; allocated is the sum of blocks the filesystem
// charged. Sparse files make apparent much larger than allocated (the Colima
// datadisk on this host is 100 GiB apparent and 18 GiB allocated). A caller
// that trusts either number alone will be wrong on one of those shapes, so the
// ledger records both and leaves the interpretation explicit.
type Usage struct {
	// AllocatedBytes is the sum of st_blocks*512 over distinct inodes: what
	// the filesystem actually charged for this tree.
	AllocatedBytes int64 `json:"allocated_bytes"`
	// ApparentBytes is the sum of file lengths over distinct inodes.
	ApparentBytes int64 `json:"apparent_bytes"`
	// Files counts distinct regular-file inodes reached under the path.
	Files int64 `json:"files"`
	// Dirs counts directories walked.
	Dirs int64 `json:"dirs"`
	// Missing records that the path did not exist. This is not an error: a
	// sandbox that is already gone measures zero, and that is the normal
	// result when reconciling a run that cleaned up correctly.
	Missing bool `json:"missing,omitempty"`
	// Partial records that the walk hit unreadable entries and the totals are
	// therefore a lower bound. A silent partial sum is how an accounting bug
	// hides, so callers can surface this rather than treat a truncated walk as
	// a shrinking tree.
	Partial bool `json:"partial,omitempty"`
}

// blockSize is the unit st_blocks is denominated in. POSIX fixes this at 512
// regardless of the filesystem's own block size.
const blockSize = 512

// Measure walks path and reports its usage.
//
// Symlinks are never followed: a link into another tree belongs to that tree,
// and following one would attribute another owner's bytes to this run (and can
// loop). Hardlinks and any other multiply-linked inode are counted once, so
// the total reflects what deleting the tree would actually return.
//
// A missing path is reported as zero usage with Missing set, not as an error.
func Measure(path string) (Usage, error) {
	var u Usage

	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			u.Missing = true
			return u, nil
		}
		return u, err
	}

	// Inodes already counted, so a hardlink or a second reference to the same
	// file does not get charged twice.
	seen := make(map[inodeKey]struct{})

	err := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			// Best-effort: an unreadable subtree must not abort the whole
			// measurement, but it must be visible in the result.
			u.Partial = true
			//nolint:nilerr // deliberate: surfaced via u.Partial, not by aborting the walk
			return nil
		}
		if d.IsDir() {
			u.Dirs++
			return nil
		}
		// Only regular files consume blocks. Symlinks, sockets, devices and
		// fifos are skipped; d.Type() is the un-followed type from lstat.
		if d.Type() != 0 {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			u.Partial = true
			return nil //nolint:nilerr // recorded via u.Partial; aborting the walk would report a shrinking tree
		}
		key, alloc, ok := statUsage(info)
		if !ok {
			// No syscall-level stat available: fall back to apparent size
			// only, and mark the result partial so the gap is not silent.
			u.Partial = true
			u.ApparentBytes += info.Size()
			u.Files++
			return nil
		}
		if _, dup := seen[key]; dup {
			return nil
		}
		seen[key] = struct{}{}
		u.Files++
		u.ApparentBytes += info.Size()
		u.AllocatedBytes += alloc
		return nil
	})

	return u, err
}

// inodeKey identifies an inode uniquely across devices.
type inodeKey struct {
	dev uint64
	ino uint64
}

// statUsage extracts the inode identity and allocated bytes from a FileInfo.
// It returns ok=false when the platform stat is unavailable, so the caller can
// degrade explicitly instead of silently reporting zero.
func statUsage(info os.FileInfo) (key inodeKey, allocated int64, ok bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st == nil {
		return inodeKey{}, 0, false
	}
	// Dev/Ino widths differ per platform (int32 on darwin, uint64 on linux).
	// The conversion is identity-preserving for an inode identity used only as
	// a map key, so a sign-extended negative device number is still unique.
	return inodeKey{dev: uint64(uint32(st.Dev)), ino: uint64(st.Ino)}, //nolint:gosec // identity key only, not arithmetic
		int64(st.Blocks) * blockSize,
		true
}

// Sub returns the usage delta a-b, floored at zero per field. It answers "how
// much did this path grow" without ever reporting negative growth, which would
// otherwise appear when a tree shrinks between samples.
func (u Usage) Sub(b Usage) Usage {
	nonNeg := func(x int64) int64 {
		if x < 0 {
			return 0
		}
		return x
	}
	return Usage{
		AllocatedBytes: nonNeg(u.AllocatedBytes - b.AllocatedBytes),
		ApparentBytes:  nonNeg(u.ApparentBytes - b.ApparentBytes),
		Files:          nonNeg(u.Files - b.Files),
		Dirs:           nonNeg(u.Dirs - b.Dirs),
		Partial:        u.Partial || b.Partial,
	}
}
