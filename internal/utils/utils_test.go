package utils

import (
	"strings"
	"testing"
)

func TestColorizeJSONKeys(t *testing.T) {
	in := "{\n  \"phase\": \"Running\",\n  \"message\": \"weird \\\" : value\"\n}"
	out := ColorizeJSONKeys(in)
	if got := strings.Count(out, AnsiCyan); got != 2 {
		t.Errorf("expected exactly 2 colored keys, got %d: %q", got, out)
	}
	if !strings.Contains(out, AnsiCyan+`"phase"`+AnsiReset+":") {
		t.Errorf("key not colored: %q", out)
	}
	if !strings.Contains(out, `"weird \" : value"`) {
		t.Errorf("value must stay uncolored and unmodified: %q", out)
	}
}
