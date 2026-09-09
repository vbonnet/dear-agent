# Authenticated Build Authority Specification

<!-- Last audited at: 2026-09-08 -->

**Version:** 1.0
**Status:** Draft
**Scope:** `internal/buildauthority`.

## Overview

`internal/buildauthority` is the sole staging authority for a fixed authenticated
AGM and disk-watchdog artifact pair. It admits a closed input set, constructs the
pair in a private bounded workspace, returns only sealed verified artifacts and
an immutable receipt, and owns process-quiescent identity-checked cleanup. It
never installs or activates an artifact.

The numbered statements are product requirements in EARS form. The tables are
normative constants referenced by those requirements. Test, PR, merge,
installation, activation, and runtime receipts are evidence layers rather than
additional product requirements.

## Interface and scope

**BUILD-AUTH-001** The build authority shall expose one Stage(context.Context, Request) operation.
**BUILD-AUTH-002** The build authority shall accept only the Go executable, Git executable, GOROOT, GOMODCACHE, state root, standalone repository, and full exact revision as request data, and shall not treat any raw path as admitted authority.
**BUILD-AUTH-003** The build authority shall own the supported platform, role packages, output names, tool pins, build arguments, linker stamp, direct and derived child environments, child schedule constraints, resource bounds, and timeouts as fixed module policy.
**BUILD-AUTH-004** The build authority shall not install an artifact.
**BUILD-AUTH-005** The build authority shall not activate an artifact.
**BUILD-AUTH-006** The build authority shall not mutate a live target, launch configuration, daemon, endpoint, sandbox, provider object, or reaper record.
**BUILD-AUTH-007** If the request, platform, or any primitive in the exact no-scratch capture transaction fails, then the build authority shall close every already-retained capability and return a nil pair and one non-nil typed refusal.
**BUILD-AUTH-008** If a no-scratch condition in BUILD-AUTH-007 fails, then the build authority shall create no task root, claim no allocation, and return no Recovery.
**BUILD-AUTH-009** When staging succeeds, the build authority shall return one non-nil sealed opaque pair and a nil error.
**BUILD-AUTH-010** When staging succeeds, the build authority shall back the pair with one shared synchronization and retained-handle owner.
**BUILD-AUTH-011** When a receipt is requested before, during, or after close, the build authority shall return the same deep-copied value.
**BUILD-AUTH-012** When a receipt is returned, the build authority shall include exactly the claims in the normative receipt table.
**BUILD-AUTH-013** When a receipt is returned, the build authority shall omit every staging path and replaceable handle.
**BUILD-AUTH-014** When copied pair interfaces are closed repeatedly or concurrently, the build authority shall perform cleanup at most once.
**BUILD-AUTH-015** When copied pair interfaces are closed repeatedly or concurrently, the build authority shall make every caller wait for and receive the same close result.
**BUILD-AUTH-016** When the first Close begins on a successfully staged pair, the build authority shall execute the normative cleanup transaction exactly once.
**BUILD-AUTH-017** When the normative cleanup transaction runs, the build authority shall execute every ordered action and authorization condition in the normative cleanup table.
**BUILD-AUTH-018** If the first Close completes the normative cleanup transaction without a primary, descriptor-close, or cleanup refusal, then the build authority shall return nil.
**BUILD-AUTH-019** If Stage fails after task-root allocation, then the build authority shall execute the same normative cleanup transaction before returning a nil pair and non-nil typed refusal.

## Authority admission

**BUILD-AUTH-020** When a supplied authority path is admitted, the build authority shall require the path to be clean, absolute, and byte-for-byte equal to its evaluated physical path.
**BUILD-AUTH-021** When an authority leaf, ancestor, or tree entry is inspected, the build authority shall enforce the normative owner and mode policy.
**BUILD-AUTH-022** When an authority leaf, ancestor, or tree entry is inspected, the build authority shall enforce the normative descriptor-bound ACL policy.
**BUILD-AUTH-023** When an authority root or tree is inspected, the build authority shall enforce the normative local ownership-enforcing mount policy.
**BUILD-AUTH-024** When an authority tree is inspected, the build authority shall enforce the normative same-filesystem policy.
**BUILD-AUTH-025** If authority evidence is incomplete, malformed, unknown, or observably changing, then the build authority shall refuse admission.
**BUILD-AUTH-026** When authority relationships are checked, the build authority shall require StateRoot, Repository, GOROOT, and GOMODCACHE to be pairwise physically disjoint.
**BUILD-AUTH-027** When authority relationships are checked, the build authority shall require GitExecutable to remain outside StateRoot, Repository, and GOMODCACHE.
**BUILD-AUTH-028** When an authority tree is captured, the build authority shall enforce the applicable entry, per-file, aggregate-byte, symlink, mount, and phase bounds.
**BUILD-AUTH-029** When an authority tree is captured, the build authority shall encode its digest with the exact normative manifest and ACL wire formats.
**BUILD-AUTH-030** When StateRoot is admitted, the build authority shall require the exact effective-user-owned directory policy declared for StateRoot.
**BUILD-AUTH-031** When StateRoot, physical `/`, an authority-tree root, or a cleanup root is retained, the build authority shall bundle one os.Root with one separately opened no-follow same-identity descriptor and captured identity, and when `/dev/null` or an executable is retained it shall seal the applicable no-follow descriptor and identity as one non-forgeable capability for its normative lifetime.
**BUILD-AUTH-032** When a module-owned direct-command executable role is admitted, the build authority shall enforce the normative bounded nonscript Mach-O policy and seal it only as the distinct nominal Go, compiler, or Git authority; other GOROOT tool leaves remain authenticated tree content and a generic executable or process request shall confer no direct-command authority.
**BUILD-AUTH-033** When GOROOT is admitted, the build authority shall require GOROOT/bin/go to be the captured thin-arm64 Go executable.
**BUILD-AUTH-034** When GOROOT is admitted, the build authority shall authenticate the bounded complete distribution.
**BUILD-AUTH-035** When a GOROOT symlink is admitted, the build authority shall require the normative contained relative chain to terminate at an already captured protected regular file within forty links.
**BUILD-AUTH-036** When an active pkg/tool/darwin_arm64 executable is admitted, the build authority shall enforce the common Mach-O policy.
**BUILD-AUTH-037** When the platform is admitted, the build authority shall require the exact platform in the normative tool-pin table and retain the fixed physical-root and null-device capabilities.
**BUILD-AUTH-038** When the Go authority is admitted, the build authority shall require the exact Go command report in the normative tool-pin table.
**BUILD-AUTH-039** When either sealed Go environment profile is probed, the build authority shall require its exact keyed canonical-JSON Go projection, including computed `GOTELEMETRY=off` and empty `GOTELEMETRYDIR`.
**BUILD-AUTH-040** When the Go authority is admitted, the build authority shall require the effective GOCACHEPROG value to be empty.
**BUILD-AUTH-041** If GOROOT/go.env is missing, then the build authority shall refuse the Go authority.
**BUILD-AUTH-042** When GOROOT/go.env is admitted or revalidated, the build authority shall use the normative physical-root-anchored fresh-leaf transaction and require the leaf to match the retained GOROOT row, digest, closed grammar, and assignment allowlist.
**BUILD-AUTH-043** When the compiler authority is derived from the authenticated GOROOT leaf, the build authority shall invoke that distinct retained absolute compiler capability directly under the preallocation profile and require the exact compiler-full-version report in the normative tool-pin table.
**BUILD-AUTH-044** When the Git authority is admitted, the build authority shall require the exact audited whole-file digest.
**BUILD-AUTH-045** When the Git authority is admitted, the build authority shall require the normative Apple Git-155 architecture, subtype, and loader policy.
**BUILD-AUTH-046** When the Git authority is admitted, the build authority shall require the exact ordered version and build-options transcript.
**BUILD-AUTH-047** When the Git authority is admitted, the build authority shall require the complete fixed-operation builtin inventory.
**BUILD-AUTH-048** When a fixed Git operation is probed or executed, the build authority shall require child-free builtin execution.
**BUILD-AUTH-049** When GOMODCACHE is admitted, the build authority shall authenticate its bounded complete metadata tree.
**BUILD-AUTH-050** When GOMODCACHE is admitted, the build authority shall accept only protected directories and regular files.
**BUILD-AUTH-051** When Repository is admitted, the build authority shall require root-or-effective-user ancestry and an effective-user-owned standalone-worktree root and captured descendants.
**BUILD-AUTH-052** When Repository is admitted, the build authority shall require its Git, common, object, and active-configuration paths to match the retained in-tree admission envelope and shall freeze the complete selective administrative row set under the normative three-class policy.
**BUILD-AUTH-053** When source local configuration is admitted, the build authority shall require every normalized key and value to match the normative closed source-config table.
**BUILD-AUTH-054** If Repository contains a forbidden routing, replacement, shallow, promisor, include, lock, or execution-affecting state, then the build authority shall refuse admission.
**BUILD-AUTH-055** When the source object store is inventoried, the build authority shall admit only canonical loose objects and complete matching version-2 pack/index pairs as copied content.
**BUILD-AUTH-056** When the source object store is inventoried, the build authority shall recognize only the exact bounded regular entries in the normative object-auxiliary table as noncontent metadata.
**BUILD-AUTH-057** When private object content is copied, the build authority shall omit every source-object auxiliary named by BUILD-AUTH-056.
**BUILD-AUTH-058** If the source object store contains a routing marker, incomplete pair, symlink, special entry, lock, or unknown name, then the build authority shall refuse admission.

## Private execution and exact source

**BUILD-AUTH-059** When the complete single-use capturedNoScratchInputs authority has been constructed, the build authority shall create one cryptographically named cleanup-owned task child through the retained StateRoot using the exact bounded exclusive allocator and treat successful `mkdirat` as allocation.
**BUILD-AUTH-060** When task-child `mkdirat` succeeds, the build authority shall immediately record the generated name and retained StateRoot identity as an allocated-but-unobserved task before any fallible retain, then either validate and record its identity or preserve it with `unknown` Recovery and `Recovery.TaskRoot` nil.
**BUILD-AUTH-061** When workspace or object-spool entries are created, the build authority shall enforce the exact initial workspace and retained-spool lifecycle declared in the normative tables.
**BUILD-AUTH-062** When git-path/git is created, the build authority shall make it the sole policy symlink and bind it to the retained Git identity.
**BUILD-AUTH-063** When private Git policy files are created, the build authority shall create exactly the three empty 0600 regular files declared in the workspace policy.
**BUILD-AUTH-064** When a module-owned direct child environment is constructed, the build authority shall construct one of exactly two sealed 39-row profile types from an empty set and byte-sort its unique complete assignments.
**BUILD-AUTH-065** When task allocation has not started, the build authority shall use the exact task-path-free preallocation direct profile only for the five normative non-source plans from retained physical `/` and the later four source plans from retained Repository, with the root sentinels absent and no task child.
**BUILD-AUTH-066** When task allocation succeeds, the build authority shall use the exact task-private profile for every later module-owned direct child, derive every pinned-Go-owned descendant only through the normative 50-, 51-, or 52-row projection, and omit every unlisted key.
**BUILD-AUTH-067** When a fixed command plan invokes Go, Git, or the compiler probe directly, the build authority shall execute only the applicable nominal retained absolute executable authority.
**BUILD-AUTH-068** When a command phase runs, the build authority shall enforce its exact direct-or-derived argv, cwd, environment, stdin, phase, output, diagnostic, and resource contract and shall not publish a successful result until child quiescence, empty stderr, and closure without DescriptorClose of every supervisor-owned transient child-I/O endpoint due at that boundary are proved.
**BUILD-AUTH-069** When a command phase runs, the build authority shall honor context cancellation and the whole-transaction deadline.
**BUILD-AUTH-070** When a command is about to invoke its named process owner or that invocation returns, the build authority shall run every applicable retained-authority revalidation in the fixed normative order, including the physical-root and retained-null checks around every dependent command, and shall run the complete command-end bracket even when request validation, setup, Start, execution, output validation, descriptor closure, or quiescence failed.
**BUILD-AUTH-071** When a pair is about to be returned, the build authority shall revalidate every retained authority, including physical root, null device, nominal executables, selective source rows, and sealed manifest claims.
**BUILD-AUTH-072** When the first module-owned direct operational Go child is about to start, the build authority shall require the task-private keyed projection, empty private GOTMPDIR and GOCACHE trees, and the normative physical-root/null bracket for pinned-Go-owned nil-stdin descendants.
**BUILD-AUTH-073** When a module-owned direct Git or Go child completes or the next module-owned child, final return, or cleanup is about to begin, the build authority shall enforce the applicable private-derived-tree limits and unchanged-workspace invariants.
**BUILD-AUTH-074** When the requested revision is parsed, the build authority shall require one lowercase full object ID whose width matches the admitted SHA-1 or SHA-256 repository format.
**BUILD-AUTH-075** When custom source-object and tree preflight has succeeded, the build authority shall require private Git to resolve the requested revision to the identical commit ID.
**BUILD-AUTH-076** When the requested commit is admitted, the build authority shall require the custom parser and private Git transcript to agree before deriving one authenticated UTC committer timestamp.
**BUILD-AUTH-077** When private Git is about to dereference an object, the build authority shall require the complete bounded custom preflight for that private store to have succeeded and the object spool to be empty.
**BUILD-AUTH-078** When source objects are preflighted, the build authority shall structurally validate every physical loose and packed representation.
**BUILD-AUTH-079** When physical object representations share an object ID, the build authority shall charge them separately and require equal canonical type, size, and secondary SHA-256 claims.
**BUILD-AUTH-080** When physical representations are inflated, the build authority shall enforce the independent 16-GiB aggregate inflated-representation ceiling.
**BUILD-AUTH-081** When object representations are resolved, the build authority shall enforce the independent 16-GiB aggregate resolved-byte ceiling charged once per physical representation.
**BUILD-AUTH-082** When delta objects are resolved, the build authority shall enforce the normative instruction, total-instruction, depth, ancestry, cycle, and base-completeness policy.
**BUILD-AUTH-083** When Git is about to traverse the selected revision, the build authority shall require the selected commit and tree graph to have been hash-verified and expanded within the normative metadata and materialized-tree limits.
**BUILD-AUTH-084** When a committed attribute source is admitted, the build authority shall require a regular blob within the per-file and aggregate attribute limits.
**BUILD-AUTH-085** When the private repository is initialized, the build authority shall use the exact init form in the normative Git transaction table.
**BUILD-AUTH-086** When private source materialization runs, the build authority shall not invoke clone, transport, or a shell.
**BUILD-AUTH-087** When private init configuration is retained, the build authority shall preserve only validated core.filemode, core.symlinks, core.ignoreCase, and core.precomposeUnicode booleans.
**BUILD-AUTH-088** When the private repository uses SHA-1, the build authority shall configure repository format version 0 without extensions.objectFormat.
**BUILD-AUTH-089** When the private repository uses SHA-256, the build authority shall configure repository format version 1 with extensions.objectFormat=sha256.
**BUILD-AUTH-090** When source objects are copied, the build authority shall copy every admitted loose-object and complete matching pack/index content file.
**BUILD-AUTH-091** When one source object file is copied, the build authority shall use retained roots to create an exclusive equal-byte equal-digest destination with a different identity.
**BUILD-AUTH-092** When the private repository is populated, the build authority shall omit refs, source config, hooks, logs, index, metadata attributes, grafts, shallow state, routing metadata, and accelerators.
**BUILD-AUTH-093** When object copying begins or ends, the build authority shall require the source and destination content manifests to match their retained claims.
**BUILD-AUTH-094** When object copying completes, the build authority shall repeat the complete bounded object preflight against the private store and empty the spool before any private Git object dereference.
**BUILD-AUTH-095** When a private object query or checkout is about to run, the build authority shall replace and reparse destination configuration with the byte-exact closed form in the normative private-config table.
**BUILD-AUTH-096** When private object integrity is checked, the build authority shall execute the exact child-free fsck form in the normative Git transaction table.
**BUILD-AUTH-097** When fsck succeeds, the build authority shall require lazy-fetch-disabled cat-file to confirm the exact commit.
**BUILD-AUTH-098** When checkout is about to begin, the build authority shall require Git's exact NUL-framed tree inventory to equal the custom expanded manifest.
**BUILD-AUTH-099** When checkout is about to begin, the build authority shall resolve attributes for every exact path from the exact revision.
**BUILD-AUTH-100** If an exact path returns an attribute state outside the normative closed attribute-state table, then the build authority shall refuse checkout.
**BUILD-AUTH-101** When raw blobs are streamed, the build authority shall use bounded git cat-file --batch -Z sessions over unique exact blob IDs.
**BUILD-AUTH-102** When a filtered projection is streamed, the build authority shall use the exact argv-scoped --attr-source=<revision> command form.
**BUILD-AUTH-103** When checkout is about to begin, the build authority shall require every filtered path projection to equal its raw blob in size and SHA-256.
**BUILD-AUTH-104** When checkout runs, the build authority shall check out the detached exact commit under the normative hooks, filters, and single-worker policy.
**BUILD-AUTH-105** When checkout completes, the build authority shall require every tracked entry's no-follow raw content hash and exact mode to match the custom tree.
**BUILD-AUTH-106** When checkout or either role build completes, the build authority shall require HEAD to name the exact detached commit.
**BUILD-AUTH-107** When checkout or either role build completes, the build authority shall require the exact porcelain-status command to return empty output.
**BUILD-AUTH-108** When checkout or either role build completes, the build authority shall require the exact ref-inventory command to return empty output.
**BUILD-AUTH-109** When module metadata is admitted, the build authority shall require repository-root tracked regular exact-blob go.mod and go.sum files.
**BUILD-AUTH-110** If module metadata declares a module or filesystem replacement, then the build authority shall refuse dependency discovery.
**BUILD-AUTH-111** When caller working-tree dirt does not enter the private exact checkout, the build authority shall ignore that dirt.

