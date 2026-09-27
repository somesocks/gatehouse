package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"gatehouse/inputform"
	"gatehouse/lisp"
)

func evaluateInputForm(source string) (error, lisp.Expr) {
	return lisp.Evaluate(`(import (input @native:gatehouse/input/v1) `+source+`)`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewInputModule()},
	})
}

func TestInputFormConstructorsReturnTaggedValues(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{`(input/form/text "name" "Name")`, `(input/form/text "name" "Name")`},
		{`(input/form/text "name" "Name" (input/form/text/min-length 1) (input/form/text/max-length 80))`, `(input/form/text "name" "Name" (input/form/text/min-length 1) (input/form/text/max-length 80))`},
		{`(input/form/boolean "send" "Send updates")`, `(input/form/boolean "send" "Send updates")`},
		{`(input/form/number "amount" "Amount")`, `(input/form/number "amount" "Amount")`},
		{`(input/form/number "count" "Count" (input/form/number/min -10) (input/form/number/max 100) (input/form/number/integer))`, `(input/form/number "count" "Count" (input/form/number/min -10) (input/form/number/max 100) (input/form/number/integer))`},
		{`(input/form/options "size" "Size" (list "small" "large"))`, `(input/form/options "size" "Size" ("small" "large"))`},
		{`(input/form/list (input/form/text "tags" "Tags"))`, `(input/form/list (input/form/text "tags" "Tags"))`},
		{`(input/form/list (input/form/text "tags" "Tags" (input/form/text/min-length 1)))`, `(input/form/list (input/form/text "tags" "Tags" (input/form/text/min-length 1)))`},
		{`(input/form/optional (input/form/list (input/form/object "contacts" "Contacts" (input/form/text "name" "Name"))))`, `(input/form/optional (input/form/list (input/form/object "contacts" "Contacts" (input/form/text "name" "Name"))))`},
		{`(input/form "Review" (input/form/text "name" "Name") (input/form/optional (input/form/object "location" "Location" (input/form/options "region" "Region" (list "north" "south")))))`, `(input/form "Review" (input/form/text "name" "Name") (input/form/optional (input/form/object "location" "Location" (input/form/options "region" "Region" ("north" "south")))))`},
		{`(input/form "Review" (input/form/object "first" "First" (input/form/text "name" "Name")) (input/form/object "second" "Second" (input/form/text "name" "Name")))`, `(input/form "Review" (input/form/object "first" "First" (input/form/text "name" "Name")) (input/form/object "second" "Second" (input/form/text "name" "Name")))`},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, result := evaluateInputForm(test.source)
			if err != nil || result.String() != test.want {
				t.Fatalf("Evaluate() = (%s, %v), want %s", result, err, test.want)
			}
		})
	}
}

