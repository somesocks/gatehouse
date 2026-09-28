package agent

import (
	"strings"

	"gatehouse/inputform"
	"gatehouse/lisp"
)

const gatehouseInputModuleID = "native:gatehouse/input/v1"

// NewInputModule exposes form constructors and a narrow JSON-text wait hook.
// The default agent prelude wraps ask-json with json/decode as input/ask.
func NewInputModule(requestInput ...func(inputform.Form) (error, string)) lisp.HostModule {
	exports := []lisp.HostExport{
		inputConstructor("form", "(input/form title fields...) -> Input Form", "Creates a form with a non-blank title and at least one field.", `(input/form "Review" (input/form/text "name" "Name"))`, `(input/form "Review" (input/form/text "name" "Name"))`, inputForm),
		inputConstructor("form/boolean", "(input/form/boolean id label) -> Input Field", "Creates a boolean field.", `(input/form/boolean "updates" "Receive updates")`, `(input/form/boolean "updates" "Receive updates")`, inputSimpleField("boolean")),
		inputConstructor("form/files", "(input/form/files id label constraint...) -> Input Field", "Uploads session files and returns an array of file summaries.", `(input/form/files "photos" "Photos" (input/form/files/max-files 5))`, `(input/form/files "photos" "Photos" (input/form/files/max-files 5))`, inputSimpleField("files")),
		inputConstructor("form/files/max-files", "(input/form/files/max-files count) -> Input Constraint", "Limits the number of uploaded files to a positive count.", `(input/form/files/max-files 5)`, `(input/form/files/max-files 5)`, inputFileMaxFiles),
		inputConstructor("form/files/media-types", "(input/form/files/media-types types) -> Input Constraint", "Accepts declared media types such as image/png or image/*.", `(input/form/files/media-types (list "image/*"))`, `(input/form/files/media-types ("image/*"))`, inputFileMediaTypes),
		inputConstructor("form/number", "(input/form/number id label constraint...) -> Input Field", "Creates a numeric field. Bounds are integer-valued; number/integer restricts answers to whole numbers.", `(input/form/number "count" "Count" (input/form/number/min 0) (input/form/number/integer))`, `(input/form/number "count" "Count" (input/form/number/min 0) (input/form/number/integer))`, inputSimpleField("number")),
		inputConstructor("form/number/integer", "(input/form/number/integer) -> Input Constraint", "Requires a numeric answer to be an integer.", `(input/form/number/integer)`, `(input/form/number/integer)`, inputNumberInteger),
		inputConstructor("form/number/min", "(input/form/number/min integer) -> Input Constraint", "Sets an inclusive integer-valued lower bound for a numeric answer.", `(input/form/number/min 0)`, `(input/form/number/min 0)`, inputBound("number", "min", false)),
		inputConstructor("form/number/max", "(input/form/number/max integer) -> Input Constraint", "Sets an inclusive integer-valued upper bound for a numeric answer.", `(input/form/number/max 100)`, `(input/form/number/max 100)`, inputBound("number", "max", false)),
		inputConstructor("form/list", "(input/form/list field) -> Input Field", "Repeats a field as a JSON array under that field's id. Wrap the list in input/form/optional to allow the whole field to be omitted.", `(input/form/list (input/form/text "tags" "Tags"))`, `(input/form/list (input/form/text "tags" "Tags"))`, inputList),
		inputConstructor("form/object", "(input/form/object id label fields...) -> Input Field", "Groups fields into a nested result object.", `(input/form/object "contact" "Contact" (input/form/text "name" "Name"))`, `(input/form/object "contact" "Contact" (input/form/text "name" "Name"))`, inputObject),
		inputConstructor("form/options", "(input/form/options id label choices) -> Input Field", "Creates a choice field from distinct, non-blank strings.", `(input/form/options "size" "Size" (list "small" "large"))`, `(input/form/options "size" "Size" ("small" "large"))`, inputOptions),
		inputConstructor("form/optional", "(input/form/optional field) -> Input Field", "Marks a field as optional. An unanswered optional field is omitted from the result.", `(input/form/optional (input/form/text "notes" "Notes"))`, `(input/form/optional (input/form/text "notes" "Notes"))`, inputOptional),
		inputConstructor("form/text", "(input/form/text id label constraint...) -> Input Field", "Creates a text field. Length constraints count Unicode code points.", `(input/form/text "name" "Name" (input/form/text/min-length 1))`, `(input/form/text "name" "Name" (input/form/text/min-length 1))`, inputSimpleField("text")),
		inputConstructor("form/text/min-length", "(input/form/text/min-length integer) -> Input Constraint", "Sets the minimum text length in Unicode code points.", `(input/form/text/min-length 1)`, `(input/form/text/min-length 1)`, inputBound("text", "min-length", true)),
		inputConstructor("form/text/max-length", "(input/form/text/max-length integer) -> Input Constraint", "Sets the maximum text length in Unicode code points.", `(input/form/text/max-length 80)`, `(input/form/text/max-length 80)`, inputBound("text", "max-length", true)),
	}
	var request func(inputform.Form) (error, string)
	if len(requestInput) != 0 {
		request = requestInput[0]
	}
	exports = append(exports, inputConstructor("ask-json", "(input/ask-json form) -> String", "Requests input and waits for a JSON object result. The default prelude exposes input/ask, which decodes this text with the JSON module.", `(input/ask-json (input/form "Review" (input/form/text "name" "Name")))`, `"{\"name\":\"Ada\"}"`, func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("input/ask requires one form"), nil
		}
		err, form := inputFormDescriptor(arguments[0])
		if err != nil {
			return err, nil
		}
		if request == nil {
			return lisp.Errorf("input/ask is unavailable"), nil
		}
		err, result := request(form)
		if err != nil {
			return err, nil
		}
		return nil, lisp.String(result)
	}))
	return lisp.HostModule{ID: gatehouseInputModuleID, Exports: exports}
}