## Dependency and consumed-input closure

**BUILD-AUTH-112** When dependency discovery runs, the build authority shall execute the exact ordered argv and output contract in the normative Go discovery table and validate every reachable pinned-Go-owned child family against the normative argv, directory, environment, count, and schedule closure.
**BUILD-AUTH-113** When dependency discovery runs, the build authority shall require the private checkout to be the only main module.
**BUILD-AUTH-114** When dependency discovery runs, the build authority shall derive the fixed AGM and disk-watchdog role graphs.
**BUILD-AUTH-115** When dependency discovery returns inputs, the build authority shall classify every reported source, embed, syso, assembly, module, and toolchain input beneath one authenticated authority.
**BUILD-AUTH-116** When a Go source, embed, syso, or recursively resolved assembler-source include enters either role closure, the build authority shall require a no-follow regular authenticated checkout, selected-module, or GOROOT entry.
**BUILD-AUTH-117** If a symlink or unsupported entry enters either role closure, then the build authority shall refuse the build.
**BUILD-AUTH-118** When a tracked symlink remains outside both role closures, the build authority shall never follow its target during build or cleanup.
**BUILD-AUTH-119** When an external graph module is selected, the build authority shall authenticate its cached .mod against one unique /go.mod sum.
**BUILD-AUTH-120** When an external module supplies a package to either role, the build authority shall authenticate its complete extracted tree and ZIP against one unique module h1: sum.
**BUILD-AUTH-121** When .ziphash evidence is checked, the build authority shall treat it only as supplementary cache-consistency evidence.
**BUILD-AUTH-122** When go mod verify evidence is checked, the build authority shall treat it only as supplementary cache-consistency evidence.
**BUILD-AUTH-123** When the fixed pair is built, the build authority shall execute the exact supplementary mod-verify operations in the normative Go transaction schedule and require the six module-list and two mod-verify calls to emit none of the role-call Git/tool-ID bundles.
**BUILD-AUTH-124** When either role build is about to start or has just completed, the build authority shall require the exact role graphs and selected-module manifests to match their authenticated claims.
**BUILD-AUTH-125** When a pair is about to be returned, the build authority shall require the exact role graphs and selected-module manifests to match their authenticated claims.
**BUILD-AUTH-126** When any ZIP operation is about to allocate storage proportional to entry count or path bytes, the build authority shall complete the bounded EOCD or ZIP64 structural preflight.
**BUILD-AUTH-127** When selected module ZIPs are admitted, the build authority shall enforce the independent per-ZIP and transaction-wide limits in the normative resource table.
**BUILD-AUTH-128** When a selected module cache path is derived, the build authority shall require the canonical case-escaped module path and version defined by the pinned Go module-cache format.
**BUILD-AUTH-129** When a selected module ZIP entry is admitted, the build authority shall require the canonical unescaped <module>@<version>/ prefix.
**BUILD-AUTH-130** When a selected module ZIP is semantically checked, the build authority shall enforce the normative clean-path, collision, root-go.mod, module-file-size, and total-content policy.
**BUILD-AUTH-131** When extracted-tree or ZIP hashing runs, the build authority shall check context cancellation through bounded readers.
**BUILD-AUTH-132** When extracted-tree or ZIP hashing runs, the build authority shall consume retained authority handles without reopening an unretained caller path.
**BUILD-AUTH-133** When assembler includes are resolved, the build authority shall search the authenticated source directory, private action-object directory, and authenticated GOROOT include directory in that exact order.
**BUILD-AUTH-134** When an authenticated package-local go_asm.h exists, the build authority shall admit that source-directory file.
**BUILD-AUTH-135** When package-local go_asm.h is absent, the build authority shall admit only the exact empty action-local header followed by the compiler -asmhdr replacement lifecycle.
**BUILD-AUTH-136** If an assembler include is derived outside BUILD-AUTH-134 or BUILD-AUTH-135, then the build authority shall refuse the build.
**BUILD-AUTH-137** When importcfg, archive, object, symabis, cache, or action-work intermediates are admitted, the build authority shall require them beneath the private GOTMPDIR or GOCACHE trees.
**BUILD-AUTH-138** When private derived intermediates are checked, the build authority shall enforce the normative combined count, size, type, no-symlink, and no-special-entry policy.

## Build and output admission

**BUILD-AUTH-139** When either role is built, the build authority shall execute the exact role command in the normative role and build table and validate its authenticated action-graph-derived operational child multiset and causal partial order without requiring one host-independent total chronology.
**BUILD-AUTH-140** When either role is built, the build authority shall use the shared exact four-assignment linker stamp.
**BUILD-AUTH-141** When artifact build information is admitted, the build authority shall require the exact experiment-bearing embedded Go version.
**BUILD-AUTH-142** When artifact build information is admitted, the build authority shall require the exact role package path.
**BUILD-AUTH-143** When artifact build information is admitted, the build authority shall require the exact main-module record and derived tagless pseudo-version.
**BUILD-AUTH-144** When artifact build information is admitted, the build authority shall require DefaultGODEBUG to equal the authenticated value from that role package's fixed go list result.
**BUILD-AUTH-145** When artifact dependencies are admitted, the build authority shall require the exact authenticated role closure in Go 1.27.1 module order with exact path, version, h1: sum, and nil replacement.
**BUILD-AUTH-146** When artifact settings are admitted, the build authority shall require the exact ordered thirteen-setting vector.
**BUILD-AUTH-147** When an artifact is measured, the build authority shall bind build-info parsing, Mach-O validation, and SHA-256 hashing to one retained no-follow output identity.
**BUILD-AUTH-148** When an output is admitted, the build authority shall require an effective-user-owned regular executable containing exactly one thin arm64 image.
**BUILD-AUTH-149** When an output is admitted, the build authority shall enforce the common Mach-O loader policy.
**BUILD-AUTH-150** When an output is admitted, the build authority shall retain its identity, size, modification time, and SHA-256 digest.
**BUILD-AUTH-151** When a pair is about to be returned, the build authority shall recheck every retained output claim against the same identity.
**BUILD-AUTH-152** If either build, artifact verification, retained-input check, graph comparison, or final authority revalidation fails, then the build authority shall return a nil pair and one non-nil typed refusal.
**BUILD-AUTH-153** If one role succeeds and the pair transaction later fails, then the build authority shall expose no partial artifact.
**BUILD-AUTH-154** If the pair transaction fails, then the build authority shall make no fallback to ambient tools, network content, working-tree source, an older revision, or a weaker permission policy.

## Process, errors, and cleanup

**BUILD-AUTH-155** When a module-owned direct child is started, the build authority shall require one exact tagged stdin source, require the normative process-wide reaping regime, place the child in a new Darwin process group, and register it with the owning supervisor before any later failure.
**BUILD-AUTH-156** When a child stop is latched, the build authority shall attempt group termination only while leader pinning and the admitted reaping regime remain proved, allow only one consecutive waitid EINTR retry after one positive channel-free five-millisecond wait, otherwise enter the normative no-signal handoff, and assign exactly one designated Cmd.Wait caller.
**BUILD-AUTH-157** When child I/O is configured, the build authority shall require a tagged retained-null or fixed bounded-input-pipe source, own every explicit pipe, retain at most the bounded diagnostic prefix, count all observed diagnostic bytes, select only complete bounded structured output or one fixed-protocol concrete raw stream plan, and withhold every successful result until quiescence, empty stderr, and closure without DescriptorClose of every supervisor-owned transient child-I/O endpoint due at that boundary are proved.
**BUILD-AUTH-158** When a child supervisor is about to return synchronously or after background handoff, the build authority shall give its owned I/O workers the exact normative one-second completion deadline without relying on exec.Cmd.WaitDelay and preserve the task if a concrete stream worker holding only value claims and task-spool authority remains incomplete.
**BUILD-AUTH-159** When cleanup is about to be authorized after child execution, the build authority shall prove direct-child exit and an empty created process group.
**BUILD-AUTH-160** If cancellation, timeout, termination, wait, output draining, or group inspection leaves quiescence uncertain, then the build authority shall avoid claiming task-root removal.
**BUILD-AUTH-161** If complete task-root absence is not proved, then the build authority shall place its generated path and last verified StateRoot and task-root identities only in the non-rendered recovery fields of the typed refusal.
**BUILD-AUTH-162** When a child or operation fails, the build authority shall preserve the first failing primitive's attribution without wrapper relabeling through the closed non-process, descriptor-read, absence, sentinel, allocator, and process tables and shall render only their fixed labels, sanitized cause codes, and applicable child fields.
**BUILD-AUTH-163** When a child or operation fails, the build authority shall not expose raw child output, raw OS or tool errors, environment values, source content, or commit-controlled filenames.
**BUILD-AUTH-164** When cleanup is authorized, the build authority shall preflight the complete owned-entry ledger and named task child against retained identities before removing any content.
**BUILD-AUTH-165** When task content is removed, the build authority shall use retained-root no-follow operations confined to the task-root filesystem.
**BUILD-AUTH-166** When the task child is about to be removed, the build authority shall require an observed task-root identity and recheck its named identity through the retained StateRoot.
**BUILD-AUTH-167** When the rechecked task child is empty and identical, the build authority shall remove only that child.
**BUILD-AUTH-168** If task-root identity was never observed, quiescence is uncertain, non-root descriptor closure fails, or preflight before the first cleanup unlink encounters a replaced identity, unsafe ownership or mode, nested mount, symlink ambiguity, or incomplete proof, then the build authority shall preserve the complete task and return the corresponding typed refusal without claiming complete task-root absence.
**BUILD-AUTH-169** When cleanup proof becomes incomplete after the deletion barrier, the build authority shall stop further removal, preserve the remainder, avoid broad recursive or pathname-only deletion, and not claim complete task-root absence.
**BUILD-AUTH-170** When failures coexist, the build authority shall preserve the first Primary-eligible failure in write-once Primary, retain at most the first later non-close command-end or block-end authority failure only as the normative private sanitized record, aggregate acquired-descriptor close failures only in DescriptorClose, retain process-quiescence uncertainty only in Cleanup, expose no secondary public slot, and return populated Primary, DescriptorClose, and Cleanup slots in that exact order.

## Normative outcomes, refusals, and cleanup

Stage and Close have only these outcomes; `(nil, nil)` and a non-nil pair with a
non-nil error are invalid Stage results.

| Event | Pair result | Error result |
| --- | --- | --- |
| Stage success | One non-nil sealed pair | `nil` |
| Stage refusal | `nil` | One non-nil typed refusal |
| Close success | Not applicable | `nil` |
| Close refusal | Not applicable | The one stable non-nil typed refusal returned to every Close caller |

A typed refusal implements `error` plus `Report() FailureReport`; `errors.As`
can recover that interface, and every `Report` call returns the same deep-copied
value. The report contains these fields and no raw wrapped cause:

| Field | Exact contract |
| --- | --- |
| `Primary` | Optional write-once sanitized record for the first Primary-eligible initiating failure; descriptor closure and cleanup/quiescence never compete for it, and later non-close command-end or block-end authority failures never replace, relabel, or extend it |
| `DescriptorClose` | Optional sanitized record aggregating one or more acquired-descriptor close failures; its operation is the earliest failing closure category in transaction order |
| `Cleanup` | Optional sanitized record for uncertain quiescence or for refused, failed, or incomplete identity-gated removal; uncertain quiescence skips removal, so these operations cannot coexist |
| Populated-slot invariant | At least one slot is populated; populated slots retain `Primary`, `DescriptorClose`, `Cleanup` order |
| Sanitized record | One fixed phase, one fixed operation, and a nonempty deduplicated cause-code array in the fixed order below |
| `Child` | Optional only on the applicable sanitized record and only after a child starts; at most one exists in the complete report, selects the earliest failed or uncertain child in fixed command order, and includes optional observed signed exit status, total diagnostic byte count, truncation Boolean, and SHA-256 of the retained bounded diagnostic prefix |
| Exit status | Present only for terminal status admitted by the event loop before public-status sealing: nonnegative exit code for normal exit or negative signal number for signaled exit; absent otherwise, and a background `Cmd.Wait` result is private and never supplies or mutates it |
| `Recovery` | Optional exactly when complete absence of an allocated task child was not proved; includes the generated absolute task path, the last verified StateRoot identity, and the task-root identity only when that identity was observed |
| Root identity record | Device, inode, UID, complete mode, and filesystem identity captured by the last successful descriptor-bound verification |
| Raw state | Captured diagnostic bytes and raw OS or tool causes remain private and are absent from the report and unwrap chain |

Primary-eligible means an initiating request, validation, authority, process,
output, source, dependency, build, verification, or final failure. Acquired-
descriptor closure and process-quiescence or removal conditions use only their
dedicated slots and never compete for Primary. Primary is write-once for the
first Primary-eligible failure.

After every named process-owner invocation returns, including a return caused
by request validation, setup, or `Start` failure, every applicable command-end
authority check runs in its fixed order. A precheck failure invokes no process
owner and has no command-end bracket; it closes every transient resource it
acquired. Successful initial physical-root, retained-null, and sentinel
admission arms the separate block-end bracket, which runs once on every later
block exit, including a first-row precheck failure.

If Primary is empty, the earliest failed non-close command-end or block-end
check becomes Primary even when DescriptorClose or Cleanup is already
populated. If Primary is occupied, exactly the first later non-close command-end
or block-end failure in fixed check order is retained only as one private
sanitized `{phase, operation, cause}` record. It carries one cause and no raw
error, path, output, or Child; later such failures create no additional record.
The record exists only until the current block outcome is composed, never
crosses the module seam or mutates the public report, and its presence withholds
proof and prevents future execution. DescriptorClose remains dedicated to
acquired-descriptor closure and Cleanup to process-quiescence uncertainty or
identity-gated removal. There is no public Secondary or Postcondition slot, and
deterministic report order is not a claim of chronology.

`Recovery.TaskRoot` is a `*FileIdentity`: it is nil exactly when task-root
identity was never observed, and every `Report` deep copy also copies the
pointed-to value. A zero `FileIdentity` never represents missing evidence.

Fixed phase labels, in order, are `request`, `authority`, `workspace`, `source`,
`dependencies`, `build-agm`, `verify-agm`, `build-disk-watchdog`,
`verify-disk-watchdog`, `final`, and `close`. Fixed operation labels, in order,
are `validate`, `open`, `probe`, `walk`, `parse`, `hash`, `copy`, `execute`,
`compare`, `quiesce`, `close-nonroot`, `remove`, and `close-root`. Fixed cause
codes, in order, are `invalid-request`, `not-found`, `permission`, `malformed`,
`unsupported`, `unstable`, `limit`, `canceled`, `deadline`, `child-start`,
`child-exit`, `child-terminate`, `child-wait`, `child-drain`, `child-survivor`,
`child-probe`, `identity`, `descriptor-close`, `cleanup`, and
`internal-invariant`.

Rendered error text has this ASCII grammar, skipping absent slots while
retaining slot order:

```text
buildauthority: <slot>=<phase>/<operation>/<comma-separated-cause-codes>[ child(exit=<signed-decimal-or-none>,bytes=<decimal>,truncated=<true-or-false>,sha256=<64-lowercase-hex>)][; <next-slot>...]
```

`<slot>` is exactly `primary`, `descriptor-close`, or `cleanup`. The renderer
never includes `Recovery`, a path, an identity record, raw bytes, raw cause text,
an environment value, or commit-controlled content.

The shared cleanup transaction has this exact order:

| Order | Action | Authorization and result |
| ---: | --- | --- |
| 1 | Quiescence attempt | Finalize every started child only through its owning supervisor and consume its already-owned direct-child, created-group, and per-pipe EOF evidence; no cleanup caller independently signals, probes, waits, reaps, or closes a supervisor pipe; uncertainty records `Cleanup` with operation `quiesce` without replacing an existing `Primary` |
| 2 | Non-root closure | Close every retained descriptor except the task-root and StateRoot handles exactly once, even after quiescence uncertainty; any failure records `DescriptorClose` |
| 3 | Disposition and conditional removal | With StateRoot and any observed task-root handle open, preflight the complete owned-entry ledger before deleting anything; run identity-gated no-follow removal only when task identity, quiescence, and every non-root close are proved, then prove owned absence; when removal is skipped or fails, perform the safe final no-follow name/identity observation and store `present` or `unknown`; any refusal records `Cleanup` |
| 4 | Root closure | Close the task-root handle when one exists and then the StateRoot handle exactly once whether step 3 succeeded, failed, or was skipped; any failure records `DescriptorClose`, labeled `close-nonroot` when step 2 also failed and otherwise `close-root` |
| 5 | Stable result | Use the stored `removed`, `present`, or `unknown` disposition; attach `Recovery` for `present` or `unknown`, and return the single deterministic typed refusal or `nil` |

Because rendered slots always retain report order, a cleanup-quiescence failure
plus a descriptor-close failure renders `descriptor-close` before `cleanup`
even though the quiescence attempt occurred first.

### Closed non-process failure attribution

Before allocation, the first Primary-eligible failing primitive owns the
write-once Primary phase and operation; a wrapper never relabels it, and
descriptor closure or cleanup/quiescence never competes for it. Every
non-process no-scratch boundary uses this closed table:

