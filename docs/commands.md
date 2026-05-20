# kungfu command reference

Every command shares three persistent flags:

| Flag             | Meaning                                                            |
| ---------------- | ------------------------------------------------------------------ |
| `--no-color`     | Disable ANSI colour. Honoured automatically when `NO_COLOR` is set. |
| `--config`       | Use a specific config file instead of the XDG default.             |
| `--target`       | Comma-separated targets, or `all`. Empty uses each command's default. |
| `--scope`        | `personal`, `project`, or (where supported) `both`.                |

The list, show, remove, and update commands treat **empty `--target`** as
"every configured target" so they're useful for discovery; install uses
`default_targets` from config to keep it predictable.

---

## `kungfu self-update`

Replace the running `kungfu` binary with the latest release from GitHub.

```
kungfu self-update [--check] [--version <tag>] [--yes|-y]
```

| Flag         | Notes                                                                  |
| ------------ | ---------------------------------------------------------------------- |
| `--check`    | Print whether an update is available, then exit 0 without installing. |
| `--version`  | Install a specific tag (e.g. `v0.1.0`) instead of the latest release. |
| `--yes`, `-y`| Skip the confirmation prompt.                                          |

The flow: GET the GitHub "latest release" API → download the
`kungfu_<version>_<os>_<arch>.tar.gz` for your platform + the matching
checksums file → verify sha256 → extract → smoke-test the new binary →
atomically rename it over the running one.

The binary must be in a directory writable by your user; if it lives in
`/usr/local/bin`, you'll need to rerun under `sudo` (or reinstall to
`$HOME/.local/bin`).

Exit codes: 0 already-up-to-date or successfully updated, 2 user
declined, 3 network / I/O / checksum failure.

---

## `kungfu version`

Print build info.

```
kungfu version [--json]
```

Output includes the binary version, short commit, build date, Go version,
and platform. `--json` emits the same fields as JSON for scripts.

Exit codes: 0 always.

---

## `kungfu new`

Scaffold a new skill from a built-in template.

```
kungfu new <name>
  [--template basic|document|data|api-wrapper]
  [--description "..."]
  [--dir <parent>]
  [--yes] [--force]
```

| Flag             | Default                                | Notes                                                             |
| ---------------- | -------------------------------------- | ----------------------------------------------------------------- |
| `--template`     | prompt; `basic` if `--yes`             | Pick a starting point.                                            |
| `--description`  | prompt                                 | Trigger condition; `Use this skill when ` is prepended.           |
| `--dir`          | current working directory              | Parent directory; the skill goes at `<dir>/<name>`.                |
| `--yes`, `-y`    | false                                  | Skip prompts. Requires `--description`.                            |
| `--force`        | false                                  | Overwrite an existing destination.                                 |

The scaffold is guaranteed to pass `kungfu lint` cleanly. A self-lint runs
after Apply; any errors mean a template regression and cause exit 4.

Exit codes: 0 success, 1 invalid input, 2 destination collision, 3 I/O
failure, 4 internal self-lint failure.

Examples:

```sh
kungfu new csv-formatter
kungfu new --yes --template api-wrapper --description "the user wants to call an HTTP API" weather-api
```

---

## `kungfu lint`

Validate a skill directory against the rule set.

```
kungfu lint <path> [--strict] [--json] [--fix]
```

Rules ship with stable, grep-able IDs (`category/kebab-name`) so you can pin
fixes in scripts. Errors block install; warnings are advisory.

| Category      | Rule IDs                                                                                                                                                  |
| ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `frontmatter` | `missing`, `malformed`, `name-missing`, `name-mismatch`, `name-format`, `description-missing`, `description-too-long`, `allowed-tools-type`               |
| `description` | `no-trigger-phrase`, `vague`                                                                                                                              |
| `body`        | `empty`                                                                                                                                                   |
| `references`  | `broken`                                                                                                                                                  |
| `filenames`   | `non-ascii`                                                                                                                                               |

| Flag        | Notes                                                                                  |
| ----------- | -------------------------------------------------------------------------------------- |
| `--strict`  | Exit non-zero on warnings as well as errors.                                            |
| `--json`    | Emit diagnostics as JSON: `{path, diagnostics: [...], summary: {errors, warnings}}`.   |
| `--fix`     | Trim trailing whitespace and re-serialize frontmatter cleanly. Structural fixes are out of scope. |

Exit codes: 0 clean (or warnings without `--strict`), 1 errors, 2 warnings
under `--strict`, 3 I/O failure.

---

## `kungfu install`

Install a skill into one or more targets. The source may be a local
directory or a GitHub reference.

```
kungfu install <source>
  [--target ...] [--scope ...]
  [--ref <ref>] [--no-cache]
  [--force] [--dry-run] [--no-lint] [--yes]
```

Accepted source forms:

| Source                                                  | Meaning                                            |
| ------------------------------------------------------- | -------------------------------------------------- |
| `./my-skill` or `/abs/path/skill`                       | Local directory containing a SKILL.md.             |
| `user/repo`                                             | GitHub default branch.                             |
| `user/repo@v1.0.0`                                      | Tag (or branch / short SHA / full SHA).            |
| `user/repo/path/to/skill[@ref]`                         | Subdirectory inside the repo.                      |
| `github.com/user/repo[…]` or `https://github.com/...`   | Same forms with explicit host.                     |

