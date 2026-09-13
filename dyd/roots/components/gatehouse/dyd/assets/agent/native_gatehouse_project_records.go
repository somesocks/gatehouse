package agent

import (
	"strings"

	"gatehouse/lisp"
)

// ProjectRecords contains authorized project record operations.
type ProjectRecords struct {
	Schemas         ProjectRecordSchemasGet
	Schema          ProjectRecordSchemaGet
	SchemaCreate    ProjectRecordSchemaCreate
	SchemaUpdate    ProjectRecordSchemaUpdate
	SchemaRemove    ProjectRecordSchemaRemove
	Attributes      ProjectRecordAttributesGet
	Attribute       ProjectRecordAttributeGet
	AttributeCreate ProjectRecordAttributeCreate
	AttributeUpdate ProjectRecordAttributeUpdate
	AttributeRemove ProjectRecordAttributeRemove
	Records         ProjectRecordsGet
	Record          ProjectRecordGet
	RecordCreate    ProjectRecordCreate
	RecordRemove    ProjectRecordRemove
	Values          ProjectRecordValuesGet
	ValuesMutate    ProjectRecordValuesMutate
}

type ProjectRecordSchemasGet func() (error, []ProjectRecordSchema)
type ProjectRecordSchemaGet func(id string) (error, *ProjectRecordSchema)
type ProjectRecordSchemaCreate func(name, label, description string) (error, ProjectRecordSchema)
type ProjectRecordSchemaUpdate func(id, label, description string) (error, *ProjectRecordSchema)
type ProjectRecordSchemaRemove func(id string) (error, bool)
type ProjectRecordAttributesGet func(schemaID string) (error, []ProjectRecordAttribute)
type ProjectRecordAttributeGet func(schemaID, id string) (error, *ProjectRecordAttribute)
type ProjectRecordAttributeCreate func(schemaID string, attribute ProjectRecordAttributeInput) (error, ProjectRecordAttribute)
type ProjectRecordAttributeUpdate func(schemaID, id string, attribute ProjectRecordAttributeUpdateInput) (error, *ProjectRecordAttribute)
type ProjectRecordAttributeRemove func(schemaID, id string) (error, bool)
type ProjectRecordsGet func(schemaID string) (error, []ProjectRecord)
type ProjectRecordGet func(schemaID, id string) (error, *ProjectRecord)
type ProjectRecordCreate func(schemaID string, values []ProjectRecordValueInput) (error, ProjectRecord, []ProjectRecordValue)
type ProjectRecordRemove func(schemaID, id string) (error, bool)
type ProjectRecordValuesGet func(schemaID, recordID string) (error, []ProjectRecordValue)
type ProjectRecordValuesMutate func(schemaID, recordID string, mutation ProjectRecordValuesMutation) (error, ProjectRecordValuesMutationResult)

type ProjectRecordSchema struct {
	ID          string
	Name        string
	Label       string
	Description string
	AuthorID    string
	CreatedAt   string
}

type ProjectRecordAttribute struct {
	ID           string
	Name         string
	Label        string
	Description  string
	Type         string
	TargetSchema *string
	Cardinality  string
	Uniqueness   string
	Display      string
	DisplayOrder int
	AuthorID     string
	CreatedAt    string
}

type ProjectRecord struct {
	ID        string
	AuthorID  string
	CreatedAt string
}

type ProjectRecordValue struct {
	ID        string
	Attribute string
	Value     any
	Sensitive bool
	AuthorID  string
	CreatedAt string
}

type ProjectRecordAttributeInput struct {
	Name         string
	Label        string
	Description  string
	Type         string
	TargetSchema *string
	Cardinality  string
	Uniqueness   string
	Display      string
	DisplayOrder int
}

type ProjectRecordAttributeUpdateInput struct {
	Label        string
	Description  string
	Type         string
	TargetSchema *string
	Cardinality  string
	Uniqueness   string
	Display      string
	DisplayOrder int
}

type ProjectRecordValueInput struct {
	Attribute string
	Value     any
	Sensitive bool
}

type ProjectRecordValueUpdate struct {
	ID        string
	Value     any
	Sensitive bool
}

type ProjectRecordValuesMutation struct {
	Create []ProjectRecordValueInput
	Update []ProjectRecordValueUpdate
	Remove []string
}

type ProjectRecordValuesMutationResult struct {
	Created []ProjectRecordValue
	Removed []string
}

