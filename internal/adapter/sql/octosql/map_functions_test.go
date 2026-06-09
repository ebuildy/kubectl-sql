package octosql

import (
	"testing"

	"github.com/cube2222/octosql/octosql"
)

func TestMapGet(t *testing.T) {
	d := mapGetFunction().Descriptors[0]
	mapVal := octosql.NewString(`{"app":"nginx","tier":"web"}`)

	got, err := d.Function([]octosql.Value{mapVal, octosql.NewString("app")})
	if err != nil {
		t.Fatal(err)
	}
	if got.TypeID != octosql.TypeIDString || got.Str != "nginx" {
		t.Errorf(`map_get(map,"app") = %v, want "nginx"`, got)
	}

	missing, _ := d.Function([]octosql.Value{mapVal, octosql.NewString("absent")})
	if missing.TypeID != octosql.TypeIDNull {
		t.Errorf("map_get(map,absent) = %v, want NULL", missing)
	}

	notMap, _ := d.Function([]octosql.Value{octosql.NewString("plain"), octosql.NewString("app")})
	if notMap.TypeID != octosql.TypeIDNull {
		t.Errorf("map_get(non-map,...) = %v, want NULL", notMap)
	}
}

func TestLength_JSONMapString(t *testing.T) {
	mapVal := octosql.NewString(`{"app":"nginx","tier":"web"}`)
	if got := callLength(t, mapVal); got != 2 {
		t.Errorf("length(JSON map) = %d, want 2 (keys)", got)
	}
	// A plain string still counts characters.
	if got := callLength(t, octosql.NewString("nginx")); got != 5 {
		t.Errorf("length(plain string) = %d, want 5", got)
	}
}

func TestKeys_JSONMapString(t *testing.T) {
	out := callStringFnKeys(t, octosql.NewString(`{"tier":"web","app":"nginx"}`))
	got := make([]string, len(out.List))
	for i, e := range out.List {
		got[i] = e.Str
	}
	// keys are returned sorted
	want := []string{"app", "tier"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("keys(JSON map) = %v, want %v (sorted)", got, want)
	}
}

func TestContains_JSONMapString(t *testing.T) {
	mapVal := octosql.NewString(`{"app":"nginx","tier":"web"}`)
	if !callContains(t, mapVal, octosql.NewString("nginx")) {
		t.Error(`contains(map, "nginx") = false, want true (value match)`)
	}
	if callContains(t, mapVal, octosql.NewString("app")) {
		t.Error(`contains(map, "app") = true, want false (keys are not matched, values are)`)
	}
	// Plain string falls back to substring.
	if !callContains(t, octosql.NewString("nginx-ingress"), octosql.NewString("ingress")) {
		t.Error(`contains("nginx-ingress","ingress") = false, want true`)
	}
}

// callStringFnKeys runs the keys() String descriptor on a value.
func callStringFnKeys(t *testing.T, v octosql.Value) octosql.Value {
	t.Helper()
	for _, d := range keysFunction().Descriptors {
		if d.TypeFn == nil {
			continue
		}
		if _, ok := d.TypeFn([]octosql.Type{{TypeID: octosql.TypeIDString}}); !ok {
			continue
		}
		out, err := d.Function([]octosql.Value{v})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	t.Fatal("no keys String descriptor matched")
	return octosql.Value{}
}
