//go:build integration

package integration

import (
	"regexp"
	"testing"
)

func TestRandomNameFormat(t *testing.T) {
	pattern := regexp.MustCompile(`^[a-z]+-[a-z]+-[0-9a-f]{4}$`)
	for i := 0; i < 50; i++ {
		name := randomName()
		if !pattern.MatchString(name) {
			t.Errorf("randomName() = %q, does not match pattern", name)
		}
	}
}