var (
	projectRecordSchemasListDocumentation     = capabilityDocumentation{"(project/records/schemas/list) -> List", "Returns record schemas in the linked project.", "(project/records/schemas/list)", "((id . \"prs_...\") (name . \"contacts\") (label . \"Contacts\") (description . \"People to contact\") (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordSchemaGetDocumentation       = capabilityDocumentation{"(project/records/schemas/get id) -> List | Null", "Returns a record schema by durable prs_ ID, or null when unavailable.", "(project/records/schemas/get \"prs_...\")", "((id . \"prs_...\") (name . \"contacts\") (label . \"Contacts\") (description . \"People to contact\") (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordSchemaCreateDocumentation    = capabilityDocumentation{"(project/records/schemas/create name label description) -> List", "Creates a record schema. Name is its immutable lowercase snake-case machine name.", "(project/records/schemas/create \"contacts\" \"Contacts\" \"People to contact\")", "((id . \"prs_...\") (name . \"contacts\") (label . \"Contacts\") (description . \"People to contact\") (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordSchemaUpdateDocumentation    = capabilityDocumentation{"(project/records/schemas/update id label description) -> List | Null", "Updates a record schema label and description.", "(project/records/schemas/update \"prs_...\" \"Contacts\" \"People to contact\")", "((id . \"prs_...\") (name . \"contacts\") (label . \"Contacts\") (description . \"People to contact\") (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordSchemaRemoveDocumentation    = capabilityDocumentation{"(project/records/schemas/remove id) -> Boolean", "Removes a record schema. Returns false when unavailable.", "(project/records/schemas/remove \"prs_...\")", "#t"}
	projectRecordAttributesListDocumentation  = capabilityDocumentation{"(project/records/attributes/list schema-id) -> List", "Returns attributes in a record schema.", "(project/records/attributes/list \"prs_...\")", "((id . \"pra_...\") (name . \"email\") (label . \"Email\") (description . \"\") (type . \"text\") (target_schema) (cardinality . \"one\") (uniqueness . \"global\") (display . \"primary\") (display_order . 0) (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordAttributeGetDocumentation    = capabilityDocumentation{"(project/records/attributes/get schema-id id) -> List | Null", "Returns an attribute by durable pra_ ID, or null when unavailable.", "(project/records/attributes/get \"prs_...\" \"pra_...\")", "((id . \"pra_...\") (name . \"email\") (label . \"Email\") (description . \"\") (type . \"text\") (target_schema) (cardinality . \"one\") (uniqueness . \"global\") (display . \"primary\") (display_order . 0) (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordAttributeCreateDocumentation = capabilityDocumentation{"(project/records/attributes/create schema-id name label description type target-schema cardinality uniqueness display display-order) -> List", "Creates an attribute. Name is immutable lowercase snake case; target-schema is null except for record attributes; display-order is a non-negative integer and defaults to 0 when omitted.", "(project/records/attributes/create \"prs_...\" \"organization\" \"Organization\" \"Owning organization\" \"record\" \"prs_...\" \"one\" \"none\" \"secondary\" 0)", "((id . \"pra_...\") (name . \"organization\") (label . \"Organization\") (description . \"Owning organization\") (type . \"record\") (target_schema . \"prs_...\") (cardinality . \"one\") (uniqueness . \"none\") (display . \"secondary\") (display_order . 0) (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordAttributeUpdateDocumentation = capabilityDocumentation{"(project/records/attributes/update schema-id id label description type target-schema cardinality uniqueness display display-order) -> List | Null", "Replaces mutable attribute details. Target-schema is null except for record attributes; display-order is a non-negative integer and defaults to 0 when omitted.", "(project/records/attributes/update \"prs_...\" \"pra_...\" \"Organization\" \"Owning organization\" \"record\" \"prs_...\" \"one\" \"none\" \"secondary\" 0)", "((id . \"pra_...\") (name . \"organization\") (label . \"Organization\") (description . \"Owning organization\") (type . \"record\") (target_schema . \"prs_...\") (cardinality . \"one\") (uniqueness . \"none\") (display . \"secondary\") (display_order . 0) (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordAttributeRemoveDocumentation = capabilityDocumentation{"(project/records/attributes/remove schema-id id) -> Boolean", "Removes an attribute. Returns false when unavailable.", "(project/records/attributes/remove \"prs_...\" \"pra_...\")", "#t"}
	projectRecordsListDocumentation           = capabilityDocumentation{"(project/records/list schema-id) -> List", "Returns records in a schema.", "(project/records/list \"prs_...\")", "((id . \"prr_...\") (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordGetDocumentation             = capabilityDocumentation{"(project/records/get schema-id id) -> List | Null", "Returns a record by durable prr_ ID, or null when unavailable.", "(project/records/get \"prs_...\" \"prr_...\")", "((id . \"prr_...\") (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordCreateDocumentation          = capabilityDocumentation{"(project/records/create schema-id values) -> List", "Creates a record. Values is a list of (attribute-name . value) pairs. Sensitive values remain sensitive-tainted and are stored as sensitive.", "(project/records/create \"prs_...\" (list (pair \"email\" \"ada@example.test\")))", "((record . ((id . \"prr_...\") (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))) (values . (((id . \"prv_...\") (attribute . \"pra_...\") (value . \"ada@example.test\") (sensitive . #f) (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))))"}
	projectRecordRemoveDocumentation          = capabilityDocumentation{"(project/records/remove schema-id id) -> Boolean", "Removes a record. Returns false when unavailable.", "(project/records/remove \"prs_...\" \"prr_...\")", "#t"}
	projectRecordValuesListDocumentation      = capabilityDocumentation{"(project/records/values/list schema-id record-id) -> List", "Returns values for a record. Sensitive values are sensitive-tainted.", "(project/records/values/list \"prs_...\" \"prr_...\")", "((id . \"prv_...\") (attribute . \"pra_...\") (value . \"ada@example.test\") (sensitive . #f) (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\"))"}
	projectRecordValuesMutateDocumentation    = capabilityDocumentation{"(project/records/values/mutate schema-id record-id create update remove) -> List", "Mutates record values. Create uses (attribute-name . value) pairs, update uses (value-id . value) pairs, and remove is a list of value IDs. Sensitive values remain sensitive-tainted and are stored as sensitive.", "(project/records/values/mutate \"prs_...\" \"prr_...\" (list (pair \"email\" \"ada@example.test\")) (list) (list))", "((created . (((id . \"prv_...\") (attribute . \"pra_...\") (value . \"ada@example.test\") (sensitive . #f) (author_id . \"wag_...\") (created_at . \"2026-01-01T00:00:00.000Z\")))) (removed))"}
)

func projectRecordsExports(records *ProjectRecords) []lisp.HostExport {
	schemas, schema, attributes, attribute, recordList, record, values := unavailableProjectRecordsList("project/records/schemas/list"), unavailableProjectRecordsGet("project/records/schemas/get"), unavailableProjectRecordsList("project/records/attributes/list"), unavailableProjectRecordsGet("project/records/attributes/get"), unavailableProjectRecordsList("project/records/list"), unavailableProjectRecordsGet("project/records/get"), unavailableProjectRecordsList("project/records/values/list")
	schemaCreate, schemaUpdate, schemaRemove := unavailableCreate("project/records/schemas/create"), unavailableCreate("project/records/schemas/update"), unavailableRemove("project/records/schemas/remove")
	attributeCreate, attributeUpdate, attributeRemove := unavailableCreate("project/records/attributes/create"), unavailableCreate("project/records/attributes/update"), unavailableRemove("project/records/attributes/remove")
	recordCreate, recordRemove, valuesMutate := unavailableCreate("project/records/create"), unavailableRemove("project/records/remove"), unavailableCreate("project/records/values/mutate")
	if records != nil {
		if records.Schemas != nil {
			schemas = projectRecordSchemasListFunction(records.Schemas)
		}
		if records.Schema != nil {
			schema = projectRecordSchemaGetFunction(records.Schema)
		}
		if records.Attributes != nil {
			attributes = projectRecordAttributesListFunction(records.Attributes)
		}
		if records.Attribute != nil {
			attribute = projectRecordAttributeGetFunction(records.Attribute)
		}
		if records.Records != nil {
			recordList = projectRecordsListFunction(records.Records)
		}
		if records.Record != nil {
			record = projectRecordGetFunction(records.Record)
		}
		if records.Values != nil {
			values = projectRecordValuesListFunction(records.Values)
		}
		if records.SchemaCreate != nil {
			schemaCreate = projectRecordSchemaCreateFunction(records.SchemaCreate)
		}
		if records.SchemaUpdate != nil {
			schemaUpdate = projectRecordSchemaUpdateFunction(records.SchemaUpdate)
		}
		if records.SchemaRemove != nil {
			schemaRemove = projectRecordSchemaRemoveFunction(records.SchemaRemove)
		}
		if records.AttributeCreate != nil {
			attributeCreate = projectRecordAttributeCreateFunction(records.AttributeCreate)
		}
		if records.AttributeUpdate != nil {
			attributeUpdate = projectRecordAttributeUpdateFunction(records.AttributeUpdate)
		}
		if records.AttributeRemove != nil {
			attributeRemove = projectRecordAttributeRemoveFunction(records.AttributeRemove)
		}
		if records.RecordCreate != nil {
			recordCreate = projectRecordCreateFunction(records.RecordCreate)
		}
		if records.RecordRemove != nil {
			recordRemove = projectRecordRemoveRecordFunction(records.RecordRemove)
		}
		if records.ValuesMutate != nil {
			valuesMutate = projectRecordValuesMutateFunction(records.ValuesMutate)
		}
	}
	return []lisp.HostExport{
		{Name: "records/schemas/list", Value: document(lisp.Function(schemas), projectRecordSchemasListDocumentation)},
		{Name: "records/schemas/get", Value: document(lisp.FunctionNonLeaky(schema), projectRecordSchemaGetDocumentation)},
		{Name: "records/schemas/create", Value: document(lisp.FunctionNonLeaky(schemaCreate), projectRecordSchemaCreateDocumentation)},
		{Name: "records/schemas/update", Value: document(lisp.FunctionNonLeaky(schemaUpdate), projectRecordSchemaUpdateDocumentation)},
		{Name: "records/schemas/remove", Value: document(lisp.Function(schemaRemove), projectRecordSchemaRemoveDocumentation)},
		{Name: "records/attributes/list", Value: document(lisp.Function(attributes), projectRecordAttributesListDocumentation)},
		{Name: "records/attributes/get", Value: document(lisp.FunctionNonLeaky(attribute), projectRecordAttributeGetDocumentation)},
		{Name: "records/attributes/create", Value: document(lisp.FunctionNonLeaky(attributeCreate), projectRecordAttributeCreateDocumentation)},
		{Name: "records/attributes/update", Value: document(lisp.FunctionNonLeaky(attributeUpdate), projectRecordAttributeUpdateDocumentation)},
		{Name: "records/attributes/remove", Value: document(lisp.Function(attributeRemove), projectRecordAttributeRemoveDocumentation)},
		{Name: "records/list", Value: document(lisp.Function(recordList), projectRecordsListDocumentation)},
		{Name: "records/get", Value: document(lisp.FunctionNonLeaky(record), projectRecordGetDocumentation)},
		{Name: "records/create", Value: document(lisp.FunctionNonLeaky(recordCreate), projectRecordCreateDocumentation)},
		{Name: "records/remove", Value: document(lisp.Function(recordRemove), projectRecordRemoveDocumentation)},
		{Name: "records/values/list", Value: document(lisp.FunctionNonLeaky(values), projectRecordValuesListDocumentation)},
		{Name: "records/values/mutate", Value: document(lisp.FunctionNonLeaky(valuesMutate), projectRecordValuesMutateDocumentation)},
	}
}

func unavailableProjectRecordsList(name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func([]lisp.Expr) (error, lisp.Expr) { return lisp.Errorf("%s is unavailable", name), nil }
}
func unavailableProjectRecordsGet(name string) func([]lisp.Expr) (error, lisp.Expr) {
	return func([]lisp.Expr) (error, lisp.Expr) { return lisp.Errorf("%s is unavailable", name), nil }
}

func projectRecordMutationError(name string, err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "duplicate key value violates unique constraint"), strings.Contains(message, "UNIQUE constraint failed"):
		switch name {
		case "project/records/schemas/create":
			return lisp.Errorf("%s failed: a schema with that name already exists", name)
		case "project/records/attributes/create":
			return lisp.Errorf("%s failed: an attribute with that name already exists", name)
		default:
			return lisp.Errorf("%s failed: a record value marked unique already exists", name)
		}
	case strings.Contains(message, "project record attribute is unavailable"):
		return lisp.Errorf("%s failed: the record attribute is unavailable", name)
	case strings.Contains(message, "violates foreign key constraint"), strings.Contains(message, "FOREIGN KEY constraint failed"):
		switch name {
		case "project/records/schemas/remove":
			return lisp.Errorf("%s failed: the schema has dependent records or attributes", name)
		case "project/records/attributes/create":
			return lisp.Errorf("%s failed: the record reference target is unavailable", name)
		case "project/records/attributes/update":
			return lisp.Errorf("%s failed: the attribute change conflicts with existing record values", name)
		case "project/records/remove":
			return lisp.Errorf("%s failed: the record is referenced by another record", name)
		}
	}
	return lisp.Errorf("%s failed", name)
}

func projectRecordSchemasListFunction(get ProjectRecordSchemasGet) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("project/records/schemas/list requires no arguments"), nil
		}
		err, schemas := get()
		if err != nil {
			return lisp.Errorf("project/records/schemas/list failed"), nil
		}
		return projectRecordSchemaValues(schemas, "project/records/schemas/list")
	}
}
func projectRecordSchemaGetFunction(get ProjectRecordSchemaGet) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("project/records/schemas/get requires an id"), nil
		}
		err, id := projectRecordID(arguments[0], "project/records/schemas/get")
		if err != nil {
			return err, nil
		}
		err, schema := get(id)
		if err != nil {
			return lisp.Errorf("project/records/schemas/get failed"), nil
		}
		if schema == nil {
			return nil, lisp.Null()
		}
		return projectRecordSchemaValue(*schema, "project/records/schemas/get")
	}
}
func projectRecordSchemaCreateFunction(create ProjectRecordSchemaCreate) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 3 {
			return lisp.Errorf("project/records/schemas/create requires name, label, and description"), nil
		}
		err, values := projectRecordStrings(arguments, "project/records/schemas/create")
		if err != nil {
			return err, nil
		}
		err, schema := create(values[0], values[1], values[2])
		if err != nil {
			return projectRecordMutationError("project/records/schemas/create", err), nil
		}
		return projectRecordSchemaValue(schema, "project/records/schemas/create")
	}
}
func projectRecordSchemaUpdateFunction(update ProjectRecordSchemaUpdate) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 3 {
			return lisp.Errorf("project/records/schemas/update requires id, label, and description"), nil
		}
		err, values := projectRecordStrings(arguments, "project/records/schemas/update")
		if err != nil {
			return err, nil
		}
		err, schema := update(values[0], values[1], values[2])
		if err != nil {
			return projectRecordMutationError("project/records/schemas/update", err), nil
		}
		if schema == nil {
			return nil, lisp.Null()
		}
		return projectRecordSchemaValue(*schema, "project/records/schemas/update")
	}
}
func projectRecordSchemaRemoveFunction(remove ProjectRecordSchemaRemove) func([]lisp.Expr) (error, lisp.Expr) {
	return projectRecordRemoveFunction(remove, "project/records/schemas/remove", 1)
}

