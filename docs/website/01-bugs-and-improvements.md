# kungfu — bugs and improvements

A code-grounded review of the current `main` branch. Items are ordered by
severity so engineering can triage top-down, and section 4 lists the gaps
the marketing site should **not** promise yet.

File references use absolute repo paths plus line numbers so each finding
is one click from the code.

---

## 1. Critical bugs

Anything that could corrupt user state, fail silently, or pose a security
risk.

### 1.1 `install.sh` silently skips checksum verification when no hasher is present
- **Where:** `/Users/mike/Projects/kungfu/install.sh:124-131`
- **What:** If neither `sha256sum` nor `shasum` is found, the script logs a
  warning and continues to install the unverified binary. A curl-pipe-sh
  installer that proceeds without checksumming when the verification tool
  is missing is a textbook supply-chain weakness — and on a minimal
  container it's the failure mode that happens by accident.
- **Fix:** Error out (exit 1) instead. The README already promises checksum
  verification, so degrading to "warning" violates the contract. If a
  pure-shell fallback is desired, ship one (a tiny `openssl dgst -sha256`
  fallback would cover almost every box).

### 1.2 `kungfu self-update` is broken on Windows (assumes `.tar.gz`)
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/selfupdate.go:130`,
  `extractBinaryFromTarball` at line 332
- **What:** The archive name is hard-coded to `.tar.gz`, but
  `.goreleaser.yaml:40-42` ships Windows as `.zip`. The download will 404
  on Windows. Even if the URL were right, `extractBinaryFromTarball`
  cannot read zip files, and the binary name search at line 355 compares
  against `binaryName` (the local executable's basename) but Windows
  archives ship `kungfu.exe` while a developer build run from `go run`
  may resolve `binPath` to a Go cache path with no `.exe` suffix at all.
- **Fix:** Branch on `runtime.GOOS == "windows"` to download the `.zip`,
  use `archive/zip`, and normalise the binary name to `kungfu.exe`
  on Windows regardless of the running process's executable name.

### 1.3 Local install copies symlinks verbatim, with no escape check
- **Where:** `/Users/mike/Projects/kungfu/internal/skill/install.go:115-120`
- **What:** `copyTree` recreates `os.ModeSymlink` entries via
  `os.Symlink(link, target)` without validating the link target. A
  hostile local skill (or a careless author with `references/data ->
  /Users/me/.aws`) can plant a symlink in the user's skills directory
  that an agent will then dereference at read time. The remote-install
  path already strips symlinks (`extract.go:74-78`); local should too.
- **Fix:** Either skip symlinks (matching the extract behaviour and the
  package docstring's "Symlinks are recreated" sentence is then wrong
  and should also be corrected) or reject symlinks whose resolved target
  escapes the source root before copying.

### 1.4 Backup-rollback in `skill.Install` swallows `RemoveAll` failure but reports success
- **Where:** `/Users/mike/Projects/kungfu/internal/skill/install.go:89-92`
- **What:** When `--force` overwrites an existing skill, the old copy is
  renamed to `dst + ".bak-<rand>"`. The final `os.RemoveAll(backup)`
  error is returned, but the new copy is already in place and visible to
  the agent — so a partial-failure leaves a leftover `<name>.bak-<rand>`
  directory next to the installed skill that `list` does not surface and
  the user cannot easily discover. Worse, the user sees `install failed`
  and may retry, doubling up backups.
- **Fix:** Treat a backup-removal failure as non-fatal: log it as a
  warning ("could not remove backup; you can `rm -rf <path>` manually")
  and return success. The committing rename already won.

### 1.5 `cappedReader` cannot read a file that is exactly `MaxTarballSize`
- **Where:** `/Users/mike/Projects/kungfu/internal/fetch/github.go:258-277`
- **What:** When `r.n == r.Max` on a subsequent call, the reader returns
  `(0, io.EOF)` and sets `exceeded = true`, even though no overflow
  occurred. A perfectly-100MB tarball is wrongly rejected as "exceeded";
  the symptom is a confusing `tarball exceeded 104857600 bytes` for an
  archive that is, in fact, exactly the cap.
- **Fix:** Track whether we ever observed `Read` returning bytes beyond
  the cap (the `+1` probe should set `exceeded` only when the underlying
  reader returned more than `remain` bytes) rather than treating "we have
  filled the cap" as the same condition as "we overflowed the cap".

### 1.6 No SHA pinning of the resolved commit before fetch
- **Where:** `/Users/mike/Projects/kungfu/internal/fetch/github.go:76-96`,
  `FetchTarball` at line 101
- **What:** `ResolveRef` returns a commit SHA, then `FetchTarball`
  downloads `codeload.github.com/.../tar.gz/<sha>`. There is no integrity
  check that the bytes GitHub returned correspond to that SHA — a
  redirector or compromised mirror could ship a different tree under the
  SHA URL and we'd cache it forever. (The same SHA stamped in
  `kungfu_sha` provenance is therefore an unverified assertion.)
- **Fix:** Either accept that this is "trust GitHub TLS" and document it
  explicitly, or compute the git tree hash from the extracted tarball
  and verify against `ResolveRef`'s commit object's tree SHA. The latter
  is real provenance.

### 1.7 `update --all` ignores the resolved-but-unused fetched tarball failure path's scratch cleanup ordering
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/update.go:286-376`
- **What:** Inside `executeUpdatePlan`, every path that fails after
  `os.MkdirTemp` does `os.RemoveAll(scratch)` — but the lint-fail branch
  at line 352-358 calls `os.RemoveAll(scratch)` before
  `renderLintHuman`, which is fine, while the `skill.Install` failure
  branch calls `os.RemoveAll(scratch)` twice (lines 361 and 367). Not a
  data-corruption bug but it indicates the cleanup logic is hand-rolled
  per branch and easy to get wrong; a single `defer os.RemoveAll` at the
  top of the loop body would prevent future divergence.
