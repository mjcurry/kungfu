// Package lint orchestrates a set of Rules against a skill and aggregates
// the resulting Diagnostics into a Report.
//
// Diagnostic, Severity, and Rule are defined in the rules subpackage to
// avoid an import cycle (rule implementations need to refer to Diagnostic,
// and the lint package needs to refer to Rule). This file re-exports them
// as type aliases so callers can use lint.Diagnostic and rules.Diagnostic
// interchangeably.
package lint

import "github.com/mjcurry/kungfu/internal/lint/rules"

// Diagnostic is a single finding emitted by a Rule. Alias of rules.Diagnostic.
type Diagnostic = rules.Diagnostic

// Severity describes how serious a diagnostic is. Alias of rules.Severity.
type Severity = rules.Severity

// Rule inspects a skill and returns any problems it finds. Alias of
// rules.Rule.
type Rule = rules.Rule

// Severity constants re-exported for ergonomic access.
const (
	SeverityWarning = rules.SeverityWarning
	SeverityError   = rules.SeverityError
)