| Flag         | Notes                                                                                  |
| ------------ | -------------------------------------------------------------------------------------- |
| `--ref`      | Override the ref from a GitHub source. Flag wins over `@suffix`.                       |
| `--no-cache` | Skip the tarball cache; refetch from GitHub.                                            |
| `--force`    | Overwrite an existing destination.                                                      |
| `--dry-run`  | Print planned actions, do nothing.                                                      |
| `--no-lint`  | Skip the pre-install lint pass.                                                         |
| `--yes`      | Skip the pre-install confirmation prompt (remote installs).                             |

Remote installs append provenance frontmatter to the destination so a later
`kungfu update` can re-fetch it; local installs do not. Tarballs are cached
at `$XDG_CACHE_HOME/kungfu/tarballs/` for 7 days.

When a repo is a *skill collection* (multiple `SKILL.md` files in nested
directories), install auto-picks the nested skill if exactly one is
present; it errors with a list and the subpath syntax suggestion when
there are several.

Exit codes: 0 success, 1 lint failure or every target unsupported, 2
destination collision without `--force`, 3 partial / total I/O failure, 5
network or tarball failure, 6 unrecognised source, 7 extracted source has
no SKILL.md.

Examples:

```sh
kungfu install ./csv-formatter --target claude,codex
kungfu install user/repo@v1.0.0 --target all
kungfu install user/repo --no-cache
kungfu install user/repo/skills/csv --target claude --dry-run
```

---

## `kungfu list`

List installed skills across configured targets.

```
kungfu list [--target ...] [--scope ...] [--long|-l] [--json]
```

| Flag       | Notes                                                          |
| ---------- | -------------------------------------------------------------- |
| `--long`   | Include a description column (truncated to the terminal width). |
| `--json`   | Emit an array of `{name, target, scope, path, description, allowed_tools, has_errors}`. |

Empty `--target` means *every* configured target (list is for discovery, not
just defaults). Broken skills (those failing lint) get a leading marker:
`⚠` in colour mode, `[!]` under `--no-color`. Sorted by (name, target, scope).

Exit codes: 0 always (2 on I/O error).

---

## `kungfu show`

Print a skill's metadata + body.

```
kungfu show <name> [--target ...] [--scope ...] [--raw] [--path]
```

| Flag     | Notes                                                                                |
| -------- | ------------------------------------------------------------------------------------ |
| `--raw`  | Print SKILL.md verbatim.                                                              |
| `--path` | Print only the absolute path (useful as ``cd "$(kungfu show foo --path)"``).        |

When a name resolves to multiple targets, show errors with a disambiguation
list rather than picking arbitrarily; pass `--target` to choose.

The default output renders the markdown body via
[glamour](https://github.com/charmbracelet/glamour) when stdout is a
terminal; under `--no-color` or non-TTY it falls back to raw text.

Exit codes: 0 success, 1 not found or ambiguous, 2 I/O failure.

---

## `kungfu remove`

Remove a skill from one or more targets.

```
kungfu remove <name> [--target ...] [--scope ...] [--yes|-y] [--dry-run]
```

Searches every matching `(target, scope, directory)` and removes the skill
from each. The default behaviour is interactive: it lists the matches and
prompts before deleting. Pass `--yes` to skip the prompt.

Exit codes: 0 success, 1 not found, 2 user declined, 3 partial / total I/O
failure.

---

## `kungfu update`

Re-fetch a previously-installed skill using its provenance frontmatter and
re-install it.

```
kungfu update [<name>]
  [--target ...] [--scope ...]
  [--ref <ref>]
  [--all] [--dry-run] [--yes|-y]
```

| Flag       | Notes                                                                                                            |
| ---------- | ---------------------------------------------------------------------------------------------------------------- |
| `--all`    | Update every updatable skill across all matching locations. Required if no `<name>` is given.                     |
| `--ref`    | Override the ref to fetch. Empty re-fetches each skill's stored `kungfu_ref` (e.g. "main" gets fresh content).   |
| `--dry-run`| Print the plan, change nothing.                                                                                   |
| `--yes`    | Skip the confirmation prompt.                                                                                     |

Only skills with `kungfu_source` provenance are updatable; local installs
and `kungfu new` scaffolds are skipped. Resolves each ref through GitHub,
compares to the stored SHA, and re-installs the ones that have moved using
the same atomic copy + backup flow as `install`.

Exit codes: 0 success or everything up to date, 1 invalid invocation (no
name + no `--all`, or named skill has no provenance), 2 user declined, 3
partial / total update failure.

Examples:

```sh
# Refresh every skill that tracks a moving ref:
kungfu update --all

# Update one skill:
kungfu update csv-formatter

# Bump a pinned skill to a newer tag:
kungfu update csv-formatter --ref v2.0.0

# Preview without changing anything:
kungfu update --all --dry-run
```