- **Fix:** Restructure the per-row work into a helper that owns its own
  defer-based cleanup; the outer loop just collects success/failure.

---

## 2. High-value improvements

Real user-facing wins; ship before the website launches if possible.

### 2.1 No `kungfu cache` subcommand (listed on the roadmap but not built)
- **Where:** Roadmap entry in `/Users/mike/Projects/kungfu/README.md:212`
  vs. `internal/cli/root.go:87-95` (no `cache` command registered)
- **What:** The cache lives under `$XDG_CACHE_HOME/kungfu/tarballs` and
  grows unbounded within the 7-day TTL window. Users with no
  introspection tool will find their cache after they wonder where their
  disk went.
- **Fix:** Add `kungfu cache list|clear|verify` — small, isolated commands
  on top of the existing `fetch.Cache` type.

### 2.2 No `--json` on `install`, `update`, `remove`, or `new`
- **Where:** Each command's `cmd.Flags()` call (e.g.
  `/Users/mike/Projects/kungfu/internal/cli/install.go:94-99`)
- **What:** Only `lint`, `list`, and `version` support JSON output. CI
  pipelines that want machine-readable install results have to scrape
  stdout/stderr.
- **Fix:** Standardize a `--json` flag across mutating commands that emits
  the same `{ok, target, scope, path}` shape used by `list`.

### 2.3 `kungfu install` confirmation prompt has no `--no-input` discipline
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/install.go:521-541`,
  `promptConfirm` at line 570
- **What:** When stdin is non-interactive (CI, pipe) the prompt reads an
  empty line and defaults to "yes" silently. A user piping `echo |
  kungfu install user/repo` discovers their CI just installed something
  without an explicit `--yes`.
- **Fix:** Detect non-interactive stdin and require `--yes` rather than
  defaulting to true. The remote install path already accepts `--yes`;
  make it required when not a TTY.

### 2.4 No `kungfu search` / no `kungfu uninstall` (aliased only as `rm`)
- **Where:** Root command tree in
  `/Users/mike/Projects/kungfu/internal/cli/root.go:87-95`
- **What:** `remove` exists but `uninstall` is more discoverable to users
  who reach for `apt` / `npm` / `brew` muscle memory. Add as an alias.
  Separately, there is no `search` even against the user's own
  installed skills (`list | grep` is what people do today).
- **Fix:** Add `uninstall` alias to `remove`; add `list --search <term>`
  or a dedicated `search` that grep-greps name + description.

### 2.5 `kungfu update` cannot pin a ref permanently
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/update.go:78-159`
- **What:** Passing `--ref v2` to `update` updates the skill but does not
  change the stored `kungfu_ref` to `v2` — the next `update --all` will
  resolve the *original* stored ref. Reading `update.go:341-344`: yes
  the `Ref` is stamped to `r.plannedRef`, so this actually works for one
  skill — but only if the install actually succeeded. A user reading the
  flag's help text ("ref to fetch instead of each skill's stored
  kungfu_ref") would not expect a side effect on stored state.
- **Fix:** Document the side effect explicitly in the flag help, or add a
  separate `kungfu pin <name> <ref>` so the operations stay distinct.