| First failing primitive | Primary phase and operation |
| --- | --- |
| Missing context, malformed request shape, or invalid public value knowable without authority | `request/validate` |
| Entry context already canceled or expired | `request/validate` |
| Unsupported runtime platform or static admitted-authority predicate | `authority/validate` |
| Initial Darwin `SIGCHLD` query or other named platform capability probe | `authority/probe` |
| Open, no-follow retain, or descriptor acquisition for StateRoot, `/`, `/dev/null`, Go, compiler, GOROOT, GOMODCACHE, or Git | `authority/open` |
| Query descriptor identity, ancestry, ACL, mount, or raw-device facts for an authority | `authority/probe` |
| Enumerate a complete GOROOT or GOMODCACHE capture or frame its bounded manifest | `authority/walk` |
| Parse `GOROOT/go.env`, descriptor ACL bytes, or retained executable Mach-O bytes | `authority/parse` |
| Hash an authority leaf, regular-content row, or complete authority manifest | `authority/hash` |
| Compare a pinned tool value, disjointness relation, independent claim, or repeated authority snapshot | `authority/compare` |
| Open or retain Repository, `.git`, config, admin, packed-ref, or object-store state | `source/open` |
| Query descriptor identity, ancestry, ACL, or mount facts for source state | `source/probe` |
| Enumerate the selective `.git` envelope or frame repository/source-object manifests | `source/walk` |
| Parse the revision against admitted object format, config, packed refs, object names, pack/index name-pair framing, descriptor ACL bytes, or a source-row wire image | `source/parse` |
| Hash source config, object content, auxiliary content, or a source manifest | `source/hash` |
| Validate an already parsed source policy predicate | `source/validate` |
| Compare source-row output, physical paths, formats, origins, independent claims, or a repeated source snapshot | `source/compare` |
| Context cancellation or expiry between primitives | The phase and operation assigned to the next primitive, with `canceled` or `deadline` |
| Impossible sealed-plan, dependency, or adapter state | The current phase with `validate/internal-invariant` |

At an authority boundary, orchestration may reuse pure parsers, classifiers,
and claim comparators, but it may not invoke a cause-only composite that spans
two or more public operations. Each fallible primitive returns its owning
operation with the classified cause. Descriptor stat and mount probing remain
distinct from raw ACL acquisition and ACL parsing; a fixed-size `ReaderAt`
failure belongs to the consuming parse or hash operation. Every primitive that
acquires a transient leaf transfers one explicit exact-once close obligation to
its immediate owner, whose close outcome remains separately reportable. A
later wrapper may preserve this attribution but never infer or replace it.

Descriptor reads have no generic public `read` operation. A read that supplies a
parser is owned by `parse`; one that supplies a digest by `hash`; a directory
record by `walk`; and a metadata or absence observation by `probe`. For a
fixed-size regular-file or symlink-text read, EOF succeeds only after the exact
captured byte count. Premature EOF, an unexpected short count, or other I/O
failure against that captured size is `unstable`; permission is `permission`;
and context termination is `canceled` or `deadline`. Directory EOF is normal
only after the final bounded batch and a post-enumeration identity/security
snapshot equal to the pre-read snapshot. A non-EOF directory-read error uses
the owning phase's `walk` operation with `permission`, `canceled`, `deadline`,
or `unstable`; bound exhaustion is `walk/limit`. `malformed` is reserved for
complete bytes rejected by the consuming parser.

Required, optional, and forbidden path observations use this closed rule:

| Observation | Mapping |
| --- | --- |
| Required row cleanly absent on initial admission | Owning phase, `open/not-found` |
| Optional row cleanly absent on initial admission | Success; retain an exact absent claim |
| Forbidden row cleanly absent on initial admission | Success; retain an exact absent claim |
| Forbidden row present on initial admission | Owning phase, `validate/unsupported` |
| Permission while observing any required, optional, or forbidden row | Owning phase, `probe/permission` |
| Other unexpected lookup result while observing absence or presence | Owning phase, `probe/unstable` |
| Required or initially present optional row disappears, or an initially absent optional or forbidden row appears, during revalidation | Owning phase, `compare/unstable` |
| Expected present row resolves to a different object | Owning phase, `compare/identity` |

The physical-root sentinels are an exact specialization:

| Root or sentinel observation | Mapping |
| --- | --- |
| Initial retained open of physical `/` fails | `authority/open` with `not-found`, `permission`, `identity`, `unsupported`, or `unstable` from the closed path classifier |
| `/go.mod`, `/go.work`, or `/.git` unexpectedly present before the non-source block | `authority/validate/unsupported` |
| Permission while looking up a sentinel before or after the block | `authority/probe/permission` |
| Any other unexpected sentinel lookup error before or after the block | `authority/probe/unstable` |
| A sentinel newly present after the block | `authority/compare/unstable` |
| Physical-root identity mismatch | `authority/compare/identity` |
| Other root metadata, ACL, mount, mode, owner, or security drift | `authority/compare/unstable` |

A syntactically valid but policy-disallowed fact is `unsupported`; malformed
bytes are `malformed`; an exceeded bound is `limit`; missing data is
`not-found`; access denial is `permission`; unexpected I/O or concurrent drift
is `unstable`; and a named/retained object, ancestry, alias, or path-value
mismatch is `identity`. A well-framed wrong version, setting, or transcript is
`compare/unsupported`, except a wrong retained path is `compare/identity`;
malformed, missing, duplicate, extra, or reordered wire rows are
`parse/malformed`. Exit zero with nonempty stderr is the current phase's
`parse/malformed`; stdout stays unavailable and Primary owns the bounded
status-zero Child diagnostic. Delayed validation of a caller revision is
`source/parse/invalid-request`; an unrecognized internal state is
`internal-invariant`.

After complete no-scratch capture, workspace allocation uses this closed table:

| First failing allocation primitive | Mapping and allocation state |
| --- | --- |
| Missing entropy, retain, or other required adapter; impossible candidate or path shape | `workspace/validate/internal-invariant`; no allocation or Recovery |
| Cryptographic reader fails or returns fewer than exactly 12 bytes | `workspace/open/unstable`; no allocation or Recovery |
| Context check after candidate framing observes cancellation or expiry | `workspace/open/canceled` or `workspace/open/deadline`; no allocation or Recovery |
| Exclusive `mkdirat` returns `EEXIST` | Consume the candidate and retry; no allocation or Recovery |
| 100 `EEXIST` candidates are exhausted | `workspace/open/limit`; no allocation or Recovery |
| Other exclusive `mkdirat` failure | `workspace/open` with `not-found`, `permission`, `identity`, `unsupported`, or `unstable`; no allocation or Recovery |
| Exclusive `mkdirat` succeeds | Immediately record generated name plus retained StateRoot identity as an unobserved allocation before any fallible retain; every later failure has Recovery but no fabricated task-root identity |

Before allocation, failure to close any acquired non-StateRoot descriptor adds
`DescriptorClose/close/close-nonroot/descriptor-close`, including a transient
leaf whose admission failed before it could become a retained capability. Each
acquired descriptor has one exact-once close owner independent of Primary.
StateRoot close failure uses `close-root` unless an earlier non-root failure
already owns the aggregate slot. A no-scratch refusal has no Recovery.

## Normative platform and tool pins

The immutable value-only receipt contains exactly these claims:

| Receipt field | Exact content |
| --- | --- |
| Authority version | `sandbox-gc-build-authority/v1` |
| Revision | The admitted full lowercase object ID |
| Platform | `darwin/arm64` |
| Tool digests | Separate SHA-256 arrays for the admitted Go and Git executable identities |
| Manifest digests | Separate SHA-256 arrays for the `goroot/v1`, `repository/v1`, `source-object-content/v1`, `source-tree/v1`, `gomodcache/v1`, and `selected-modules/v1` domains |
| Protected stamp | The exact four-assignment linker stamp below |
| Roles | A fixed two-element array containing exact output name, main-package path, and artifact SHA-256 in AGM then disk-watchdog order |

| Claim | Exact policy |
| --- | --- |
| Platform | `darwin/arm64`; every other platform refuses before allocation |
| Go executable digest | `548608a910c46de32c65a3934f461b1787acf6ddd371044826068d8503b8509b` |
| Go command | `go version go1.27.1 darwin/arm64` |
| Go command wire | Exactly 33 LF-terminated bytes; SHA-256 `c528ba02b39880f1357f51dee70e41c1ef03e3a70c7a03055c080532b9635f58` |
| `GOVERSION` | `go1.27.1` |
| Compiler probe | Retained absolute `<GOROOT>/pkg/tool/darwin_arm64/compile -V=full` |
| Compiler full version | `compile version go1.27.1 X:nojsonv2,nogreenteagc,norandomizedheapbase64,nosizespecializedmalloc` |
| Compiler report wire | The displayed tokens separated by one ASCII space and followed by LF; exactly 96 bytes; SHA-256 `2ecca23a89d79ae35458843c1c1d9f3b037ad8fcfa864184169279c2787fa380` |
| Embedded Go version | `go1.27.1-X:nojsonv2,nogreenteagc,norandomizedheapbase64,nosizespecializedmalloc` |
| Git digest | `be4afb2b003904725826250de9fb76567bbacf82323457b5a1ec26706b66bcae` |
| Git implementation | Apple Git-155, arm64-all slice, no arm64e slice and no `libxcselect` load |
| Git version wire | Exactly 225 bytes; SHA-256 `70c39c3d0e3fa158a62a5602140a4370d77caa639269a6bd7ec324773d08aafa` |
| Git builtin wire | Exactly 1,467 bytes comprising 144 unique strictly byte-ascending LF-terminated tokens; SHA-256 `7712b98c146e53d5a26119533901a748176f9743e60e8675a396c4f103dd8625` |
| Required Git builtins | `init`, `rev-parse`, `config`, `fsck`, `cat-file`, `ls-tree`, `check-attr`, `checkout`, `status`, `log`, `for-each-ref` |

The exact Git version/build-options transcript contains these lines once, in
this order, with no other line:

```text
git version 2.50.1 (Apple Git-155)
cpu: arm64
no commit associated with this build
sizeof-long: 8
sizeof-size_t: 8
shell-path: /bin/sh
feature: fsmonitor--daemon
libcurl: 8.7.1
zlib: 1.2.12
SHA-1: SHA1_DC
SHA-256: SHA256_BLK
```

The keyed Go environment probe runs once under each profile with this exact
ordered projection after path placeholders are substituted:

| Order | Key | Preallocation value | Task-private value |
| ---: | --- | --- | --- |
| 1 | `GOROOT` | retained GOROOT | retained GOROOT |
| 2 | `GOMODCACHE` | retained GOMODCACHE | retained GOMODCACHE |
| 3 | `GOCACHE` | `off` | private GOCACHE |
| 4 | `GOCACHEPROG` | empty | empty |
| 5 | `GOENV` | empty | empty |
| 6 | `GOFLAGS` | `-mod=readonly` | `-mod=readonly` |
| 7 | `GOWORK` | `off` | `off` |
| 8 | `GOTOOLCHAIN` | `local` | `local` |
| 9 | `GOPROXY` | `off` | `off` |
| 10 | `GOSUMDB` | `off` | `off` |
| 11 | `GOVCS` | `*:off` | `*:off` |
| 12 | `GOTELEMETRY` | computed `off` | computed `off` |
| 13 | `GOTELEMETRYDIR` | computed empty | computed empty |
| 14 | `GO111MODULE` | `on` | `on` |
| 15 | `GOEXPERIMENT` | `none` | `none` |
| 16 | `GOFIPS140` | `off` | `off` |
| 17 | `GO_EXTLINK_ENABLED` | `0` | `0` |
| 18 | `CGO_ENABLED` | `0` | `0` |
| 19 | `GOHOSTOS` | `darwin` | `darwin` |
| 20 | `GOHOSTARCH` | `arm64` | `arm64` |
| 21 | `GOTOOLDIR` | `<retained GOROOT>/pkg/tool/darwin_arm64` | `<retained GOROOT>/pkg/tool/darwin_arm64` |
| 22 | `GOVERSION` | `go1.27.1` | `go1.27.1` |
| 23 | `GOOS` | `darwin` | `darwin` |
| 24 | `GOARCH` | `arm64` | `arm64` |
| 25 | `GOAMD64` | empty | empty |
| 26 | `GOARM64` | `v8.0` | `v8.0` |

`GOTELEMETRY` and `GOTELEMETRYDIR` are computed report values, not process
environment assignments. Exact empty `HOME` makes pinned Darwin Go leave its
telemetry directory uninitialized.

The keyed projection argv contains the 26 keys in the semantic table order,
but pinned Go 1.27.1 emits members in this exact lexicographic order:
`CGO_ENABLED`, `GO111MODULE`, `GOAMD64`, `GOARCH`, `GOARM64`, `GOCACHE`,
`GOCACHEPROG`, `GOENV`, `GOEXPERIMENT`, `GOFIPS140`, `GOFLAGS`, `GOHOSTARCH`,
`GOHOSTOS`, `GOMODCACHE`, `GOOS`, `GOPROXY`, `GOROOT`, `GOSUMDB`,
`GOTELEMETRY`, `GOTELEMETRYDIR`, `GOTOOLCHAIN`, `GOTOOLDIR`, `GOVCS`,
`GOVERSION`, `GOWORK`, `GO_EXTLINK_ENABLED`. Expected stdout is the single
canonical byte image produced by `json.NewEncoder`, default HTML escaping,
`SetIndent("", "\t")`, then one `Encode` call over the expected string map.
It has exactly 26 unique members and one final LF. Every represented dynamic
path must be valid UTF-8 before encoding; an equivalent JSON form is not
admitted.

Authenticated `GOROOT/go.env` contains only comments, blanks, and exactly one
occurrence of each of these literal assignments:

```text
GOPROXY=https://proxy.golang.org,direct
GOSUMDB=sum.golang.org
GOTOOLCHAIN=auto
```

Every assignment is overridden by the fixed nonempty child value. Missing,
duplicate, differently valued, malformed, or unknown assignments and any
`GOCACHEPROG` assignment refuse.

Initial admission and every applicable command bracket use the same exact
`revalidateLiveGoEnvironment` transaction. It first resolves the original
absolute GOROOT pathname no-follow from retained physical `/` and proves that
it still names the retained GOROOT root and ancestry. Beneath that root it opens
a fresh no-follow `go.env` leaf and immediately assigns one exact-once owner.
The operation-attributed primitives then perform descriptor metadata and mount
probes, raw ACL acquisition and separate ACL parsing, the exact-size parser-
owned `ReaderAt` read, literal grammar parsing, content hashing, post-read
descriptor observation, and comparison with the captured tree row and digest.
The owner closes the fresh leaf on every path. Finally, even after leaf
validation or close failure, the transaction repeats the physical-root-
anchored GOROOT pathname resolution and comparison whenever its retained
prerequisites remain usable. The child-consumed path is therefore proved to
name the retained tree on both sides of every fresh-leaf transaction.

The initial live claim cannot be published from the captured tree row alone,
and revalidation never reuses an earlier leaf descriptor. A close failure stays
in DescriptorClose even when an earlier primitive owns Primary; no cause-only
composite may erase the initiating operation or the acquired leaf's close
result.

### Exact no-scratch capture and probes

No-scratch capture follows this exact eight-step, two-block order:

| Step | Exact boundary |
| ---: | --- |
| 1 | Validate the request and entry context; admit Darwin/arm64 and the initial `SIGCHLD` state |
| 2 | Retain StateRoot, physical `/`, `/dev/null`, Go, GOROOT, GOMODCACHE, Git, Repository, real `.git`, config, and initial object root; parse config for the provisional object format and prove every required disjointness relation |
| 3 | Complete bounded GOROOT and GOMODCACHE captures, nominal Go/compiler/Git admission, literal `GOROOT/go.env` parsing, and their manifests; seal the preallocation environment and five non-source plans |
| 4 | Validate physical `/`, `/dev/null`, and absence of `/go.mod`, `/go.work`, and `/.git`; run the five non-source probes below from `/` in order |
| 5 | Revalidate physical root, null device, sentinels, and the three nominal executable roles after the non-source block |
| 6 | Finish the selective `.git` classification, packed-ref parse, closed object inventory, and exact `goroot/v1`, `gomodcache/v1`, `repository/v1`, and `source-object-content/v1` manifests; seal the four source plans |
| 7 | Run source paths, source formats, source local config, and source active config from the retained physical Repository, in that order |
| 8 | Revalidate and compare every retained authority, sentinel, selective row set, config, inventory, transcript, and manifest; only complete success constructs the single-use `capturedNoScratchInputs` aggregate |

That aggregate contains the retained StateRoot, physical-root, null-device,
nominal Go/compiler/Git, GOROOT, GOMODCACHE, Repository administration, config,
packed-ref, object-inventory, and revision capabilities; the exact
`goroot/v1`, `gomodcache/v1`, `repository/v1`, and
`source-object-content/v1` manifests; and the sealed direct environments and
the opaque whole-transaction window plus nine fixed plans. It is one shared
pending/moved/closed move cell: no
constituent, copy, raw path, manifest digest, or generic process plan authorizes
workspace allocation.

The five non-source plans have argument zero equal to their applicable nominal
retained absolute executable, run from retained physical `/`, bind the retained
null-device descriptor as stdin, use the exact 39-row preallocation environment
and 30-second limit, and require exit zero plus empty stderr:

| Order | Exact argv after argument zero | Exact stdout |
| ---: | --- | --- |
| 1 | Go: `version` | The exact 33-byte Go command wire in the tool-pin table |
| 2 | Go: `env -json GOROOT GOMODCACHE GOCACHE GOCACHEPROG GOENV GOFLAGS GOWORK GOTOOLCHAIN GOPROXY GOSUMDB GOVCS GOTELEMETRY GOTELEMETRYDIR GO111MODULE GOEXPERIMENT GOFIPS140 GO_EXTLINK_ENABLED CGO_ENABLED GOHOSTOS GOHOSTARCH GOTOOLDIR GOVERSION GOOS GOARCH GOAMD64 GOARM64` | The exact canonical JSON projection above |
| 3 | Compiler: `-V=full` | The exact 96-byte compiler report wire in the tool-pin table |
| 4 | Git: `version --build-options` | The exact 225-byte transcript below |
| 5 | Git: `--git-dir=/dev/null --list-cmds=builtins` | The exact 1,467-byte builtin wire; required positions are `cat-file=12`, `check-attr=13`, `checkout=17`, `config=28`, `for-each-ref=47`, `fsck=50`, `init=60`, `log=63`, `ls-tree=66`, `rev-parse=111`, and `status=123` |

