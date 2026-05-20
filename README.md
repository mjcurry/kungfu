<p align="center">
  <img src="kungfu-logo.png" alt="kungfu" width="220">
</p>

# kungfu

> "I know kung fu." — the package manager for your agent skills. **One CLI, every agent.**

`kungfu` installs, lints, and manages [Agent Skills](https://agentskills.io)
— `SKILL.md` directories that teach AI agents a new capability via the
progressive-disclosure pattern. With one command, ship the same skill to
every agent on your machine:

```sh
kungfu install user/repo --target claude,codex,cursor,copilot
```

Or scaffold your own:

```sh
kungfu new my-skill                # interactive
kungfu new --yes --template data my-skill --description "..."
```

<p align="center">
  <img src="loading-skills.gif" alt="Loading skills" width="480">
</p>

## Supported targets

| Target  | Personal dir         | Project dir       |
| ------- | -------------------- | ----------------- |
| claude  | `~/.claude/skills`   | `.claude/skills`  |
| codex   | `~/.codex/skills`    | `.codex/skills`   |
| cursor  | *(none — project only)* | `.cursor/skills`  |
| copilot | `~/.copilot/skills`  | `.github/skills`  |

Custom targets and per-target overrides are configurable; see
[Configuration](#configuration).

## Commands

```
kungfu new     <name>   [--template ...]      # scaffold a new skill from a built-in template
kungfu install <source> [--target ...]        # install (local path or GitHub: user/repo[@ref][/sub])
kungfu list                                   # list installed skills (every target)
kungfu show    <name>   [--target ...]        # print a skill (markdown-rendered)
kungfu remove  <name>   [--target ...]        # remove from matching targets
kungfu lint    <path>                         # validate against the rule set
kungfu version                                # print build info
```

Run `kungfu <command> --help` for the full flag listing.

## Quick start

```sh
make build                                    # → ./bin/kungfu

# Make your own skill from a built-in template:
./bin/kungfu new my-skill                     # interactive
./bin/kungfu lint my-skill                    # scaffolds are guaranteed lint-clean

# Or fetch someone else's:
./bin/kungfu install user/repo --target all   # claude + codex + cursor + copilot
./bin/kungfu install user/repo@v1.0.0         # pin to a tag
./bin/kungfu install user/repo/skills/csv     # subpath inside a monorepo

./bin/kungfu list                             # see what's installed where
./bin/kungfu show my-skill --target claude    # disambiguate with --target
```

## Installing from GitHub

`kungfu install` accepts several source forms in addition to local paths:

| Source                                   | Meaning                                            |
| ---------------------------------------- | -------------------------------------------------- |
| `user/repo`                              | default branch                                      |
| `user/repo@v1.0.0`                       | tag (or branch / short SHA / full SHA)             |
| `user/repo/path/to/skill`                | subdirectory inside the repo                       |
| `user/repo/path/to/skill@v1.0.0`         | subdirectory at a specific ref                     |
| `github.com/user/repo[…]`                | same forms with an explicit host                   |
| `https://github.com/user/repo[…]`        | a pasted browser URL                               |

Each remote install stamps the destination's frontmatter with provenance:

```yaml
kungfu_source: github.com/user/repo
kungfu_ref: v1.0.0
kungfu_sha: a1b2c3d4e5f6…           # 40-char commit SHA
kungfu_installed_at: 2026-05-19T03:04:05Z
```

Tarballs are cached at `$XDG_CACHE_HOME/kungfu/tarballs/` for 7 days. Use
`--no-cache` to bypass, `--ref` to set the ref via flag, and `--yes` to skip
the pre-install confirmation.

## Scaffolding new skills

`kungfu new` walks you through creating a skill that will pass `kungfu lint`
cleanly on first run. Four built-in templates ship today:

| Template      | Use it for…                                                   |
| ------------- | ------------------------------------------------------------- |
| `basic`       | a minimal SKILL.md with placeholders.                         |
| `document`    | producing structured prose documents (reports, memos, etc.).  |
| `data`        | inspecting tabular data; ships a stdlib-Python helper script. |
| `api-wrapper` | calling an HTTP API behind an env-driven `curl` wrapper.      |

Interactive use prompts for template and description; pass `--yes
--template … --description …` to drive it from CI.

## Configuration

`kungfu` reads `$XDG_CONFIG_HOME/kungfu/config.toml`, falling back to
`~/.config/kungfu/config.toml`. The file is optional — defaults apply when
it is absent.

```toml
default_targets = ["claude"]
default_scope   = "personal"        # "personal" | "project"

[targets.claude]
personal_dir = "~/.claude/skills"
project_dir  = ".claude/skills"

[targets.codex]
personal_dir = "~/.codex/skills"
project_dir  = ".codex/skills"

[targets.cursor]
personal_dir = ""                   # cursor has no personal scope
project_dir  = ".cursor/skills"

[targets.copilot]
personal_dir = "~/.copilot/skills"
project_dir  = ".github/skills"
```

Each `[targets.<name>]` section is a partial override of the built-in
defaults — only set the fields you want to change. Adding a new
`[targets.<name>]` section registers a custom target.

Resolution order for the active target list: `--target` flag →
`default_targets` from config → `["claude"]`. For scope:
`--scope` flag → `default_scope` from config → `"personal"`.

## Lint

`kungfu lint` runs a set of rules with stable, grep-able IDs. Errors block
install; warnings are advisory unless `--strict` is passed.

| Category      | Rule IDs                                                                                                                                                  |
| ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `frontmatter` | `missing`, `malformed`, `name-missing`, `name-mismatch`, `name-format`, `description-missing`, `description-too-long`, `allowed-tools-type`               |
| `description` | `no-trigger-phrase`, `vague`                                                                                                                              |
| `body`        | `empty`                                                                                                                                                   |
| `references`  | `broken`                                                                                                                                                  |
| `filenames`   | `non-ascii`                                                                                                                                               |

Exit codes: `0` clean (or warnings without `--strict`), `1` errors, `2`
warnings under `--strict`, `3` I/O failure. `--json` emits machine-readable
output; `--fix` trims trailing whitespace and re-serializes the frontmatter.

<p align="center">
  <img src="i-know-kungfu.gif" alt="I know kung fu" width="320">
</p>

## Documentation

- [The SKILL.md format](docs/skill-format.md) — required and optional fields,
  directory layout, and a complete example.

## Author

[Mike Curry](https://github.com/mjcurry)

## License

[MIT](LICENSE)
