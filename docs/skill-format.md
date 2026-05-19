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