One deep private `nonSourceBlock.run` owns this complete five-command segment;
the commands are not reorderable caller operations. For each row it samples
one opaque command window, runs every ordered precheck, invokes that row's one
named process owner only after all prechecks succeed, runs the fixed private
output validator only if the supervisor releases candidate stdout, and then
runs every ordered postcheck after the invocation returns. The invocation is
the exact attempt boundary: request validation, setup, or `Start` failure after
entry still requires the command-end bracket, while successful `Start` is not
required. A precheck failure invokes no process owner, has no command-end
bracket, closes every transient resource it acquired, and prevents all later
children. Structured stdout, proof, the next child, and the outer caller remain
unavailable until the applicable bracket succeeds.

Prechecks and postchecks use the same left-to-right order in this closed table:

| Named non-source materializer | Exact ordered precheck and postcheck row |
| --- | --- |
| Go version | physical root; retained null; retained GOROOT tree and physical pathname binding; live `go.env`; Go executable role |
| Go environment | physical root; retained null; retained GOROOT tree and physical pathname binding; live `go.env`; Go executable role |
| Compiler version | physical root; retained null; retained GOROOT tree and physical pathname binding; compiler executable role |
| Git version | physical root; retained null; Git executable role |
| Git builtin inventory | physical root; retained null; Git executable role |

The physical-root sentinels are block-level checks, not per-command checks.
Successful initial step-4 physical-root, retained-null, and sentinel admission
arms the block-end bracket. Step 5 then runs exactly once on every later block
exit, including a first-row precheck failure before any named invocation,
whenever its retained prerequisites remain usable. Its exact order is physical
root, retained null, `/go.mod`, `/go.work`, `/.git`, Go role, compiler role, and
Git role. An unattempted command has no command-end bracket; the armed block-end
bracket is distinct. If initial step-4 admission fails, the bracket is not armed
and the transaction closes only what it retained. All checks in an entered
command-end or block-end bracket are attempted in order even after an earlier
check fails.

The process owner provides exactly five named non-source materializers: Go
version, Go environment, compiler version, Git version, and Git builtin
inventory. Each accepts only its nominal plan and a module-minted command-
window token, copies the token's exact context and deadlines into one fully
keyed private `processRequest`, and invokes the preallocation supervisor. A
production-source guard admits exactly those five request literals with their
exact direct fields and forbids a generic producer for this block. Later source
and task-private commands require separate named materializers and closed
guards; the generic supervisor carrier or an environment value alone confers no
command authority.

Each of the four source plans uses the same direct profile, retained null stdin,
deadline, and successful empty-stderr gate, but runs only from the retained
physical Repository. Physical root and `/dev/null` remain retained for the
whole transaction and are revalidated before and after every command that
depends on them, including later `core.hooksPath=/dev/null` bindings. No task
child exists before step 8 completes.

## Normative filesystem and parser policy

| Boundary | Exact policy |
| --- | --- |
| General owners and modes | Every ancestry component admits root or effective UID; Repository root and captured descendants require effective UID; other toolchain/cache leaves admit root or effective UID; group/world write and all special mode bits refuse except for the exact specialized `/dev/null` row below |
| StateRoot | Existing effective-UID-owned directory with exact `0700` and no special mode bits |
| Task child | Exclusively created effective-UID-owned directory with exact `0700` and no special mode bits |
| Physical `/` | Retained root-owned directory capability with exact `0755` permissions and no special mode bits on the admitted local ownership-enforcing mount; identity, mode, ACL, mount, and security facts remain stable for the complete transaction |
| Physical-root sentinels | No-follow lookup proves `/go.mod`, `/go.work`, and `/.git` absent as every possible entry kind before and after the non-source probe block; no repository is discovered through cwd |
| `/dev/null` | Retained physical root-owned character-device capability with complete mode `020666`, link count one, and Darwin raw-device major 3 and minor 2; this fixed device is the sole intentional group/world-write exception, and it is retained for the complete transaction and revalidated around every stdin, path-channel, or hooks binding that depends on it |
| Retained root | One `os.Root` plus one separately opened descriptor; both identities match before use, the descriptor supplies ACL and mount evidence, and neither is substituted by a later pathname reopen |
| Executable roles | The retained Go executable, retained compiler leaf, and retained Git executable are distinct nominal capabilities; none is recoverable from a generic executable or process plan |
| ACL | Any ALLOW ACE granting write-data, append-data, delete, delete-child, write-attributes, write-extended-attributes, write-security, take-ownership, generic-write, or generic-all rights refuses regardless of principal; deny-only and read/execute-only ACEs are inert |
| ACL encoding | Descriptor-bound `ATTR_CMN_EXTENDED_SECURITY`; validate length, reference, filesec magic, count, kind, flags, rights, and host endian, then hash only the canonical ACL wire format below |
| Mount | Every authority requires `MNT_LOCAL`, absence of `MNT_IGNORE_OWNERSHIP`, and no descendant FSID/device transition; retain only `f_flags & MNT_VISFLAGMASK` |
| Identity | No-follow opened identity, complete stable metadata, SHA-256 where content-bearing, and applicable before/after revalidation |
| Manifest | Use the exact manifest wire format below; the inspected root is separately framed and excluded from the descendant entry ceiling |
| Mach-O commands | Use the exact Mach-O container, header, command, string, subtype, and system-load policy below |
| Git loose object | Validate canonical path/OID, exact bounded zlib stream, declared type/size, reconstructed hash, and no trailing or truncated representation |
| Git pack/index | Validate version 2, hash width/name, fanout/order/offsets/checksums, pack header/trailer, entry CRC/extents, zlib termination, delta headers/opcodes/bases, graph cycles/depth/ancestry, and every physical representation charge |
| Source object identities | Every loose object, pack, index, and admitted auxiliary is a no-follow regular file with link count one; all captured content-file identities are pairwise distinct |
| Selected Git tree | Hash-verify the exact commit grammar and reconstructed tree objects; admit only `040000`, `100644`, `100755`, and `120000`; reject gitlinks, invalid UTF-8, folded-NFC collisions or `.git`, cycles, excessive depth/path, empty tree, or any materialized-tree charge overflow |

### Exact manifest and ACL wire formats

Every manifest digest is SHA-256 over this exact concatenation. All integers
are unsigned big endian; a signed FSID word is encoded as its 32-bit bit
pattern. Text labels below denote raw bytes and do not include quotation marks.

```text
u64 26
26 raw ASCII bytes: buildauthority-manifest/v1
u64 claim-domain-length
claim-domain raw ASCII bytes
u64 65
65-byte root record
u64 descendant-record-count
for each globally raw-relative-path-byte-sorted descendant:
    u64 descendant-record-length
    descendant record
```

| Claim | Exact domain |
| --- | --- |
| GOROOT | `goroot/v1` |
| Repository admission envelope | `repository/v1` |
| Source object content | `source-object-content/v1` |
| Selected source tree | `source-tree/v1` |
| GOMODCACHE metadata | `gomodcache/v1` |
| Selected modules | `selected-modules/v1` |

Kind codes are root `0`, directory `1`, regular `2`, and symlink `3`. The root
record is:

```text
u8 kind=0
u32 complete-mode
u32 uid
u32 gid
u64 size
32-byte ACL digest
u32 f_fsid.val[0]
u32 f_fsid.val[1]
u32 (f_flags & MNT_VISFLAGMASK)
```

Every descendant begins with `u64 path-length`, raw path bytes, `u8 kind`,
`u32 complete-mode`, `u32 uid`, `u32 gid`, `u64 size`, and the 32-byte ACL
digest. A directory ends there. A regular file appends its 32-byte content
SHA-256. A symlink appends `u64 link-text-length`, raw relative link text, and
the 32-byte target-record digest of its final protected regular-file target.
That target-record digest is SHA-256 over the target's `u64 record-length`
followed by its complete regular-file descendant record.
GOROOT is the only authority tree that admits symlinks. Chains are at most forty
links and each raw link text is at most 4 KiB; absolute, dangling,
directory-target, cyclic, escaping, or over-depth links refuse. The inspected
root is not charged against the descendant count.

`complete-mode` is zero-extended `st_mode`; a size is nonnegative before its
conversion to `u64`. Device, inode, modification, and change metadata remain
session identity evidence and are not serialized in a manifest.

The ACL digest is SHA-256 over the exact byte sequence below. Every integer is
unsigned big endian; the domain is seventeen raw ASCII bytes and quotation
marks are not encoded.

```text
u64 17
17 raw ASCII bytes: darwin-filesec/v1
u8 tag
```

Tag `0` ends immediately and represents only an absent or zero-length descriptor
attribute. Tag `1` appends:

```text
u32 filesec-magic
16 raw owner-GUID bytes
16 raw group-GUID bytes
u32 entry-count
u32 ACL-flags
for each ACE in source order:
    16 raw qualifier bytes
    u32 ACE-flags
    u32 ACE-rights
```

Tag `1` filesec magic is exactly `KAUTH_FILESEC_MAGIC` (`0x012cc16d`). Its
entry count is either `KAUTH_FILESEC_NOACL`, in which case no ACE follows, or
zero through 128, in which case exactly that many ACEs follow. Absent ACL
evidence, an explicit filesec no-ACL record, and an actual empty ACL therefore
have three distinct canonical encodings. ACL flags admit only
`KAUTH_ACL_DEFER_INHERIT` and
`KAUTH_ACL_NO_INHERIT`. ACE flags contain exactly one of `KAUTH_ACE_PERMIT` or
`KAUTH_ACE_DENY` plus only inherited, file-inherit, directory-inherit,
limit-inherit, and only-inherit. Rights admit vnode bits 1 through 13,
synchronize, and the four generic bits. Unknown bits refuse. Any permit
containing write-data, append-data, delete, delete-child, write-attributes,
write-extended-attributes, write-security, take-ownership, generic-write, or
generic-all refuses.

### Exact Mach-O grammar

The parser reads from the retained `ReaderAt` and exact file size, recovers any
panic as malformed input, and accepts at most 128 MiB. Go, every active
`pkg/tool/darwin_arm64` executable, and both outputs are little-endian thin
`MH_MAGIC_64`, `CPU_TYPE_ARM64`, subtype `CPU_SUBTYPE_ARM64_ALL` with zero
capability bits, and `MH_EXECUTE`. Git may use that thin form or big-endian
32-bit `FAT_MAGIC` with 1 through 32 unique architecture rows, exactly one
ARM64/ALL row, and no other ARM64 subtype. FAT64 and byte-swapped fat refuse.
Every slice has nonzero size, alignment exponent at most 30, aligned offset,
checked in-file extent, begins after the complete fat table, and does not
overlap another slice. Architecture rows are unique by raw CPU/subtype pair.
Every slice begins with little-endian `MH_MAGIC_64`; its inner CPU and raw
subtype equal its outer row. The selected ARM64 slice receives the complete
command validation below.

The ARM64 header has at most 65,536 commands and 16 MiB of command bytes. Every
command is at least eight bytes, is eight-byte aligned, stays within
`sizeofcmds`; the 32-byte header plus `sizeofcmds` stays within the slice, and
all command sizes sum exactly to `sizeofcmds`. The only
commands are `LC_SEGMENT_64`, `LC_SYMTAB`, `LC_DYSYMTAB`, `LC_LOAD_DYLIB`,
`LC_LOAD_WEAK_DYLIB`, `LC_REEXPORT_DYLIB`, `LC_LAZY_LOAD_DYLIB`,
`LC_LOAD_UPWARD_DYLIB`, `LC_LOAD_DYLINKER`, `LC_UUID`, `LC_CODE_SIGNATURE`,
`LC_DYLD_INFO_ONLY`, `LC_MAIN`, `LC_FUNCTION_STARTS`, `LC_DATA_IN_CODE`,
`LC_SOURCE_VERSION`, `LC_BUILD_VERSION`, `LC_DYLD_EXPORTS_TRIE`, and
`LC_DYLD_CHAINED_FIXUPS`. Every command-specific fixed structure, section or
tool count, offset, and file extent is checked with exact sizes; unknown,
legacy, and other required-dyld commands refuse.

The 64-bit Mach header is exactly 32 bytes and has reserved word zero. Git
slices have exactly `MH_NOUNDEFS|MH_DYLDLINK|MH_TWOLEVEL|MH_PIE`; Go, active
Go tools, and produced artifacts have exactly `MH_DYLDLINK|MH_PIE`. The
following command grammar uses checked unsigned arithmetic and interprets file
offsets relative to the start of the containing thin file or fat slice:

| Command family | Exact command size and extent policy |
| --- | --- |
| `LC_SEGMENT_64` | Exactly `72 + 80*nsects`; checked VM and file ranges are within their address spaces; nonzero segment file ranges do not overlap; each section VM range is inside its segment; section types other than `S_ZEROFILL=0x1`, `S_GB_ZEROFILL=0x0c`, and `S_THREAD_LOCAL_ZEROFILL=0x12` have file ranges inside both slice and segment; every `8*nreloc` relocation range is inside the slice |
| `LC_SYMTAB` | Exactly 24; `16*nsyms` symbol bytes and the declared string table are within the slice; every symbol string index is in range and reaches a NUL within that table |
| `LC_DYSYMTAB` | Exactly 80 and requires one `LC_SYMTAB`; local/external/undefined symbol ranges stay within its symbol count; table-of-contents uses 8-byte rows, 64-bit module table 56-byte rows, external references and indirect symbols 4-byte rows, and external/local relocations 8-byte rows, with every checked extent inside the slice |
| Five admitted dylib commands | At least 24 and exactly the fixed structure plus NUL-terminated path and zero padding to an eight-byte boundary |
| `LC_LOAD_DYLINKER` | At least 12 and exactly the fixed structure plus NUL-terminated path and zero padding to an eight-byte boundary |
| `LC_UUID` | Exactly 24 |
| `LC_CODE_SIGNATURE`, `LC_FUNCTION_STARTS`, `LC_DATA_IN_CODE`, `LC_DYLD_EXPORTS_TRIE`, `LC_DYLD_CHAINED_FIXUPS` | Exactly 16; `dataoff+datasize` is checked and at most the slice size; a zero `datasize` reads no byte and may retain any `dataoff` through the slice end |
| `LC_DYLD_INFO_ONLY` | Exactly 48; each rebase, bind, weak-bind, lazy-bind, and export offset/size extent is within the slice |
| `LC_MAIN` | Exactly 24; entry offset lies inside one nonempty file-backed segment whose initial protections include execute |
| `LC_SOURCE_VERSION` | Exactly 16 |
| `LC_BUILD_VERSION` | Exactly `24 + 8*ntools`; platform is `PLATFORM_MACOS=1`; `ntools` is at most 64; tool IDs are nonzero and unique; minimum OS, SDK, and tool versions use one 16-bit major plus 8-bit minor and patch fields; no trailing bytes remain |

There is exactly one `LC_MAIN` and one `LC_LOAD_DYLINKER`. `LC_SYMTAB`,
`LC_DYSYMTAB`, `LC_UUID`, `LC_CODE_SIGNATURE`, `LC_DYLD_INFO_ONLY`,
`LC_FUNCTION_STARTS`, `LC_DATA_IN_CODE`, `LC_SOURCE_VERSION`,
`LC_BUILD_VERSION`, `LC_DYLD_EXPORTS_TRIE`, and
`LC_DYLD_CHAINED_FIXUPS` occur at most once each. Segment and dylib commands
may repeat; segment names and direct dylib paths are unique. A zero-sized
segment-file or non-zerofill section-file extent, zero-count section-relocation
extent, empty SYMTAB or DYSYMTAB table, and empty DYLD_INFO stream uses offset
zero. The five `linkedit_data_command` variants instead use their checked
half-open slice-relative extent, so `sliceSize,0` is a valid empty cursor and
`sliceSize+1,0` refuses.

Each dylib command string starts exactly at byte 24; the dylinker string starts
exactly at byte 12. Each has one nonempty NUL-terminated path and only zero
padding afterward. Five dylib variants are accepted. Each path byte is printable
ASCII `0x21` through `0x7e`, slash is the only separator, and the path is
component-clean. A dylib absolute path is
beneath `/usr/lib/` or `/System/Library/`; relative paths, dot components,
`@rpath`, `@loader_path`, `@executable_path`, and prefix lookalikes refuse.
Exactly one `LC_LOAD_DYLINKER` names `/usr/lib/dyld`. `LC_RPATH`,
`LC_DYLD_ENVIRONMENT`, ID/prebound path commands, and nonzero string padding
refuse. This validates direct load names against the declared system trust base;
it does not recursively parse system libraries.

### Closed source local configuration

The local parser accepts at most 1 MiB and 4,096 records, ASCII-case-folds
section and variable names, retains subsection bytes, and rejects malformed
quoting, NUL or control bytes, includes, and duplicate normalized
section/subsection/variable triples. Every unlisted key refuses.

| Key or pattern | Cardinality and exact admitted value |
| --- | --- |
| `core.repositoryformatversion` | Required once; `0` for SHA-1 or `1` for SHA-256 |
| `core.bare` | Required once; canonical `false` |
| `core.filemode`, `core.logallrefupdates`, `core.ignorecase`, `core.precomposeunicode` | Optional once each; canonical `true` or `false`; never copied |
| `extensions.objectformat` | Absent for SHA-1; exactly `sha256` for SHA-256 |
| `extensions.refstorage` | Optional only for format 1; exactly `files`; reported ref format remains `files` |
| `extensions.worktreeconfig` | Optional canonical Boolean; whenever present, active `.git/config.worktree` is absent and sterile origin/scope output contains no worktree-scope row |
| `core.hookspath` | Optional bounded opaque value; never resolved, copied, logged, or used; every source command shadows it with `-c core.hooksPath=/dev/null` |
| `user.name`, `user.email`, `beads.role` | Optional once each; bounded printable opaque metadata; never copied or used |
| `remote.<name>.url` | Required once per declared remote; nonempty and control-free; command-style `<transport>::` URLs refuse |
| `remote.<name>.fetch` | Required once per declared remote; exactly `+refs/heads/*:refs/remotes/<same-name>/*` |
| `branch.<name>.remote` | Optional once; names one declared remote |
| `branch.<name>.merge` | Optional once; one full `refs/heads/*` name |
| `branch.<name>.vscode-merge-base` | Optional once; one bounded control-free remote-tracking name |

