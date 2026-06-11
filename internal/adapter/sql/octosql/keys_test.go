package octosql

import (
	"testing"

	"github.com/cube2222/octosql/octosql"
)

// structType builds a struct Type with the given field names (string-typed).
func structType(names ...string) octosql.Type {
	fields := make([]octosql.StructField, len(names))
	for i, n := range names {
		fields[i] = octosql.StructField{Name: n, Type: octosql.String}
	}
	return octosql.Type{
		TypeID: octosql.TypeIDStruct,
		Struct: struct{ Fields []octosql.StructField }{Fields: fields},
	}
}

// callKeys runs the keys descriptor end-to-end: TypeFn captures the field names
// from the struct type, then Function emits them. Returns the resulting strings.
func callKeys(t *testing.T, structT octosql.Type, value octosql.Value) []string {
	t.Helper()
	d := keysFunction().Descriptors[0]
	out, ok := d.TypeFn([]octosql.Type{structT})
	if !ok {
		t.Fatalf("keys TypeFn rejected struct type")
	}
	if out.TypeID != octosql.TypeIDList {
		t.Fatalf("keys should return a List, got %v", out.TypeID)
	}
	res, err := d.Function([]octosql.Value{value})
	if err != nil {
		t.Fatalf("keys returned error: %v", err)
	}
	if res.TypeID != octosql.TypeIDList {
		t.Fatalf("keys value should be a List, got %v", res.TypeID)
	}
	keys := make([]string, len(res.List))
	for i, e := range res.List {
		keys[i] = e.Str
	}
	return keys
}

func TestFunctionMap_RegistersKeys(t *testing.T) {
	if _, ok := FunctionMap()["keys"]; !ok {
		t.Fatal("FunctionMap missing 'keys'")
	}
}

func TestKeys_Struct(t *testing.T) {
	st := structType("app", "tier")
	val := octosql.NewStruct([]octosql.Value{octosql.NewString("nginx"), octosql.NewString("web")})

	got := callKeys(t, st, val)
	want := []string{"app", "tier"}
	if len(got) != len(want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestKeys_EmptyStruct(t *testing.T) {
	if got := callKeys(t, structType(), octosql.NewStruct(nil)); len(got) != 0 {
		t.Errorf("keys(empty struct) = %v, want []", got)
	}
}

func TestKeys_RejectsNonStruct(t *testing.T) {
	d := keysFunction().Descriptors[0]
	for _, ty := range []octosql.Type{octosql.String, octosql.Int, {TypeID: octosql.TypeIDList}} {
		if _, ok := d.TypeFn([]octosql.Type{ty}); ok {
			t.Errorf("keys TypeFn accepted non-struct type %v", ty.TypeID)
		}
	}
}
