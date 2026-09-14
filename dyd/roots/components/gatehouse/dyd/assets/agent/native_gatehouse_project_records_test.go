package agent

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"gatehouse/lisp"
)

func TestGatehouseProjectRecordsInspectAndMutate(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (list
    (project/records/schemas/list)
    (project/records/attributes/list "schema")
    (project/records/list "schema")
    (project/records/values/list "schema" "record")))`, lisp.EvalOptions{HostModules: []lisp.HostModule{NewProjectModuleWithRecords(nil, nil, nil, &ProjectRecords{
		Schemas: func() (error, []ProjectRecordSchema) {
			return nil, []ProjectRecordSchema{{ID: "schema", Name: "contacts", Label: "Contacts", Description: "People", AuthorID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z"}}
		},
		Attributes: func(schemaID string) (error, []ProjectRecordAttribute) {
			if schemaID != "schema" {
				t.Fatalf("attribute schema = %q", schemaID)
			}
			return nil, []ProjectRecordAttribute{{ID: "attribute", Name: "email", Label: "Email", Type: "text", Cardinality: "one", Uniqueness: "global", Display: "primary", AuthorID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z"}}
		},
		Records: func(schemaID, cursor string) (error, []ProjectRecord) {
			if schemaID != "schema" || cursor != "" {
				t.Fatalf("records = (%q, %q)", schemaID, cursor)
			}
			return nil, []ProjectRecord{{ID: "record", AuthorID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z"}}
		},
		Values: func(schemaID, recordID, cursor string) (error, []ProjectRecordValue) {
			if schemaID != "schema" || recordID != "record" || cursor != "" {
				t.Fatalf("values = (%q, %q, %q)", schemaID, recordID, cursor)
			}
			return nil, []ProjectRecordValue{{ID: "value", Attribute: "attribute", Value: "sensitive", Sensitive: true, AuthorID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z"}}
		},
	})}})
	if err != nil || !lisp.IsSensitive(result) {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}

	err, result = lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/records/create "schema" (list (pair "email" value))))`, lisp.EvalOptions{
		Bindings: []lisp.Binding{{Name: "value", Value: lisp.MarkSensitive(lisp.String("ada@example.test"))}},
		HostModules: []lisp.HostModule{NewProjectModuleWithRecords(nil, nil, nil, &ProjectRecords{RecordCreate: func(schemaID string, values []ProjectRecordValueInput) (error, ProjectRecord, []ProjectRecordValue) {
			if schemaID != "schema" || len(values) != 1 || values[0].Attribute != "email" || values[0].Value != "ada@example.test" || !values[0].Sensitive {
				t.Fatalf("record create = (%q, %#v)", schemaID, values)
			}
			return nil, ProjectRecord{ID: "record", AuthorID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z"}, []ProjectRecordValue{{ID: "value", Attribute: "attribute", Value: "ada@example.test", Sensitive: true, AuthorID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z"}}
		}})},
	})
	if err != nil || !lisp.IsSensitive(result) {
		t.Fatalf("Evaluate() create = (%s, %v)", result, err)
	}
}

func TestGatehouseProjectRecordListCursors(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (list
    (project/records/list "schema" "prr_cursor")
    (project/records/values/list "schema" "record" "prv_cursor")))`, lisp.EvalOptions{HostModules: []lisp.HostModule{NewProjectModuleWithRecords(nil, nil, nil, &ProjectRecords{
		Records: func(schemaID, cursor string) (error, []ProjectRecord) {
			if schemaID != "schema" || cursor != "prr_cursor" {
				t.Fatalf("records cursor = (%q, %q)", schemaID, cursor)
			}
			return nil, []ProjectRecord{}
		},
		Values: func(schemaID, recordID, cursor string) (error, []ProjectRecordValue) {
			if schemaID != "schema" || recordID != "record" || cursor != "prv_cursor" {
				t.Fatalf("values cursor = (%q, %q, %q)", schemaID, recordID, cursor)
			}
			return nil, []ProjectRecordValue{}
		},
	})}})
	if err != nil || result.String() != `(null null)` {
		t.Fatalf("Evaluate() cursor pages = (%s, %v)", result, err)
	}

	err, result = lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (list
    (error/value (error/catch (project/records/list "schema" "")))
    (error/value (error/catch (project/records/values/list "schema" "record" "")))
    (error/value (error/catch (project/records/list "schema" "cursor" "extra")))
    (error/value (error/catch (project/records/values/list "schema" "record" "cursor" "extra")))))`, lisp.EvalOptions{HostModules: []lisp.HostModule{NewProjectModuleWithRecords(nil, nil, nil, &ProjectRecords{
		Records: func(string, string) (error, []ProjectRecord) {
			t.Fatal("records callback was called for invalid arguments")
			return nil, nil
		},
		Values: func(string, string, string) (error, []ProjectRecordValue) {
			t.Fatal("values callback was called for invalid arguments")
			return nil, nil
		},
	})}})
	if err != nil || result.String() != `("project/records/list requires a non-empty id" "project/records/values/list requires a non-empty id" "project/records/list requires a schema id and optional cursor" "project/records/values/list requires schema id, record id, and optional cursor")` {
		t.Fatalf("Evaluate() invalid cursors = (%s, %v)", result, err)
	}
}

