package inputform

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormValidateResult(t *testing.T) {
	minimumLength, maximumLength := int64(2), int64(3)
	minimum, maximum := int64(0), int64(10)
	form := Form{Version: Version, Type: "form", Title: "Review", Fields: []Field{
		{ID: "title", Label: "Title", Type: "text", MinLength: &minimumLength, MaxLength: &maximumLength},
		{ID: "notes", Label: "Notes", Type: "text", Optional: true},
		{ID: "approved", Label: "Approved", Type: "boolean"},
		{ID: "amount", Label: "Amount", Type: "number", Min: &minimum, Max: &maximum},
		{ID: "count", Label: "Count", Type: "number", Integer: true},
		{ID: "region", Label: "Region", Type: "options", Choices: []string{"north", "south"}},
		{ID: "contacts", Label: "Contacts", Type: "list", Item: &Field{Type: "object", Fields: []Field{{ID: "name", Label: "Name", Type: "text"}}}},
	}}
	encoded, err := json.Marshal(form)
	if err != nil {
		t.Fatal(err)
	}
	if err, restored := Decode(encoded); err != nil || restored == nil {
		t.Fatalf("Decode() = (%#v, %v)", restored, err)
	}
	base := `{"title":"éx","approved":false,"amount":1.5e0,"count":2e0,"region":"north","contacts":[{"name":"Ada"}]}`
	if err := form.ValidateResult(json.RawMessage(base)); err != nil {
		t.Fatalf("ValidateResult(%s): %v", base, err)
	}
	for _, test := range []struct {
		name, result, contains string
	}{
		{"missing required", `{"approved":false}`, `"title" is required`},
		{"null is not absence", `{"title":null,"approved":false,"amount":1,"count":2,"region":"north","contacts":[]}`, `"title": requires a JSON string`},
		{"unicode length", `{"title":"é","approved":false,"amount":1,"count":2,"region":"north","contacts":[]}`, "text length is out of bounds"},
		{"upper bound", `{"title":"abc","approved":false,"amount":11,"count":2,"region":"north","contacts":[]}`, "number is out of bounds"},
		{"integer only", `{"title":"abc","approved":false,"amount":1,"count":2.5,"region":"north","contacts":[]}`, "number is out of bounds"},
		{"oversized exponent", `{"title":"abc","approved":false,"amount":1e100000000,"count":2,"region":"north","contacts":[]}`, "supported JSON number"},
		{"invalid choice", `{"title":"abc","approved":false,"amount":1,"count":2,"region":"east","contacts":[]}`, "configured choice"},
		{"invalid child", `{"title":"abc","approved":false,"amount":1,"count":2,"region":"north","contacts":[{}]}`, `"name" is required`},
		{"unknown key", `{"title":"abc","approved":false,"amount":1,"count":2,"region":"north","contacts":[],"extra":true}`, `"extra" is unknown`},
		{"scalar instead of object", `["one"]`, "requires a JSON object"},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := form.ValidateResult(json.RawMessage(test.result))
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("ValidateResult() = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestFormRejectsUnknownSchemaAndPreservesIntegerBounds(t *testing.T) {
	for _, source := range []string{
		`{"version":1,"type":"form","title":"Review","fields":[{"id":"name","label":"Name","type":"text","unrecognized":true}]}`,
		`{"version":2,"type":"form","title":"Review","fields":[{"id":"name","label":"Name","type":"text"}]}`,
		`{"version":1,"type":"form","title":"Review","fields":[{"id":"name","label":"Name","type":"text"},{"id":"name","label":"Again","type":"text"}]}`,
	} {
		if err, _ := Decode([]byte(source)); err == nil {
			t.Fatalf("Decode(%s) accepted invalid form", source)
		}
	}
	const large = int64(9007199254740993)
	form := Form{Version: Version, Type: "form", Title: "Review", Fields: []Field{{ID: "count", Label: "Count", Type: "number", Min: func() *int64 { n := large; return &n }()}}}
	encoded, err := json.Marshal(form)
	if err != nil {
		t.Fatal(err)
	}
	err, decoded := Decode(encoded)
	if err != nil || decoded.Fields[0].Min == nil || *decoded.Fields[0].Min != large {
		t.Fatalf("integer bound round-trip = (%#v, %v)", decoded, err)
	}
	if err := decoded.ValidateResult(json.RawMessage(`{"count":9007199254740992}`)); err == nil {
		t.Fatal("accepted number below an exact integer bound")
	}
}

func TestFormValidatesNestedLists(t *testing.T) {
	form := Form{Version: Version, Type: "form", Title: "Matrix", Fields: []Field{{
		ID: "matrix", Label: "Matrix", Type: "list", Item: &Field{Type: "list", Item: &Field{Type: "number"}},
	}}}
	if err := form.ValidateResult(json.RawMessage(`{"matrix":[[1,2.5],[3]]}`)); err != nil {
		t.Fatalf("nested list result: %v", err)
	}
	if err := form.ValidateResult(json.RawMessage(`{"matrix":[[1],["wrong"]]}`)); err == nil || !strings.Contains(err.Error(), "requires a JSON number") {
		t.Fatalf("invalid nested list item error = %v", err)
	}
}
