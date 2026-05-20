<p align="center">
  <img src="loading-skills.gif" alt="Tank loading skills into Neo" width="520">
</p>

<h1 align="center">kungfu</h1>

<p align="center">
  <i>The package manager for your agent skills. One CLI, every agent.</i>
</p>

<p align="center">
  <a href="https://github.com/mjcurry/kungfu/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/mjcurry/kungfu/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://github.com/mjcurry/kungfu/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/mjcurry/kungfu?display_name=tag&sort=semver"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/github/license/mjcurry/kungfu"></a>
  <a href="go.mod"><img alt="Go" src="https://img.shields.io/github/go-mod/go-version/mjcurry/kungfu"></a>
</p>

`kungfu` installs, lints, scaffolds, and updates [Agent Skills](https://agentskills.io)
— `SKILL.md` directories that teach AI agents a new capability via the
progressive-disclosure pattern. One CLI manages the same skill across every
agent on your machine: **Claude, Codex, Cursor, and Copilot**. Skills you
install from GitHub carry provenance so a later `kungfu update` brings them
back into sync with one command.

## Demo

<!-- TODO: record an asciinema cast of new → lint → install → list → update
     and embed it here. See docs/release.md for the recording convention. -->

> ⏱ A ~30-second demo cast is planned for v1.0; until it lands, the
> [Quickstart](#quickstart) section walks through the same flow with copy-pastable
> commands.

## Install

### Homebrew (macOS, Linux)

```sh
brew install mjcurry/kungfu/kungfu
```

### curl | sh (macOS, Linux)

```sh
curl -fsSL https://raw.githubusercontent.com/mjcurry/kungfu/main/install.sh | sh
```

The script detects your OS / arch, downloads the matching archive from the
[latest release](https://github.com/mjcurry/kungfu/releases/latest), verifies
its sha256 checksum, and drops the binary into `/usr/local/bin` (or
`$HOME/.local/bin` if `/usr/local/bin` is read-only).

### go install

```sh
go install github.com/mjcurry/kungfu/cmd/kungfu@latest
```

### Manual download

Grab a `kungfu_<version>_<os>_<arch>.tar.gz` (or `.zip` on Windows) from the
[releases page](https://github.com/mjcurry/kungfu/releases), verify against
the matching `kungfu_<version>_checksums.txt`:

```sh
shasum -a 256 -c kungfu_<version>_checksums.txt
```

then move `kungfu` somewhere on your PATH.

## Quickstart

```sh
# 1. Scaffold a new skill from a built-in template, lint-clean by construction:
kungfu new csv-formatter
kungfu lint csv-formatter

# 2. Or fetch one from GitHub:
kungfu install https://github.com/nextlevelbuilder/ui-ux-pro-max-skill \
    --target claude,codex,cursor,copilot

# 3. See what's installed where:
kungfu list

# 4. Read a skill the way an agent would:
kungfu show ui-ux-pro-max-skill

# 5. Bring everything tracking a moving ref up to date:
kungfu update --all
```

## Supported agents

| Agent     | Personal scope                  | Project scope        | Status |
| --------- | ------------------------------- | -------------------- | ------ |
| Claude    | `~/.claude/skills`              | `.claude/skills`     | ✅      |
| Codex     | `~/.codex/skills`               | `.codex/skills`      | ✅      |
| Cursor    | *(none — project scope only)*   | `.cursor/skills`     | ✅      |
| Copilot   | `~/.copilot/skills`             | `.github/skills`     | ✅      |

The format every agent reads is described in [docs/skill-format.md](docs/skill-format.md).

## Commands

| Command            | What it does                                                       |
| ------------------ | ------------------------------------------------------------------ |
| `kungfu new`       | Scaffold a new skill from a built-in template (lint-clean).        |
| `kungfu lint`      | Validate a skill against the rule set (stable, grep-able IDs).     |
| `kungfu install`   | Install from a local path or a GitHub source.                      |
| `kungfu list`      | List installed skills across configured targets.                   |
| `kungfu show`      | Print a skill's metadata + body (markdown-rendered).               |
| `kungfu remove`    | Remove a skill from one or more targets.                           |
| `kungfu update`    | Re-fetch a previously-installed skill using its stored provenance. |
| `kungfu version`   | Print build info (also `--json`).                                  |

Full flag listing, exit codes, and examples in [docs/commands.md](docs/commands.md).

## How it works

`SKILL.md` is an [open, cross-agent format](https://agentskills.io). Every
supported agent agrees on the directory shape — frontmatter on top,
progressive-disclosure markdown below, optional `scripts/`, `references/`,
`assets/` subdirectories. `kungfu` is disciplined file management over that
format, not agent-specific magic.

When you `kungfu install user/repo`, the skill is fetched from GitHub
(tarball, checksum-verified, cached at `$XDG_CACHE_HOME/kungfu/tarballs/`),
linted before it is allowed near your skills directories, then atomically
copied into each configured target via a `.tmp-<random>` → rename → cleanup
sequence that survives interruption. Four provenance fields are appended to
the installed `SKILL.md`:

```yaml
kungfu_source: github.com/user/repo
kungfu_ref: v1.0.0
kungfu_sha: a1b2c3d4e5f6…
kungfu_installed_at: 2026-05-19T03:04:05Z
```

`kungfu update` reads those back, re-resolves the ref, and re-installs when
the SHA has moved. Skills you created with `kungfu new` or installed locally
have no provenance and are skipped — they're not on a remote leash.

## Roadmap

Beyond v1:

- **More hosts.** GitLab, Bitbucket, Codeberg, and self-hosted Git via an
  http URL grammar.
- **`kungfu publish`** that pushes a skill to a tap-style index.
- **`kungfu search`** against a public skill directory.
- **`kungfu test`** that runs a skill's bundled `scripts/test.sh` (or
  language-detected runner) so reviewers don't have to.
- **`kungfu doctor`** that diagnoses common misconfigurations (missing
  targets, stale caches, broken paths).
- **`kungfu cache`** subcommands (`list`, `clear`, `verify`).
- **Recorded demo** under `docs/demo.cast`.

## Contributing

Issues and pull requests are welcome. Run the full check suite before
opening a PR:

```sh
make test     # go test ./...
make lint     # go vet + gofmt -l
make build    # ./bin/kungfu
```

Releases are tag-driven via goreleaser; see [docs/release.md](docs/release.md).

## License

[MIT](LICENSE) © Mike Curry.

<p align="center">
  <img src="i-know-kungfu.gif" alt="I know kung fu" width="320">
</p>
