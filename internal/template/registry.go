// Package template renders the built-in skill scaffolds shipped with
// kungfu. Each scaffold lives under files/<name>/; files ending in .tmpl
// pass through text/template, others are copied verbatim. Rendered file
// permissions match what skill.FileMode prescribes so a scaffolded skill
// is byte-identical (modes included) to one fetched from a remote source.
package template

import "embed"

// templateFS embeds every built-in template directory.
//
//go:embed all:files
var templateFS embed.FS

// templatesRoot is the directory inside templateFS that holds named template
// subdirectories.
const templatesRoot = "files"