Presence of any `include*`, `alias.*`, `hook.*`, `filter.*`,
`promisor.*`, `protocol.*`, credential helper, executable diff/merge
driver, upload/receive-pack control, `url.*.insteadOf`,
`url.*.pushInsteadOf`, stored `core.useReplaceRefs`, object/worktree/ref
routing key, executable proxy/SSH/fsmonitor key, or
`remote.*.{promisor,partialclonefilter,uploadpack,receivepack,proxy,proxyAuthMethod}`
refuses even when its value is empty or false. Stored `core.hookspath` is the
only hook-related exception.

Every source-repository Git child uses `--no-pager` and these exact
command-scoped overrides in addition to the preallocation environment:

| Key | Exact value |
| --- | --- |
| `core.hooksPath` | `/dev/null` |
| `protocol.allow` | `never` |
| `core.useReplaceRefs` | `false` |
| `core.commitGraph`, `core.multiPackIndex`, `core.fsmonitor` | `false`, `false`, `false` |
| `pack.readReverseIndex`, `pack.useBitmaps` | `false`, `false` |
| `maintenance.auto`, `fetch.writeCommitGraph` | `false`, `false` |
| `gc.auto` | `0` |

Active-origin output contains only admitted local rows and these exact
module-authored command rows. System, global, and XDG sources remain sterile.

### Selective retained `.git` envelope

Repository admission recursively enumerates the complete retained real `.git`
directory no-follow within the repository entry, regular-byte, and per-file
bounds. Every row satisfies the effective-UID, mode, ACL, mount, kind, and
identity policy and enters exactly one class:

| Class | Exact closure |
| --- | --- |
| Authority and manifest | `.git`, required `.git/config`, optional `.git/packed-refs`, and the complete closed `.git/objects` subtree enter `repository/v1`; canonical loose objects and complete pack/index content pairs also enter `source-object-content/v1`; admitted object auxiliaries remain repository-only |
| Forbidden | `.git/config.worktree`, `.git/commondir`, `.git/gitdir`, `.git/shallow`, `.git/info/grafts`, either object alternate file, loose `.git/refs/replace` descendants, object promisor/temporary/unknown rows, any symlink or special entry, and any basename equal to `.lock` or ending in `.lock`; packed refs supply the corresponding replacement-ref check |
| Inert administration | Every remaining directory or regular file outside `.git/objects`; retain its raw name, kind, descriptor identity, presence, bounds, and security facts, but read and hash none of its content bytes and include it in neither manifest |

Revalidation repeats the complete classification and refusal scan. Every
authority row and both manifests remain unchanged, and the complete inert row
set retains identical name, kind, descriptor identity, presence, and security
claims. Addition, removal, replacement, or reclassification refuses;
same-inode inert byte changes are allowed because those bytes are never read or
consulted. No path outside Repository's single `.git` child is enumerated.

### Closed object-store auxiliary grammar

`H` below means exactly 40 lowercase hexadecimal bytes for SHA-1 or 64 for
SHA-256. Every auxiliary is a bounded, digested, no-follow protected regular
file that is omitted from the private copy.

| Location or form | Exact admitted closure |
| --- | --- |
| Directories | `info/`, `pack/`, canonical two-hex loose directories, optional `info/commit-graphs/`, and optional `pack/multi-pack-index.d/`; either optional directory may be empty |
| Commit graph | Either `info/commit-graph`, or `info/commit-graphs/commit-graph-chain` plus exactly its named `graph-H.graph` files |
| Dumb-transport pack list | `info/packs` containing only blank lines or `P pack-H.pack`; every named pack exists, but the list may be a stale subset because no admitted command consumes it |
| Per-pack companions | `pack/pack-H.rev`, `.bitmap`, `.keep`, or `.mtimes` only when the same H has one admitted complete `.pack` and `.idx` pair |
| Monolithic MIDX | `pack/multi-pack-index` with optional `multi-pack-index-H.rev` and `multi-pack-index-H.bitmap`; H equals the primary MIDX trailing checksum |
| Incremental MIDX | `pack/multi-pack-index.d/multi-pack-index-chain`, exact named `multi-pack-index-H.midx` layers, and optional same-H `.rev` or `.bitmap` files; the chain and layers have exact closure with no orphan |

`info/alternates`, `info/http-alternates`, every `.promisor`, every
temporary or lock file including `objects/maintenance.lock`, an orphan
auxiliary, and every other object-root, `info`, or `pack` name refuse.

### Source administrative-state boundary

The source repository is an admission envelope, not a source checkout. Its
administrative state has this closed handling:

| Class | Exact handling |
| --- | --- |
| Captured authority | Retain the Repository root bundle, real in-tree `.git` directory, directly parsed `.git/config`, exact physical Git/common/object paths, admitted format claims, and the closed object-content and auxiliary inventories |
| Source Git allowance | Run only the source-path, source-format, source-local-config, and source-active-config transcript rows below; these commands are metadata-only and receive the fixed source overrides |
| Presence-refused state | Gitfile or linked worktree, external common/object path, `config.worktree`, include, alternate, HTTP alternate, graft, shallow marker, loose or packed `refs/replace` name, promisor/partial-clone state, a basename matching the exact `.lock` predicate anywhere in captured `.git`, and every unrecognized object-store name including temporary forms |
| Inert administrative state | `HEAD`, ordinary loose refs, packed-ref object IDs after the bounded name scan below, reflogs, index, hooks, non-lock operation-state files, and filter-repo records are not resolved, copied, or used by an admitted source command to select or dereference content |
| Caller worktree | Tracked, modified, and untracked pathname state outside the captured administrative envelope is neither inspected nor consumed; only the exact committed object graph enters the private checkout |
| Object consumers | Revision resolution, timestamp corroboration, fsck, object presence, tree, attributes, raw blobs, projections, checkout, HEAD, status, and ref inventory run only in the private repository after its semantic preflight |

Ordinary ref storage may therefore be inert while replacement names remain a
presence refusal. The custom administrative inventory detects a loose
`refs/replace` path and structurally scans packed-ref names without resolving
their object IDs.

Within the captured `.git` administrative envelope, any file or directory
basename equal to `.lock` or ending in `.lock` is lock state and refuses. The
working tree outside that envelope is ignored; object-store temporary names are
already refused as unrecognized by the closed object grammar.

The optional `packed-refs` file is a protected no-follow regular file bounded
to 16 MiB and 1,000,000 LF-terminated rows. It contains at most one leading
`# pack-refs with: ` header whose unique traits occur only in `peeled`,
`fully-peeled`, `sorted` order and are each followed by one space, then records
whose ref names are raw-byte strictly increasing, each with one full lowercase
object ID, one space, and one valid full `refs/` name. An
immediately following peeled row is caret plus one full lowercase object ID.
Blank lines, CR, NUL, unknown headers/traits, detached peeled rows, duplicate
names, malformed IDs, or invalid ref names refuse. A valid ref name has no
empty, dot, or dot-dot component; no component begins with dot or ends in dot
or `.lock`; and the name contains no `..`, control, space, tilde, caret, colon,
question mark, asterisk, open bracket, backslash, consecutive slash, or `@{`.
Any scanned name equal to or beneath `refs/replace` refuses; every other record
is inert.

### Exact object, commit, tree, and path grammar

One physical representation is one canonical loose-object file or one indexed
pack entry. Duplicate object IDs are distinct physical representations and
consume every representation, inflated-byte, and resolved-byte charge
independently. Canonical object bytes are the ASCII object type, one space, the
minimal unsigned decimal body length, one NUL, and the exact body. The object
ID is the admitted repository hash over those canonical bytes. A base object
has type `commit`, `tree`, `blob`, or `tag`; a delta inherits its completely
resolved base type. Reserved packed types refuse.

Inflated representation bytes are the complete loose header and body or the
complete packed base/delta payload after zlib. Resolved bytes are the final body
length and are charged once for every physical representation even when a
cached result avoids repeated computation. A delta copy or insert opcode counts
as one instruction; header varints do not. Cumulative ancestry bytes are the
checked sum of resolved body lengths from the ultimate base through the
requested result, inclusive. A zero opcode, missing base, cycle, overflow, or
one-over charge refuses. Representations sharing an object ID have identical
resolved type and body length and identical SHA-256 over canonical object
bytes; this secondary claim detects a same-ID disagreement independently of
the repository hash.

Full inflation and delta resolution use only the cleanup-owned object spool.
Spool files are exclusively created effective-UID-owned `0600` regular files
opened read/write; symlinks, hard-linked aliases, directories below the spool,
and special files refuse. After exact write, sync, identity, size, link, and
content proof, the same retained descriptor is sealed as a bounded `ReaderAt`;
no semantic consumer reopens the pathname. Removal holds that descriptor
through unlink and requires stable identity/content metadata, link count exactly
one then zero, and claimed-name absence. The sum of live logical file sizes
never exceeds 2 GiB, and the spool contains at most 4,096 files. The spool is
empty after source semantic preflight, after private-store semantic preflight,
and immediately before every Git operation that can dereference an object.

Every resolved commit or tree representation is at most 16 MiB before `fsck`.
The selected commit body has exactly one
`tree <full-id>`, exactly one `author`, exactly one `committer`, zero or more
`parent <full-id>` rows, and one blank separator. Other nonempty header names
contain only lowercase ASCII letters, digits, and hyphens. A continuation
begins with one space and follows an existing header. NUL, a continuation
without a header, malformed full ID, missing separator, a duplicate `tree`,
`author`, or `committer` singleton, or malformed final committer
`seconds +/-HHMM` refuses. Seconds are canonical
signed decimal; offset hours are 00 through 23 and minutes 00 through 59. The
message following the separator is opaque bounded bytes. The parsed committer
instant and private Git transcript agree before conversion to UTC RFC3339.

A tree body is a complete repetition of ASCII mode, one space, nonempty raw
entry name, one NUL, and one raw repository-width object ID. On-wire mode
`40000` maps to semantic `040000`; the other exact on-wire modes are `100644`,
`100755`, and `120000`. Tree mode names a tree object and every other admitted
mode names a blob. Names contain neither NUL nor slash and are not `.` or `..`.
Entries are strictly ordered by Git's raw-byte tree comparator, treating a tree
name as followed by `/` and another name as followed by NUL at the first length
difference. Duplicate names, invalid target types, trailing bytes, cycles, and
an empty selected tree refuse.

Expansion charges every directory reference and leaf occurrence, including a
repeated tree or blob ID. Each materialized component is valid UTF-8. Its
collision key is NFC, then full case fold, then NFC using the pinned
`golang.org/x/text` v0.41.0 tables. Raw duplicates, equal collision keys in one
directory, file/directory prefix conflicts, and a collision key equal to
`.git` refuse. The expanded leaf inventory retains raw relative paths, modes,
object IDs, sizes, and symlink text and is the custom value compared with the
NUL-framed Git tree projection.

### Closed attribute-state table

`check-attr --all` omission means `unspecified`; returned values are the exact
NUL-framed Git states below. Duplicate path/name triplets, a path outside the
submitted exact inventory, malformed framing, or output beyond the structured
limit refuses.

| Attribute | Admitted states |
| --- | --- |
| `filter`, `ident`, `working-tree-encoding`, `crlf` | `unspecified` or explicit `unset` only |
| `eol` | `unspecified`, explicit `unset`, or string value `lf` |
| `text` | `unspecified`, explicit `unset`, explicit `set`, or string value `auto` |
| Every other valid attribute name | Any bounded Git `set`, `unset`, or string value; inert for checkout conversion and unable to relax a row above |

The committed `.gitattributes` sources are regular selected-tree blobs within
the per-file and aggregate attribute bounds. The private global attributes file
is empty and private `.git/info/attributes` is absent. Regardless of admitted
states, every per-path filtered projection remains byte-for-byte, size, and
SHA-256 equal to the raw blob.

GOROOT may contain only protected regular files/directories and contained
relative symlinks whose raw link text and final protected regular target record
enter its manifest; target file identity remains session-only evidence.
GOMODCACHE may contain only protected regular files/directories.
Only the closed object-store auxiliaries above may be recognized and omitted;
routing, promisor, lock, orphan, or unknown metadata refuses.

## Normative workspace and environment

The workspace constructor accepts only the single-use
`capturedNoScratchInputs` move cell. Pointer aliases share one
pending/moved/closed state: the first winning pending alias atomically transfers
the retained set once, while every losing concurrent, repeated, moved, or
closed alias refuses before touching a handle. It then makes at most 100
allocation attempts through the retained StateRoot:

| Allocation boundary | Exact policy |
| --- | --- |
| Candidate | Read exactly 12 fresh cryptographic bytes and encode 24 lowercase hexadecimal digits after the fixed `.sandbox-gc-build-` prefix |
| Context | Check cancellation or deadline after candidate framing and before `mkdirat` |
| Mutation | Perform exactly one exclusive mode-`0700` `mkdirat` for the candidate with no pathname precheck |
| Collision | `EEXIST` consumes that candidate, creates no allocation or Recovery claim, and retries with 12 new bytes |
| Exhaustion | 100 collisions refuse as `workspace/open/limit` with no allocation or Recovery |
| Allocation | Successful `mkdirat` immediately records the generated child name plus retained StateRoot identity as an unobserved task before any fallible retain or identity capture |

Every other entropy, adapter, context, and `mkdirat` outcome uses the closed
allocation failure table. Successful `mkdirat` is the allocation boundary even
when task-root retention fails; the shared cleanup transaction then preserves
the child with `unknown` disposition and Recovery lacking a task-root identity.

Before Git initializes private metadata or Go creates bounded intermediates,
the task root contains exactly these initial objects:

| Initial path | Exact initial object |
| --- | --- |
| `tmp/`, `gotmp/`, `gopath/`, `gocache/`, `git-template/`, `git-exec/`, `git-hooks/`, `object-spool/`, `checkout/`, `outputs/` | Exclusively created effective-UID-owned empty directory with exact `0700` and no special bits |
| `git-path/` | Exclusively created effective-UID-owned directory with exact `0700`, containing only `git` |
| `git-global.conf`, `git-attributes`, `git-excludes` | Exclusively created effective-UID-owned empty regular file with exact `0600` |
| `git-path/git` | Sole policy symlink; its raw absolute target and resolved identity bind the retained Git executable |

Git may create bounded private metadata and checkout state only in `checkout`.
Go may create derived intermediate state only in `gotmp` and `gocache`.
Verified fixed-name artifacts exist only in `outputs`. At every child boundary,
`tmp`, `gopath`, `git-template`, `git-exec`, `git-hooks`, `git-path`, and the
three policy files remain in their declared initial state; any drift refuses
through the same post-child derived-tree check. No private home directory
exists.

### Ownership-ledger policy

| Boundary | Exact policy |
| --- | --- |
| Creation | Immediately after any successful module creation syscall, record its exact relative path and expected kind/policy as unobserved; retained no-follow identity capture marks the row observed |
| Derived capture | After every proved child quiescence, a bounded no-follow scan records every admitted private/derived descendant before the next child or cleanup; failed, incomplete, unexpected, or unobserved rows make the task non-removable |
| Rename, replacement, or unlink | Validate every source and destination claim, perform the one mutating syscall without retry, re-prove source absence and destination or retained-open-inode identity, then atomically retire/install active rows; ambiguity makes the ledger non-removable |
| Spool retirement | Retire a completed spool row only after its retained descriptor proves link count one-to-zero and claimed-name absence |
| Freeze | After quiescence and before the first cleanup unlink, freeze the complete ledger only when no transient, retired, or unobserved row remains and every spool generation is complete |
| Full preflight | Before deleting anything, require exact path membership and every kind, identity, owner, mode, ACL, mount, link, and named-task binding for the entire frozen ledger; any mismatch deletes nothing and preserves the task |
| Removal | After full preflight, remove observed claims through retained roots in reverse ownership order and re-prove each identity at unlink; later drift stops removal, preserves the remainder, and forbids `removed` even though Darwin cannot roll back earlier proved unlinks |
| Incomplete spool | Any active or incomplete spool generation, including one with no completed file claim, blocks workspace removal |

### Object-spool policy

| Boundary | Exact policy |
| --- | --- |
| Creation | Every spool file is created exclusively and no-follow relative to the retained object-spool root |
| Retained capability | Open read/write; after exact write, sync, size, identity, link-count-one, and content proof, seal the same descriptor as a bounded `ReaderAt`; semantic consumers never reopen its pathname |
| Contents | Only effective-UID-owned regular files with exact `0600` and link count one; directories, symlinks, hard-linked aliases, and special entries refuse |
| Capacity | At most 4,096 files and 2 GiB maximum simultaneous logical bytes |
| Removal | Hold the claimed descriptor through unlink; require stable identity/content metadata, exact link-count transition from one to zero, and final claimed-name absence before close |
| Darwin race limit | A replacement raced after the last name check may be unlinked; the retained original then fails the one-to-zero proof, the task is preserved, success is forbidden, and the replacement is not claimed untouched |
| Generations | Each semantic object-resolution generation is removed under the same retained-capability proof and the directory is proved empty after use |
| Git gate | The directory is proved empty before every Git child that may dereference an object |
| Failure | Any uncertain type, ownership, mode, capacity, removal, or emptiness result refuses through the shared post-allocation cleanup transaction |