func projectRecordAttributesListFunction(get ProjectRecordAttributesGet) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("project/records/attributes/list requires a schema id"), nil
		}
		err, schemaID := projectRecordID(arguments[0], "project/records/attributes/list")
		if err != nil {
			return err, nil
		}
		err, attributes := get(schemaID)
		if err != nil {
			return lisp.Errorf("project/records/attributes/list failed"), nil
		}
		return projectRecordAttributeValues(attributes, "project/records/attributes/list")
	}
}
func projectRecordAttributeGetFunction(get ProjectRecordAttributeGet) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 2 {
			return lisp.Errorf("project/records/attributes/get requires schema and attribute ids"), nil
		}
		err, values := projectRecordStrings(arguments, "project/records/attributes/get")
		if err != nil {
			return err, nil
		}
		err, attribute := get(values[0], values[1])
		if err != nil {
			return lisp.Errorf("project/records/attributes/get failed"), nil
		}
		if attribute == nil {
			return nil, lisp.Null()
		}
		return projectRecordAttributeValue(*attribute, "project/records/attributes/get")
	}
}
func projectRecordAttributeCreateFunction(create ProjectRecordAttributeCreate) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 9 && len(arguments) != 10 {
			return lisp.Errorf("project/records/attributes/create requires schema id, name, label, description, type, target schema, cardinality, uniqueness, display, and optional display order"), nil
		}
		err, schemaID := projectRecordID(arguments[0], "project/records/attributes/create")
		if err != nil {
			return err, nil
		}
		err, attribute := projectRecordAttributeInput(arguments[1:], "project/records/attributes/create", true)
		if err != nil {
			return err, nil
		}
		err, created := create(schemaID, attribute)
		if err != nil {
			return projectRecordMutationError("project/records/attributes/create", err), nil
		}
		return projectRecordAttributeValue(created, "project/records/attributes/create")
	}
}
func projectRecordAttributeUpdateFunction(update ProjectRecordAttributeUpdate) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 9 && len(arguments) != 10 {
			return lisp.Errorf("project/records/attributes/update requires schema id, attribute id, label, description, type, target schema, cardinality, uniqueness, display, and optional display order"), nil
		}
		err, ids := projectRecordStrings(arguments[:2], "project/records/attributes/update")
		if err != nil {
			return err, nil
		}
		err, input := projectRecordAttributeInput(arguments[2:], "project/records/attributes/update", false)
		if err != nil {
			return err, nil
		}
		err, attribute := update(ids[0], ids[1], ProjectRecordAttributeUpdateInput{Label: input.Label, Description: input.Description, Type: input.Type, TargetSchema: input.TargetSchema, Cardinality: input.Cardinality, Uniqueness: input.Uniqueness, Display: input.Display, DisplayOrder: input.DisplayOrder})
		if err != nil {
			return projectRecordMutationError("project/records/attributes/update", err), nil
		}
		if attribute == nil {
			return nil, lisp.Null()
		}
		return projectRecordAttributeValue(*attribute, "project/records/attributes/update")
	}
}
func projectRecordAttributeRemoveFunction(remove ProjectRecordAttributeRemove) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 2 {
			return lisp.Errorf("project/records/attributes/remove requires schema and attribute ids"), nil
		}
		err, ids := projectRecordStrings(arguments, "project/records/attributes/remove")
		if err != nil {
			return err, nil
		}
		err, removed := remove(ids[0], ids[1])
		if err != nil {
			return projectRecordMutationError("project/records/attributes/remove", err), nil
		}
		return nil, lisp.Boolean(removed)
	}
}