func inputConstructor(name, signature, description, example, result string, call func([]lisp.Expr) (error, lisp.Expr)) lisp.HostExport {
	return lisp.HostExport{Name: name, Value: document(lisp.FunctionNonLeaky(call), capabilityDocumentation{signature, description, example, result})}
}

func inputForm(arguments []lisp.Expr) (error, lisp.Expr) {
	form := inputTagged("form", arguments...)
	if err := validateInputForm(form); err != nil {
		return err, nil
	}
	return nil, form
}

// validateInputForm checks the tagged tree independently of the constructors,
// since Lisp callers can also assemble or quote these values themselves.
func validateInputForm(form lisp.Expr) error {
	if err := inputPublic([]lisp.Expr{form}); err != nil {
		return err
	}
	err, values := lisp.RequireList(form)
	if err != nil || len(values) < 3 {
		return lisp.Errorf("input/form requires a title and at least one field")
	}
	err, tag := lisp.RequireSymbol(values[0])
	if err != nil || tag != "input/form" {
		return lisp.Errorf("requires an input form")
	}
	if err := inputPublic(values[1:]); err != nil {
		return err
	}
	if err := inputLabel(values[1]); err != nil {
		return lisp.Errorf("input/form requires a non-blank title")
	}
	return inputFields(values[2:])
}

func inputObject(arguments []lisp.Expr) (error, lisp.Expr) {
	if len(arguments) < 3 {
		return lisp.Errorf("input/form/object requires an id, label, and at least one field"), nil
	}
	if err := inputPublic(arguments); err != nil {
		return err, nil
	}
	if err := inputIdentity(arguments[:2]); err != nil {
		return err, nil
	}
	if err := inputFields(arguments[2:]); err != nil {
		return err, nil
	}
	return nil, inputTagged("form/object", arguments...)
}

func inputSimpleField(kind string) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) < 2 || (kind == "boolean" && len(arguments) != 2) {
			return lisp.Errorf("input/form/%s requires an id and label%s", kind, inputConstraintSuffix(kind)), nil
		}
		if err := inputPublic(arguments); err != nil {
			return err, nil
		}
		if err := inputIdentity(arguments[:2]); err != nil {
			return err, nil
		}
		if err := inputConstraints(kind, arguments[2:]); err != nil {
			return err, nil
		}
		return nil, inputTagged("form/"+kind, arguments...)
	}
}

func inputConstraintSuffix(kind string) string {
	if kind == "boolean" {
		return ""
	}
	return " and optional constraints"
}

func inputBound(kind, name string, nonnegative bool) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("input/form/%s/%s requires one integer", kind, name), nil
		}
		if err := inputPublic(arguments); err != nil {
			return err, nil
		}
		err, value := lisp.RequireInteger(arguments[0])
		if err != nil || (nonnegative && value < 0) {
			return lisp.Errorf("input/form/%s/%s requires %s integer", kind, name, inputBoundDescription(nonnegative)), nil
		}
		return nil, inputTagged("form/"+kind+"/"+name, arguments[0])
	}
}

func inputBoundDescription(nonnegative bool) string {
	if nonnegative {
		return "a non-negative"
	}
	return "an"
}

func inputNumberInteger(arguments []lisp.Expr) (error, lisp.Expr) {
	if len(arguments) != 0 {
		return lisp.Errorf("input/form/number/integer requires no arguments"), nil
	}
	return nil, inputTagged("form/number/integer")
}