### 2.6 No lock file / reproducible-install primitive
- **Where:** No `kungfu.lock` or equivalent anywhere in the codebase
- **What:** Provenance is stamped per-skill into each agent's
  installation, but there is no project-level manifest. Teams cannot
  share "install these N skills at these refs" with a single command.
- **Fix:** Spec out a `kungfu.toml` / `kungfu install --from-manifest`
  flow. This is the single biggest missing feature for an enterprise
  user; even a minimal MVP (`{name, source, ref}` array) would be
  valuable.

### 2.7 `kungfu lint --fix` does not actually fix the most common issues
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/lint.go:166-207`
- **What:** Trims trailing whitespace and re-serializes YAML. It does not
  fix `frontmatter/name-mismatch` (the most-hit error during
  development), nor does it normalize description casing, nor rename the
  directory to match `name`. Users assume `--fix` means "do the obvious
  thing".
- **Fix:** Either expand the auto-fix set (at minimum: directory rename
  for name-mismatch when the user has write permission), or rename the
  flag to `--format` to set expectations honestly.

### 2.8 `list` runs a full lint pass on every discovered skill
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/list.go:140-143`
- **What:** `gatherListItems` calls `linter.Lint(s.Dir)` for every skill,
  which re-reads `SKILL.md` and walks the entire skill tree for
  `references/broken` and `filenames/non-ascii`. With a dozen skills
  installed across four targets this is 48 lint passes per `kungfu list`
  invocation. Imperceptible today, noticeable at 100+ skills.
- **Fix:** Either cache `HasErrors` by `(path, mtime)` or run lint only
  under an opt-in flag (`list --check`).

### 2.9 `install` confirmation prompt blocks `Ctrl-D` / EOF distinguishing
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/update.go:391-405`,
  `/Users/mike/Projects/kungfu/internal/cli/install.go:570-590`
- **What:** Two separate yes/no helpers exist (`promptConfirm` in
  install.go and `readPromptYes` in update.go) with subtly different
  semantics — `readPromptYes` ignores the read error entirely, so EOF on
  stdin returns the default. A scripted "press enter" that closes the
  pipe before the prompt is reached effectively votes yes.
- **Fix:** Unify on a single `ui.Prompter.Confirm` (already implemented
  in `internal/ui/prompt.go`) and surface `ErrPromptAborted` instead of
  silently defaulting.

### 2.10 `kungfu install <github>` will retry the GitHub default-branch lookup even when offline
- **Where:** `/Users/mike/Projects/kungfu/internal/fetch/github.go:53-70`
  (60s `HTTP.Timeout`)
- **What:** No retry, no offline-mode awareness, and the error returned
  ("fetch: default-branch request: …") names the implementation
  detail. Combined with the install command wrapping it as
  `&ExitError{Code: 5, Err: …}`, the user sees a stack of
  `install: fetch: default-branch request: Get "…": dial tcp …`. Friendly
  it is not.
- **Fix:** Detect `net.OpError` / `context.DeadlineExceeded` and emit
  `install: could not reach GitHub — check your connection or pin a
  specific ref with --ref`.

### 2.11 No SIGINT/SIGTERM handling — Ctrl-C during a download leaves no cleanup message
- **Where:** `/Users/mike/Projects/kungfu/cmd/kungfu/main.go:13-21`,
  `/Users/mike/Projects/kungfu/internal/cli/root.go:101-103`
- **What:** `Execute` uses `context.Background()`. Ctrl-C kills the
  process; the deferred `os.RemoveAll` calls do run, but the user sees
  no acknowledgement and the `kungfu` exit code is 130 (from the
  signal), bypassing the documented exit-code contract.
- **Fix:** Wire `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`
  in `main` and pass that into `ExecuteContext`. Print "aborted" on
  cancellation and exit 2 to match the rest of the prompt-decline UX.

### 2.12 `install.sh` uses `mv` across filesystems (can fail on `/tmp` → `/usr/local/bin`)
- **Where:** `/Users/mike/Projects/kungfu/install.sh:211`
- **What:** `mv "$TMPDIR_KUNGFU/kungfu" "$target"` — `mktemp -d` may have
  picked a path on a different filesystem (e.g., `/private/tmp` on macOS
  vs `/usr/local/bin`). On Linux some hardened mounts return
  `EXDEV`. `mv` handles the cross-fs case by copying, but the error
  message if it fails is the bare `mv` error which gives the user
  nothing actionable.
- **Fix:** Prefer `install -m 0755` (POSIX, copies + sets mode), or
  explicitly `cp` followed by `chmod` + `rm`, with a clear error if
  either step fails. Also verify the binary exists at the expected path
  after extraction (line 205) rather than discovering it via the `mv`
  failure.

---

## 3. Polish & nice-to-haves

Small wins; do as time allows.

### 3.1 Empty `--target=` flag string error is unhelpful
- **Where:** `/Users/mike/Projects/kungfu/internal/target/target.go:107-109`
- **What:** `target: no target specified` is technically true but the user
  who wrote `--target=""` may not connect that to "I need to omit the
  flag entirely or pass a name".
- **Fix:** "Use --target=all, or comma-separated names like
  --target=claude,codex; passing an empty value is rejected to avoid
  silent fallbacks."

### 3.2 `kungfu new` strips `Use this skill when ` but only at the start
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/new.go:243-251`
- **What:** Case-insensitive prefix strip handles the most common typo
  but not "Use this skill **whenever**…" or other near-misses.
