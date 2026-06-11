package octosql

import (
	"sort"
	"testing"
	"time"

	"github.com/cube2222/octosql/octosql"
	"github.com/cube2222/octosql/physical"
	"github.com/stretchr/testify/assert"
)

// callLength invokes the length descriptor matching the given value's TypeID and
// returns the resulting int. It fails the test if no descriptor matches. Mirrors
// octosql's typecheck loop (logical/function.go), where the LAST matching
// descriptor wins, so a descriptor accidentally matching the wrong type would be
// caught here too.
func callLength(t *testing.T, argType octosql.Type, v octosql.Value) int64 {
	t.Helper()
	details := lengthFunction()
	var match *physical.FunctionDescriptor
	for i, d := range details.Descriptors {
		if d.TypeFn == nil {
			continue
		}
		if _, ok := d.TypeFn([]octosql.Type{argType}); !ok {
			continue
		}
		match = &details.Descriptors[i]
	}
	if match == nil {
		t.Fatalf("no length descriptor for type %v", argType)
		return 0
	}
	out, err := match.Function([]octosql.Value{v})
	if err != nil {
		t.Fatalf("length(%v) returned error: %v", v.TypeID, err)
	}
	if out.TypeID != octosql.TypeIDInt {
		t.Fatalf("length should return Int, got %v", out.TypeID)
	}
	return out.Int
}

// mapListType is the runtime type of a FieldTypeMap column: List<Any>.
func mapListType() octosql.Type {
	return octosql.Type{
		TypeID: octosql.TypeIDList,
		List:   struct{ Element *octosql.Type }{Element: &octosql.Any},
	}
}

// listOfStringType is the runtime type of a FieldTypeList column: List<String>.
func listOfStringType() octosql.Type {
	elem := octosql.String
	return octosql.Type{
		TypeID: octosql.TypeIDList,
		List:   struct{ Element *octosql.Type }{Element: &elem},
	}
}

// newMapValue builds a flat List<Any> value of alternating key/value elements,
// the runtime representation of a FieldTypeMap column.
func newMapValue(kvs ...octosql.Value) octosql.Value {
	return octosql.NewList(kvs)
}

func TestFunctionMap_RegistersLength(t *testing.T) {
	m := FunctionMap()
	if _, ok := m["length"]; !ok {
		t.Fatal("FunctionMap missing 'length'")
	}
}

func TestFunctionNames(t *testing.T) {
	names := FunctionNames()

	for _, want := range []string{"map_get", "map_contains_key", "map_values", "length", "contains", "keys", "upper", "lower"} {
		assert.Contains(t, names, want)
	}

	// Operators and multi-word keyword phrases from octosql's function map are
	// not callable as name(...) and must be excluded from completion candidates.
	for _, notWant := range []string{"+", "-", "*", "/", "<", "<=", ">", ">=", "=", "!=", "~", "~*", "[]", "is null", "is not null", "not in"} {
		assert.NotContains(t, names, notWant)
	}

	assert.True(t, sort.StringsAreSorted(names))
}

func TestLength_List(t *testing.T) {
	v := octosql.NewList([]octosql.Value{octosql.NewInt(1), octosql.NewInt(2), octosql.NewInt(3)})
	if got := callLength(t, listOfStringType(), v); got != 3 {
		t.Errorf("length(list of 3) = %d, want 3", got)
	}
	if got := callLength(t, listOfStringType(), octosql.NewList(nil)); got != 0 {
		t.Errorf("length(empty list) = %d, want 0", got)
	}
}

func TestLength_Tuple(t *testing.T) {
	v := octosql.NewTuple([]octosql.Value{octosql.NewString("a"), octosql.NewString("b")})
	if got := callLength(t, octosql.Type{TypeID: octosql.TypeIDTuple}, v); got != 2 {
		t.Errorf("length(tuple of 2) = %d, want 2", got)
	}
}

func TestLength_Struct(t *testing.T) {
	v := octosql.NewStruct([]octosql.Value{octosql.NewInt(1), octosql.NewInt(2), octosql.NewInt(3), octosql.NewInt(4)})
	if got := callLength(t, octosql.Type{TypeID: octosql.TypeIDStruct}, v); got != 4 {
		t.Errorf("length(struct of 4) = %d, want 4", got)
	}
}

func TestLength_String(t *testing.T) {
	if got := callLength(t, octosql.String, octosql.NewString("nginx")); got != 5 {
		t.Errorf("length(\"nginx\") = %d, want 5", got)
	}
	if got := callLength(t, octosql.String, octosql.NewString("")); got != 0 {
		t.Errorf("length(\"\") = %d, want 0", got)
	}
	// Multi-byte: counts runes, not bytes.
	if got := callLength(t, octosql.String, octosql.NewString("café")); got != 4 {
		t.Errorf("length(\"café\") = %d, want 4 (runes)", got)
	}
}

func TestLength_Map(t *testing.T) {
	mapVal := newMapValue(
		octosql.NewString("app"), octosql.NewString("nginx"),
		octosql.NewString("tier"), octosql.NewString("web"),
	)
	if got := callLength(t, mapListType(), mapVal); got != 2 {
		t.Errorf("length(map of 2 keys) = %d, want 2", got)
	}
	if got := callLength(t, mapListType(), newMapValue()); got != 0 {
		t.Errorf("length(empty map) = %d, want 0", got)
	}
}