func TestGatehouseProjectRecordsRejectSecretWrites(t *testing.T) {
	err, _ := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/records/create "schema" (list (pair "email" value))))`, lisp.EvalOptions{
		Bindings: []lisp.Binding{{Name: "value", Value: lisp.MarkSecret(lisp.String("secret"))}},
		HostModules: []lisp.HostModule{NewProjectModuleWithRecords(nil, nil, nil, &ProjectRecords{RecordCreate: func(string, []ProjectRecordValueInput) (error, ProjectRecord, []ProjectRecordValue) {
			t.Fatal("record create callback was called with a secret")
			return nil, ProjectRecord{}, nil
		}})},
	})
	if err == nil || !strings.Contains(err.Error(), "must not use secret") {
		t.Fatalf("Evaluate() secret record value error = %v", err)
	}
}

func TestGatehouseProjectRecordAttributeErrorsAndHelp(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (error/value (error/catch (project/records/attributes/create "schema" "organization" "Organization" "Owning organization" "record" "target" "one" "none" "secondary"))))`, lisp.EvalOptions{HostModules: []lisp.HostModule{NewProjectModuleWithRecords(nil, nil, nil, &ProjectRecords{AttributeCreate: func(string, ProjectRecordAttributeInput) (error, ProjectRecordAttribute) {
		return errors.New("target schema is unavailable"), ProjectRecordAttribute{}
	}})}})
	if err != nil || result.String() != `"project/records/attributes/create failed"` {
		t.Fatalf("Evaluate() attribute error = (%s, %v)", result, err)
	}
	err, result = lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (project/records/attributes/create "schema" "attachment" "Attachment" "Source file" "file" null "one" "none" "secondary"))`, lisp.EvalOptions{HostModules: []lisp.HostModule{NewProjectModuleWithRecords(nil, nil, nil, &ProjectRecords{AttributeCreate: func(schemaID string, input ProjectRecordAttributeInput) (error, ProjectRecordAttribute) {
		if schemaID != "schema" || input.Type != "file" || input.TargetSchema != nil {
			t.Fatalf("file attribute create = (%q, %#v)", schemaID, input)
		}
		return nil, ProjectRecordAttribute{ID: "attribute", Name: input.Name, Label: input.Label, Description: input.Description, Type: input.Type, Cardinality: input.Cardinality, Uniqueness: input.Uniqueness, Display: input.Display, AuthorID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z"}
	}})}})
	if err != nil || !strings.Contains(result.String(), `(type . "file")`) {
		t.Fatalf("Evaluate() file attribute = (%s, %v)", result, err)
	}

	err, result = lisp.Evaluate(`(help 'project/records/attributes/create)`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModuleWithRecords(nil, nil, nil, &ProjectRecords{}),
			NewSessionModule(nil, nil, nil, nil),
			NewPolicyModule(nil),
			NewWebModule(),
		},
	})
	if err != nil || !strings.Contains(result.String(), "file") || !strings.Contains(result.String(), "target-schema") {
		t.Fatalf("Evaluate() record attribute help = (%s, %v)", result, err)
	}
}

