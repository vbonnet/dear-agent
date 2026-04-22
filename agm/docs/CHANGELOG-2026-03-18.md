# AGM Session List UI Changes

**Date:** 2026-03-18
**Component:** `agm session list` output formatting
**Files Modified:**
- `internal/ui/table.go`
- `cmd/agm/SPEC.md`
- `docs/CHANGELOG-2026-03-18.md`

## Changes

### 1. Column Headers Added
Added column headers to all three layout modes (minimal, compact, full) to clarify what each column represents.

**Before:**
```
ACTIVE (7)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
●  diagraming                      oss       claude  ~/src                      21h ago
```

**After:**
```
ACTIVE (7)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
   NAME                      UUID      WORKSPACE  AGENT   PROJECT                 ACTIVITY
●  diagraming                f8310771  oss        claude  ~/src                    21h ago
```

### 2. UUID Column Added
Added UUID column showing the first segment of the Claude UUID (before the first "-").

**Display:**
- Shows first UUID segment (e.g., "f8310771" from "f8310771-...")
- Shows "-" if no Claude UUID exists
- Column width: 8 characters (fixed in minimal/compact, dynamic in full layout)

**Implementation:**
- Created `extractShortUUID()` helper function
- Added `uuid` field to `columnWidths` struct
- Updated all three layout modes (minimal, compact, full)

### 3. Alphabetical Sorting
Implemented alphabetical sorting with special handling for ACTIVE sessions.

**Sort Order:**
- **ACTIVE sessions:** Sort by attachment status first (● attached, then ◐ detached), then alphabetically by name
- **All other groups (STOPPED, STALE, ARCHIVED):** Sort alphabetically by name

**Implementation:**
- Created `sortGroups()` function in `internal/ui/table.go`
- Called from `FormatTable()` after grouping sessions

### 4. TMUX Column Removed
Removed the TMUX column as it was always identical to the NAME column.

**Rationale:**
- The TMUX column showed `m.Tmux.SessionName`, which is always the same as `m.Name`
- Displaying duplicate information wastes horizontal space
- No parent/child relationship information was actually being displayed in this column

**Implementation:**
- Modified `shouldShowTmuxColumn()` to always return `false`
- Kept all the conditional rendering logic intact (no breaking changes)

### 5. Activity Column Right-Aligned
Changed ACTIVITY column to be right-aligned so "ago" lines up vertically.

**Before:** Left-aligned, uneven "ago" positions
**After:** Right-aligned with consistent 10-character width

**Implementation:**
- Changed format from `%-10s` (left-align) to `%10s` (right-align)
- Applied to all three layout modes

### 6. Footer Commands Updated
Updated footer to show actual AGM commands instead of non-existent ones.

**Before:**
```
💡 Resume: agm resume <name>  |  Stop: agm stop <name>  |  Clean: agm clean
```

**After:**
```
💡 Resume: agm resume <name>  |  Archive: agm session archive <name>  |  Kill: agm session kill <name>
```

### 7. Column Header Alignment Fixed
Fixed misalignment between column headers and data when data is shorter than header text.

**Problem:**
When all sessions had short workspace names (e.g., "oss" = 3 chars), the column width was set to 3, but the header "WORKSPACE" is 9 chars, causing misalignment.

**Solution:**
Added minimum width constraints for all columns based on their header text:
- NAME: 4 chars minimum
- UUID: 4 chars minimum
- WORKSPACE: 9 chars minimum
- AGENT: 5 chars minimum
- PROJECT: 7 chars minimum

**Test Coverage:**
Added `TestColumnWidthsMinimumHeaderWidth` to prevent regression.

## Layout Modes

### Minimal (<80 columns)
```
NAME                  WORKSPACE  AGENT   ACTIVITY
```

### Compact (80-99 columns)
```
NAME                            WORKSPACE  AGENT   PROJECT                    ACTIVITY
```

### Full (≥100 columns)
```
NAME                            WORKSPACE  AGENT   PROJECT                    ACTIVITY
```

## Status Symbols

| Symbol | Meaning |
|--------|---------|
| ● | Active session with attached clients |
| ◐ | Active session with no attached clients |
| ○ | Stopped session |
| ⊗ | Stale session |
| ◉ | Archived session |

## Testing

No test changes required:
- `internal/ui/hierarchy_test.go` tests hierarchy rendering, not TMUX column display
- No existing tests validated TMUX column presence
- Sorting logic is straightforward and visually verifiable

## Documentation Updates

- Updated `cmd/agm/SPEC.md` FR5 to document list output format, column headers, and sort order
- Created this changelog for future reference