- **Fix:** Either keep as-is and document, or be slightly more permissive
  with regex; the cost of false positives is low because the description
  is shown back to the user before render.

### 3.3 Two separate yes/no prompt helpers (`promptConfirm` + `readPromptYes`) with subtly different semantics
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/install.go:570`
  and `/Users/mike/Projects/kungfu/internal/cli/update.go:391`
- **What:** `promptConfirm` uses `bufio.NewReader.ReadString`;
  `readPromptYes` does a single 64-byte `r.Read`. The latter will
  truncate a long line and silently treat the remainder as the answer to
  the next prompt.
- **Fix:** Consolidate on one. The `internal/ui` package already has
  `Prompter.Confirm`; reuse it everywhere.

### 3.4 Tautological context check in `runRemoteInstall`
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/install.go:136-139`
- **What:** `if ctx == nil { ctx = cmd.Context() }` — both branches assign
  the same value. Dead code.
- **Fix:** Delete the conditional, or replace with
  `ctx = context.Background()` for the nil-guard semantics presumably
  intended.

### 3.5 `kungfu show <name>` ambiguity error names the targets but not how to disambiguate per-scope
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/show.go:75-82`
- **What:** Says "disambiguate with --target" but if the same skill is in
  both personal and project scope for the same target, `--target` alone
  won't resolve it. The user has to discover `--scope`.
- **Fix:** Update the error message to suggest `--target X --scope Y`
  when the ambiguity is scope-driven.

### 3.6 `frontmatter/name-format` rule duplicated against `skill.ValidateName`
- **Where:** `/Users/mike/Projects/kungfu/internal/lint/rules/frontmatter.go:25`
  vs `/Users/mike/Projects/kungfu/internal/skill/name.go:17`
- **What:** Two regexes that differ subtly: the lint rule supports a
  `namespace:` prefix, the skill validator does not. A user could create
  a name with `kungfu new ckm:banner-design` and have it rejected, then
  install a skill of that name from GitHub and have it accepted.
- **Fix:** Make `ValidateName` accept the namespace prefix or document
  that scaffolded names are strictly stricter than installed names (and
  why).

### 3.7 `kungfu list` `[!]` marker for broken skills is documented but `--no-color` detection at render time is awkward
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/list.go:178-185`
- **What:** `if ui.NoColor()` is checked per row; `marker` width differs
  between branches ("   " vs `"[!]"` is 3 chars vs the emoji rendering
  in color mode), which may misalign columns on terminals that
  partial-width-render the emoji.
- **Fix:** Pick a single 3-char marker that doesn't depend on color
  detection, e.g. `"!! "` or `" * "`.

