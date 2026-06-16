// Package spellchecker is the strutil-backed adapter for the spell-checking
// port (internal/port/spellchecker). It is the ONLY package in the repository
// that imports github.com/adrg/strutil; all other code depends on the port, so
// the similarity metric and threshold can be retuned (or the library swapped)
// without touching call sites.
package spellchecker

import (
	"github.com/adrg/strutil"
	"github.com/adrg/strutil/metrics"

	"github.com/ebuildy/kubectl-sql/internal/port/spellchecker"
)

// threshold is the minimum Jaro-Winkler similarity a candidate must reach to be
// offered as a correction. It is deliberately conservative: a wrong suggestion
// is worse than none, so we only correct high-confidence single-typo cases. In
// practice genuine single-mistyped-identifier corrections (transposition, single
// substitution/insertion/deletion) score >= 0.89, while unrelated tokens score
// well below 0.65 — 0.85 sits comfortably in that gap. It is a private adapter
// detail, covered by the adapter's unit tests, so it can be retuned without
// affecting callers.
const threshold = 0.85

// jaroWinkler is a case-insensitive Jaro-Winkler metric. It rewards a shared
// prefix, which suits keyword/identifier typos where the first letters are
// usually correct.
type jaroWinklerChecker struct {
	metric *metrics.JaroWinkler
}

// New returns a strutil-backed SpellChecker using the case-insensitive
// Jaro-Winkler metric.
func New() spellchecker.SpellChecker {
	m := metrics.NewJaroWinkler()
	m.CaseSensitive = false
	return &jaroWinklerChecker{metric: m}
}

// ClosestMatch returns the candidate most similar to target whose score is at
// least the threshold. Ties in score are broken deterministically: the shortest
// candidate wins, then lexicographic order. Returns "", false when no candidate
// clears the threshold.
func (c *jaroWinklerChecker) ClosestMatch(target string, candidates []string) (string, bool) {
	best := ""
	bestScore := 0.0
	found := false
	for _, cand := range candidates {
		if !lengthAllowed(target, cand) {
			continue
		}
		score := strutil.Similarity(target, cand, c.metric)
		if score < threshold {
			continue
		}
		if !found || score > bestScore || (score == bestScore && tieBreakLess(cand, best)) {
			best = cand
			bestScore = score
			found = true
		}
	}
	return best, found
}

// lengthAllowed reports whether candidate is close enough in length to target
// to be a plausible correction. A correction should be roughly the same length
// as the typo, never a wildly longer unrelated token (e.g. "toot" must not match
// "replicationcontrollers"). The cap is 30% longer than the target, but always
// at least one character longer so common single-character additions (e.g.
// "nam" -> "name", "pod" -> "pods") still pass. Length is measured in runes.
func lengthAllowed(target, candidate string) bool {
	t := len([]rune(target))
	maxLen := t + t*30/100
	if maxLen < t+1 {
		maxLen = t + 1
	}
	return len([]rune(candidate)) <= maxLen
}

// tieBreakLess reports whether candidate a should win a score tie over b:
// the shorter candidate wins, then lexicographic order.
func tieBreakLess(a, b string) bool {
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}