func projectRecordsListFunction(get ProjectRecordsGet) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("project/records/list requires a schema id"), nil
		}
		err, schemaID := projectRecordID(arguments[0], "project/records/list")
		if err != nil {
			return err, nil
		}
		err, records := get(schemaID)
		if err != nil {
			return lisp.Errorf("project/records/list failed"), nil
		}
		return projectRecordValues(records, "project/records/list")
	}
}
func projectRecordGetFunction(get ProjectRecordGet) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 2 {
			return lisp.Errorf("project/records/get requires schema and record ids"), nil
		}
		err, ids := projectRecordStrings(arguments, "project/records/get")
		if err != nil {
			return err, nil
		}
		err, record := get(ids[0], ids[1])
		if err != nil {
			return lisp.Errorf("project/records/get failed"), nil
		}
		if record == nil {
			return nil, lisp.Null()
		}
		return projectRecordValue(*record, "project/records/get")
	}
}
func projectRecordCreateFunction(create ProjectRecordCreate) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 2 {
			return lisp.Errorf("project/records/create requires a schema id and values"), nil
		}
		err, schemaID := projectRecordID(arguments[0], "project/records/create")
		if err != nil {
			return err, nil
		}
		err, values := projectRecordValueInputs(arguments[1], "project/records/create")
		if err != nil {
			return err, nil
		}
		err, record, created := create(schemaID, values)
		if err != nil {
			return projectRecordMutationError("project/records/create", err), nil
		}
		err, recordValue := projectRecordValue(record, "project/records/create")
		if err != nil {
			return err, nil
		}
		err, createdValues := projectRecordValueValues(created, "project/records/create")
		if err != nil {
			return err, nil
		}
		return nil, lisp.List(lisp.Pair("record", recordValue), lisp.Pair("values", createdValues))
	}
}
func projectRecordRemoveRecordFunction(remove ProjectRecordRemove) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 2 {
			return lisp.Errorf("project/records/remove requires schema and record ids"), nil
		}
		err, ids := projectRecordStrings(arguments, "project/records/remove")
		if err != nil {
			return err, nil
		}
		err, removed := remove(ids[0], ids[1])
		if err != nil {
			return projectRecordMutationError("project/records/remove", err), nil
		}
		return nil, lisp.Boolean(removed)
	}
}
func projectRecordRemoveFunction(remove func(string) (error, bool), name string, argumentsCount int) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != argumentsCount {
			return lisp.Errorf("%s requires an id", name), nil
		}
		err, id := projectRecordID(arguments[argumentsCount-1], name)
		if err != nil {
			return err, nil
		}
		err, removed := remove(id)
		if err != nil {
			return projectRecordMutationError(name, err), nil
		}
		return nil, lisp.Boolean(removed)
	}
}
func projectRecordValuesListFunction(get ProjectRecordValuesGet) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 2 {
			return lisp.Errorf("project/records/values/list requires schema and record ids"), nil
		}
		err, ids := projectRecordStrings(arguments, "project/records/values/list")
		if err != nil {
			return err, nil
		}
		err, values := get(ids[0], ids[1])
		if err != nil {
			return lisp.Errorf("project/records/values/list failed"), nil
		}
		return projectRecordValueValues(values, "project/records/values/list")
	}
}
func projectRecordValuesMutateFunction(mutate ProjectRecordValuesMutate) func([]lisp.Expr) (error, lisp.Expr) {
	return func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 5 {
			return lisp.Errorf("project/records/values/mutate requires schema id, record id, create, update, and remove"), nil
		}
		err, ids := projectRecordStrings(arguments[:2], "project/records/values/mutate")
		if err != nil {
			return err, nil
		}
		err, create := projectRecordValueInputs(arguments[2], "project/records/values/mutate")
		if err != nil {
			return err, nil
		}
		err, update := projectRecordValueUpdates(arguments[3], "project/records/values/mutate")
		if err != nil {
			return err, nil
		}
		err, remove := projectRecordIDs(arguments[4], "project/records/values/mutate")
		if err != nil {
			return err, nil
		}
		err, result := mutate(ids[0], ids[1], ProjectRecordValuesMutation{Create: create, Update: update, Remove: remove})
		if err != nil {
			return projectRecordMutationError("project/records/values/mutate", err), nil
		}
		err, created := projectRecordValueValues(result.Created, "project/records/values/mutate")
		if err != nil {
			return err, nil
		}
		removed := make([]lisp.Expr, 0, len(result.Removed))
		for _, id := range result.Removed {
			removed = append(removed, lisp.String(id))
		}
		return nil, lisp.List(lisp.Pair("created", created), lisp.Pair("removed", lisp.List(removed...)))
	}
}

