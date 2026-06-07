package octosql

import (
	"fmt"

	"github.com/cube2222/octosql/octosql"
	"github.com/cube2222/octosql/physical"
)

func FunctionMap() map[string]physical.FunctionDetails {

	return map[string]physical.FunctionDetails{
		"length": {
			Descriptors: []physical.FunctionDescriptor{
				{
					ArgumentTypes: []octosql.Type{{TypeID: octosql.TypeIDList}},
					TypeFn: func(args []octosql.Type) (octosql.Type, bool) {
						return octosql.Int, true
					},
					Strict: true,
					Function: func(values []octosql.Value) (octosql.Value, error) {
						return octosql.NewInt(3), nil // octosql.NewInt(int64(len(values[0].List))), nil
					},
				},
				{
					ArgumentTypes: []octosql.Type{{TypeID: octosql.TypeIDTuple}},
					TypeFn: func(args []octosql.Type) (octosql.Type, bool) {
						return octosql.Int, true
					},
					Strict: true,
					Function: func(values []octosql.Value) (octosql.Value, error) {
						return octosql.NewInt(int64(len(values[0].Tuple))), nil
					},
				},
				{
					ArgumentTypes: []octosql.Type{{TypeID: octosql.TypeIDAny}},
					TypeFn: func(args []octosql.Type) (octosql.Type, bool) {
						return octosql.Int, true
					},
					Strict: true,
					Function: func(values []octosql.Value) (octosql.Value, error) {
						fmt.Printf("length called with argument of type %s", values[0].TypeID)
						return octosql.NewInt(int64(len(values[0].List))), nil
					},
				},
			},
		},
	}
}