func TestGatehouseProjectRecordAttributeDisplayOrder(t *testing.T) {
	for _, test := range []struct {
		name       string
		expression string
		display    int
		update     bool
	}{
		{name: "create legacy default", expression: `(project/records/attributes/create "schema" "organization" "Organization" "Owning organization" "record" "target" "one" "none" "secondary")`, display: 0},
		{name: "create explicit", expression: `(project/records/attributes/create "schema" "organization" "Organization" "Owning organization" "record" "target" "one" "none" "secondary" 23)`, display: 23},
		{name: "update legacy default", expression: `(project/records/attributes/update "schema" "attribute" "Organization" "Owning organization" "record" "target" "one" "none" "secondary")`, display: 0, update: true},
		{name: "update explicit", expression: `(project/records/attributes/update "schema" "attribute" "Organization" "Owning organization" "record" "target" "one" "none" "secondary" 23)`, display: 23, update: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			attribute := ProjectRecordAttribute{ID: "attribute", Name: "organization", Label: "Organization", Description: "Owning organization", Type: "record", Cardinality: "one", Uniqueness: "none", Display: "secondary", DisplayOrder: test.display, AuthorID: "agent", CreatedAt: "2026-01-01T00:00:00.000Z"}
			err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  `+test.expression+`)`, lisp.EvalOptions{HostModules: []lisp.HostModule{NewProjectModuleWithRecords(nil, nil, nil, &ProjectRecords{
				AttributeCreate: func(schemaID string, input ProjectRecordAttributeInput) (error, ProjectRecordAttribute) {
					if test.update || schemaID != "schema" || input.DisplayOrder != test.display {
						t.Fatalf("attribute create = (%q, %#v)", schemaID, input)
					}
					return nil, attribute
				},
				AttributeUpdate: func(schemaID, id string, input ProjectRecordAttributeUpdateInput) (error, *ProjectRecordAttribute) {
					if !test.update || schemaID != "schema" || id != "attribute" || input.DisplayOrder != test.display {
						t.Fatalf("attribute update = (%q, %q, %#v)", schemaID, id, input)
					}
					return nil, &attribute
				},
			})}})
			if err != nil || !strings.Contains(result.String(), "(display_order . "+strconv.Itoa(test.display)+")") {
				t.Fatalf("Evaluate() display order = (%s, %v)", result, err)
			}
		})
	}

	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (error/value (error/catch (project/records/attributes/create "schema" "organization" "Organization" "Owning organization" "record" "target" "one" "none" "secondary" -1))))`, lisp.EvalOptions{HostModules: []lisp.HostModule{NewProjectModuleWithRecords(nil, nil, nil, &ProjectRecords{
		AttributeCreate: func(string, ProjectRecordAttributeInput) (error, ProjectRecordAttribute) {
			t.Fatal("attribute create callback was called with a negative display order")
			return nil, ProjectRecordAttribute{}
		},
	})}})
	if err != nil || result.String() != `"project/records/attributes/create has invalid attribute input"` {
		t.Fatalf("Evaluate() negative display order = (%s, %v)", result, err)
	}
}

func TestGatehouseProjectRecordMutationErrorsAreSafe(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{name: "project/records/create", err: errors.New(`insert project record value: ERROR: duplicate key value violates unique constraint "gatehouse_project_record_values_global_unique" (SQLSTATE 23505): Key (value_key)=(text:private@example.test) already exists`), want: "project/records/create failed: a record value marked unique already exists"},
		{name: "project/records/values/mutate", err: errors.New("create project record value: project record attribute is unavailable"), want: "project/records/values/mutate failed: the record attribute is unavailable"},
		{name: "project/records/schemas/remove", err: errors.New("remove project record schema: ERROR: update or delete on table violates foreign key constraint"), want: "project/records/schemas/remove failed: the schema has dependent records or attributes"},
		{name: "project/records/attributes/create", err: errors.New("insert project record attribute: FOREIGN KEY constraint failed"), want: "project/records/attributes/create failed: the record reference target is unavailable"},
		{name: "project/records/remove", err: errors.New("remove project record: ERROR: update or delete on table violates foreign key constraint"), want: "project/records/remove failed: the record is referenced by another record"},
		{name: "project/records/remove", err: errors.New("remove project record: FOREIGN KEY constraint failed"), want: "project/records/remove failed: the record is referenced by another record"},
		{name: "project/records/create", err: errors.New("database connection is unavailable"), want: "project/records/create failed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := projectRecordMutationError(test.name, test.err).Error()
			if got != test.want {
				t.Fatalf("projectRecordMutationError() = %q, want %q", got, test.want)
			}
			if strings.Contains(got, "private@example.test") {
				t.Fatalf("projectRecordMutationError() leaked a record value: %q", got)
			}
		})
	}
}

func TestGatehouseProjectRecordsUnavailable(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (project @native:gatehouse/project/v1)
  (list
    (error/value (error/catch (project/records/schemas/list)))
    (error/value (error/catch (project/records/create "schema" (list))))
    (error/value (error/catch (project/records/values/mutate "schema" "record" (list) (list) (list))))))`, lisp.EvalOptions{HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil)}})
	if err != nil || result.String() != `("project/records/schemas/list is unavailable" "project/records/create is unavailable" "project/records/values/mutate is unavailable")` {
		t.Fatalf("Evaluate() unavailable records = (%s, %v)", result, err)
	}
}
