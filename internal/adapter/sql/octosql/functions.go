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
// or struct, or the number of characters in a string. Returns NULL on NULL input.
func lengthFunction() physical.FunctionDetails {
	intReturn := func([]octosql.Type) (octosql.Type, bool) { return octosql.Int, true }
	return physical.FunctionDetails{
		Descriptors: []physical.FunctionDescriptor{
			{
				ArgumentTypes: []octosql.Type{{TypeID: octosql.TypeIDList}},
				TypeFn:        intReturn,
				Strict:        true,
				Function: func(values []octosql.Value) (octosql.Value, error) {
					return octosql.NewInt(int64(len(values[0].List))), nil
				},
			},
			{
				ArgumentTypes: []octosql.Type{{TypeID: octosql.TypeIDTuple}},
				TypeFn:        intReturn,
				Strict:        true,
				Function: func(values []octosql.Value) (octosql.Value, error) {
					return octosql.NewInt(int64(len(values[0].Tuple))), nil
				},
			},
			{
				ArgumentTypes: []octosql.Type{{TypeID: octosql.TypeIDStruct}},
				TypeFn:        intReturn,
				Strict:        true,
				Function: func(values []octosql.Value) (octosql.Value, error) {
					return octosql.NewInt(int64(len(values[0].Struct))), nil
				},
			},
			{
				ArgumentTypes: []octosql.Type{{TypeID: octosql.TypeIDString}},
				TypeFn:        intReturn,
				Strict:        true,
				Function: func(values []octosql.Value) (octosql.Value, error) {
					return octosql.NewInt(int64(len([]rune(values[0].Str)))), nil
				},
			},
		},
	}
}
