package octosql

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/cube2222/octosql/octosql"
	"github.com/cube2222/octosql/physical"
)

// FunctionMap returns the custom SQL functions registered on top of octosql's
// built-ins. These are merged into the engine's function map at query time.
func FunctionMap() map[string]physical.FunctionDetails {
	return map[string]physical.FunctionDetails{
		"length":   lengthFunction(),
		"contains": containsFunction(),
		"keys":     keysFunction(),
		"map_get":  mapGetFunction(),
	}
}

// asJSONMap reports whether s is a JSON object string and, if so, returns the
// decoded map. Map columns (labels, annotations) are carried as JSON-object
// strings, so the string-typed length/keys/contains/map_get descriptors use this
// to operate on the per-row map. A plain (non-object) string returns ok=false and
// falls back to ordinary string semantics.
func asJSONMap(s string) (map[string]interface{}, bool) {
	t := strings.TrimSpace(s)
	if len(t) == 0 || t[0] != '{' {
		return nil, false
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(t), &m); err != nil {
		return nil, false
	}
	return m, true
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

// lengthFunction implements length(x): elements in a list/tuple/struct, characters
// in a plain string, or — when the string is a JSON object (a map column) — the
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
			descriptor(octosql.TypeIDTuple, func(v []octosql.Value) (octosql.Value, error) {
				return octosql.NewInt(int64(len(v[0].Tuple))), nil
			}),
			descriptor(octosql.TypeIDStruct, func(v []octosql.Value) (octosql.Value, error) {
				return octosql.NewInt(int64(len(v[0].Struct))), nil
			}),
			descriptor(octosql.TypeIDString, func(v []octosql.Value) (octosql.Value, error) {
				if m, ok := asJSONMap(v[0].Str); ok {
					return octosql.NewInt(int64(len(m))), nil
				}
				return octosql.NewInt(int64(len([]rune(v[0].Str)))), nil
			}),
		},
	}
}

// containsFunction implements contains(container, needle) -> bool:
//   - string:  substring check, OR (when the string is a JSON-object map column)
//     true if any map VALUE equals the needle
//   - list:    true if any element equals the needle
//   - struct:  true if any field value equals the needle
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

	anyEqual := func(elems []octosql.Value, needle octosql.Value) bool {
		for _, e := range elems {
			if e.TypeID == needle.TypeID && e.Equal(needle) {
				return true
			}
		}
		return false
	}

	return physical.FunctionDetails{
		Descriptors: []physical.FunctionDescriptor{
			descriptor(octosql.TypeIDString, func(v []octosql.Value) (octosql.Value, error) {
				needle := valueToPlainString(v[1])
				if m, ok := asJSONMap(v[0].Str); ok {
					for _, mv := range m {
						if sv, isStr := mv.(string); isStr && sv == needle {
							return octosql.NewBoolean(true), nil
						}
					}
					return octosql.NewBoolean(false), nil
				}
				return octosql.NewBoolean(strings.Contains(v[0].Str, needle)), nil
			}),
			descriptor(octosql.TypeIDList, func(v []octosql.Value) (octosql.Value, error) {
				return octosql.NewBoolean(anyEqual(v[0].List, v[1])), nil
			}),
			descriptor(octosql.TypeIDStruct, func(v []octosql.Value) (octosql.Value, error) {
				return octosql.NewBoolean(anyEqual(v[0].Struct, v[1])), nil
			}),
		},
	}
}

// keysFunction implements keys(x) -> list of keys (strings):
//   - struct: the field names (from the type, captured at typecheck)
//   - string: when the string is a JSON-object map column, its sorted keys
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
				TypeFn: singleArgKindFn(octosql.TypeIDString, listOfString),
				Function: func(v []octosql.Value) (octosql.Value, error) {
					m, ok := asJSONMap(v[0].Str)
					if !ok {
						return octosql.NewList(nil), nil
					}
					ks := sortedKeys(m)
					elems := make([]octosql.Value, len(ks))
					for i, k := range ks {
						elems[i] = octosql.NewString(k)
					}
					return octosql.NewList(elems), nil
				},
			},
		},
	}
}

// mapGetFunction implements map_get(map, key) -> string|null: looks up a key in a
// map column (a JSON-object string) and returns its value, or NULL if absent. This
// backs the field['key'] access syntax (see rewriteDottedFields).
func mapGetFunction() physical.FunctionDetails {
	strOrNull := octosql.TypeSum(octosql.String, octosql.Null)
	return physical.FunctionDetails{
		Descriptors: []physical.FunctionDescriptor{
			{
				Strict: true,
				TypeFn: func(types []octosql.Type) (octosql.Type, bool) {
					if len(types) != 2 || types[0].TypeID != octosql.TypeIDString || types[1].TypeID != octosql.TypeIDString {
						return octosql.Type{}, false
					}
					return strOrNull, true
				},
				Function: func(v []octosql.Value) (octosql.Value, error) {
					m, ok := asJSONMap(v[0].Str)
					if !ok {
						return octosql.NewNull(), nil
					}
					val, exists := m[v[1].Str]
					if !exists {
						return octosql.NewNull(), nil
					}
					if s, isStr := val.(string); isStr {
						return octosql.NewString(s), nil
					}
					b, err := json.Marshal(val)
					if err != nil {
						return octosql.NewNull(), nil
					}
					return octosql.NewString(string(b)), nil
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
