package octosql

import (
	"testing"

	"github.com/cube2222/octosql/octosql"
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