Every module-owned direct child receives one of exactly two sealed 39-row
environments constructed from an empty set, never from `os.Environ`. Its
`Cmd.Env` is nonnil, so `os/exec` cannot synthesize `PWD`. Every module-owned
no-input command binds the retained null-device descriptor as stdin; none
leaves `Cmd.Stdin` nil or reopens `/dev/null` by pathname. Path-bearing rows are
exactly:

| Key group | Preallocation profile | Task-private profile |
| --- | --- | --- |
| `HOME`, `XDG_CONFIG_HOME` | explicit empty-string entry for each key | explicit empty-string entry for each key |
| `TMPDIR`, `TMP`, `TEMP` | `/dev/null` | private `tmp` |
| `GOTMPDIR` | `/dev/null` | private `gotmp` |
| `GOPATH` | `/dev/null` | private `gopath` |
| `GOCACHE` | `off` | private `gocache` |
| `PATH` | `/dev/null` | private `git-path` only |
| `GIT_CONFIG_GLOBAL` | `/dev/null` | private empty `git-global.conf` |
| `GIT_EXEC_PATH` | `/dev/null` | private empty `git-exec` |
| `GIT_TEMPLATE_DIR` | `/dev/null` | private empty `git-template` |
| `GOROOT`, `GOMODCACHE` | retained admitted authorities | retained admitted authorities |

Both direct profiles contain exactly these additional rows:

| Key group | Exact value |
| --- | --- |
| `GOENV` | `off` |
| `GOFLAGS` | `-mod=readonly` |
| `GOWORK`, `GOTOOLCHAIN` | `off`, `local` |
| `GOPROXY`, `GOSUMDB`, `GOVCS` | `off`, `off`, `*:off` |
| `GO111MODULE` | `on` |
| `GOEXPERIMENT`, `GOFIPS140` | `none`, `off` |
| `GO_EXTLINK_ENABLED`, `CGO_ENABLED` | `0`, `0` |
| `GOOS`, `GOARCH`, `GOARM64` | `darwin`, `arm64`, `v8.0` |
| `GIT_CONFIG_NOSYSTEM`, `GIT_ATTR_NOSYSTEM` | `1`, `1` |
| `GIT_NO_LAZY_FETCH`, `GIT_NO_REPLACE_OBJECTS` | `1`, `1` |
| `GIT_TERMINAL_PROMPT`, `GIT_OPTIONAL_LOCKS` | `0`, `0` |
| `GIT_PROTOCOL_FROM_USER` | `0` |
| `LANG`, `LC_ALL`, `TZ` | `C`, `C`, `UTC` |

After exact dynamic values are inserted, the constructor rejects duplicate
keys, renders one complete `KEY=value` assignment per key, and applies Go
`sort.Strings`. The resulting raw-byte order is exactly:

```text
CGO_ENABLED GIT_ATTR_NOSYSTEM GIT_CONFIG_GLOBAL GIT_CONFIG_NOSYSTEM
GIT_EXEC_PATH GIT_NO_LAZY_FETCH GIT_NO_REPLACE_OBJECTS GIT_OPTIONAL_LOCKS
GIT_PROTOCOL_FROM_USER GIT_TEMPLATE_DIR GIT_TERMINAL_PROMPT GO111MODULE
GOARCH GOARM64 GOCACHE GOENV GOEXPERIMENT GOFIPS140 GOFLAGS GOMODCACHE GOOS
GOPATH GOPROXY GOROOT GOSUMDB GOTOOLCHAIN GOTMPDIR GOVCS GOWORK
GO_EXTLINK_ENABLED HOME LANG LC_ALL PATH TEMP TMP TMPDIR TZ XDG_CONFIG_HOME
```

The preallocation direct profile contains no task path and is used only for
`go version`, the keyed `go env` projection, the direct retained compiler
`-V=full` probe, Git version/build-options, Git builtin inventory, and the four
source-metadata rows. Every later module-owned direct child uses the
task-private direct profile. Go, Git, and the compiler probe are invoked only
through their applicable nominal retained absolute authority.

Dynamic-loader variables, compiler overrides, Git object/config/index routing,
`GIT_ATTR_SOURCE`, default hash/ref variables, `GIT_TEST_*`, workspace or
modfile selection, proxies, private-module routing, coverage, caller caches,
askpass/SSH controls, `GOTELEMETRY`, `GOTELEMETRYDIR`,
`TEST_TELEMETRY_DIR`, `GO_TELEMETRY_CHILD`, `GO_TELEMETRY_CHILD_UPLOAD`, and
every unlisted key are absent. `GOTELEMETRY` and `GOTELEMETRYDIR` are computed
Go report values, not accepted process controls. The filtered projection
command uses the exact argv-scoped global option
`--attr-source=<revision>` rather than adding `GIT_ATTR_SOURCE` to the process
environment.

Pinned cmd/go derives its descendant base E from the task-private vector. It
replaces `GOENV=off` in place with `GOENV=`; appends in this exact order
`GOAUTH=netrc`, `GOHOSTARCH=arm64`, `GOHOSTOS=darwin`, `GOTELEMETRY=off`,
`GOTOOLDIR=<GOROOT>/pkg/tool/darwin_arm64`, `GOVERSION=go1.27.1`,
`GCCGO=gccgo`, `AR=ar`, `CC=cc`, and `CXX=c++`; and then appends
`GCM_INTERACTIVE=never`. Its conditional `GIT_TERMINAL_PROMPT=0` is already
present. E is therefore exactly 50 ordered rows. Only `vcs.run1`,
`codehost.run`, `Builder.toolID`, and `Shell.runOut` are reachable under the
fixed authenticated schedule; every derived projection is:

| Go-owned child family | Exact directory and environment |
| --- | --- |
| Git descendants | Actual cwd is the physical private checkout; E followed by `PWD=<physical-private-checkout>`; exactly 51 rows |
| Compile, asm, and link `Builder.toolID` `-V=full` probes | Empty `Cmd.Dir`, actual cwd the physical private checkout; E only; exactly 50 rows and no `PWD` |
| Operational compile | Cwd and `PWD` are the physical private checkout; E followed by that `PWD`, then `TOOLEXEC_IMPORTPATH=<authenticated-package-description>`; exactly 52 rows |
| Operational assembler | Cwd and `PWD` are the exact authenticated GOROOT, GOMODCACHE, or private-source package directory; E followed by that `PWD`, then the package's `TOOLEXEC_IMPORTPATH`; exactly 52 rows |
| Normal operational link | Empty `Cmd.Dir`, actual cwd the physical private checkout; E followed by the exact main/action `TOOLEXEC_IMPORTPATH`, then call-site `GOROOT=<GOROOT>`; final de-duplication removes E's earlier GOROOT and retains those final two rows in that order; exactly 51 rows and no `PWD` |

`work.makeCfgChangedEnv` is empty for exact Darwin/arm64 and `GOARM64=v8.0`;
compile and assembler add no other call-site row. Each of the twelve role
`list -deps -json` calls and two role builds emits seven Git children and three
tool-ID probes: exactly 98 Git descendants and 42 tool-ID probes. The twelve
role lists emit no operational child. Each build additionally emits its
authenticated graph- and cache-derived operational compile, assembler, and
link multiset in the action graph's causal partial order. The build argv omits
`-p`, the environment omits `GOMAXPROCS`, and pinned Go derives worker
parallelism and compiler `-c=N` from host `runtime.GOMAXPROCS(0)` and concurrent
ready compile actions. Every child argv, directory, and environment is exact,
but independent-ready children may interleave; no host-independent total
chronology is required. The six module-list and two `mod verify` calls emit none
of the fixed seven-plus-three bundles.

These pinned-Go-owned children are the one version-specific stdin exception:
their `Cmd.Stdin` is nil and authenticated Go reopens `/dev/null` by pathname.
Physical root and null-device claims are revalidated around each enclosing
top-level Go call. The exception never authorizes a module-owned nil stdin.

## Normative resource and time limits

| Authority or operation | Exact limit |
| --- | --- |
| Executable | 128 MiB |
| GOROOT | 65,536 descendants, root excluded; 2 GiB aggregate regular bytes; 128 MiB/regular file; 4 KiB/link text; 40 links/resolution chain |
| GOMODCACHE | 500,000 descendants, root excluded; 16 GiB aggregate regular bytes; 512 MiB/regular file |
| Source local config | 1 MiB; 4,096 normalized records |
| Packed refs | 16 MiB; 1,000,000 LF-terminated rows |
| Repository metadata/object files | 1,000,000 descendants, inspected roots excluded; 64 GiB aggregate regular bytes; 8 GiB/pack |
| Physical Git representations | 1,000,000 loose-plus-pack entries, duplicates included |
| Git representation inflation | 512 MiB/representation; 16 GiB aggregate inflated bytes charged per physical representation |
| Git resolved objects | 512 MiB/resolved object; separate 16 GiB aggregate resolved bytes charged per physical representation |
| Git deltas | 1,000,000 instructions/delta; 16,000,000 total; depth 64; 1 GiB cumulative logical ancestry |
| Object spool | 4,096 exclusive `0600` regular files; 2 GiB maximum simultaneous logical bytes; empty before every Git object dereference |
| Commit/tree metadata | 16 MiB/object; tree depth 256; component 255 bytes; relative path 1,023 bytes |
| Materialized source tree | 250,000 implied directories plus leaves; 8 GiB summed by repeated path references; 512 MiB/file; 4 KiB/symlink |
| Attribute sources | 1 MiB/file; 16 MiB aggregate |
| Private `GOTMPDIR` plus `GOCACHE` | Initially empty; 250,000 directories plus regular files; 8 GiB aggregate; 512 MiB/file; no symlink/special entry; charged after every Go child |
| One module ZIP | 100,000 entries; 500 MiB compressed and 500 MiB uncompressed; Store or Deflate only |
| All selected module ZIPs | 4,096 ZIPs; 500,000 entries; 64 MiB central-directory path bytes; 16 GiB uncompressed |
| Structured Git/Go stdout | 256 MiB/child; raw and filtered blob streams are separately charged to source-tree limits |
| Private diagnostics | 1 MiB/child plus total byte count, truncation state, and SHA-256 |
| Mach-O container | 128 MiB; 32 fat rows; 65,536 ARM64 load commands; 16 MiB ARM64 command bytes |
| Process-group inventory | 4,096 unique positive PIDs/snapshot |
| Supervisor poll request | No programmed wait longer than 5 milliseconds between waitid/notification samples; OS scheduling latency is not a claimed wall-clock bound |
| Consecutive `waitid` EINTR after a latched stop | Exactly one retry after one positive channel-free five-millisecond wait; the second consecutive interruption before any valid result uses the no-signal/no-probe background handoff |
| Post-`SIGKILL` terminal-observation window | 5 seconds |
| Survivor follow-up | At most 1,000 snapshots before a monotonic deadline 5 seconds after the survivor-signal attempt; no programmed wait longer than 5 milliseconds |
| Child output-drain wait | 1 second |
| Preallocation probe | 30 seconds |
| Object inspection, init/copy, fsck, checkout, list, verify | 5 minutes/phase |
| Role build | 20 minutes/role |
| Whole transaction | 1 hour |

Size and count limits are admission ceilings, not completion guarantees. A
phase or transaction deadline still refuses an otherwise within-bound workload.
The private derived-tree limit is a phase-boundary admission gate rather than a
continuous filesystem quota.

At entry, `noScratch.capture` samples the one-hour transaction deadline from
the same injected monotonic clock used by the process supervisor and retains
that opaque transaction window through every later validated state.
Immediately before the first precheck for each non-source command,
`nonSourceBlock` samples that clock again and mints a private command-window
token binding the exact caller context, including cancellation without a
deadline, the retained transaction window, and the new phase sample. After
precheck success, only the named materializer may consume that token; it places
the exact context and both derived deadlines in `processRequest` and cannot
accept raw times or a replacement context. The effective command bound is the
earliest of the caller-context deadline, retained transaction deadline, and
phase sample plus 30 seconds. Caller context may shorten either module deadline
but never extend it, and the supervisor observes time through that same clock.

For every selected external module, cache paths use the pinned Go module-cache
case-escaping format, while ZIP entries begin with the canonical unescaped
`<module>@<version>/` prefix. A bounded EOCD/ZIP64 pass validates counts,
offsets, central-directory path bytes, integer overflow, encryption, methods,
and declared sizes before any operation allocates proportional entry storage.
The semantic pass then enforces clean paths, Unicode case-fold and
file/directory collisions, root `go.mod`, module-file sizes, and total content
bounds. Extracted-tree and ZIP `h1:` hashing readers check context cancellation
and never reopen an unretained caller path.

## Normative Go discovery and verification transaction

Each discovery pass invokes the retained Go executable from the private checkout
with the task-private environment and these exact argv vectors in order:

| Order | Projection | Exact argv after the executable |
| ---: | --- | --- |
| 1 | Selected module graph | `list -m -json all` |
| 2 | AGM package graph | `list -deps -json github.com/vbonnet/dear-agent/agm/cmd/agm` |
| 3 | Disk-watchdog package graph | `list -deps -json github.com/vbonnet/dear-agent/cmd/disk-watchdog` |

Each stdout is a complete concatenated JSON-object stream with no trailing
nonspace bytes and success has empty diagnostic output. Module paths are unique,
there is exactly one main module whose directory is the private checkout, and no
module has a replacement. Package import paths are unique within each role
stream, the requested role package occurs exactly once, and no package reports
`Incomplete`, `Error`, or `DepsErrors`. Every reported directory and input field
is classified beneath an authenticated authority before admission.

The exact transaction schedule is:

| Order | Go operation or boundary |
| ---: | --- |
| 1 | Initial discovery pass and authentication of the resulting graphs and selected-module manifests |
| 2 | `mod verify`, with exact stdout `all modules verified` plus one LF and empty diagnostic output |
| 3 | Discovery pass immediately before the AGM build; compare graphs and manifests to the authenticated claims |
| 4 | AGM build and output verification |
| 5 | Discovery pass immediately after the AGM build; compare graphs and manifests |
| 6 | Discovery pass immediately before the disk-watchdog build; compare graphs and manifests |
| 7 | Disk-watchdog build and output verification |
| 8 | Discovery pass immediately after the disk-watchdog build; compare graphs and manifests |
| 9 | `mod verify`, with the same exact successful output |
| 10 | Final discovery pass immediately before return; compare graphs and manifests |

The six discovery passes contain six module-list calls and twelve role-list
calls. Each role-list call emits its exact seven-row Go-owned Git sequence plus
one compile, assembler, and linker tool-ID probe and no operational
`Shell.runOut` child. Each of the two builds emits that same seven-plus-three
bundle and its role-specific authenticated action-graph-derived operational
multiset. Consequently the full schedule has exactly 98 Go-owned Git children
and 42 tool-ID probes. Module-list and `mod verify` calls emit none of those
bundles. The fixed recorder validates every argv, cwd, environment, and causal
edge; the wrapper-free full-schedule trace validates counts, both operational
multisets and partial orders, and absence of a fifth cmd/go constructor.

## Normative private-config table

The private `.git/config` is an effective-UID-owned `0600` regular file with LF
line endings and exactly one final LF. Keys and sections appear exactly in the
order below; indentation is one tab and Boolean text is lowercase. The SHA-1
form uses `repositoryformatversion = 0` and omits the entire `extensions`
section. The SHA-256 form uses `repositoryformatversion = 1` and includes the
shown section.

```text
[core]
	repositoryformatversion = <0-or-1>
	filemode = <true-or-false>
	bare = false
	logallrefupdates = false
	symlinks = <true-or-false>
	ignorecase = <true-or-false>
	precomposeunicode = <true-or-false>
	hooksPath = "<quoted-task-path>/git-hooks"
	attributesFile = "<quoted-task-path>/git-attributes"
	excludesFile = "<quoted-task-path>/git-excludes"
	autocrlf = false
	eol = lf
	fsmonitor = false
	commitGraph = false
	multiPackIndex = false
	useReplaceRefs = false
[extensions]
	objectFormat = sha256
[checkout]
	workers = 1
[maintenance]
	auto = false
[gc]
	auto = 0
[pack]
	readReverseIndex = false
	useBitmaps = false
[fetch]
	writeCommitGraph = false
[protocol]
	allow = never
[commit]
	gpgSign = false
[tag]
	gpgSign = false
[log]
	showSignature = false
[merge]
	verifySignatures = false
[submodule]
	recurse = false
```

`filemode`, `symlinks`, `ignorecase`, and `precomposeunicode` are the four
validated private-init Boolean probe results. No source value supplies them.
The quoted-path encoder maps backslash, double quote, LF, tab, and backspace to
`\\`, `\"`, `\n`, `\t`, and `\b`; it refuses every other C0 byte and DEL.
Rendering uses an exclusive sibling temporary file, closes it successfully,
renames it through the retained checkout root, reopens the destination
no-follow, and requires both exact bytes and the exact semantic vector above.
No other section, key, duplicate, comment, blank line, or trailing byte remains.

## Normative Git transaction

