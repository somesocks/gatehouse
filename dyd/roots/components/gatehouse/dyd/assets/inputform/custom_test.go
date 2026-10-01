package inputform

import (
	"encoding/json"
	"testing"
)

func TestCustomFieldDescriptorAndJSONValues(t *testing.T) {
	field := Field{ID: "value", Label: "Value", Type: "custom", Custom: &CustomField{URL: "/app/tools/session-file-picker/", Inputs: json.RawMessage(`{"limit":9007199254740993}`), Capabilities: []string{SessionFileList}}}
	form := Form{Version: Version, Type: "form", Title: "Custom", Fields: []Field{field}}
	encoded, err := json.Marshal(form)
	if err != nil {
		t.Fatal(err)
	}
	err, restored := Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`null`, `true`, `9007199254740993`, `"file"`, `{"nested":[1,false]}`} {
		if err := restored.ValidateResult(json.RawMessage(`{"value":` + value + `}`)); err != nil {
			t.Fatalf("custom JSON %s: %v", value, err)
		}
	}
	if err := restored.ValidateResult(json.RawMessage(`{}`)); err == nil {
		t.Fatal("missing required custom field accepted")
	}
	field.Optional = true
	form.Fields[0] = field
	if err := form.ValidateResult(json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	form.Fields = []Field{{ID: "entries", Label: "Entries", Type: "list", Item: &Field{Type: "object", Fields: []Field{field}}}}
	if err := form.Validate(); err == nil {
		t.Fatal("custom field inside list accepted")
	}
	for _, capabilities := range [][]string{{"project.file.list"}, {SessionFileList, SessionFileList}} {
		field.Custom.Capabilities = capabilities
		if err := field.Custom.Validate(); err == nil {
			t.Fatalf("invalid capabilities accepted: %v", capabilities)
		}
	}
}
