package octosql

import (
	"sort"
	"strings"

	"github.com/cube2222/octosql/octosql"
	"github.com/cube2222/octosql/physical"
)

// FunctionMap returns the custom SQL functions registered on top of octosql's
// built-ins. These are merged into the engine's function map at query time.
func FunctionMap() map[string]physical.FunctionDetails {
	return map[string]physical.FunctionDetails{
		"length":           lengthFunction(),
		"contains":         containsFunction(),
		"keys":             keysFunction(),
		"map_get":          mapGetFunction(),
		"map_contains_key": mapContainsKeyFunction(),
		"map_values":       mapValuesFunction(),
	}
}

// isMapListType reports whether t is the runtime type of a FieldTypeMap column: a
// List whose Element type is Any. A map column carries its row value as a flat,
// alternating key/value list ([k1, v1, k2, v2, ...]); a regular FieldTypeList
// column has a concrete (non-Any) Element type (e.g. String). This is a static
// (typecheck-time) check on the column type, distinguishing the two List shapes.
func isMapListType(t octosql.Type) bool {
	return t.TypeID == octosql.TypeIDList && t.List.Element != nil && t.List.Element.TypeID == octosql.TypeIDAny
}

func sortedKeys(m map[string]interface{}) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// singleArgKindFn builds a TypeFn matching exactly one argument of the given kind.
func singleArgKindFn(id octosql.TypeID, out octosql.Type) func([]octosql.Type) (octosql.Type, bool) {
	return func(types []octosql.Type) (octosql.Type, bool) {
		if len(types) != 1 || types[0].TypeID != id {
			return octosql.Type{}, false
		}
		return out, true
	}
}

// singleArgListFn builds a TypeFn matching exactly one List argument whose Element
// type satisfies wantMapList (true: a map's flat key/value list with Element=Any;
// false: a regular list with a concrete Element type).
func singleArgListFn(wantMapList bool, out octosql.Type) func([]octosql.Type) (octosql.Type, bool) {
	return func(types []octosql.Type) (octosql.Type, bool) {
		if len(types) != 1 || types[0].TypeID != octosql.TypeIDList {
			return octosql.Type{}, false
		}
		if isMapListType(types[0]) != wantMapList {
			return octosql.Type{}, false
		}
		return out, true
	}
}

// lengthFunction implements length(x): elements in a list/tuple/struct, characters
// in a plain string, or — for a map column (List<Any> of [k1,v1,k2,v2,...]) — the
// number of map keys.
func lengthFunction() physical.FunctionDetails {
	descriptor := func(id octosql.TypeID, fn func([]octosql.Value) (octosql.Value, error)) physical.FunctionDescriptor {
		return physical.FunctionDescriptor{Strict: true, TypeFn: singleArgKindFn(id, octosql.Int), Function: fn}
	}
	return physical.FunctionDetails{
		Descriptors: []physical.FunctionDescriptor{
			descriptor(octosql.TypeIDList, func(v []octosql.Value) (octosql.Value, error) {
				return octosql.NewInt(int64(len(v[0].List))), nil
			}),
			{
				Strict: true,
				TypeFn: singleArgListFn(true, octosql.Int),
				Function: func(v []octosql.Value) (octosql.Value, error) {
					return octosql.NewInt(int64(len(v[0].List) / 2)), nil
				},
			},
			descriptor(octosql.TypeIDTuple, func(v []octosql.Value) (octosql.Value, error) {
				return octosql.NewInt(int64(len(v[0].Tuple))), nil
			}),
			descriptor(octosql.TypeIDStruct, func(v []octosql.Value) (octosql.Value, error) {
				return octosql.NewInt(int64(len(v[0].Struct))), nil
			}),
			descriptor(octosql.TypeIDString, func(v []octosql.Value) (octosql.Value, error) {
				return octosql.NewInt(int64(len([]rune(v[0].Str)))), nil
			}),
		},
	}
}

// containsFunction implements contains(container, needle) -> bool:
//   - string: substring check
//   - list:   true if any element equals the needle
//   - map column (List<Any> of [k1,v1,k2,v2,...]): true if any VALUE (odd-indexed
//     element) equals the needle — keys are not matched
//   - struct: true if any field value equals the needle
func containsFunction() physical.FunctionDetails {
	descriptor := func(id octosql.TypeID, fn func([]octosql.Value) (octosql.Value, error)) physical.FunctionDescriptor {
		return physical.FunctionDescriptor{
			Strict: true,
			TypeFn: func(types []octosql.Type) (octosql.Type, bool) {
				if len(types) != 2 || types[0].TypeID != id {
					return octosql.Type{}, false
				}
				return octosql.Boolean, true
			},
			Function: fn,
		}
	}

	return physical.FunctionDetails{
		Descriptors: []physical.FunctionDescriptor{
			descriptor(octosql.TypeIDString, func(v []octosql.Value) (octosql.Value, error) {
				needle := valueToPlainString(v[1])
				return octosql.NewBoolean(strings.Contains(v[0].Str, needle)), nil
			}),
			descriptor(octosql.TypeIDList, func(v []octosql.Value) (octosql.Value, error) {
				return octosql.NewBoolean(anyEqual(v[0].List, v[1])), nil
			}),
			{
				Strict: true,
				TypeFn: func(types []octosql.Type) (octosql.Type, bool) {
					if len(types) != 2 || !isMapListType(types[0]) {
						return octosql.Type{}, false
					}
					return octosql.Boolean, true
				},
				Function: func(v []octosql.Value) (octosql.Value, error) {
					values := make([]octosql.Value, 0, len(v[0].List)/2)
					for i := 1; i < len(v[0].List); i += 2 {
						values = append(values, v[0].List[i])
					}
					return octosql.NewBoolean(anyEqual(values, v[1])), nil
				},
			},
			descriptor(octosql.TypeIDStruct, func(v []octosql.Value) (octosql.Value, error) {
				return octosql.NewBoolean(anyEqual(v[0].Struct, v[1])), nil
			}),
		},
	}
}

