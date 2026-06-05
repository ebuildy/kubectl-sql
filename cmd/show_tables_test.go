package cmd

import (
	"strings"
	"testing"
)

func TestShowTablesDetection(t *testing.T) {
	cases := []struct {
		input    string
		detected bool
	}{
		{"SHOW TABLES", true},
		{"show tables", true},
		{"Show Tables", true},
		{"  SHOW TABLES  ", true},
		{"SELECT name FROM pods", false},
		{"SHOW TABLES extra", false},
	}
	for _, tc := range cases {
		got := strings.EqualFold(strings.TrimSpace(tc.input), "show tables")
		if got != tc.detected {
			t.Errorf("input %q: expected detected=%v got=%v", tc.input, tc.detected, got)
		}
	}
}
