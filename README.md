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
kungfu install ./my-skill --target claude,codex,cursor,copilot
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
kungfu list                                   # list installed skills (every target)
kungfu install <source> [--target ...]        # install to one or more targets
kungfu remove  <name>   [--target ...]        # remove from matching targets
kungfu show    <name>   [--target ...]        # print a skill (markdown-rendered)
kungfu lint    <path>                         # validate against the rule set
kungfu version                                # print build info
```

Run `kungfu <command> --help` for the full flag listing.

## Quick start

```sh
make build                                    # → ./bin/kungfu

./bin/kungfu lint ./my-skill                  # check before shipping
./bin/kungfu install ./my-skill --target all  # claude + codex + cursor + copilot
./bin/kungfu list                             # see what's installed where
./bin/kungfu show my-skill --target claude    # disambiguate with --target
```

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