// anyEqual reports whether needle is present (by type and value equality) in elems.
func anyEqual(elems []octosql.Value, needle octosql.Value) bool {
	for _, e := range elems {
		if e.TypeID == needle.TypeID && e.Equal(needle) {
			return true
		}
	}
	return false
}

// keysFunction implements keys(x) -> list of keys (strings):
//   - struct: the field names (from the type, captured at typecheck)
//   - map column (List<Any> of [k1,v1,k2,v2,...]): the even-indexed (key) elements
func keysFunction() physical.FunctionDetails {
	stringElem := octosql.String
	listOfString := octosql.Type{
		TypeID: octosql.TypeIDList,
		List:   struct{ Element *octosql.Type }{Element: &stringElem},
	}

	var structFieldNames []string
	return physical.FunctionDetails{
		Descriptors: []physical.FunctionDescriptor{
			{
				Strict: true,
				TypeFn: func(types []octosql.Type) (octosql.Type, bool) {
					if len(types) != 1 || types[0].TypeID != octosql.TypeIDStruct {
						return octosql.Type{}, false
					}
					structFieldNames = structFieldNames[:0]
					for _, f := range types[0].Struct.Fields {
						structFieldNames = append(structFieldNames, f.Name)
					}
					return listOfString, true
				},
				Function: func([]octosql.Value) (octosql.Value, error) {
					elems := make([]octosql.Value, len(structFieldNames))
					for i, name := range structFieldNames {
						elems[i] = octosql.NewString(name)
					}
					return octosql.NewList(elems), nil
				},
			},
			{
				Strict: true,
				TypeFn: singleArgListFn(true, listOfString),
				Function: func(v []octosql.Value) (octosql.Value, error) {
					elems := make([]octosql.Value, 0, len(v[0].List)/2)
					for i := 0; i < len(v[0].List); i += 2 {
						elems = append(elems, v[0].List[i])
					}
					return octosql.NewList(elems), nil
				},
			},
		},
	}
}

// mapGetFunction implements map_get(map, key) -> any|null: looks up a key in a map
// column (List<Any> of [k1,v1,k2,v2,...]) and returns its value in its native
// octosql type, or NULL if absent. This backs the field['key'] access syntax (see
// rewriteDottedFields).
func mapGetFunction() physical.FunctionDetails {
	anyOrNull := octosql.TypeSum(octosql.Any, octosql.Null)
	return physical.FunctionDetails{
		Descriptors: []physical.FunctionDescriptor{
			{
				Strict: true,
				TypeFn: func(types []octosql.Type) (octosql.Type, bool) {
					if len(types) != 2 || !isMapListType(types[0]) || types[1].TypeID != octosql.TypeIDString {
						return octosql.Type{}, false
					}
					return anyOrNull, true
				},
				Function: func(v []octosql.Value) (octosql.Value, error) {
					list := v[0].List
					for i := 0; i+1 < len(list); i += 2 {
						if list[i].Str == v[1].Str {
							return list[i+1], nil
						}
					}
					return octosql.NewNull(), nil
				},
			},
		},
	}
}

// mapContainsKeyFunction implements map_contains_key(map, key) -> bool: true if a
// map column (List<Any> of [k1,v1,k2,v2,...]) has the given key.
func mapContainsKeyFunction() physical.FunctionDetails {
	return physical.FunctionDetails{
		Descriptors: []physical.FunctionDescriptor{
			{
				Strict: true,
				TypeFn: func(types []octosql.Type) (octosql.Type, bool) {
					if len(types) != 2 || !isMapListType(types[0]) || types[1].TypeID != octosql.TypeIDString {
						return octosql.Type{}, false
					}
					return octosql.Boolean, true
				},
				Function: func(v []octosql.Value) (octosql.Value, error) {
					list := v[0].List
					for i := 0; i+1 < len(list); i += 2 {
						if list[i].Str == v[1].Str {
							return octosql.NewBoolean(true), nil
						}
					}
					return octosql.NewBoolean(false), nil
				},
			},
		},
	}
}

// mapValuesFunction implements map_values(map) -> list of values in their native
// octosql types: the odd-indexed (value) elements of a map column (List<Any> of
// [k1,v1,k2,v2,...]). Values are emitted in key order (anyToMapValue sorts keys),
// for deterministic output.
func mapValuesFunction() physical.FunctionDetails {
	// Declared with a nil Element type (rather than List<Any>, the map column
	// shape) so the renderer's isMapListType check does not mistake this plain
	// list of values for a flat key/value map list.
	listOfValues := octosql.Type{
		TypeID: octosql.TypeIDList,
		List:   struct{ Element *octosql.Type }{Element: nil},
	}
	return physical.FunctionDetails{
		Descriptors: []physical.FunctionDescriptor{
			{
				Strict: true,
				TypeFn: singleArgListFn(true, listOfValues),
				Function: func(v []octosql.Value) (octosql.Value, error) {
					elems := make([]octosql.Value, 0, len(v[0].List)/2)
					for i := 1; i < len(v[0].List); i += 2 {
						elems = append(elems, v[0].List[i])
					}
					return octosql.NewList(elems), nil
				},
			},
		},
	}
}

// valueToPlainString returns the bare string content of a value for substring
// matching: the raw text for strings, otherwise octosql's String() rendering.
func valueToPlainString(v octosql.Value) string {
	if v.TypeID == octosql.TypeIDString {
		return v.Str
	}
	return v.String()
}