### 3.8 Frontmatter `kungfu_*` keys are stamped in the source tarball before lint, then re-rendered
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/install.go:266-272`
- **What:** Lint runs *after* provenance is stamped; the YAML
  re-serialization can change line numbers, so any lint diagnostic with
  a line ref points one line off from what the user's editor will show
  when they view the unmodified upstream source. Minor but confusing.
- **Fix:** Lint *before* stamping; stamp into the prepared copy that
  ships to the destination only.

### 3.9 `references/broken` walks the markdown body parse on every lint, even when run by `list`
- **Where:** `/Users/mike/Projects/kungfu/internal/lint/rules/references.go:43-92`
- **What:** Goldmark parse + AST walk + per-link stat is the most
  expensive rule. Combined with #2.8 above, lists of many skills become
  slow.
- **Fix:** Add a `SkipFilesystemRules` option to the linter so `list`'s
  "do you have errors?" check can avoid the costly per-file work.

### 3.10 Missing `--quiet` / `-q` everywhere
- **Where:** All commands
- **What:** Users wiring `kungfu install` into Makefiles want to suppress
  the `fetching …` / `downloaded tarball …` chatter and keep only
  errors and the final summary.
- **Fix:** Add a global `--quiet` that silences `ui.Muted.Render` output.

### 3.11 `install --dry-run` does not run a network call to resolve refs
- **Where:** `/Users/mike/Projects/kungfu/internal/cli/install.go:128-208`
  — it *does* call `ResolveRef` and `FetchTarball` before noticing dry-run
- **What:** The dry-run still hits GitHub (default-branch GET + ref
  resolve + tarball download). Surprising for a `--dry-run`.
- **Fix:** Either skip the fetch entirely under dry-run (output: "would
  fetch <source>@<ref> and install to …") or document that dry-run is
  "show me the install plan after I've already paid the network cost".

---

## 4. Roadmap gaps the website should not promise yet

The marketing site/PRD/style guide should treat these as **aspirational**,
not v1 features. Phrasing them as "today" overstates the product.

| Gap | Where in the product today |
| --- | --- |
| **Only GitHub is supported.** Source parser rejects every other host; the README's roadmap entry for GitLab/Bitbucket/Codeberg has zero implementation. | `internal/fetch/source.go:88-97` |
| **No `kungfu search` / no public index.** README roadmap names it but nothing exists; the website should not show search UI mockups as if they work. | README roadmap; no `search.go` |
| **No `kungfu publish`.** Same as above; this is roadmap-only. | README roadmap |
| **No `kungfu test`.** Skill `scripts/test.sh` conventions exist only in templates; there is no runner. | README roadmap |
| **No `kungfu doctor`.** Common-misconfig diagnoser is roadmap-only. | README roadmap |
| **No `kungfu cache` subcommand.** Tarballs accumulate; users have no first-class way to inspect or clear them (see #2.1). | No `cache.go` cmd file |
| **No lock file / project manifest** (see #2.6). Teams cannot share a reproducible "install these skills" command today; the website should not imply they can. | No `kungfu.toml` parser |
| **No Homebrew tap yet.** README correctly notes "on the roadmap"; the website should match that hedge and not show `brew install kungfu` as a primary install path. | README "A Homebrew tap is on the roadmap" |
| **Provenance integrity is "trust GitHub TLS" only** (see #1.6). The website should not claim cryptographic verification of installed skill contents — only commit-SHA recording. | `internal/fetch/github.go` |
| **`update` does not detect upstream `SKILL.md` deletions.** If the remote skill folder was renamed or removed, the user gets a fetch failure rather than a helpful "this skill no longer exists upstream — remove with `kungfu remove`?". | `internal/cli/update.go:298-376` |
| **No telemetry, no analytics, no error reporting.** Fine as a stance, but if the website implies "we know about issues" it would be misleading; surface a "report a bug" link instead of pretending there's an automated channel. | None implemented |
| **No skill signing / author trust model.** Anyone's `user/repo` can be installed; there is no allowlist, no author identity check, no review status. Website should not show a "verified skills" badge. | None implemented |

---

## Test-coverage gaps worth filling

- **No `cache_test.go`** in `/Users/mike/Projects/kungfu/internal/fetch/`.
  TTL expiry, concurrent `Put` (two `kungfu install` processes racing),
  and partial-write recovery are all untested. The `Put` atomic-rename
  contract is the kind of thing that breaks silently on a filesystem
  with surprising semantics.
- **`cappedReader` boundary** at `MaxTarballSize` exactly (see #1.5) —
  no test exercises the equal-to-cap case.
- **Windows end-to-end install on a real runner** — `selfupdate_test.go`
  now covers the zip extractor and archive-name dispatch cross-platform
  (after the v0.1.5 fix for #1.2). The new `install-windows` job in
  `.github/workflows/install-test.yml` drives `install.ps1` on
  `windows-latest` against a snapshot, but `kungfu self-update` itself
  is still only exercised via unit tests; an end-to-end self-update on a
  real Windows runner against a snapshot release would close the loop.
- **Local install symlink handling** — no test covers the symlink
  escape risk in #1.3.
- **Concurrent install of the same skill into the same target** — the
  `.tmp-<rand>` staging directory in `skill.Install` is random-suffixed,
  so two processes won't collide on staging, but they will race on the
  final rename and the backup. No test exercises this.
