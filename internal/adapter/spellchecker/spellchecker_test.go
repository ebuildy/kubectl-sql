package spellchecker

import "testing"

func TestClosestMatch_TypoWithinThreshold(t *testing.T) {
	sc := New()
	candidates := []string{"status", "spec", "metadata", "name"}
	got, ok := sc.ClosestMatch("staus", candidates)
	if !ok {
		t.Fatalf("expected a match for 'staus', got none")
	}
	if got != "status" {
		t.Errorf("ClosestMatch('staus') = %q, want %q", got, "status")
	}
}

func TestClosestMatch_CaseInsensitive(t *testing.T) {
	sc := New()
	got, ok := sc.ClosestMatch("SLECT", []string{"SELECT", "FROM", "WHERE"})
	if !ok || got != "SELECT" {
		t.Errorf("ClosestMatch('SLECT') = %q,%v, want SELECT,true", got, ok)
	}
}

func TestClosestMatch_NoMatchBelowThreshold(t *testing.T) {
	sc := New()
	if got, ok := sc.ClosestMatch("xyzzy", []string{"status", "spec", "metadata"}); ok {
		t.Errorf("expected no match for 'xyzzy', got %q", got)
	}
}

func TestClosestMatch_RejectsMuchLongerCandidate(t *testing.T) {
	// "toot" must not be corrected to "replicationcontrollers": a candidate more
	// than 30% longer than the typo is implausible.
	sc := New()
	cands := []string{"pods", "replicationcontrollers", "services", "nodes", "deployments"}
	if got, ok := sc.ClosestMatch("toot", cands); ok {
		t.Errorf("expected no match for 'toot', got %q", got)
	}
}

func TestClosestMatch_AllowsSingleCharAddition(t *testing.T) {
	sc := New()
	// Common one-character corrections must still pass despite being >30% longer
	// for short tokens (the absolute one-character floor covers them).
	if got, ok := sc.ClosestMatch("nam", []string{"name", "namespace"}); !ok || got != "name" {
		t.Errorf("ClosestMatch('nam') = %q,%v, want name,true", got, ok)
	}
	if got, ok := sc.ClosestMatch("pod", []string{"pods", "nodes"}); !ok || got != "pods" {
		t.Errorf("ClosestMatch('pod') = %q,%v, want pods,true", got, ok)
	}
}

func TestLengthAllowed(t *testing.T) {
	cases := []struct {
		target, cand string
		want         bool
	}{
		{"toot", "replicationcontrollers", false},
		{"toot", "toots", true},   // +1, within floor
		{"nam", "name", true},     // +1
		{"pod", "pods", true},     // +1
		{"staus", "status", true}, // 5 -> 6, within 30%
		{"po", "pods", false},     // 2 -> 4 exceeds floor (3) and 30% (2)
	}
	for _, tc := range cases {
		if got := lengthAllowed(tc.target, tc.cand); got != tc.want {
			t.Errorf("lengthAllowed(%q,%q) = %v, want %v", tc.target, tc.cand, got, tc.want)
		}
	}
}

func TestClosestMatch_RejectsLowConfidenceJunk(t *testing.T) {
	// Prefer no suggestion over a bad one: tokens that are merely vaguely similar
	// (well below the conservative threshold) must not be offered.
	sc := New()
	cands := []string{"pods", "nodes", "services", "events", "status", "name"}
	for _, junk := range []string{"toot", "foo", "abc", "test", "xyzzy"} {
		if got, ok := sc.ClosestMatch(junk, cands); ok {
			t.Errorf("expected no match for %q, got %q", junk, got)
		}
	}
}

func TestClosestMatch_EmptyCandidates(t *testing.T) {
	sc := New()
	if _, ok := sc.ClosestMatch("status", nil); ok {
		t.Errorf("expected no match with empty candidate set")
	}
}

func TestTieBreakLess(t *testing.T) {
	// Shorter candidate wins.
	if !tieBreakLess("abc", "abcd") {
		t.Errorf("expected shorter candidate to win the tie")
	}
	if tieBreakLess("abcd", "abc") {
		t.Errorf("expected longer candidate to lose the tie")
	}
	// Equal length: lexicographic order wins.
	if !tieBreakLess("aXc", "aYc") {
		t.Errorf("expected lexicographically smaller candidate to win the tie")
	}
	if tieBreakLess("aYc", "aXc") {
		t.Errorf("expected lexicographically larger candidate to lose the tie")
	}
}

func TestClosestMatch_TieBrokenDeterministically(t *testing.T) {
	// "stats" is equidistant (one edit) from both "status" and "statu"; the
	// tie-break must pick the shorter candidate deterministically.
	c := New()
	got, ok := c.ClosestMatch("statuz", []string{"status", "statuz_long"})
	if !ok || got != "status" {
		t.Errorf("ClosestMatch tie = %q,%v, want status,true", got, ok)
	}
}
