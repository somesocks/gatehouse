package agent

import (
	"strings"
	"testing"

	"gatehouse/lisp"
)

func TestInputCustomConstructors(t *testing.T) {
	for _, test := range []struct {
		source string
		valid  bool
	}{
		{`(input/form/custom "file" "File" "/app/tools/session-file-picker/" (json/object))`, true},
		{`(input/form/optional (input/form/custom "file" "File" "/app/tools/session-file-picker/" (json/number "9007199254740993") (input/form/custom/capabilities (list "session.file.list"))))`, true},
		{`(input/form/custom "file" "File" "/app/tools/session-file-picker/" "not JSON")`, false},
		{`(input/form/custom "file" "File" "javascript:alert(1)" (json/object))`, false},
		{`(input/form/custom "file" "File" "/app/tools/session-file-picker/" (json/object) (input/form/custom/capabilities (list "project.file.list")))`, false},
		{`(input/form/custom "file" "File" "/app/tools/session-file-picker/" (json/object) (input/form/custom/capabilities (list "session.file.upload" "session.file.upload")))`, false},
	} {
		err, value := lisp.Evaluate(`(import (json @native:json/v1) (input @native:gatehouse/input/v1) (input/form "Select" `+test.source+`))`, lisp.EvalOptions{HostModules: []lisp.HostModule{NewInputModule()}})
		if (err == nil) != test.valid {
			t.Fatalf("%s: %v", test.source, err)
		}
		if !test.valid {
			continue
		}
		err, form := inputFormDescriptor(value)
		if err != nil {
			t.Fatal(err)
		}
		if form.Fields[0].Custom == nil || form.Fields[0].Custom.URL != "/app/tools/session-file-picker/" {
			t.Fatalf("custom descriptor = %#v", form)
		}
		if strings.Contains(test.source, "9007199254740993") && string(form.Fields[0].Custom.Inputs) != "9007199254740993" {
			t.Fatal("custom inputs lost numeric precision")
		}
	}
}
