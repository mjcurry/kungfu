# The SKILL.md format

A **skill** is a directory that teaches an agent a single, well-scoped
capability. The directory is identified by a `SKILL.md` file at its root: a
Markdown document with a YAML *frontmatter* block describing the skill,
followed by a Markdown *body* containing the instructions the agent reads when
the skill is invoked.

Skills follow a progressive-disclosure model: an agent first sees only the
`name` and `description`, and reads the body (and any bundled files) only once
it decides the skill is relevant.

## Directory layout

```
my-skill/
├── SKILL.md       # required — the skill definition
├── scripts/       # optional — executable helpers the skill can run
├── references/    # optional — longer reference material to read on demand
└── assets/        # optional — templates, fixtures, and other static files
```

Only `SKILL.md` is required. The optional subdirectories are a convention for
organizing supporting material; `kungfu` does not require or police them.

## Frontmatter

The file must begin with a YAML frontmatter block fenced by `---` lines:

| Field           | Required | Type       | Description                                                                 |
| --------------- | -------- | ---------- | --------------------------------------------------------------------------- |
| `name`          | yes      | string     | Unique identifier for the skill. Conventionally kebab-case.                 |
| `description`   | yes      | string     | When the skill should be used — written as trigger conditions for an agent. |
| `allowed-tools` | no       | list of strings | Restricts the tools the skill may use. Omit to allow all.              |

Unrecognized fields are permitted and preserved: `kungfu` round-trips a skill
without discarding frontmatter it does not model, so you can add your own
metadata safely.

### Provenance fields (set by `kungfu install <github-source>`)

When you install a skill from a remote source, `kungfu install` appends
four fields to the destination's frontmatter so `kungfu update` can later
re-fetch it. Skills you scaffold with `kungfu new` or install locally do
not have these fields.

| Field                 | Type   | Description                                                                                  |
| --------------------- | ------ | -------------------------------------------------------------------------------------------- |
| `kungfu_source`       | string | Canonical source path, e.g. `github.com/user/repo` or `github.com/user/repo/subpath`.        |
| `kungfu_ref`          | string | The user-supplied ref the skill was installed at (`v1.0.0`, `main`, a short SHA, …).         |
| `kungfu_sha`          | string | The 40-character commit SHA the install was pinned to.                                       |
| `kungfu_installed_at` | string | RFC 3339 timestamp recording when the install happened (UTC).                                |

`kungfu` writes these with quoted YAML strings; if you ever hand-edit them,
keep the values strings (no bare timestamps, no unquoted 40-digit numbers).
The `kungfu_` prefix is reserved — other tools should pick a different
namespace so the two can coexist.

## Complete example

```markdown
---
name: pdf-extract
description: Use this skill when the user wants to extract text or tables from a PDF file.
allowed-tools:
  - Read
  - Bash
---

# PDF Extract

Extract structured content from a PDF.

## Steps

1. Confirm the input path points to a `.pdf` file.
2. Run `scripts/extract.py <input> <output>`.
3. Summarize what was extracted for the user.
```

## Rules and errors

`kungfu` rejects a `SKILL.md` that:

- does not start with a `---` frontmatter fence;
- has an opening `---` with no matching closing `---`;
- contains frontmatter that is not valid YAML, or is not a mapping;
- is missing the required `name` field.

A directory is considered a skill only if it directly contains a `SKILL.md`.
When discovering skills in a tree, `kungfu` scans one level deep and skips
hidden directories (those whose name begins with `.`).