func TestMap_MapGet(t *testing.T) {
	d := mapGetFunction().Descriptors[0]
	mapVal := newMapValue(
		octosql.NewString("app"), octosql.NewString("nginx"),
		octosql.NewString("tier"), octosql.NewString("web"),
		octosql.NewString("config.json"), octosql.NewString(`{"foo": "bar"}`),
	)

	got, err := d.Function([]octosql.Value{mapVal, octosql.NewString("app")})
	if err != nil {
		t.Fatal(err)
	}

	assert.NoError(t, err)
	assert.Equal(t, octosql.TypeIDString, got.TypeID)
	assert.Equal(t, "nginx", got.Str)

	missing, _ := d.Function([]octosql.Value{mapVal, octosql.NewString("absent")})
	assert.Equal(t, octosql.TypeIDNull, missing.TypeID, "map_get(map,absent) should return NULL")

	configJSON, _ := d.Function([]octosql.Value{mapVal, octosql.NewString("config.json")})
	assert.Equal(t, octosql.TypeIDString, configJSON.TypeID)
	assert.Equal(t, `{"foo": "bar"}`, configJSON.Str)
}

func TestMap_MapGet_ReturnsNativeTime(t *testing.T) {
	d := mapGetFunction().Descriptors[0]
	ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	mapVal := newMapValue(
		octosql.NewString("kubectl.kubernetes.io/restartedAt"), octosql.NewTime(ts),
	)

	got, err := d.Function([]octosql.Value{mapVal, octosql.NewString("kubectl.kubernetes.io/restartedAt")})
	assert.NoError(t, err)
	assert.Equal(t, octosql.TypeIDTime, got.TypeID, "map_get returns the value's native type (Time)")
	assert.True(t, ts.Equal(got.Time))
}

func TestMapContainsKey(t *testing.T) {
	d := mapContainsKeyFunction().Descriptors[0]
	mapVal := newMapValue(
		octosql.NewString("app"), octosql.NewString("nginx"),
		octosql.NewString("tier"), octosql.NewString("web"),
	)

	present, err := d.Function([]octosql.Value{mapVal, octosql.NewString("app")})
	assert.NoError(t, err)
	assert.Equal(t, octosql.TypeIDBoolean, present.TypeID)
	assert.True(t, present.Boolean, "map_contains_key(map,app) should be true")

	absent, _ := d.Function([]octosql.Value{mapVal, octosql.NewString("missing")})
	assert.False(t, absent.Boolean, "map_contains_key(map,missing) should be false")
}

func TestMapValues(t *testing.T) {
	d := mapValuesFunction().Descriptors[0]
	mapVal := newMapValue(
		octosql.NewString("app"), octosql.NewString("nginx"),
		octosql.NewString("tier"), octosql.NewString("web"),
	)

	out, err := d.Function([]octosql.Value{mapVal})
	assert.NoError(t, err)
	assert.Equal(t, octosql.TypeIDList, out.TypeID)

	got := make([]string, len(out.List))
	for i, e := range out.List {
		got[i] = e.Str
	}
	// values follow key order (app, tier) for deterministic output
	assert.Equal(t, []string{"nginx", "web"}, got)

	// empty map yields empty list
	empty, _ := d.Function([]octosql.Value{newMapValue()})
	assert.Equal(t, octosql.TypeIDList, empty.TypeID)
	assert.Empty(t, empty.List, "map_values(empty map) should be empty list")
}

func TestKeys_Map(t *testing.T) {
	mapVal := newMapValue(
		octosql.NewString("app"), octosql.NewString("nginx"),
		octosql.NewString("tier"), octosql.NewString("web"),
	)
	out := callMapFnKeys(t, mapVal)
	got := make([]string, len(out.List))
	for i, e := range out.List {
		got[i] = e.Str
	}
	want := []string{"app", "tier"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("keys(map) = %v, want %v", got, want)
	}
}

func TestContains_Map(t *testing.T) {
	mapVal := newMapValue(
		octosql.NewString("app"), octosql.NewString("nginx"),
		octosql.NewString("tier"), octosql.NewString("web"),
	)
	if !callMapContains(t, mapVal, octosql.NewString("nginx")) {
		t.Error(`contains(map, "nginx") = false, want true (value match)`)
	}
	if callMapContains(t, mapVal, octosql.NewString("app")) {
		t.Error(`contains(map, "app") = true, want false (keys are not matched, values are)`)
	}
}

// callMapContains runs the contains() List<Any>-element (map) descriptor against
// a map value, mirroring octosql's last-match-wins typecheck semantics.
func callMapContains(t *testing.T, container, needle octosql.Value) bool {
	t.Helper()
	argTypes := []octosql.Type{mapListType(), {TypeID: needle.TypeID}}
	descriptors := containsFunction().Descriptors
	var match *physical.FunctionDescriptor
	for i, d := range descriptors {
		if d.TypeFn == nil {
			continue
		}
		if _, ok := d.TypeFn(argTypes); !ok {
			continue
		}
		match = &descriptors[i]
	}
	if match == nil {
		t.Fatalf("no contains descriptor for map container type")
		return false
	}
	out, err := match.Function([]octosql.Value{container, needle})
	if err != nil {
		t.Fatalf("contains(map, %v) returned error: %v", needle.TypeID, err)
	}
	return out.Boolean
}

// callMapFnKeys runs the keys() List<Any> descriptor on a map value.
func callMapFnKeys(t *testing.T, v octosql.Value) octosql.Value {
	t.Helper()
	for _, d := range keysFunction().Descriptors {
		if d.TypeFn == nil {
			continue
		}
		if _, ok := d.TypeFn([]octosql.Type{mapListType()}); !ok {
			continue
		}
		out, err := d.Function([]octosql.Value{v})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	t.Fatal("no keys map-list descriptor matched")
	return octosql.Value{}
}
