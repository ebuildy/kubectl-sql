package octosql

import (
	"testing"

	"github.com/cube2222/octosql/octosql"
)

// callContains invokes the contains descriptor matching the container's TypeID
// (using the same discriminating TypeFn octosql uses) and returns the bool result.
func callContains(t *testing.T, container, needle octosql.Value) bool {
	t.Helper()
	argTypes := []octosql.Type{{TypeID: container.TypeID}, {TypeID: needle.TypeID}}
	for _, d := range containsFunction().Descriptors {
		if d.TypeFn == nil {
			continue
		}
		if _, ok := d.TypeFn(argTypes); !ok {
			continue
		}
		out, err := d.Function([]octosql.Value{container, needle})
		if err != nil {
			t.Fatalf("contains(%v, %v) returned error: %v", container.TypeID, needle.TypeID, err)
		}
		if out.TypeID != octosql.TypeIDBoolean {
			t.Fatalf("contains should return Boolean, got %v", out.TypeID)
		}
		return out.Boolean
	}
	t.Fatalf("no contains descriptor for container type %v", container.TypeID)
	return false
}

func TestFunctionMap_RegistersContains(t *testing.T) {
	if _, ok := FunctionMap()["contains"]; !ok {
		t.Fatal("FunctionMap missing 'contains'")
	}
}

func TestContains_String(t *testing.T) {
	if !callContains(t, octosql.NewString("nginx-ingress"), octosql.NewString("ingress")) {
		t.Error(`contains("nginx-ingress", "ingress") = false, want true`)
	}
	if callContains(t, octosql.NewString("nginx"), octosql.NewString("apache")) {
		t.Error(`contains("nginx", "apache") = true, want false`)
	}
	if !callContains(t, octosql.NewString("anything"), octosql.NewString("")) {
		t.Error(`contains("anything", "") = false, want true (empty substring)`)
	}
}

func TestContains_List(t *testing.T) {
	list := octosql.NewList([]octosql.Value{
		octosql.NewString("a"), octosql.NewString("b"), octosql.NewString("c"),
	})
	if !callContains(t, list, octosql.NewString("b")) {
		t.Error(`contains([a,b,c], "b") = false, want true`)
	}
	if callContains(t, list, octosql.NewString("z")) {
		t.Error(`contains([a,b,c], "z") = true, want false`)
	}
	// Different element type does not match.
	if callContains(t, list, octosql.NewInt(1)) {
		t.Error(`contains([a,b,c], 1) = true, want false`)
	}
	if callContains(t, octosql.NewList(nil), octosql.NewString("x")) {
		t.Error(`contains([], "x") = true, want false`)
	}
}

func TestContains_Struct(t *testing.T) {
	// Struct values carry only field values (names live in the type), so contains
	// checks membership against the field values.
	s := octosql.NewStruct([]octosql.Value{
		octosql.NewString("nginx"), octosql.NewInt(3),
	})
	if !callContains(t, s, octosql.NewString("nginx")) {
		t.Error(`contains(struct{nginx,3}, "nginx") = false, want true`)
	}
	if !callContains(t, s, octosql.NewInt(3)) {
		t.Error(`contains(struct{nginx,3}, 3) = false, want true`)
	}
	if callContains(t, s, octosql.NewString("apache")) {
		t.Error(`contains(struct{nginx,3}, "apache") = true, want false`)
	}
}