| Phase | Exact behavior |
| --- | --- |
| No-scratch admission | Execute only the exact eight-step capture transaction: retain nominal/root/null/tree/source capabilities; run five non-source plans from physical `/`; finish selective source classification and all four named manifests; run four source plans from Repository; revalidate; then seal the move-once aggregate. No task child exists and write-denied evidence admits only device writes to `/dev/null` |
| Workspace allocation | Consume only the pending complete no-scratch aggregate and use the exact 100-attempt exclusive allocator; successful task `mkdirat` is allocation and every later refusal uses the shared cleanup transaction even when task-root identity was not observed |
| Source semantic preflight | Without invoking Git, inflate and resolve every source physical representation through the spool, prove same-ID equivalence, decode the exact selected commit/tree/path grammar, derive the custom timestamp claim, and empty the spool |
| Init | Invoke private `init --quiet` with explicit empty template, admitted object format, files ref format, fixed branch, and checkout path; query the four private-init filesystem Booleans before replacing config |
| Copy | Descriptor-bound exclusive copies of every canonical loose object and complete pack/index pair; equal bytes/digests, different identities, source/destination content-manifest equality around copy; no refs, source config, hooks, logs, index, shallow state, routing metadata, or auxiliaries |
| Private semantic preflight | Repeat complete representation resolution, same-ID equivalence, and selected commit/tree validation against only the private store; compare its claims with the source preflight and empty the spool |
| Private configuration | Replace config with the byte-exact private-config table, reparse it, and retain only the four private-init Booleans; SHA-1 omits `extensions`, SHA-256 contains only `extensions.objectFormat=sha256`, and both omit `extensions.refStorage` |
| Revision and time | Against only the preflighted private store, require `<revision>^{commit}` equality and exact agreement between the custom committer seconds/offset and the fixed `log` transcript before deriving UTC RFC3339 |
| Integrity | Run the exact child-free fsck transcript followed by lazy-fetch-disabled exact commit-presence proof |
| Tree | Require the complete `ls-tree -rlz --full-tree -r` NUL projection to equal the custom expanded leaf inventory, including normalized six-digit modes and sizes |
| Attributes | Admit bounded regular committed attribute blobs; parse every exact-path NUL triplet through the closed state table; the private global attributes file is empty and private `.git/info/attributes` is absent |
| Raw and projected blobs | Stream unique exact blob IDs through bounded `cat-file --batch -Z`; stream each exact path through standalone `--attr-source` filtered projection; require raw equality by size and SHA-256, charged across repeated paths |
| Checkout | Check out the detached exact commit with command-scoped `/dev/null` hooks layered over the private config's empty hook directory, no filter driver, and one worker; post-walk every tracked entry no-follow and require raw Git object hash and exact mode equality |
| Final Git state | Require exact detached HEAD, empty exact porcelain status, and empty exact ref inventory after checkout and after each role build |

Every module-invoked Git command uses the retained absolute executable and
task-private environment. Its global prefix is exactly `--no-pager`,
`-C <physical-work-dir>`,
then these `-c` assignments in order:

```text
core.hooksPath=/dev/null
protocol.allow=never
core.useReplaceRefs=false
core.commitGraph=false
core.multiPackIndex=false
core.fsmonitor=false
pack.readReverseIndex=false
pack.useBitmaps=false
maintenance.auto=false
fetch.writeCommitGraph=false
gc.auto=0
```

The physical work directory is Repository for the four source rows, task root
for init, and checkout for every other private row. Within that envelope, the
exact transcript forms are:

| Purpose | Exact subcommand and output contract |
| --- | --- |
| Source paths | `rev-parse --path-format=absolute --show-toplevel --absolute-git-dir --git-common-dir --git-path objects`; four exact LF-terminated physical paths matching Repository, `.git`, `.git`, and `.git/objects` |
| Source formats | `rev-parse --show-object-format=storage --show-ref-format`; exactly `sha1` or `sha256`, LF, then `files`, LF |
| Source local config | `config --null --show-origin --show-scope --local --list`; one `local NUL file:.git/config NUL key LF value NUL` record per directly parsed row, in source order |
| Source active config | `config --null --show-origin --show-scope --list`; the same local records followed by one `command NUL command line: NUL key LF value NUL` record per fixed `-c` assignment in prefix order, with no system, global, XDG, worktree, or other origin |
| Init | `init --quiet --template=<private-empty-git-template> --object-format=<sha1-or-sha256> --ref-format=files --initial-branch=authority-staging <checkout>`; empty stdout |
| Init Boolean | `config --type=bool --default=<default> --get <key>` separately with `core.filemode=true`, `core.symlinks=true`, `core.ignorecase=false`, and `core.precomposeunicode=false` as key/default pairs; exactly `true` or `false` plus LF |
| Revision | `rev-parse --verify --end-of-options <revision>^{commit}`; exactly the lowercase revision plus LF |
| Commit time | `log -1 --no-show-signature --format=%ct%x00%cI%x00 <revision> --`; exactly decimal seconds, NUL, strict ISO committer time, NUL, LF, both equal the custom commit parse |
| Integrity | `fsck --no-references --no-reflogs --full --strict --no-dangling --no-progress <revision>`; empty stdout |
| Commit presence | `cat-file -e <revision>^{commit}`; empty stdout |
| Tree | `ls-tree -rlz --full-tree -r <revision>`; each leaf is six ASCII mode digits, space, `blob`, space, lowercase full OID, space, right-aligned unsigned decimal size in a minimum seven-column ASCII-space-padded field, tab, raw relative path, NUL |
| Attributes | `check-attr --source=<revision> --all --stdin -z`; each input is raw relative path plus NUL and each output is exact input path, NUL, attribute name, NUL, Git state/value, NUL |
| Raw blobs | `cat-file --batch -Z`; each input is one unique lowercase full blob OID plus NUL and each response is OID, space, literal `blob`, space, minimal unsigned decimal size, NUL, exact body, NUL |
| Projection | global `--attr-source=<revision>` before the subcommand, then `cat-file --filters --path=<path> <blob-id>`; exact raw-equivalent bytes |
| Checkout | `checkout --detach --force --no-recurse-submodules <revision> --`; empty stdout |
| HEAD | `rev-parse --verify HEAD^{commit}`; exact revision plus LF |
| Status | `status --porcelain=v1 -z --untracked-files=all --ignore-submodules=all --no-renames`; empty stdout |
| Refs | `for-each-ref --format=%(refname)`; empty stdout |

All unspecified stdout is empty. Every successful fixed command has empty
stderr, and its result is unavailable until child quiescence and closure without
DescriptorClose of every supervisor-owned transient child-I/O endpoint due at
that boundary are also proved; every diagnostic is privately bounded. The
source path, format, local-config, and active-config rows are the only commands
that run against Repository; none can dereference an object. Init and its four
Boolean probes are the only private commands before private-store semantic
preflight and never read an object. Every other private row begins only after
that preflight and an empty-spool proof.

Config transcript keys use Git's ASCII-lowercased section and variable names
while retaining subsection bytes; the directly parsed semantic rows and fixed
command rows compare in that normalized form.

Every module-owned form and this complete Go 1.27.1 VCS-stamping sequence for
each role-package query and role build is mechanically proved builtin-only and
child-free with private empty `GIT_EXEC_PATH` and Git-only PATH.
`<short-revision>` is exactly the first twelve lowercase hexadecimal characters
of the admitted full revision:

| Go-owned argv after literal `git` | Exact stdout in the private repository |
| --- | --- |
| `status --porcelain` | Empty |
| `-c log.showsignature=false log -1 --format=%H:%ct` | Full revision, colon, canonical decimal committer seconds, LF |
| `config extensions.objectformat` | For SHA-1, exit 1 with empty stdout and empty diagnostic; for SHA-256, success with literal `sha256` plus LF and empty diagnostic |
| `-c log.showsignature=false log --no-decorate -n1 --format=format:%H %ct %D --end-of-options <full-revision> --` | Full revision, space, canonical decimal committer seconds, space, literal `HEAD`, with no LF |
| `for-each-ref --format=%(refname) --merged=<full-revision>` | Empty |
| `-c log.showsignature=false log --no-decorate -n1 --format=format:%H %ct %D --end-of-options <short-revision> --` | The same full-revision, seconds, and `HEAD` bytes as the full-revision row, with no LF |
| `cat-file --end-of-options blob <full-revision>:go.mod` | Exact authenticated repository-root `go.mod` blob bytes |

Every row except the SHA-1 config case exits zero, and every row has empty
diagnostic output.

These subprocesses resolve literal `git` through the sole private PATH symlink,
use the exact 50-row pinned-Go descendant base E followed by only
`PWD=<physical-checkout>`, and therefore receive exactly 51 environment rows.
Their actual cwd is the same checkout. They begin only after private preflight
and empty-spool proof and do not receive the module-invoked prefix. Each of the
twelve role-package `list -deps -json` invocations across the six discovery
passes and each of the two role builds emits these seven forms in the listed
order, for exactly 98 Go-owned Git children. The six `list -m -json all`
invocations and both `mod verify` invocations emit none. The two long-log
outputs, status stamp, and cat-file bytes agree with the already authenticated
commit and tree claims; no other Go-owned Git argv or environment projection is
admitted.

## Normative role, build, and output policy

| Order | Output | Exact main package |
| ---: | --- | --- |
| 1 | `agm` | `github.com/vbonnet/dear-agent/agm/cmd/agm` |
| 2 | `disk-watchdog` | `github.com/vbonnet/dear-agent/cmd/disk-watchdog` |

Each role command is the retained Go executable, runs in the private checkout,
and has this fixed argument structure:

```text
build
-buildvcs=true
-mod=readonly
-pgo=off
-ldflags=<one exact linker argument>
-o
<fixed private role output>
<exact role package>
```

The single linker argument contains these assignments in order:

```text
-X github.com/vbonnet/dear-agent/pkg/version.Version=dev
-X github.com/vbonnet/dear-agent/pkg/version.GitCommit=<full revision>
-X github.com/vbonnet/dear-agent/pkg/version.BuildDate=<UTC RFC3339 commit time>
-X github.com/vbonnet/dear-agent/pkg/version.BuiltBy=sandbox-gc-authority-v1
```

No `-trimpath`, `-a`, `-p`, caller flag, or caller output is added. The direct
environment omits `GOMAXPROCS`. Pinned Go uses host
`runtime.GOMAXPROCS(0)` for its action-worker count and derives each operational
compiler `-c=N` from that value and concurrent ready compile actions. Once the
authenticated graphs and admitted cache state are fixed, each role's
operational compile/assembler/link child multiset and action dependencies are
fixed, but independent-ready actions may interleave in any order consistent
with that causal partial order. Output admission requires the exact per-child
argv, cwd, environment, multiset, and causal edges, not one total chronology or
host-independent `-c` value.

The exact output BuildInfo settings appear once and in this order:

```text
-buildmode=exe
-compiler=gc
-ldflags=<the one exact protected four-assignment string>
DefaultGODEBUG=<exact DefaultGODEBUG field from the authenticated role-package go list result>
CGO_ENABLED=0
GOARCH=arm64
GOEXPERIMENT=none
GOOS=darwin
GOARM64=v8.0
vcs=git
vcs.revision=<full exact revision>
vcs.time=<exact UTC RFC3339 commit time>
vcs.modified=false
```

The Main record contains path `github.com/vbonnet/dear-agent`, empty sum, nil
replacement, and version
`v0.0.0-<UTC YYYYMMDDHHMMSS>-<first 12 revision hex>`. Each role's ordered
Deps exactly equal the authenticated modules supplying its transitive package
closure in Go 1.27.1 module order, including path, version, `h1:` sum, and nil
replacement. Dependency counts are derived rather than hardcoded.

## Normative Darwin child supervisor and cleanup proof

The trusted hosting process keeps `SIGCHLD` neither ignored nor configured with
`SA_NOCLDWAIT`, never changes that disposition while a build-authority
transaction or background reaper exists, and never invokes a wildcard or
foreign-PID `wait`, `waitpid`, `wait3`, `wait4`, or `waitid` that could consume a
build-authority child. The module checks the raw Darwin `sigaction` before each
`Start` and immediately before each numeric group signal; an unsafe or changed
disposition refuses or seals quiescence unknown. Absence of a competing broad
reaper is an audited in-process codebase invariant, not something Darwin can
prove dynamically; the BDD ownership guard scans production source outside
this package for such calls and documents this trusted embedding condition.
Ordinary `os/exec.Cmd.Wait` calls for other exact PIDs do not violate it.

The Darwin/arm64 old-action result passed to raw `SYS_SIGACTION` has exact size
16 and alignment eight: handler `uintptr` at offset 0, `uint32` mask at offset
8, and `int32` flags at offset 12. `SIG_IGN` is handler value 1 and
`SA_NOCLDWAIT` is flag bit `0x20`. The initial no-scratch authority probe uses a
nil new-action pointer, captures all 16 returned bytes, requires handler not 1
and that flag clear, and retains the exact tuple for the transaction. Its
syscall failure maps to `Primary/authority/probe/internal-invariant`; an unsafe
initial action maps to `Primary/authority/validate/unsupported`. Before every
`Start`, a syscall failure maps to
`Primary/<current-command-phase>/probe/internal-invariant` and a nonidentical
or unsafe tuple maps to `Primary/<current-command-phase>/probe/unstable`; no
child starts, no Child field or supervisor uncertainty is invented, and shared
cleanup still classifies an existing allocation. After `Start` but before
terminal waitid evidence, any query failure, nonidentical tuple, or unsafe tuple
maps to `Cleanup/close/quiesce/child-wait`, sends no numeric signal, and enters
the common background handoff. After terminal evidence, the same failure before
a survivor signal adds `child-wait`, sends no signal or later probe, and
continues to synchronous reap with the already observed Child status preserved.

One supervisor instance and its explicit pipes exist before each module-owned
direct-child `Start`. Every fixed plan contains exactly one tagged stdin source:
the retained null-device capability for a no-input command or one bounded
parent-owned byte input for a fixed protocol. The generic request cannot encode
nil, an untyped reader, or another stdin mode. Pinned-Go-owned descendants are
the sole nil-stdin exception; they remain within the enclosing direct Go
child's group and root/null revalidation bracket rather than becoming
separately registered supervisors. The supervisor's one event-loop goroutine is the only caller of nonreaping Darwin `waitid`, a
process-group signal, or a process-group probe. Exactly one designated reaper
calls `Cmd.Wait`, either synchronously after terminal proof or as the bounded
failure path's background component after the event loop has permanently
stopped signaling and probing. Pipe readers and the optional stdin writer
can publish immutable notifications to that owner but cannot
signal, probe, wait, reap, close a root, or choose a report result. No path uses
`Process.Wait`, `Process.Kill`, `Cmd.Cancel`, or `WaitDelay`.

