package target

import (
	"errors"
	"fmt"
	"strings"
)

// Location is a resolved (target, scope, directory) tuple. It is the unit
// commands iterate over once flags have been parsed.
type Location struct {
	// Target is the target this location belongs to.
	Target Target
	// Scope is the scope (personal or project) this location represents.
	Scope Scope
	// Dir is the absolute directory holding skills for this combination.
	Dir string
}

// Locations resolves the (target, scope, dir) tuples for each of the given
// targets at the given scope.
//
// When a target cannot satisfy scope (e.g. cursor + personal) the
// combination is skipped: onSkip is invoked with the target and a
// human-readable reason. If onSkip is nil, the first skip becomes an error
// instead.
//
// projectRoot is required when scope is ScopeProject and ignored otherwise.
func Locations(targets []Target, scope Scope, projectRoot string, onSkip func(t Target, reason string)) ([]Location, error) {
	if !scope.IsValid() {
		return nil, fmt.Errorf("target: invalid scope %q", scope)
	}
	out := make([]Location, 0, len(targets))
	for _, t := range targets {
		dir, err := t.Dir(scope, projectRoot)
		if err != nil {
			if onSkip == nil {
				return nil, err
			}
			onSkip(t, scopeSkipReason(err))
			continue
		}
		out = append(out, Location{Target: t, Scope: scope, Dir: dir})
	}
	return out, nil
}

// scopeSkipReason produces a short human-readable explanation extracted
// from a Dir() error.
func scopeSkipReason(err error) string {
	msg := err.Error()
	if i := strings.LastIndex(msg, ":"); i >= 0 {
		msg = strings.TrimSpace(msg[i+1:])
	}
	if msg == "" {
		return "unsupported scope"
	}
	return msg
}

// ErrNoSupportedLocations is returned by helpers that compute the effective
// set of locations to operate on when every (target, scope) combination was
// skipped as unsupported.
var ErrNoSupportedLocations = errors.New("target: no supported (target, scope) combinations")