func TestInputFormConstructorsRejectInvalidShapes(t *testing.T) {
	for _, test := range []struct {
		source   string
		contains string
	}{
		{`(input/form "Review")`, "at least one field"},
		{`(input/form " " (input/form/text "name" "Name"))`, "non-blank title"},
		{`(input/form "Review" "not a field")`, "requires a field"},
		{`(input/form "Review" '(input/form/text "name"))`, "requires an id and label"},
		{`(input/form "Review" '(input/form/text "name" "Name" extra))`, "supported constraints"},
		{`(input/form "Review" '(input/form/unknown "name" "Name"))`, "supported field"},
		{`(input/form "Review" (input/form/text "name" "Name") (input/form/optional (input/form/boolean "name" "Another name")))`, "distinct ids"},
		{`(input/form "Review" (input/form/list (input/form/text "name" "Names")) (input/form/boolean "name" "Checked"))`, "distinct ids"},
		{`(input/form/object "group" "Group" (input/form/text "name" "Name") (input/form/text "name" "Another name"))`, "distinct ids"},
		{`(input/form/text " " "Name")`, "non-blank id"},
		{`(input/form/boolean "ok" " ")`, "non-blank label"},
		{`(input/form/boolean "ok" "OK" (input/form/text/min-length 1))`, "requires an id and label"},
		{`(input/form/text/min-length)`, "requires one integer"},
		{`(input/form/text/max-length -1)`, "non-negative integer"},
		{`(input/form/text/min-length "1")`, "non-negative integer"},
		{`(input/form/number/min "1.5")`, "requires an integer"},
		{`(input/form/number/integer 1)`, "requires no arguments"},
		{`(input/form/text "name" "Name" (input/form/number/min 1))`, "supported constraints"},
		{`(input/form/number "count" "Count" (input/form/text/max-length 2))`, "supported constraints"},
		{`(input/form/text "name" "Name" (input/form/text/min-length 1) (input/form/text/min-length 2))`, "duplicated"},
		{`(input/form/number "count" "Count" (input/form/number/integer) (input/form/number/integer))`, "duplicated"},
		{`(input/form/text "name" "Name" (input/form/text/max-length 2) (input/form/text/min-length 3))`, "minimum must not exceed maximum"},
		{`(input/form/number "count" "Count" (input/form/number/min 3) (input/form/number/max 2))`, "minimum must not exceed maximum"},
		{`(input/form/number "count" "Count" '(input/form/number/min "0"))`, "requires an integer"},
		{`(input/form/text "name" "Name" '(input/form/text/pattern "a"))`, "supported constraints"},
		{`(input/form/text "name" "Name" (taint/sensitive/mark (input/form/text/min-length 1)))`, "must not be sensitive"},
		{`(input/form/options "size" "Size" (list))`, "non-empty list"},
		{`(input/form/options "size" "Size" (list "small" "small"))`, "distinct"},
		{`(input/form/options "size" "Size" (list " "))`, "non-blank strings"},
		{`(input/form/options "size" "Size" (pair "small" "large"))`, "non-empty list"},
		{`(input/form/list)`, "requires one field"},
		{`(input/form/list (input/form/optional (input/form/text "tags" "Tags")))`, "non-optional field"},
		{`(input/form/list (input/form "Review" (input/form/text "name" "Name")))`, "supported field"},
		{`(input/form/optional (input/form/optional (input/form/text "name" "Name")))`, "non-optional field"},
		{`(input/form/optional (input/form "Review" (input/form/text "name" "Name")))`, "supported field"},
		{`(input/form/text (taint/secret/mark "name") "Name")`, "must not be sensitive"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, _ := evaluateInputForm(test.source)
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("Evaluate() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestInputFormValidatesHandBuiltTaggedValues(t *testing.T) {
	for _, test := range []struct {
		source   string
		contains string
	}{
		{`'(input/form "Review" (input/form/object "location" "Location" (input/form/text "name" "Name")))`, ""},
		{`'(input/form "Review" (input/form/list (input/form/object "contacts" "Contacts" (input/form/text "name" "Name"))))`, ""},
		{`'(input/form "Review" (input/form/text "name" "Name" (input/form/text/min-length 1) (input/form/text/max-length 80)) (input/form/number "count" "Count" (input/form/number/integer)))`, ""},
		{`'(input/form "Review" (input/form/text "same" "First") (input/form/boolean "same" "Second"))`, "distinct ids"},
		{`'(input/form "Review" (input/form/number "count" "Count" (input/form/number/min 10) (input/form/number/max 3)))`, "minimum must not exceed maximum"},
		{`'(input/form "Review" (input/form/text "name" "Name" (input/form/text/min-length -1)))`, "non-negative integer"},
		{`'(input/form "Review" (input/form/number "count" "Count" (input/form/number/integer "yes")))`, "supported constraints"},
		{`'(input/form "Review" (input/form/integer "count" "Count"))`, "supported field"},
		{`'(input/form "Review" (input/form/list (input/form/optional (input/form/text "name" "Name"))))`, "non-optional field"},
		{`'(input/form "Review" (input/form/optional (input/form/optional (input/form/text "x" "X"))))`, "non-optional field"},
		{`'(input/form "Review" (input/form/load "surface" "Surface" "path" null))`, "supported field"},
		{`'(input/form "Review" (input/form/file "picture" "Picture"))`, "supported field"},
		{`'(input/form "Review" (input/form/text "name" . "Name"))`, "requires a field"},
		{`(taint/secret/mark '(input/form "Review" (input/form/text "name" "Name")))`, "must not be sensitive"},
	} {
		t.Run(test.source, func(t *testing.T) {
			err, result := evaluateInputForm(test.source)
			if err != nil {
				t.Fatalf("Evaluate(): %v", err)
			}
			err = validateInputForm(result)
			if test.contains == "" {
				if err != nil {
					t.Fatalf("validateInputForm(): %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("validateInputForm() error = %v, want %q", err, test.contains)
			}
		})
	}
}

func TestInputFormModuleDocumentsConstructorsWithoutAsk(t *testing.T) {
	module := NewInputModule()
	if module.ID != gatehouseInputModuleID {
		t.Fatalf("module ID = %q", module.ID)
	}
	for _, export := range module.Exports {
		if export.Name == "ask" {
			t.Fatal("input/ask must wait for the effectful module implementation")
		}
	}
	err, result := evaluateInputForm(`(help input/form/text)`)
	if err != nil || !strings.Contains(result.String(), "Creates a text field") {
		t.Fatalf("help input/form/text = (%s, %v)", result, err)
	}
}

func TestInputFormDescriptorUsesVersionedJSON(t *testing.T) {
	err, value := evaluateInputForm(`(input/form "Review" (input/form/optional (input/form/list (input/form/object "contacts" "Contacts" (input/form/text "name" "Name" (input/form/text/min-length 1))))) (input/form/number "count" "Count" (input/form/number/min 0) (input/form/number/integer)))`)
	if err != nil {
		t.Fatal(err)
	}
	err, form := inputFormDescriptor(value)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(form)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"type":"form","title":"Review","fields":[{"id":"contacts","label":"Contacts","type":"list","optional":true,"item":{"type":"object","fields":[{"id":"name","label":"Name","type":"text","min_length":1}]}},{"id":"count","label":"Count","type":"number","min":0,"integer":true}]}`
	if string(encoded) != want {
		t.Fatalf("form JSON = %s, want %s", encoded, want)
	}
	if err := form.ValidateResult(json.RawMessage(`{"contacts":[{"name":"Ada"}],"count":0}`)); err != nil {
		t.Fatalf("validate serialized form result: %v", err)
	}
}

func TestInputAskPreludeDecodesJSONAndHidesNativeHook(t *testing.T) {
	called := false
	modules := agentResourceModules([]lisp.HostModule{
		NewProjectModule(nil, nil, nil), NewSessionModule(nil, nil, nil, nil), NewPolicyModule(nil), NewWebModule(),
	})
	for index := range modules {
		if modules[index].ID == gatehouseInputModuleID {
			modules[index] = NewInputModule(func(form inputform.Form) (error, string) {
				called = true
				if form.Title != "Review" || len(form.Fields) != 1 || form.Fields[0].ID != "name" {
					t.Errorf("input form = %#v", form)
				}
				return nil, `{"name":"Ada"}`
			})
		}
	}
	err, result := lisp.Evaluate(`(list (json/string/value (json/object/get (input/ask (input/form "Review" (input/form/text "name" "Name"))) "name")) (error? (error/catch (input/ask-json (input/form "Review" (input/form/text "name" "Name"))))) )`, lisp.EvalOptions{
		Prelude: agentPrelude, HostModules: modules,
	})
	if err != nil || result.String() != `("Ada" #t)` || !called {
		t.Fatalf("input/ask prelude = (%s, %v), callback called = %t", result, err, called)
	}
}

func TestInputAskRequiresAValidFormAndAvailableCallback(t *testing.T) {
	for _, test := range []struct {
		source, want string
	}{
		{`(input/ask-json)`, "input/ask requires one form"},
		{`(input/ask-json "not a form")`, "input/form requires a title and at least one field"},
		{`(input/ask-json (input/form "Review" (input/form/text "name" "Name")))`, "input/ask is unavailable"},
	} {
		err, _ := evaluateInputForm(test.source)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("Evaluate(%s) error = %v, want %q", test.source, err, test.want)
		}
	}
}