| State or event | Exact supervisor behavior |
| --- | --- |
| Input source | A retained-null plan binds the already retained null descriptor directly and has no writer; a bounded-input plan creates exactly one explicit stdin pipe and immutable bounded writer; nil or an unrecognized tag is `validate/internal-invariant` before `Start` |
| Pipe setup | Create explicit stdout and stderr pipes before `Start`; a bounded-input command also receives its explicit stdin pipe; start the two readers only after all pipe setup succeeds, while the bounded input writer starts only after successful `Start`. Setup failure closes every opened end exactly once and returns the closed OS-classified `open` refusal without a Child field or supervisor-created quiescence uncertainty; shared cleanup still classifies any task allocated before this command |
| Start failure | Close every opened pipe end exactly once, unblock and join every started reader, and return `child-start`; no PID, PGID, wait, signal, probe, Child field, or supervisor-created quiescence uncertainty is invented, while shared cleanup still classifies any task allocated before this command |
| Start success | `Setpgid=true` and `Pgid=0`; successful `Start` is the new process-group-establishment proof because the Darwin spawn returns an error if that requested setup fails and zero requests the child PID as PGID. Record the positive PID as both leader and PGID and publish the started supervisor without a racy post-exit `getpgid`; close only the parent's copies of the child-side stdout/stderr write ends and optional stdin read end, each exactly once |
| Event loop | At every iteration, sample all pending stop notifications in the fixed precedence below and call Darwin `waitid(P_PID=1, pid, ..., WEXITED|WNOHANG|WNOWAIT)`; `si_pid=0` means no terminal result, while the exact PID with `CLD_EXITED`, `CLD_KILLED`, or `CLD_DUMPED` is terminal and remains unreaped. After a no-result sample, program the next poll for no later than five milliseconds after that sample or the earlier monotonic stop/exit deadline; no deliberate sleep is longer than five milliseconds, but OS scheduling latency is not claimed as a wall-clock reaction guarantee |
| Wait record | The Darwin/arm64 `siginfo_t` has size 104 and alignment eight, with 32-bit signed `si_signo`, `si_errno`, and `si_code` at offsets 0, 4, and 8; 32-bit signed `si_pid` at 12; 32-bit unsigned `si_uid` at 16; 32-bit signed `si_status` at 20; eight-byte `si_addr`, `si_value`, and signed `si_band` at 24, 32, and 40; and seven eight-byte unsigned pad words at 48. The exact typed object is zeroed before each call. A terminal result requires `SIGCHLD=20`, zero `si_errno`, the exact PID, and exactly one pair: `CLD_EXITED=1` with status 0 through 255; `CLD_KILLED=2` with status in {1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,24,25,26,27,30,31}; or `CLD_DUMPED=3` with status in {3,4,5,6,7,8,10,11,12}. Before a stop is latched, every EINTR resamples notifications and all monotonic deadlines and uses an interruptible wait no longer than five milliseconds before retry. After a stop is latched, exactly one consecutive retry follows one positive channel-free five-millisecond wait; a valid result resets the count, and a second consecutive EINTR adds `child-wait` and enters the no-signal/no-probe background handoff. Every other syscall, identity, or record anomaly adds `child-wait` and makes quiescence uncertain |
| Wait anomaly | Any non-EINTR wait syscall, identity, or record anomaly, including `ECHILD`, seals quiescence unknown and adds `child-wait`. Because leader pinning is no longer proved, the event loop performs no PID/PGID signal or membership probe, permanently stops waitid observation, and enters the common background handoff |
| Stop precedence | At one sampling boundary the order is phase-or-transaction deadline, caller cancellation, the selected output mode's limit, fixed-protocol streamed-output parse failure, stdin write failure, stdout/stderr read failure, then natural terminal state; the context error is sampled once per boundary. When terminal waitid evidence arrives, sample pending facts exactly once and latch either the higher-priority fact or natural terminal state. Later pipe, parser, or writer faults add cleanup uncertainty but cannot suppress or replace that terminal latch |
| Early stop | After a no-result waitid sample, any latched reason before terminal observation rechecks the admitted `SIGCHLD` disposition and sends exactly one `SIGKILL` to the negative recorded PGID; successful `Start`, the audited no-foreign-reaper regime, and the no-result sample prove that the leader is still live or will remain an unreaped zombie across the signal. Disposition drift adds `child-wait`, sends no signal, and enters the common background handoff; only the event-loop owner signals |
| Signal result | Success is final. `ESRCH` or `EPERM` is provisional and becomes success only after the exact leader-only zombie snapshot below; without that proof it adds `child-terminate`. Every other result immediately adds `child-terminate` and makes quiescence uncertain |
| Exit bound | The first numeric group-`SIGKILL` attempt, regardless of syscall result, latches one monotonic attempt instant and a deadline exactly five seconds later, rechecked before every waitid call and EINTR retry. If terminal evidence remains absent at that deadline, add `child-wait`, seal quiescence unknown, perform no membership probe or later group signal, and enter the common background handoff |
| Background handoff | Atomically latch one monotonic handoff instant, permanently stop signals, probes, and waitid calls, seal the public Child status as absent, and start one background component of the same supervisor that makes the sole blocking `Cmd.Wait` call until it returns. Concurrently give the optional writer and both readers until exactly handoff plus one second to complete; at that deadline close their parent endpoints to unblock and join them, add `child-drain` for every missing completion or EOF, and return without joining the reaper. The eventual Wait result is private liveness state only and never mutates the sealed report, disposition, Recovery, or prior receipt; task disposition remains `present` or `unknown` and Recovery is present |
| Group snapshot | After terminal waitid evidence and before reaping, issue `kern.proc.pgrp` into a preallocated buffer of exactly 4,096 `KinfoProc` rows; refuse kernel overflow before further allocation and require returned length to be an exact row multiple with unique positive PIDs and matching PGID |
| Empty-group proof | Exactly one row names the recorded leader PID and PGID with Darwin `SZOMB=5`, and no nonleader row exists. An empty result, missing leader, live or stopped leader, wrong state/PGID, duplicate, overflow, or syscall error adds `child-probe` and makes quiescence uncertain |
| Survivor | Any nonleader row adds `child-survivor` and makes quiescence uncertain. Before one further group `SIGKILL`, recheck the retained sigaction tuple. Query failure or drift adds `child-wait`, sends no signal or later probe, and proceeds to synchronous reap with the terminal status preserved. Otherwise, while the zombie leader pins the PGID, latch the signal-attempt instant, attempt that signal, and take at most 1,000 follow-up snapshots before the instant-plus-five-second monotonic deadline, programming no wait longer than five milliseconds; signal failure also adds `child-terminate`, snapshot failure also adds `child-probe`, scheduling latency is not a wall-clock reaction claim, and later disappearance never restores quiescence proof |
| Reap | After terminal waitid evidence and the final membership action, call `Cmd.Wait` exactly once and perform no later group signal or probe; explicit pipes prevent Wait-owned copying. Require its reliable `ProcessState` to match the waitid class and status exactly: `CLD_EXITED` matches the same exit code, while `CLD_KILLED` or `CLD_DUMPED` matches the same terminating signal and dumped/not-dumped class. Missing or contradictory state or another wait failure adds `child-wait` and makes quiescence uncertain |
| Pipe completion | The stdin writer closes its end exactly once after the complete bounded input or on stop. Readers count bytes from `Start` onward. On the synchronous path, one common monotonic deadline exactly one second after reap and group handling applies concurrently to stdin-writer completion and EOF on both output pipes; the background path instead uses the handoff deadline above |
| Pipe finalization | On complete I/O, join the writer/readers and close the parent-side pipe ends exactly once. At the common deadline, close those ends to unblock and join them; missing completion, missing EOF, or read failure adds `child-drain` |
| Output modes | Structured mode retains one complete stdout result and refuses above 256 MiB. Raw batch and filtered-projection modes do not retain a complete stdout result and instead incrementally charge protocol framing and content against their separate object, per-file, and materialized-tree ceilings |
| Stream-worker containment | Raw modes select one fixed-protocol concrete tagged plan, never a function, writer, or open interface implementation. Its fields contain only immutable authenticated value claims and a cleanup-owned task-spool capability. The pipe reader supplies copied bounded chunks through a module-owned channel and can stop and join independently. If the concrete worker misses the common deadline, expose no stream result, make quiescence unknown, and preserve the task; the worker has no StateRoot, source Repository, GOROOT, GOMODCACHE, tool, output-artifact, supervisor-report, or public-result authority |
| Descriptor accounting | Every pipe-end close failure occupies `DescriptorClose` with operation `close-nonroot`; such a failure never disappears merely because I/O completion was otherwise observed |
| Quiescence proof | Exact terminal waitid evidence, the exact leader-only zombie snapshot, resolved provisional signal status, one reliable reap, a joined optional stdin writer, and EOF on both output pipes prove quiescence for the created group; every missing or previously uncertain element makes it uncertain |
| Successful result gate | Publish the structured result or fixed-protocol stream result only after child quiescence, fixed-protocol completion when applicable, empty stderr, and closure without DescriptorClose of every supervisor-owned transient child-I/O endpoint due at that boundary; any missing proof withholds the result |

Structured stdout is one complete result retained only up to 256 MiB. Raw batch
and filtered-projection stdout never becomes a whole in-memory result: the
selected concrete plan consumes copied bounded chunks incrementally and enforces
the protocol's separate object, per-file, and materialized-tree limits.
Stderr alone supplies the Child diagnostic: the first at most 1 MiB in read
order, exact unsigned total byte count, truncation true exactly when the count
exceeds the prefix length, and SHA-256 of that prefix. Counter overflow refuses
as `limit`. Pipe scheduling cannot change this single-stream claim. An
intentional `SIGKILL` status caused by a stop trigger does not independently add
`child-exit`; an unrequested nonzero or signaled status does. Exit zero with
nonempty stderr refuses as the current phase's `parse/malformed`, attaches the
bounded status-zero Child diagnostic to Primary, and never publishes stdout.

| Observed event | Report slot, phase, operation, and cause |
| --- | --- |
| Pipe creation/setup failure before `Start` | `Primary`, current command phase, `open`, with `permission`, `limit`, or `internal-invariant` from the closed OS classification; no Child field |
| `Start` failure | `Primary`, current command phase, `execute`, `child-start`; no Child field |
| Parent pipe-end close failure | `DescriptorClose`, `close`, `close-nonroot`, `descriptor-close` |
| Exit zero with nonempty stderr | `Primary`, current command phase, `parse`, `malformed`; Child status is zero and stdout remains unavailable |
| Unrequested nonzero or signaled terminal status | `Primary`, current command phase, `execute`, `child-exit`; Child status is the nonnegative exit code or negative signal number |
| Caller cancellation | `Primary`, current command phase, `execute`, `canceled`; include observed status without adding `child-exit` when termination was intentional |
| Phase or transaction expiry | `Primary`, current command phase, `execute`, `deadline`; include observed status without adding `child-exit` when termination was intentional |
| Structured stdout limit | `Primary`, current command phase, `execute`, `limit` |
| Streamed stdout parse failure | `Primary`, current command phase, `parse`, `malformed` |
| Stdin write failure | `Primary`, current command phase, `execute`, `unstable` |
| Group-signal failure | `Cleanup`, `close`, `quiesce`, `child-terminate` |
| Exit-observation or reap failure | `Cleanup`, `close`, `quiesce`, `child-wait` |
| Pipe read, writer-completion, or common EOF-deadline failure | `Cleanup`, `close`, `quiesce`, `child-drain` |
| Observed nonleader group member | `Cleanup`, `close`, `quiesce`, `child-survivor` |
| Membership syscall or classification failure | `Cleanup`, `close`, `quiesce`, `child-probe` |

Only the earliest failed or uncertain child in fixed command order contributes
a Child field, attached to the first populated record concerning that child;
later records retain ordered cause codes without another Child field.

Task disposition exists only after task allocation and has this proof table:

| Disposition | Exact proof and recovery result |
| --- | --- |
| `removed` | Identity-gated removal of the retained named child completed through the retained roots and a final no-follow StateRoot lookup returned `ENOENT`; Recovery is absent even if a later root-handle close fails |
| `present` | Removal did not complete and a final no-follow lookup proves the generated name still denotes the retained task identity; Recovery is present |
| `unknown` | Neither proof exists, including task-root identity never observed after successful allocation, lookup error, replacement identity, disappearance not caused and verified by the owned removal, or incomplete observation; Recovery is present and omits task-root identity when none was observed |

A non-root descriptor failure blocks removal and therefore ends as `present`
or `unknown`. A task-root or StateRoot close failure after proved removal
occupies `DescriptorClose` while disposition remains `removed`. Recovery is
present exactly for an allocated task whose disposition is `present` or
`unknown`. Its task-root identity field is optional; a zero identity is never
used as a substitute for missing evidence.

## Acceptance evidence matrix

The matrix assigns evidence; it does not add behavior or authorize publication,
merge, installation, or activation.

| Requirements | Required evidence |
| --- | --- |
| 001-019 | Compile-time interface checks plus public lifecycle and receipt tests; every eight-step no-scratch failure closes the already-retained capability prefix, returns no allocation or Recovery, and survives `-race`; successful-`mkdirat` partial-allocation, descriptor, and concurrent Close cases |
| 020-058 | Public filesystem fixtures plus distinct nominal Go/compiler/Git and retained physical-root/null capability checks; exact `0755` physical-root success, one-off root-mode failures, and wrong null kind, owner, complete `020666` mode, link count, raw major, or raw minor; physical-root-anchored fresh-leaf initial and repeated `GOROOT/go.env` transactions with operation-attributed open/probe/ACL-parse/exact-read/grammar/hash/post-observation/compare, retained-row and digest binding, every primitive failure, and exact-once close including Primary plus DescriptorClose; exact Go/compiler/Git wire lengths and digests, root-sentinel outcomes, and per-use identity/security drift; golden SHA-256 vectors for all six manifest domains, fixed root framing, raw-byte ordering, root-count exclusion, control/newline names, and symlink target-record binding; selective `.git` authority/forbidden/inert classification, inert add/remove/replace/reclassify and identity/security drift, and allowed same-inode unread inert-byte change; absent/no-ACL/empty-ACL, bad-filesec-magic, endian, bounds, unknown-bit, permit/deny, and 128/129-entry ACL cases; injected exact/one-over accounting for GOROOT 65,536 descendants, 2 GiB aggregate, and 128 MiB/file, GOMODCACHE 500,000 descendants, 16 GiB aggregate, and 512 MiB/file, and Repository 1,000,000 descendants, 64 GiB aggregate, and 8 GiB/pack; dual-open root identity mismatch and pathname replacement; local/ignore-ownership, visible-flag masking, and descendant FSID/device transition; GOROOT 4-KiB and 40-link exact/one-over, absolute, dangling, directory-target, cycle, escape, and target drift; thin/fat 0/1/32/33 rows, duplicate row, table/slice overlap, alignment, outer/inner mismatch, swapped/FAT64/ARM64e, exact header flags, every allowed command family exact/one-off size, multiplicity, extent, padding, path-prefix lookalike, and panic containment; all five linkedit-data commands with zero-size interior, exact-end, and one-over cursors, a synthetic pinned-Git-shaped empty `LC_DATA_IN_CODE`, and zero-count/nonzero-offset refusal for every other extent family |
| 059-111 | Public synthetic standalone repositories; exact eight-step/two-block command order and four-manifest move-once aggregate before allocation; the deep five-command interface, exact pre/post matrix, mandatory entered-bracket completion, separately armed block-end bracket on every later exit, five named request materializers with no raw caller command or timing authority, same-clock opaque command windows, earliest caller/transaction/phase deadline, and zero allocation; first pending pointer-alias transfer success, losing/repeated/concurrent alias refusal before handle access, and exactly-once transfer or close; exact 12-byte lowercase name framing, entropy error/short read, context stop, collision without allocation, 100-candidate exhaustion, every closed noncollision `mkdirat` cause, and immediate post-success unobserved claim; exact no-home initial workspace, sole symlink, and unchanged-policy boundary; retained read/write spool-to-`ReaderAt`, same-inode one-to-zero unlink proof, rename/swap refusal, 4,096/4,097 files, 2-GiB exact/one-over, generations, empty-before-Git, and cleanup; distinct exact 39-row byte-sorted preallocation/task-private direct environments and fresh clones; hostile ambient home/telemetry/sidecar/cache/temp/proxy/loader variables absent; pinned empty-home telemetry-off/no-artifact probe and writable-home negative; every preallocation command and four source rows before allocator count changes, retained-null stdin, root/null brackets, filesystem-write-denied evidence allowing only `/dev/null`, exact 39-to-50 mutation, 51-row Git, 50-row tool-ID, 52-row compile/assembler, and 51-row link projections; object/zlib/pack/delta accounting, same-ID canonical equivalence, commit grammar, raw-tree `40000` to transcript `040000`, tree-DAG/path collision; byte-exact SHA-1/SHA-256 config; raw batch/tree/attribute framing including inert `diff=set`; copy completeness, raw projection equality, checkout, and exact Git state |
| 112-138 | Synthetic module-cache, `go.sum`, ZIP/ZIP64, cancellation, no-follow source/symlink, graph recheck, assembly, `go_asm.h`, and exact derived-tree threshold cases; all twelve role-list seven-plus-three bundles, no operational role-list child, six module-list/two-mod-verify negative traces, and exact 42 tool-ID total |
| 139-154 | Deterministic build-info; exact seven-row Go-owned Git argv/output/exit/order traces for all twelve role-package queries and both role builds; exact 98 Git and 42 tool-ID totals; no fifth pinned-Go child constructor; each role's exact operational child multiset and causal partial order with every argv/cwd/environment, allowed ready-action interleavings, and host-derived `GOMAXPROCS`/compiler `-c`; thin-Mach-O, retained-output-identity, role-order, graph comparison, and partial-pair cases plus one opt-in exact real pair build |
| 155-170 | Public real-process and instance-local supervisor cases plus tagged retained-null and bounded-input-pipe plans, nil/untyped-input refusal, pinned-Go nil-stdin/root-null exception, exit-zero/nonempty-stderr refusal and successful-result withholding until quiescence/closure/empty stderr; production-source guard for the exact 16-byte sigaction ABI, admitted/ignored/changed/query-error `SIGCHLD`, forbidden broad reapers, and allowed exact-PID waits; exact descriptor-read/EOF attribution, required/optional/forbidden observations, sentinel and allocator mappings, preallocation root/non-root close ordering, setup failure before and after task allocation, including successful `mkdirat` with failed retained open and optional recovery identity; exact write-once Primary mappings, first-postcheck-as-Primary, child-Primary plus one bounded private non-close postcheck record, proof withholding, no later record, no public secondary slot, independent DescriptorClose and Cleanup, and rendered slot order; exact-once pipe close/join, captured `Setpgid=true`/`Pgid=0`, Start/process-group-establishment failure, immediate and deliberately delayed observation of a fast exit without post-Start `getpgid`, Darwin/arm64 siginfo size/alignment/field offsets and every exact terminal code/status boundary, nonblocking waitid terminal/anomalous/stopped states, preterminal `ECHILD`/lost-identity no-signal handoff with permanently absent public status despite early or late private Wait return, post-terminal sigaction drift with no signal/probe plus synchronous status-preserving reap, single and persistent EINTR/deadline resampling with the exact positive channel-free five-millisecond retry and second-EINTR handoff, waitid-versus-ProcessState mismatch, natural/nonzero/signaled exit, cancel/deadline/limit/parse/input/read races and fixed precedence including one-read limit-before-parse publication, five-millisecond maximum programmed wait, exact handoff-plus-one-second pipe finalization before every background return, structured-versus-streamed mode validation, a greater-than-256-MiB valid streamed fixture without whole-output retention, concrete-tagged-plan and no-function/writer/open-interface source guards, incomplete-stream-worker no-result and task-preservation proof, terminal-latch versus late-I/O-fault cases, cloned-input concurrent-mutation coverage, a deterministic completion-channel scheduler, five-second post-attempt observation and `child-wait` handoff, leader pinning, required zombie leader, empty/missing/live/wrong-state snapshots, raw sysctl overflow, terminate/wait/probe/drain/survivor faults, provisional `ESRCH`/`EPERM`, 4,096/4,097 members, 1,000-snapshot follow-up, held stdout and stderr, common one-second EOF bound, signed/absent status, exact stderr prefix/count/digest/truncation, earliest-child/single-Child reporting, all disposition/Recovery rows, whole-ledger preflight before any delete, removal plus root-close failure, replacement root, nested mount, descriptor failure, and concurrent Close |
| SPEC ownership | `internal/buildauthority/SPEC.md`; its `# RELATED-SPEC` reciprocal link; the `internal/buildauthority` row and wait-ownership scenario in `agm/test/bdd/features/internal_foundation_guardrails.feature`; and the production-source scanner plus allowed-exact-PID and forbidden-wildcard/foreign-reaper fixtures in `agm/test/bdd/steps/internal_foundation_guardrails_steps.go` and `internal_foundation_guardrails_steps_test.go` |

The current Homebrew ancestry rejection is an acceptance case for
BUILD-AUTH-020 through BUILD-AUTH-048. The current source's
`objects/maintenance.lock` is an acceptance case for BUILD-AUTH-054 and
BUILD-AUTH-058; this slice does not delete or reinterpret it. The opt-in
protected-GOROOT build is mechanics evidence, not proof of protected host
provisioning. Package tests own build-authority behavior. The BDD feature, Go
step, and focused scanner fixtures own living-SPEC registration, reciprocal
linkage, and executable enforcement of the trusted process-wide wait-ownership
condition; they do not implement child supervision.

## BDD Traceability

- Feature: `agm/test/bdd/features/internal_foundation_guardrails.feature`
- Scenario: Build authority retains exclusive broad child-reaping ownership

## Test Traceability

- Unit package: `internal/buildauthority`
- Governance scanner: `agm/test/bdd/steps/internal_foundation_guardrails_steps_test.go`
