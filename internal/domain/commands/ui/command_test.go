package ui

import (
	"net"
	"reflect"
	"testing"
)

func TestBrowserURL(t *testing.T) {
	cases := []struct {
		name  string
		addr  net.Addr
		query string
		want  string
	}{
		{
			name: "loopback no query",
			addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080},
			want: "http://127.0.0.1:8080/",
		},
		{
			name:  "loopback with query is url-encoded",
			addr:  &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8080},
			query: "SELECT name FROM pods WHERE status->phase = 'Running'",
			want:  "http://127.0.0.1:8080/?sql=SELECT+name+FROM+pods+WHERE+status-%3Ephase+%3D+%27Running%27",
		},
		{
			name: "unspecified host rewritten to loopback",
			addr: &net.TCPAddr{IP: net.IPv4zero, Port: 9090},
			want: "http://127.0.0.1:9090/",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := browserURL(tc.addr, tc.query); got != tc.want {
				t.Fatalf("browserURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// fakeCompletion is a ShellCompletionRunner returning canned readline-style
// suffix candidates and the typed-word length, so we can assert the wrapper
// reconstructs full tokens.
type fakeCompletion struct {
	suffixes   [][]rune
	length     int
	prefetched string
}

func (f *fakeCompletion) Prefetch(query string) { f.prefetched = query }

func (f *fakeCompletion) Do(_ []rune, _ int) ([][]rune, int) {
	return f.suffixes, f.length
}

func TestComplete_ReconstructsFullTokens(t *testing.T) {
	fc := &fakeCompletion{
		suffixes: [][]rune{[]rune("ds"), []rune("dtemplates")},
		length:   2, // the typed word "po"
	}
	c := &UICommand{completion: fc}

	line := "SELECT name FROM po"
	got := c.Complete(line, len([]rune(line)))

	want := []string{"pods", "podtemplates"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Complete() = %v, want %v", got, want)
	}
	if fc.prefetched != line {
		t.Fatalf("Prefetch called with %q, want %q", fc.prefetched, line)
	}
}

func TestComplete_NoCandidates(t *testing.T) {
	c := &UICommand{completion: &fakeCompletion{suffixes: nil, length: 0}}
	if got := c.Complete("SELECT", 6); len(got) != 0 {
		t.Fatalf("Complete() = %v, want empty", got)
	}
}

func TestComplete_NilCompletionDisabled(t *testing.T) {
	c := &UICommand{completion: nil}
	if got := c.Complete("SELECT", 6); got != nil {
		t.Fatalf("Complete() = %v, want nil", got)
	}
}

func TestColumnsOf_SortedUnion(t *testing.T) {
	rows := []map[string]any{
		{"name": "a", "namespace": "x"},
		{"name": "b", "phase": "Running"},
	}
	got := columnsOf(rows)
	want := []string{"name", "namespace", "phase"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("columnsOf() = %v, want %v", got, want)
	}
}