func projectRecordAttributeInput(arguments []lisp.Expr, name string, create bool) (error, ProjectRecordAttributeInput) {
	if (create && len(arguments) != 8 && len(arguments) != 9) || (!create && len(arguments) != 7 && len(arguments) != 8) {
		return lisp.Errorf("%s has invalid attribute input", name), ProjectRecordAttributeInput{}
	}
	for _, argument := range arguments {
		if lisp.TaintOf(argument) == lisp.TaintSecret {
			return lisp.Errorf("%s must not use secret inputs", name), ProjectRecordAttributeInput{}
		}
	}
	start := 0
	input := ProjectRecordAttributeInput{}
	if create {
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone {
			return lisp.Errorf("%s attribute name must not be sensitive", name), ProjectRecordAttributeInput{}
		}
		err, value := lisp.RequireString(arguments[0])
		if err != nil {
			return err, input
		}
		input.Name = value
		start = 1
	}
	for _, argument := range arguments[start:] {
		if lisp.TaintOf(argument) != lisp.TaintNone {
			return lisp.Errorf("%s attribute metadata must not be sensitive", name), ProjectRecordAttributeInput{}
		}
	}
	values := arguments[start:]
	if len(values) != 7 && len(values) != 8 {
		return lisp.Errorf("%s has invalid attribute input", name), input
	}
	err, label := lisp.RequireString(values[0])
	if err != nil {
		return err, input
	}
	err, description := lisp.RequireString(values[1])
	if err != nil {
		return err, input
	}
	err, valueType := lisp.RequireString(values[2])
	if err != nil {
		return err, input
	}
	err, cardinality := lisp.RequireString(values[4])
	if err != nil {
		return err, input
	}
	err, uniqueness := lisp.RequireString(values[5])
	if err != nil {
		return err, input
	}
	err, display := lisp.RequireString(values[6])
	if err != nil {
		return err, input
	}
	if len(values) == 8 {
		err, displayOrder := lisp.RequireInteger(values[7])
		if err != nil || displayOrder < 0 || displayOrder > int64(^uint(0)>>1) {
			return lisp.Errorf("%s has invalid attribute input", name), input
		}
		input.DisplayOrder = int(displayOrder)
	}
	input.Label, input.Description, input.Type, input.Cardinality, input.Uniqueness, input.Display = label, description, valueType, cardinality, uniqueness, display
	if !lisp.IsNull(values[3]) {
		err, target := lisp.RequireString(values[3])
		if err != nil {
			return err, input
		}
		input.TargetSchema = &target
	}
	return nil, input
}

