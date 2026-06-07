package octosql

import (
	"github.com/cube2222/octosql/octosql"
	"github.com/cube2222/octosql/physical"
)

// FunctionMap returns the custom SQL functions registered on top of octosql's
// built-ins. These are merged into the engine's function map at query time.
func FunctionMap() map[string]physical.FunctionDetails {
	return map[string]physical.FunctionDetails{
		"length": lengthFunction(),
	}
}

// lengthFunction implements length(x): the number of elements in a list, tuple,
// or struct, or the number of characters in a string.
//
// Each descriptor uses a TypeFn that matches ONLY its own TypeID. octosql's
// generic ArgumentTypes matching can't express "any struct" or "any list"
// (Type.Is requires struct field names/counts and list element types to match
// exactly), so a discriminating TypeFn is the way to accept a whole kind. The
// TypeFn must return false for other kinds — a TypeFn that always returns true
// would make this descriptor win for every argument type. Strict matching makes
// octosql strip a nullable wrapper (struct|null) before dispatch, so
// length(metadata->labels) resolves to the struct descriptor.
func lengthFunction() physical.FunctionDetails {
	descriptorFor := func(id octosql.TypeID, fn func([]octosql.Value) (octosql.Value, error)) physical.FunctionDescriptor {
		return physical.FunctionDescriptor{
			Strict: true,
			TypeFn: func(types []octosql.Type) (octosql.Type, bool) {
				if len(types) != 1 || types[0].TypeID != id {
					return octosql.Type{}, false
				}
				return octosql.Int, true
			},
			Function: fn,
		}
	}

	return physical.FunctionDetails{
		Descriptors: []physical.FunctionDescriptor{
			descriptorFor(octosql.TypeIDList, func(values []octosql.Value) (octosql.Value, error) {
				return octosql.NewInt(int64(len(values[0].List))), nil
			}),
			descriptorFor(octosql.TypeIDTuple, func(values []octosql.Value) (octosql.Value, error) {
				return octosql.NewInt(int64(len(values[0].Tuple))), nil
			}),
			descriptorFor(octosql.TypeIDStruct, func(values []octosql.Value) (octosql.Value, error) {
				return octosql.NewInt(int64(len(values[0].Struct))), nil
			}),
			descriptorFor(octosql.TypeIDString, func(values []octosql.Value) (octosql.Value, error) {
				return octosql.NewInt(int64(len([]rune(values[0].Str)))), nil
			}),
		},
	}
}
