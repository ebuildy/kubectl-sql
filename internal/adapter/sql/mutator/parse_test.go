package mutator

import (
	"strings"
	"testing"

	k8s "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
)

func gp(n int64) *int64 { return &n }

func TestParseDelete(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		wantRes  string
		wantTail string
		wantOpts k8s.DeleteOptions
		wantErr  bool
	}{
		{
			name:     "with WHERE no FROM",
			query:    "DELETE pod WHERE status->phase = 'Pending'",
			wantRes:  "pod",
			wantTail: "WHERE status->phase = 'Pending'",
		},
		{
			name:     "with FROM and WHERE",
			query:    "DELETE FROM pods WHERE name = 'nginx'",
			wantRes:  "pods",
			wantTail: "WHERE name = 'nginx'",
		},
		{
			name:    "no WHERE",
			query:   "DELETE pods",
			wantRes: "pods",
		},
		{
			name:     "LIMIT tail without WHERE",
			query:    "DELETE FROM pods.json LIMIT 1",
			wantRes:  "pods.json",
			wantTail: "LIMIT 1",
		},
		{
			name:     "WHERE with ORDER BY and LIMIT",
			query:    "DELETE pod WHERE status->phase = 'Pending' ORDER BY name LIMIT 5",
			wantRes:  "pod",
			wantTail: "WHERE status->phase = 'Pending' ORDER BY name LIMIT 5",
		},
		{
			name:     "force and grace-period hints",
			query:    "DELETE /* force, grace-period=0 */ FROM pod WHERE status->phase = 'Pending'",
			wantRes:  "pod",
			wantTail: "WHERE status->phase = 'Pending'",
			wantOpts: k8s.DeleteOptions{GracePeriodSeconds: gp(0)},
		},
		{
			name:     "cascade orphan hint",
			query:    "DELETE /* cascade=orphan */ deployment WHERE name = 'web'",
			wantRes:  "deployment",
			wantTail: "WHERE name = 'web'",
			wantOpts: k8s.DeleteOptions{PropagationPolicy: "Orphan"},
		},
		{
			name:    "case-insensitive keyword",
			query:   "delete FROM Pods",
			wantRes: "Pods",
		},
		{
			name:    "no resource (WHERE only) is error",
			query:   "DELETE WHERE x = 1",
			wantErr: true,
		},
		{
			name:    "unknown hint is error",
			query:   "DELETE /* bogus_option */ FROM pods",
			wantErr: true,
		},
		{
			name:    "empty after delete is error",
			query:   "DELETE",
			wantErr: true,
		},
		{
			name:    "unterminated hint comment is error",
			query:   "DELETE /* force FROM pods",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDelete(tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDelete(%q) = %+v, want error", tt.query, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDelete(%q) unexpected error: %v", tt.query, err)
			}
			if got.resource != tt.wantRes {
				t.Errorf("resource = %q, want %q", got.resource, tt.wantRes)
			}
			if got.tail != tt.wantTail {
				t.Errorf("whereTail = %q, want %q", got.tail, tt.wantTail)
			}
			if !sameOpts(got.options, tt.wantOpts) {
				t.Errorf("options = %+v, want %+v", got.options, tt.wantOpts)
			}
		})
	}
}

func TestParseDeleteHints(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    k8s.DeleteOptions
		wantErr bool
	}{
		{name: "force", body: " force ", want: k8s.DeleteOptions{GracePeriodSeconds: gp(0)}},
		{name: "grace-period", body: "grace-period=30", want: k8s.DeleteOptions{GracePeriodSeconds: gp(30)}},
		{name: "cascade background", body: "cascade=background", want: k8s.DeleteOptions{PropagationPolicy: "Background"}},
		{name: "cascade foreground case-insensitive", body: "Cascade=Foreground", want: k8s.DeleteOptions{PropagationPolicy: "Foreground"}},
		{name: "combined", body: "force, cascade=orphan", want: k8s.DeleteOptions{GracePeriodSeconds: gp(0), PropagationPolicy: "Orphan"}},
		{name: "unknown token", body: "bogus", wantErr: true},
		{name: "force with value", body: "force=1", wantErr: true},
		{name: "grace-period missing value", body: "grace-period", wantErr: true},
		{name: "grace-period bad value", body: "grace-period=abc", wantErr: true},
		{name: "cascade bad value", body: "cascade=sideways", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDeleteHints(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDeleteHints(%q) = %+v, want error", tt.body, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDeleteHints(%q) unexpected error: %v", tt.body, err)
			}
			if !sameOpts(got, tt.want) {
				t.Errorf("options = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDeleteOptionsToFlags(t *testing.T) {
	tests := []struct {
		name string
		opts k8s.DeleteOptions
		want string
	}{
		{name: "none", opts: k8s.DeleteOptions{}, want: ""},
		{name: "force/grace 0", opts: k8s.DeleteOptions{GracePeriodSeconds: gp(0)}, want: "--force --grace-period=0"},
		{name: "grace 30", opts: k8s.DeleteOptions{GracePeriodSeconds: gp(30)}, want: "--grace-period=30"},
		{name: "cascade orphan", opts: k8s.DeleteOptions{PropagationPolicy: "Orphan"}, want: "--cascade=orphan"},
		{name: "force and cascade", opts: k8s.DeleteOptions{GracePeriodSeconds: gp(0), PropagationPolicy: "Background"}, want: "--force --grace-period=0 --cascade=background"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(deleteOptionsToFlags(tt.opts), " "); got != tt.want {
				t.Errorf("deleteOptionsToFlags() = %q, want %q", got, tt.want)
			}
		})
	}
}

func sameOpts(a, b k8s.DeleteOptions) bool {
	if a.PropagationPolicy != b.PropagationPolicy {
		return false
	}
	switch {
	case a.GracePeriodSeconds == nil && b.GracePeriodSeconds == nil:
		return true
	case a.GracePeriodSeconds == nil || b.GracePeriodSeconds == nil:
		return false
	default:
		return *a.GracePeriodSeconds == *b.GracePeriodSeconds
	}
}