func projectRecordStrings(arguments []lisp.Expr, name string) (error, []string) {
	values := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if lisp.TaintOf(argument) != lisp.TaintNone {
			return lisp.Errorf("%s inputs must not be sensitive", name), nil
		}
		err, value := lisp.RequireString(argument)
		if err != nil {
			return err, nil
		}
		values = append(values, value)
	}
	return nil, values
}
func projectRecordID(argument lisp.Expr, name string) (error, string) {
	err, values := projectRecordStrings([]lisp.Expr{argument}, name)
	if err != nil {
		return err, ""
	}
	if values[0] == "" {
		return lisp.Errorf("%s requires a non-empty id", name), ""
	}
	return nil, values[0]
}
func projectRecordIDs(argument lisp.Expr, name string) (error, []string) {
	if lisp.IsSecret(argument) {
		return lisp.Errorf("%s must not use secret inputs", name), nil
	}
	err, expressions := projectRecordList(argument)
	if err != nil {
		return lisp.Errorf("%s remove must be a list", name), nil
	}
	values := make([]string, 0, len(expressions))
	for _, expression := range expressions {
		err, id := projectRecordID(expression, name)
		if err != nil {
			return err, nil
		}
		values = append(values, id)
	}
	return nil, values
}
func projectRecordValueInputs(argument lisp.Expr, name string) (error, []ProjectRecordValueInput) {
	if lisp.IsSecret(argument) {
		return lisp.Errorf("%s must not use secret inputs", name), nil
	}
	err, entries := projectRecordList(argument)
	if err != nil {
		return lisp.Errorf("%s values must be a list", name), nil
	}
	values := make([]ProjectRecordValueInput, 0, len(entries))
	for _, entry := range entries {
		err, attribute, value := projectRecordValuePair(entry, name)
		if err != nil {
			return err, nil
		}
		values = append(values, ProjectRecordValueInput{Attribute: attribute, Value: value, Sensitive: lisp.IsSensitive(entry)})
	}
	return nil, values
}
func projectRecordValueUpdates(argument lisp.Expr, name string) (error, []ProjectRecordValueUpdate) {
	if lisp.IsSecret(argument) {
		return lisp.Errorf("%s must not use secret inputs", name), nil
	}
	err, entries := projectRecordList(argument)
	if err != nil {
		return lisp.Errorf("%s update must be a list", name), nil
	}
	values := make([]ProjectRecordValueUpdate, 0, len(entries))
	for _, entry := range entries {
		err, id, value := projectRecordValuePair(entry, name)
		if err != nil {
			return err, nil
		}
		values = append(values, ProjectRecordValueUpdate{ID: id, Value: value, Sensitive: lisp.IsSensitive(entry)})
	}
	return nil, values
}
func projectRecordValuePair(expression lisp.Expr, name string) (error, string, any) {
	if lisp.TaintOf(expression) == lisp.TaintSecret {
		return lisp.Errorf("%s must not use secret inputs", name), "", nil
	}
	key, value, ok := lisp.DeconstructPair(expression)
	if !ok || lisp.TaintOf(key) != lisp.TaintNone {
		return lisp.Errorf("%s values require non-sensitive pairs", name), "", nil
	}
	err, nameValue := lisp.RequireString(key)
	if err != nil || nameValue == "" {
		return lisp.Errorf("%s values require non-empty string names", name), "", nil
	}
	if lisp.TaintOf(value) == lisp.TaintSecret {
		return lisp.Errorf("%s values require non-secret pairs", name), "", nil
	}
	err, native := projectRecordNativeValue(value)
	if err != nil {
		return err, "", nil
	}
	return nil, nameValue, native
}
func projectRecordList(expression lisp.Expr) (error, []lisp.Expr) {
	values := []lisp.Expr{}
	for !lisp.IsNull(expression) {
		first, rest, ok := lisp.DeconstructPair(expression)
		if !ok {
			return lisp.Errorf("not a proper list"), nil
		}
		values, expression = append(values, first), rest
	}
	return nil, values
}
func projectRecordNativeValue(expression lisp.Expr) (error, any) {
	if lisp.TaintOf(expression) == lisp.TaintSecret {
		return lisp.Errorf("project record value must not be secret"), nil
	}
	if err, value := lisp.RequireString(expression); err == nil {
		return nil, value
	}
	if err, value := lisp.RequireInteger(expression); err == nil {
		return nil, value
	}
	if expression.String() == "#t" {
		return nil, true
	}
	if expression.String() == "#f" {
		return nil, false
	}
	return lisp.Errorf("project record value must be a string, integer, or boolean"), nil
}

