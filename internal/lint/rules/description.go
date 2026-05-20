package rules

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/mjcurry/kungfu/internal/skill"
)

// triggerPhrases lists the leading phrases the linter accepts at the start
// of a description. Match is case-insensitive on the first
// triggerPrefixWindow characters after trimming leading whitespace.
var triggerPhrases = []string{
	"use this skill when",
	"use when",
	"apply when",
	"invoke when",
	"use this to",
}

const triggerPrefixWindow = 80

// vagueTokens are tokens that suggest the description is too non-specific.
// Match is case-insensitive on whole-word boundaries. The list lives at the
// top of this file so it is easy to tune from a single place.
var vagueTokens = []string{
	"various",
	"many things",
	"general",
	"anything",
	"all kinds",
	"whenever needed",
}

// minDomainTerms is the number of capitalized words we want to see in a
// description before we accept it as specific enough to override a vague
// token match.
const minDomainTerms = 3

// DescriptionNoTriggerPhrase warns when the description does not open with a
// recognised trigger phrase such as "Use this skill when ...".
type DescriptionNoTriggerPhrase struct{}

func (DescriptionNoTriggerPhrase) ID() string { return "description/no-trigger-phrase" }

func (DescriptionNoTriggerPhrase) Check(s *skill.Skill) []Diagnostic {
	desc := strings.TrimSpace(s.Description)
	if desc == "" {
		return nil // covered by FrontmatterDescriptionMissing
	}
	window := desc
	if len(window) > triggerPrefixWindow {
		window = window[:triggerPrefixWindow]
	}
	lower := strings.ToLower(window)
	for _, p := range triggerPhrases {
		if strings.HasPrefix(lower, p) {
			return nil
		}
	}
	return []Diagnostic{{
		Path:     skillFile(s.Dir),
		Severity: SeverityWarning,
		Rule:     "description/no-trigger-phrase",
		Message:  `description should start with "Use this skill when..." or a similar trigger phrase`,
	}}
}

// DescriptionVague warns when the description uses vague language and lacks
// specific domain terms.
type DescriptionVague struct{}

func (DescriptionVague) ID() string { return "description/vague" }

func (DescriptionVague) Check(s *skill.Skill) []Diagnostic {
	desc := strings.TrimSpace(s.Description)
	if desc == "" {
		return nil
	}
	hit := matchingVagueToken(desc)
	if hit == "" {
		return nil
	}
	if countDomainTerms(desc) >= minDomainTerms {
		return nil
	}
	return []Diagnostic{{
		Path:     skillFile(s.Dir),
		Severity: SeverityWarning,
		Rule:     "description/vague",
		Message: "description contains vague language (\"" + hit +
			"\") and few specific terms; consider naming the concrete inputs or outputs",
	}}
}

// matchingVagueToken returns the first vague token whose whole-word
// occurrence is found in desc (case-insensitive), or "".
func matchingVagueToken(desc string) string {
	lower := strings.ToLower(desc)
	for _, tok := range vagueTokens {
		if wholeWordIndex(lower, tok) >= 0 {
			return tok
		}
	}
	return ""
}

// wholeWordIndex returns the byte index of the first whole-word occurrence
// of needle in haystack, or -1.
func wholeWordIndex(haystack, needle string) int {
	start := 0
	for {
		idx := strings.Index(haystack[start:], needle)
		if idx < 0 {
			return -1
		}
		abs := start + idx
		if isWordBoundary(haystack, abs-1) && isWordBoundary(haystack, abs+len(needle)) {
			return abs
		}
		start = abs + 1
	}
}

func isWordBoundary(s string, pos int) bool {
	if pos < 0 || pos >= len(s) {
		return true
	}
	r := rune(s[pos])
	return !(unicode.IsLetter(r) || unicode.IsDigit(r))
}

// domainTermRE matches capitalized tokens of length >= 2 starting with an
// uppercase ASCII letter (PDF, JSON, Markdown, Python). It is a rough
// proxy for "domain-specific term" without trying to do POS tagging.
var domainTermRE = regexp.MustCompile(`\b[A-Z][A-Za-z0-9]+\b`)

// countDomainTerms counts capitalized words in desc, discounting the very
// first word so a normally-capitalized sentence opener does not inflate
// the count.
func countDomainTerms(desc string) int {
	matches := domainTermRE.FindAllStringIndex(desc, -1)
	if len(matches) == 0 {
		return 0
	}
	first := matches[0]
	prefix := strings.TrimSpace(desc[:first[0]])
	if prefix == "" {
		return len(matches) - 1
	}
	return len(matches)
}