func inputFileMaxFiles(arguments []lisp.Expr) (error, lisp.Expr) {
	if len(arguments) != 1 {
		return lisp.Errorf("input/form/files/max-files requires one positive integer"), nil
	}
	if err := inputPublic(arguments); err != nil {
		return err, nil
	}
	valueErr, value := lisp.RequireInteger(arguments[0])
	if valueErr != nil || value <= 0 {
		return lisp.Errorf("input/form/files/max-files requires one positive integer"), nil
	}
	return nil, inputTagged("form/files/max-files", arguments[0])
}

func inputFileMediaTypes(arguments []lisp.Expr) (error, lisp.Expr) {
	if len(arguments) != 1 {
		return lisp.Errorf("input/form/files/media-types requires a non-empty list"), nil
	}
	if err := inputPublic(arguments); err != nil {
		return err, nil
	}
	valuesErr, values := lisp.RequireList(arguments[0])
	if valuesErr != nil || len(values) == 0 {
		return lisp.Errorf("input/form/files/media-types requires a non-empty list"), nil
	}
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		textErr, text := lisp.RequireString(value)
		if textErr != nil || !inputform.ValidMediaTypePattern(text) || seen[text] {
			return lisp.Errorf("input/form/files/media-types requires distinct valid MIME types"), nil
		}
		seen[text] = true
	}
	return nil, inputTagged("form/files/media-types", arguments[0])
}

func inputOptions(arguments []lisp.Expr) (error, lisp.Expr) {
	if len(arguments) != 3 {
		return lisp.Errorf("input/form/options requires an id, label, and list of choices"), nil
	}
	if err := inputPublic(arguments); err != nil {
		return err, nil
	}
	if err := inputIdentity(arguments[:2]); err != nil {
		return err, nil
	}
	if err := inputChoices(arguments[2]); err != nil {
		return err, nil
	}
	return nil, inputTagged("form/options", arguments...)
}

func inputOptional(arguments []lisp.Expr) (error, lisp.Expr) {
	if len(arguments) != 1 {
		return lisp.Errorf("input/form/optional requires one field"), nil
	}
	if err := inputPublic(arguments); err != nil {
		return err, nil
	}
	if _, err := inputFieldID(arguments[0], false); err != nil {
		return err, nil
	}
	return nil, inputTagged("form/optional", arguments[0])
}

func inputList(arguments []lisp.Expr) (error, lisp.Expr) {
	if len(arguments) != 1 {
		return lisp.Errorf("input/form/list requires one field"), nil
	}
	if err := inputPublic(arguments); err != nil {
		return err, nil
	}
	if _, err := inputFieldID(arguments[0], false); err != nil {
		return err, nil
	}
	return nil, inputTagged("form/list", arguments[0])
}

func inputTagged(name string, values ...lisp.Expr) lisp.Expr {
	return lisp.List(append([]lisp.Expr{lisp.Symbol("input/" + name)}, values...)...)
}

func inputPublic(values []lisp.Expr) error {
	for _, value := range values {
		if lisp.TaintOf(value) != lisp.TaintNone {
			return lisp.Errorf("input form descriptors must not be sensitive")
		}
	}
	return nil
}

func inputLabel(value lisp.Expr) error {
	err, label := lisp.RequireString(value)
	if err != nil || strings.TrimSpace(label) == "" {
		return lisp.Errorf("requires a non-blank string")
	}
	return nil
}

func inputIdentity(values []lisp.Expr) error {
	if err := inputLabel(values[0]); err != nil {
		return lisp.Errorf("input field requires a non-blank id")
	}
	if err := inputLabel(values[1]); err != nil {
		return lisp.Errorf("input field requires a non-blank label")
	}
	return nil
}

func inputChoices(value lisp.Expr) error {
	err, choices := lisp.RequireList(value)
	if err != nil || len(choices) == 0 {
		return lisp.Errorf("input/form/options requires a non-empty list of choices")
	}
	seen := make(map[string]bool, len(choices))
	for _, choice := range choices {
		if err := inputLabel(choice); err != nil {
			return lisp.Errorf("input/form/options choices must be non-blank strings")
		}
		_, text := lisp.RequireString(choice)
		if seen[text] {
			return lisp.Errorf("input/form/options choices must be distinct")
		}
		seen[text] = true
	}
	return nil
}