func projectRecordSchemaValues(schemas []ProjectRecordSchema, name string) (error, lisp.Expr) {
	values := make([]lisp.Expr, 0, len(schemas))
	for _, schema := range schemas {
		err, value := projectRecordSchemaValue(schema, name)
		if err != nil {
			return err, nil
		}
		values = append(values, value)
	}
	return nil, lisp.List(values...)
}
func projectRecordSchemaValue(schema ProjectRecordSchema, name string) (error, lisp.Expr) {
	if schema.ID == "" || schema.Name == "" || schema.Label == "" || schema.AuthorID == "" || schema.CreatedAt == "" {
		return lisp.Errorf("%s has invalid record schema metadata", name), nil
	}
	return nil, lisp.List(lisp.Pair("id", lisp.String(schema.ID)), lisp.Pair("name", lisp.String(schema.Name)), lisp.Pair("label", lisp.String(schema.Label)), lisp.Pair("description", lisp.String(schema.Description)), lisp.Pair("author_id", lisp.String(schema.AuthorID)), lisp.Pair("created_at", lisp.String(schema.CreatedAt)))
}
func projectRecordAttributeValues(attributes []ProjectRecordAttribute, name string) (error, lisp.Expr) {
	values := make([]lisp.Expr, 0, len(attributes))
	for _, attribute := range attributes {
		err, value := projectRecordAttributeValue(attribute, name)
		if err != nil {
			return err, nil
		}
		values = append(values, value)
	}
	return nil, lisp.List(values...)
}
func projectRecordAttributeValue(attribute ProjectRecordAttribute, name string) (error, lisp.Expr) {
	if attribute.ID == "" || attribute.Name == "" || attribute.Label == "" || attribute.Type == "" || attribute.Cardinality == "" || attribute.Uniqueness == "" || attribute.Display == "" || attribute.DisplayOrder < 0 || attribute.AuthorID == "" || attribute.CreatedAt == "" {
		return lisp.Errorf("%s has invalid record attribute metadata", name), nil
	}
	target := lisp.Null()
	if attribute.TargetSchema != nil {
		target = lisp.String(*attribute.TargetSchema)
	}
	return nil, lisp.List(lisp.Pair("id", lisp.String(attribute.ID)), lisp.Pair("name", lisp.String(attribute.Name)), lisp.Pair("label", lisp.String(attribute.Label)), lisp.Pair("description", lisp.String(attribute.Description)), lisp.Pair("type", lisp.String(attribute.Type)), lisp.Pair("target_schema", target), lisp.Pair("cardinality", lisp.String(attribute.Cardinality)), lisp.Pair("uniqueness", lisp.String(attribute.Uniqueness)), lisp.Pair("display", lisp.String(attribute.Display)), lisp.Pair("display_order", lisp.Integer(int64(attribute.DisplayOrder))), lisp.Pair("author_id", lisp.String(attribute.AuthorID)), lisp.Pair("created_at", lisp.String(attribute.CreatedAt)))
}
func projectRecordValues(records []ProjectRecord, name string) (error, lisp.Expr) {
	values := make([]lisp.Expr, 0, len(records))
	for _, record := range records {
		err, value := projectRecordValue(record, name)
		if err != nil {
			return err, nil
		}
		values = append(values, value)
	}
	return nil, lisp.List(values...)
}
func projectRecordValue(record ProjectRecord, name string) (error, lisp.Expr) {
	if record.ID == "" || record.AuthorID == "" || record.CreatedAt == "" {
		return lisp.Errorf("%s has invalid record metadata", name), nil
	}
	return nil, lisp.List(lisp.Pair("id", lisp.String(record.ID)), lisp.Pair("author_id", lisp.String(record.AuthorID)), lisp.Pair("created_at", lisp.String(record.CreatedAt)))
}
func projectRecordValueValues(values []ProjectRecordValue, name string) (error, lisp.Expr) {
	result := make([]lisp.Expr, 0, len(values))
	for _, value := range values {
		err, expression := projectRecordStoredValue(value, name)
		if err != nil {
			return err, nil
		}
		result = append(result, expression)
	}
	return nil, lisp.List(result...)
}
func projectRecordStoredValue(value ProjectRecordValue, name string) (error, lisp.Expr) {
	if value.ID == "" || value.Attribute == "" || value.AuthorID == "" || value.CreatedAt == "" {
		return lisp.Errorf("%s has invalid record value metadata", name), nil
	}
	expression, err := projectRecordLispValue(value.Value)
	if err != nil {
		return err, nil
	}
	if value.Sensitive {
		expression = lisp.MarkSensitive(expression)
	}
	return nil, lisp.List(lisp.Pair("id", lisp.String(value.ID)), lisp.Pair("attribute", lisp.String(value.Attribute)), lisp.Pair("value", expression), lisp.Pair("sensitive", lisp.Boolean(value.Sensitive)), lisp.Pair("author_id", lisp.String(value.AuthorID)), lisp.Pair("created_at", lisp.String(value.CreatedAt)))
}
func projectRecordLispValue(value any) (lisp.Expr, error) {
	switch value := value.(type) {
	case string:
		return lisp.String(value), nil
	case float64:
		return lisp.Integer(int64(value)), nil
	case int64:
		return lisp.Integer(value), nil
	case int:
		return lisp.Integer(int64(value)), nil
	case bool:
		return lisp.Boolean(value), nil
	default:
		return nil, lisp.Errorf("record value has an unsupported type")
	}
}
