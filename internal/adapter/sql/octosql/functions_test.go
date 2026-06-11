package octosql

import (
	"testing"

	"github.com/cube2222/octosql/octosql"
	"github.com/stretchr/testify/assert"
)

// callLength invokes the length descriptor matching the given value's TypeID and
// returns the resulting int. It fails the test if no descriptor matches. It uses
// the same discriminating TypeFn that octosql uses to pick a descriptor, so a
// descriptor accidentally matching the wrong type would be caught here too.
func callLength(t *testing.T, v octosql.Value) int64 {
	t.Helper()
	argType := octosql.Type{TypeID: v.TypeID}
	details := lengthFunction()
	for _, d := range details.Descriptors {
		if d.TypeFn == nil {
			continue
		}
		if _, ok := d.TypeFn([]octosql.Type{argType}); !ok {
			continue
		}
		out, err := d.Function([]octosql.Value{v})
		if err != nil {
			t.Fatalf("length(%v) returned error: %v", v.TypeID, err)
		}
		if out.TypeID != octosql.TypeIDInt {
			t.Fatalf("length should return Int, got %v", out.TypeID)
		}
		return out.Int
	}
	t.Fatalf("no length descriptor for type %v", v.TypeID)
	return 0
}

func TestFunctionMap_RegistersLength(t *testing.T) {
	m := FunctionMap()
	if _, ok := m["length"]; !ok {
		t.Fatal("FunctionMap missing 'length'")
	}
}

func TestLength_List(t *testing.T) {
	v := octosql.NewList([]octosql.Value{octosql.NewInt(1), octosql.NewInt(2), octosql.NewInt(3)})
	if got := callLength(t, v); got != 3 {
		t.Errorf("length(list of 3) = %d, want 3", got)
	}
	if got := callLength(t, octosql.NewList(nil)); got != 0 {
		t.Errorf("length(empty list) = %d, want 0", got)
	}
}

func TestLength_Tuple(t *testing.T) {
	v := octosql.NewTuple([]octosql.Value{octosql.NewString("a"), octosql.NewString("b")})
	if got := callLength(t, v); got != 2 {
		t.Errorf("length(tuple of 2) = %d, want 2", got)
	}
}

func TestLength_Struct(t *testing.T) {
	v := octosql.NewStruct([]octosql.Value{octosql.NewInt(1), octosql.NewInt(2), octosql.NewInt(3), octosql.NewInt(4)})
	if got := callLength(t, v); got != 4 {
		t.Errorf("length(struct of 4) = %d, want 4", got)
	}
}

func TestLength_String(t *testing.T) {
	if got := callLength(t, octosql.NewString("nginx")); got != 5 {
		t.Errorf("length(\"nginx\") = %d, want 5", got)
	}
	if got := callLength(t, octosql.NewString("")); got != 0 {
		t.Errorf("length(\"\") = %d, want 0", got)
	}
	// Multi-byte: counts runes, not bytes.
	if got := callLength(t, octosql.NewString("café")); got != 4 {
		t.Errorf("length(\"café\") = %d, want 4 (runes)", got)
	}
}

func TestMap_MapGet(t *testing.T) {
	d := mapGetFunction().Descriptors[0]
	mapVal := octosql.NewString(`{"app":"nginx","tier":"web", "config.json": \"{"foo": "bar"}\"}`)

	got, err := d.Function([]octosql.Value{mapVal, octosql.NewString("app")})
	if err != nil {
		t.Fatal(err)
	}

	assert.NoError(t, err)
	assert.Equal(t, octosql.TypeIDString, got.TypeID)
	assert.Equal(t, "nginx", got.Str)

	missing, _ := d.Function([]octosql.Value{mapVal, octosql.NewString("absent")})
	assert.Equal(t, octosql.TypeIDNull, missing.TypeID, "map_get(map,absent) should return NULL")

	notMap, _ := d.Function([]octosql.Value{octosql.NewString("plain"), octosql.NewString("app")})
	assert.Equal(t, octosql.TypeIDNull, notMap.TypeID, "map_get(non-map,...) should return NULL")

	configJSON, _ := d.Function([]octosql.Value{mapVal, octosql.NewString("config.json")})
	assert.Equal(t, octosql.TypeIDString, configJSON.TypeID)
	assert.Equal(t, `{"foo": "bar}`, configJSON.Str)
}

func TestMapContainsKey(t *testing.T) {
	d := mapContainsKeyFunction().Descriptors[0]
	mapVal := octosql.NewString(`{"app":"nginx","tier":"web"}`)

	present, err := d.Function([]octosql.Value{mapVal, octosql.NewString("app")})
	assert.NoError(t, err)
	assert.Equal(t, octosql.TypeIDBoolean, present.TypeID)
	assert.True(t, present.Boolean, "map_contains_key(map,app) should be true")

	absent, _ := d.Function([]octosql.Value{mapVal, octosql.NewString("missing")})
	assert.False(t, absent.Boolean, "map_contains_key(map,missing) should be false")

	notMap, _ := d.Function([]octosql.Value{octosql.NewString("plain"), octosql.NewString("app")})
	assert.False(t, notMap.Boolean, "map_contains_key(non-map,...) should be false")
}

func TestMapValues(t *testing.T) {
	d := mapValuesFunction().Descriptors[0]
	mapVal := octosql.NewString(`{"app":"nginx","tier":"web"}`)

	out, err := d.Function([]octosql.Value{mapVal})
	assert.NoError(t, err)
	assert.Equal(t, octosql.TypeIDList, out.TypeID)

	got := make([]string, len(out.List))
	for i, e := range out.List {
		got[i] = e.Str
	}
	// values follow key order (app, tier) for deterministic output
	assert.Equal(t, []string{"nginx", "web"}, got)

	// non-map string yields empty list
	notMap, _ := d.Function([]octosql.Value{octosql.NewString("plain")})
	assert.Equal(t, octosql.TypeIDList, notMap.TypeID)
	assert.Empty(t, notMap.List, "map_values(non-map) should be empty list")
}

// func TestLength_JSONMapString(t *testing.T) {
// 	mapVal := octosql.NewString(`{"app":"nginx","tier":"web"}`)
// 	if got := callLength(t, mapVal); got != 2 {
// 		t.Errorf("length(JSON map) = %d, want 2 (keys)", got)
// 	}
// 	// A plain string still counts characters.
// 	if got := callLength(t, octosql.NewString("nginx")); got != 5 {
// 		t.Errorf("length(plain string) = %d, want 5", got)
// 	}
// }

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

// func TestContains_JSONMapString(t *testing.T) {
// 	mapVal := octosql.NewString(`{"app":"nginx","tier":"web"}`)
// 	if !callContains(t, mapVal, octosql.NewString("nginx")) {
// 		t.Error(`contains(map, "nginx") = false, want true (value match)`)
// 	}
// 	if callContains(t, mapVal, octosql.NewString("app")) {
// 		t.Error(`contains(map, "app") = true, want false (keys are not matched, values are)`)
// 	}
// 	// Plain string falls back to substring.
// 	if !callContains(t, octosql.NewString("nginx-ingress"), octosql.NewString("ingress")) {
// 		t.Error(`contains("nginx-ingress","ingress") = false, want true`)
// 	}
// }

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
