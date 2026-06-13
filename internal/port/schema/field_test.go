package schema

import "testing"

func TestMarshalSubFieldsJSON(t *testing.T) {
	fields := []Field{
		{Name: "phase", Type: FieldTypeString},
		{Name: "containers", Type: FieldTypeList},
		{Name: "affinity", Type: FieldTypeObject, SubFields: []Field{
			{Name: "nodeAffinity", Type: FieldTypeObject},
		}},
	}

	got, err := MarshalSubFieldsJSON(fields)
	if err != nil {
		t.Fatalf("MarshalSubFieldsJSON: %v", err)
	}

	want := `[
  {
    "name": "phase",
    "type": "string"
  },
  {
    "name": "containers",
    "type": "list"
  },
  {
    "name": "affinity",
    "type": "object",
    "subFields": [
      {
        "name": "nodeAffinity",
        "type": "object"
      }
    ]
  }
]`
	if got != want {
		t.Errorf("MarshalSubFieldsJSON() = %q, want %q", got, want)
	}
}

func TestLimitDepth(t *testing.T) {
	fields := []Field{
		{Name: "a", Type: FieldTypeObject, SubFields: []Field{
			{Name: "b", Type: FieldTypeObject, SubFields: []Field{
				{Name: "c", Type: FieldTypeObject, SubFields: []Field{
					{Name: "d", Type: FieldTypeString},
				}},
			}},
		}},
	}

	got := LimitDepth(fields, 2)

	want := []Field{
		{Name: "a", Type: FieldTypeObject, SubFields: []Field{
			{Name: "b", Type: FieldTypeObject, SubFields: nil},
		}},
	}

	gotJSON, _ := MarshalSubFieldsJSON(got)
	wantJSON, _ := MarshalSubFieldsJSON(want)
	if gotJSON != wantJSON {
		t.Errorf("LimitDepth() = %s, want %s", gotJSON, wantJSON)
	}
}

func TestMarshalSubFieldsJSON_Empty(t *testing.T) {
	got, err := MarshalSubFieldsJSON(nil)
	if err != nil {
		t.Fatalf("MarshalSubFieldsJSON: %v", err)
	}
	if got != "[]" {
		t.Errorf("MarshalSubFieldsJSON(nil) = %q, want %q", got, "[]")
	}
}
