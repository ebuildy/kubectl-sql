// Package spellchecker is the spell-checking port: a domain-owned interface for
// finding the closest valid candidate to a possibly-mistyped token. Application
// code (e.g. the SQL engine) depends only on this package; the concrete string
// similarity library lives behind an adapter (see internal/adapter/spellchecker)
// so it can be swapped or tuned without touching call sites, mirroring the
// logger port / zap adapter split.
package spellchecker

// SpellChecker suggests the closest valid candidate for a possibly-mistyped token.
type SpellChecker interface {
	// ClosestMatch returns the candidate most similar to target and true when a
	// candidate clears the similarity threshold; otherwise "", false. Ties are
	// broken deterministically by the implementation.
	ClosestMatch(target string, candidates []string) (string, bool)
}