// inputConstraints checks the constraint tags, argument types, uniqueness,
// and consistency independently of the constraint constructors.
func inputConstraints(kind string, constraints []lisp.Expr) error {
	if kind == "boolean" && len(constraints) != 0 {
		return lisp.Errorf("input/form/boolean requires an id and label")
	}
	seen := make(map[string]bool, len(constraints))
	var lower, upper int64
	var hasLower, hasUpper bool
	for _, constraint := range constraints {
		if err := inputPublic([]lisp.Expr{constraint}); err != nil {
			return err
		}
		err, values := lisp.RequireList(constraint)
		if err != nil || len(values) == 0 {
			return lisp.Errorf("input/form/%s requires supported constraints", kind)
		}
		err, tag := lisp.RequireSymbol(values[0])
		if err != nil || !strings.HasPrefix(tag, "input/form/"+kind+"/") {
			return lisp.Errorf("input/form/%s requires supported constraints", kind)
		}
		name := strings.TrimPrefix(tag, "input/form/"+kind+"/")
		if seen[name] {
			return lisp.Errorf("input/form/%s constraint %s is duplicated", kind, name)
		}
		seen[name] = true
		if kind == "number" && name == "integer" && len(values) == 1 {
			continue
		}
		if kind == "files" {
			switch name {
			case "max-files":
				if len(values) != 2 {
					return lisp.Errorf("input/form/files/max-files requires one positive integer")
				}
				err, count := lisp.RequireInteger(values[1])
				if err != nil || count <= 0 {
					return lisp.Errorf("input/form/files/max-files requires one positive integer")
				}
			case "media-types":
				if len(values) != 2 {
					return lisp.Errorf("input/form/files/media-types requires a non-empty list")
				}
				if err, _ := inputFileMediaTypes(values[1:]); err != nil {
					return err
				}
			default:
				return lisp.Errorf("input/form/files requires supported constraints")
			}
			continue
		}
		lowerBound := (kind == "text" && name == "min-length") || (kind == "number" && name == "min")
		upperBound := (kind == "text" && name == "max-length") || (kind == "number" && name == "max")
		if (!lowerBound && !upperBound) || len(values) != 2 {
			return lisp.Errorf("input/form/%s requires supported constraints", kind)
		}
		err, value := lisp.RequireInteger(values[1])
		if err != nil || (kind == "text" && value < 0) {
			return lisp.Errorf("input/form/%s/%s requires %s integer", kind, name, inputBoundDescription(kind == "text"))
		}
		if lowerBound {
			lower, hasLower = value, true
		} else {
			upper, hasUpper = value, true
		}
	}
	if hasLower && hasUpper && lower > upper {
		return lisp.Errorf("input/form/%s minimum must not exceed maximum", kind)
	}
	return nil
}

func inputFields(fields []lisp.Expr) error {
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		id, err := inputFieldID(field, true)
		if err != nil {
			return err
		}
		if seen[id] {
			return lisp.Errorf("input form fields must have distinct ids within an object")
		}
		seen[id] = true
	}
	return nil
}

// inputFieldID recursively validates even quoted, hand-built tagged values.
// Optional wraps one field; it is not a separate result key.
func inputFieldID(field lisp.Expr, allowOptional bool) (string, error) {
	if err := inputPublic([]lisp.Expr{field}); err != nil {
		return "", err
	}
	err, values := lisp.RequireList(field)
	if err != nil || len(values) == 0 {
		return "", lisp.Errorf("input form requires a field")
	}
	err, tag := lisp.RequireSymbol(values[0])
	if err != nil {
		return "", lisp.Errorf("input form requires a field")
	}
	arguments := values[1:]
	if err := inputPublic(arguments); err != nil {
		return "", err
	}
	if tag == "input/form/optional" {
		if !allowOptional || len(arguments) != 1 {
			return "", lisp.Errorf("input/form/optional requires one non-optional field")
		}
		return inputFieldID(arguments[0], false)
	}
	if tag == "input/form/list" {
		if len(arguments) != 1 {
			return "", lisp.Errorf("input/form/list requires one field")
		}
		return inputFieldID(arguments[0], false)
	}
	switch tag {
	case "input/form/object":
		if len(arguments) < 3 {
			return "", lisp.Errorf("input/form/object requires an id, label, and at least one field")
		}
		if err := inputIdentity(arguments[:2]); err != nil {
			return "", err
		}
		if err := inputFields(arguments[2:]); err != nil {
			return "", err
		}
	case "input/form/text", "input/form/boolean", "input/form/number", "input/form/files":
		kind := strings.TrimPrefix(tag, "input/form/")
		if len(arguments) < 2 || (kind == "boolean" && len(arguments) != 2) {
			return "", lisp.Errorf("%s requires an id and label%s", tag, inputConstraintSuffix(kind))
		}
		if err := inputIdentity(arguments[:2]); err != nil {
			return "", err
		}
		if err := inputConstraints(kind, arguments[2:]); err != nil {
			return "", err
		}
	case "input/form/options":
		if len(arguments) != 3 {
			return "", lisp.Errorf("input/form/options requires an id, label, and list of choices")
		}
		if err := inputIdentity(arguments[:2]); err != nil {
			return "", err
		}
		if err := inputChoices(arguments[2]); err != nil {
			return "", err
		}
	default:
		return "", lisp.Errorf("input form requires a supported field")
	}
	_, id := lisp.RequireString(arguments[0])
	return id, nil
}
