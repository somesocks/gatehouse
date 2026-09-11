package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gatehouse/model"
	"gatehouse/typed_id"
)

const (
	projectRecordValueTypeText     = "text"
	projectRecordValueTypeNumber   = "number"
	projectRecordValueTypeBoolean  = "boolean"
	projectRecordValueTypeDatetime = "datetime"
	projectRecordValueTypeRecord   = "record"
)

type ProjectRecordAuthor struct {
	Principal *model.PrincipalRef
	Agent     *model.WorkspaceAgentRef
}

type ProjectRecordValueCreate struct {
	ID        string
	Attribute model.ProjectRecordAttributeRef
	Value     any
	Sensitive bool
}

type ProjectRecordValueUpdate struct {
	ID        string
	Value     any
	Sensitive *bool
}

type ProjectRecordValuesMutation struct {
	Create []ProjectRecordValueCreate
	Update []ProjectRecordValueUpdate
	Delete []string
}

type ProjectRecordValuesMutationResult struct {
	Created []model.ProjectRecordValue
	Removed []string
}

type ProjectRecordCard struct {
	Record model.ProjectRecord
	Values []ProjectRecordCardValue
}

type ProjectRecordCardValue struct {
	Attribute string
	Value     any
	Sensitive bool
	Reference *ProjectRecordReferenceDisplay
}

type ProjectRecordReferenceDisplay struct {
	SchemaLabel   string
	PrimaryValues []ProjectRecordReferenceDisplayValue
}

type ProjectRecordReferenceDisplayValue struct {
	Value     any
	Sensitive bool
}

// ProjectRecordIncomingReferenceGroup contains records that reference a record through one source attribute.
type ProjectRecordIncomingReferenceGroup struct {
	Schema         model.ProjectRecordSchemaRef
	SchemaLabel    string
	Attribute      model.ProjectRecordAttributeRef
	AttributeName  string
	AttributeLabel string
	References     []ProjectRecordIncomingReference
}

// ProjectRecordIncomingReference is one reference value pointing at a record.
type ProjectRecordIncomingReference struct {
	ValueID       string
	Record        model.ProjectRecordRef
	PrimaryValues []ProjectRecordReferenceDisplayValue
}

type projectRecordReferenceInput struct {
	Attribute string
	Value     any
}

type projectRecordValueStorage struct {
	typeName        string
	text            *string
	number          *float64
	boolean         *bool
	datetime        *string
	referenceSchema *string
	reference       *string
	key             string
	value           any
}

func projectRecordAuthorValid(author ProjectRecordAuthor, workspace model.WorkspaceRef) error {
	count := 0
	if author.Principal != nil {
		if strings.TrimSpace(author.Principal.Id) == "" {
			return fmt.Errorf("project record author principal is invalid")
		}
		count++
	}
	if author.Agent != nil {
		if author.Agent.Workspace != workspace || !typed_id.Valid(typed_id.WorkspaceAgent, author.Agent.Id) {
			return fmt.Errorf("project record author agent is invalid")
		}
		count++
	}
	if count != 1 {
		return fmt.Errorf("project record must have exactly one author")
	}
	return nil
}

func projectRecordAuthorValues(author ProjectRecordAuthor) (any, any) {
	if author.Principal != nil {
		return author.Principal.Id, nil
	}
	return nil, author.Agent.Id
}

func projectRecordAuthorFromValues(workspace model.WorkspaceRef, principal, agent sql.NullString) (*model.PrincipalRef, *model.WorkspaceAgentRef) {
	if principal.Valid {
		return &model.PrincipalRef{Id: principal.String}, nil
	}
	if agent.Valid {
		return nil, &model.WorkspaceAgentRef{Workspace: workspace, Id: agent.String}
	}
	return nil, nil
}

func normalizeProjectRecordName(name string) (string, error) {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return "", fmt.Errorf("name must be lowercase snake case")
	}
	for _, character := range name {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return "", fmt.Errorf("name must be lowercase snake case")
		}
	}
	return name, nil
}

func normalizeProjectRecordLabelAndDescription(label, description *string) error {
	*label = strings.TrimSpace(*label)
	*description = strings.TrimSpace(*description)
	if *label == "" || len(*label) > 256 || len(*description) > 4096 {
		return fmt.Errorf("label or description is invalid")
	}
	return nil
}

func normalizeProjectRecordAttribute(attribute *model.ProjectRecordAttribute) error {
	if _, err := normalizeProjectRecordName(attribute.Name); err != nil {
		return err
	}
	if err := normalizeProjectRecordLabelAndDescription(&attribute.Label, &attribute.Description); err != nil {
		return err
	}
	switch attribute.Type {
	case projectRecordValueTypeText, projectRecordValueTypeNumber, projectRecordValueTypeBoolean, projectRecordValueTypeDatetime:
		if attribute.TargetSchema != nil {
			return fmt.Errorf("only record references may have a target schema")
		}
	case projectRecordValueTypeRecord:
		if attribute.TargetSchema == nil || attribute.TargetSchema.Project != attribute.Ref.Schema.Project || !typed_id.Valid(typed_id.ProjectRecordSchema, attribute.TargetSchema.Id) {
			return fmt.Errorf("record reference target schema is invalid")
		}
	default:
		return fmt.Errorf("attribute type is invalid")
	}
	if attribute.Cardinality != "one" && attribute.Cardinality != "many" {
		return fmt.Errorf("attribute cardinality is invalid")
	}
	if attribute.Uniqueness != "none" && attribute.Uniqueness != "record" && attribute.Uniqueness != "global" {
		return fmt.Errorf("attribute uniqueness is invalid")
	}
	if attribute.Cardinality == "one" && attribute.Uniqueness == "record" {
		return fmt.Errorf("one-valued attributes cannot use record uniqueness")
	}
	if attribute.Display != "none" && attribute.Display != "primary" && attribute.Display != "secondary" {
		return fmt.Errorf("attribute display is invalid")
	}
	return nil
}

func normalizeProjectRecordValue(attribute model.ProjectRecordAttribute, value any) (projectRecordValueStorage, error) {
	result := projectRecordValueStorage{typeName: attribute.Type}
	switch attribute.Type {
	case projectRecordValueTypeText:
		text, ok := value.(string)
		if !ok {
			return result, fmt.Errorf("text value must be a string")
		}
		result.text, result.key, result.value = &text, "text:"+text, text
	case projectRecordValueTypeNumber:
		var number float64
		switch numberValue := value.(type) {
		case float64:
			number = numberValue
		case float32:
			number = float64(numberValue)
		case int:
			number = float64(numberValue)
		case int64:
			number = float64(numberValue)
		case json.Number:
			parsed, err := strconv.ParseFloat(string(numberValue), 64)
			if err != nil {
				return result, fmt.Errorf("number value is invalid")
			}
			number = parsed
		default:
			return result, fmt.Errorf("number value must be a JSON number")
		}
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return result, fmt.Errorf("number value must be finite")
		}
		result.number, result.key, result.value = &number, "number:"+strconv.FormatFloat(number, 'g', -1, 64), number
	case projectRecordValueTypeBoolean:
		boolean, ok := value.(bool)
		if !ok {
			return result, fmt.Errorf("boolean value must be a boolean")
		}
		key := "boolean:false"
		if boolean {
			key = "boolean:true"
		}
		result.boolean, result.key, result.value = &boolean, key, boolean
	case projectRecordValueTypeDatetime:
		text, ok := value.(string)
		if !ok {
			return result, fmt.Errorf("datetime value must be an RFC 3339 string")
		}
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err != nil {
			return result, fmt.Errorf("datetime value must be an RFC 3339 string")
		}
		normalized := parsed.UTC().Format(time.RFC3339Nano)
		result.datetime, result.key, result.value = &normalized, "datetime:"+normalized, normalized
	case projectRecordValueTypeRecord:
		reference, ok := value.(string)
		if !ok || !typed_id.Valid(typed_id.ProjectRecord, reference) {
			return result, fmt.Errorf("record reference value is invalid")
		}
		if attribute.TargetSchema == nil {
			return result, fmt.Errorf("record reference target schema is invalid")
		}
		referenceSchema := attribute.TargetSchema.Id
		result.referenceSchema, result.reference, result.key, result.value = &referenceSchema, &reference, "record:"+reference, reference
	default:
		return result, fmt.Errorf("attribute type is invalid")
	}
	return result, nil
}

func projectRecordTimestamp(value any) string {
	switch timestamp := value.(type) {
	case time.Time:
		return timestamp.UTC().Format(time.RFC3339Nano)
	case string:
		return timestamp
	case []byte:
		return string(timestamp)
	default:
		return ""
	}
}

func (store *Store) ProjectRecordSchemasGet(ctx context.Context, project model.ProjectRef, principal model.PrincipalRef) (error, []model.ProjectRecordSchema) {
	if err, available := store.ProjectGet(ctx, project, principal); err != nil {
		return err, nil
	} else if available == nil {
		return nil, []model.ProjectRecordSchema{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `SELECT id, name, label, description, author_principal, author_agent, created_at FROM gatehouse_project_record_schemas WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` ORDER BY created_at DESC, id DESC`, project.Workspace.Id, project.Id)
	if err != nil {
		return fmt.Errorf("get project record schemas: %w", err), nil
	}
	defer rows.Close()
	result := []model.ProjectRecordSchema{}
	for rows.Next() {
		var schema model.ProjectRecordSchema
		var principalID, agentID sql.NullString
		var createdAt any
		schema.Ref.Project = project
		if err := rows.Scan(&schema.Ref.Id, &schema.Name, &schema.Label, &schema.Description, &principalID, &agentID, &createdAt); err != nil {
			return fmt.Errorf("scan project record schema: %w", err), nil
		}
		schema.AuthorPrincipal, schema.AuthorAgent = projectRecordAuthorFromValues(project.Workspace, principalID, agentID)
		schema.CreatedAt = projectRecordTimestamp(createdAt)
		result = append(result, schema)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project record schemas: %w", err), nil
	}
	return nil, result
}

func (store *Store) ProjectRecordSchemaGet(ctx context.Context, schema model.ProjectRecordSchemaRef, principal model.PrincipalRef) (error, *model.ProjectRecordSchema) {
	if !typed_id.Valid(typed_id.ProjectRecordSchema, schema.Id) {
		return fmt.Errorf("get project record schema: ID is invalid"), nil
	}
	if err, available := store.ProjectGet(ctx, schema.Project, principal); err != nil || available == nil {
		return err, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	result := &model.ProjectRecordSchema{Ref: schema}
	var principalID, agentID sql.NullString
	var createdAt any
	err := store.QueryRowContext(ctx, `SELECT name, label, description, author_principal, author_agent, created_at FROM gatehouse_project_record_schemas WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND id = `+placeholder(3), schema.Project.Workspace.Id, schema.Project.Id, schema.Id).Scan(&result.Name, &result.Label, &result.Description, &principalID, &agentID, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return fmt.Errorf("get project record schema: %w", err), nil
	}
	result.AuthorPrincipal, result.AuthorAgent = projectRecordAuthorFromValues(schema.Project.Workspace, principalID, agentID)
	result.CreatedAt = projectRecordTimestamp(createdAt)
	return nil, result
}

func (store *Store) ProjectRecordSchemaCreate(ctx context.Context, schema model.ProjectRecordSchema, principal model.PrincipalRef, author ProjectRecordAuthor) (error, model.ProjectRecordSchema) {
	if !typed_id.Valid(typed_id.ProjectRecordSchema, schema.Ref.Id) {
		return fmt.Errorf("create project record schema: ID is invalid"), model.ProjectRecordSchema{}
	}
	if _, err := normalizeProjectRecordName(schema.Name); err != nil {
		return fmt.Errorf("create project record schema: %w", err), model.ProjectRecordSchema{}
	}
	if err := normalizeProjectRecordLabelAndDescription(&schema.Label, &schema.Description); err != nil {
		return fmt.Errorf("create project record schema: %w", err), model.ProjectRecordSchema{}
	}
	if author.Principal == nil && author.Agent == nil {
		author.Principal = &principal
	}
	if err := projectRecordAuthorValid(author, schema.Ref.Project.Workspace); err != nil {
		return fmt.Errorf("create project record schema: %w", err), model.ProjectRecordSchema{}
	}
	if err, available := store.ProjectGet(ctx, schema.Ref.Project, principal); err != nil || available == nil {
		if err != nil {
			return err, model.ProjectRecordSchema{}
		}
		return fmt.Errorf("create project record schema: project is unavailable"), model.ProjectRecordSchema{}
	}
	createdAt, err := typed_id.Timestamp(typed_id.ProjectRecordSchema, schema.Ref.Id)
	if err != nil {
		return fmt.Errorf("create project record schema: ID is invalid"), model.ProjectRecordSchema{}
	}
	schema.AuthorPrincipal, schema.AuthorAgent = author.Principal, author.Agent
	schema.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project record schema creation: %w", err), model.ProjectRecordSchema{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	authorPrincipal, authorAgent := projectRecordAuthorValues(author)
	if _, err := transaction.ExecContext(ctx, `INSERT INTO gatehouse_project_record_schemas (workspace, project, id, name, label, description, author_principal, author_agent, created_at) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`)`, schema.Ref.Project.Workspace.Id, schema.Ref.Project.Id, schema.Ref.Id, schema.Name, schema.Label, schema.Description, authorPrincipal, authorAgent, schema.CreatedAt); err != nil {
		return fmt.Errorf("insert project record schema: %w", err), model.ProjectRecordSchema{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_record_schema.create", ResourceKind: ActivityResourceKindProjectRecordSchema, ResourceProjectRecordSchema: &schema.Ref.Id}, []string{ActivityTopicProjectRecordSchema(schema.Ref)}); err != nil {
		return fmt.Errorf("append project record schema creation activity: %w", err), model.ProjectRecordSchema{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project record schema creation: %w", err), model.ProjectRecordSchema{}
	}
	return nil, schema
}

func (store *Store) ProjectRecordSchemaDetailsSetAs(ctx context.Context, reference model.ProjectRecordSchemaRef, principal model.PrincipalRef, author ProjectRecordAuthor, label, description string) (error, *model.ProjectRecordSchema) {
	if !typed_id.Valid(typed_id.ProjectRecordSchema, reference.Id) {
		return fmt.Errorf("update project record schema: ID is invalid"), nil
	}
	if err := normalizeProjectRecordLabelAndDescription(&label, &description); err != nil {
		return fmt.Errorf("update project record schema: %w", err), nil
	}
	if author.Principal == nil && author.Agent == nil {
		author.Principal = &principal
	}
	if err := projectRecordAuthorValid(author, reference.Project.Workspace); err != nil {
		return fmt.Errorf("update project record schema: %w", err), nil
	}
	if err, available := store.ProjectGet(ctx, reference.Project, principal); err != nil || available == nil {
		return err, nil
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project record schema update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	authorPrincipal, authorAgent := projectRecordAuthorValues(author)
	result, err := transaction.ExecContext(ctx, `UPDATE gatehouse_project_record_schemas SET label = `+placeholder(1)+`, description = `+placeholder(2)+`, author_principal = `+placeholder(3)+`, author_agent = `+placeholder(4)+` WHERE workspace = `+placeholder(5)+` AND project = `+placeholder(6)+` AND id = `+placeholder(7), label, description, authorPrincipal, authorAgent, reference.Project.Workspace.Id, reference.Project.Id, reference.Id)
	if err != nil {
		return fmt.Errorf("update project record schema: %w", err), nil
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return err, nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_record_schema.update", ResourceKind: ActivityResourceKindProjectRecordSchema, ResourceProjectRecordSchema: &reference.Id}, []string{ActivityTopicProjectRecordSchema(reference)}); err != nil {
		return fmt.Errorf("append project record schema update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project record schema update: %w", err), nil
	}
	return store.ProjectRecordSchemaGet(ctx, reference, principal)
}

func (store *Store) ProjectRecordSchemaRemove(ctx context.Context, reference model.ProjectRecordSchemaRef, principal model.PrincipalRef) (error, bool) {
	if !typed_id.Valid(typed_id.ProjectRecordSchema, reference.Id) {
		return fmt.Errorf("remove project record schema: ID is invalid"), false
	}
	if err, available := store.ProjectGet(ctx, reference.Project, principal); err != nil || available == nil {
		return err, false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project record schema removal: %w", err), false
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `DELETE FROM gatehouse_project_record_schemas WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND id = `+placeholder(3), reference.Project.Workspace.Id, reference.Project.Id, reference.Id)
	if err != nil {
		return fmt.Errorf("remove project record schema: %w", err), false
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return err, false
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_record_schema.remove", ResourceKind: ActivityResourceKindProjectRecordSchema, ResourceProjectRecordSchema: &reference.Id}, []string{ActivityTopicProject(reference.Project)}); err != nil {
		return fmt.Errorf("append project record schema removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project record schema removal: %w", err), false
	}
	return nil, true
}

func (store *Store) ProjectRecordAttributesGet(ctx context.Context, schema model.ProjectRecordSchemaRef, principal model.PrincipalRef) (error, []model.ProjectRecordAttribute) {
	if err, available := store.ProjectGet(ctx, schema.Project, principal); err != nil {
		return err, nil
	} else if available == nil {
		return nil, []model.ProjectRecordAttribute{}
	}
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `SELECT id, name, label, description, type, target_schema, cardinality, uniqueness, display, author_principal, author_agent, created_at FROM gatehouse_project_record_attributes WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND schema = `+placeholder(3)+` ORDER BY name`, schema.Project.Workspace.Id, schema.Project.Id, schema.Id)
	if err != nil {
		return fmt.Errorf("get project record attributes: %w", err), nil
	}
	defer rows.Close()
	attributes := []model.ProjectRecordAttribute{}
	for rows.Next() {
		attribute, err := scanProjectRecordAttribute(rows, schema)
		if err != nil {
			return err, nil
		}
		attributes = append(attributes, attribute)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project record attributes: %w", err), nil
	}
	return nil, attributes
}

func scanProjectRecordAttribute(scanner interface{ Scan(...any) error }, schema model.ProjectRecordSchemaRef) (model.ProjectRecordAttribute, error) {
	attribute := model.ProjectRecordAttribute{Ref: model.ProjectRecordAttributeRef{Schema: schema}}
	var targetSchema, principalID, agentID sql.NullString
	var createdAt any
	if err := scanner.Scan(&attribute.Ref.Id, &attribute.Name, &attribute.Label, &attribute.Description, &attribute.Type, &targetSchema, &attribute.Cardinality, &attribute.Uniqueness, &attribute.Display, &principalID, &agentID, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return attribute, err
		}
		return attribute, fmt.Errorf("scan project record attribute: %w", err)
	}
	if targetSchema.Valid {
		attribute.TargetSchema = &model.ProjectRecordSchemaRef{Project: schema.Project, Id: targetSchema.String}
	}
	attribute.AuthorPrincipal, attribute.AuthorAgent = projectRecordAuthorFromValues(schema.Project.Workspace, principalID, agentID)
	attribute.CreatedAt = projectRecordTimestamp(createdAt)
	return attribute, nil
}

func (store *Store) ProjectRecordAttributeGet(ctx context.Context, reference model.ProjectRecordAttributeRef, principal model.PrincipalRef) (error, *model.ProjectRecordAttribute) {
	if !typed_id.Valid(typed_id.ProjectRecordAttribute, reference.Id) {
		return fmt.Errorf("get project record attribute: ID is invalid"), nil
	}
	if err, available := store.ProjectGet(ctx, reference.Schema.Project, principal); err != nil || available == nil {
		return err, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	attribute, err := scanProjectRecordAttribute(store.QueryRowContext(ctx, `SELECT id, name, label, description, type, target_schema, cardinality, uniqueness, display, author_principal, author_agent, created_at FROM gatehouse_project_record_attributes WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND schema = `+placeholder(3)+` AND id = `+placeholder(4), reference.Schema.Project.Workspace.Id, reference.Schema.Project.Id, reference.Schema.Id, reference.Id), reference.Schema)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return err, nil
	}
	return nil, &attribute
}

func (store *Store) ProjectRecordAttributeCreate(ctx context.Context, attribute model.ProjectRecordAttribute, principal model.PrincipalRef, author ProjectRecordAuthor) (error, model.ProjectRecordAttribute) {
	if !typed_id.Valid(typed_id.ProjectRecordAttribute, attribute.Ref.Id) {
		return fmt.Errorf("create project record attribute: ID is invalid"), model.ProjectRecordAttribute{}
	}
	if err := normalizeProjectRecordAttribute(&attribute); err != nil {
		return fmt.Errorf("create project record attribute: %w", err), model.ProjectRecordAttribute{}
	}
	if author.Principal == nil && author.Agent == nil {
		author.Principal = &principal
	}
	if err := projectRecordAuthorValid(author, attribute.Ref.Schema.Project.Workspace); err != nil {
		return fmt.Errorf("create project record attribute: %w", err), model.ProjectRecordAttribute{}
	}
	if err, available := store.ProjectGet(ctx, attribute.Ref.Schema.Project, principal); err != nil || available == nil {
		if err != nil {
			return err, model.ProjectRecordAttribute{}
		}
		return fmt.Errorf("create project record attribute: project is unavailable"), model.ProjectRecordAttribute{}
	}
	createdAt, err := typed_id.Timestamp(typed_id.ProjectRecordAttribute, attribute.Ref.Id)
	if err != nil {
		return fmt.Errorf("create project record attribute: ID is invalid"), model.ProjectRecordAttribute{}
	}
	attribute.AuthorPrincipal, attribute.AuthorAgent = author.Principal, author.Agent
	attribute.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	var targetSchema any
	if attribute.TargetSchema != nil {
		targetSchema = attribute.TargetSchema.Id
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project record attribute creation: %w", err), model.ProjectRecordAttribute{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	authorPrincipal, authorAgent := projectRecordAuthorValues(author)
	if _, err := transaction.ExecContext(ctx, `INSERT INTO gatehouse_project_record_attributes (workspace, project, schema, id, name, label, description, type, target_schema, cardinality, uniqueness, display, author_principal, author_agent, created_at) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`, `+placeholder(12)+`, `+placeholder(13)+`, `+placeholder(14)+`, `+placeholder(15)+`)`, attribute.Ref.Schema.Project.Workspace.Id, attribute.Ref.Schema.Project.Id, attribute.Ref.Schema.Id, attribute.Ref.Id, attribute.Name, attribute.Label, attribute.Description, attribute.Type, targetSchema, attribute.Cardinality, attribute.Uniqueness, attribute.Display, authorPrincipal, authorAgent, attribute.CreatedAt); err != nil {
		return fmt.Errorf("insert project record attribute: %w", err), model.ProjectRecordAttribute{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_record_attribute.create", ResourceKind: ActivityResourceKindProjectRecordAttribute, ResourceProjectRecordAttribute: &attribute.Ref.Id}, []string{ActivityTopicProjectRecordAttribute(attribute.Ref)}); err != nil {
		return fmt.Errorf("append project record attribute creation activity: %w", err), model.ProjectRecordAttribute{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project record attribute creation: %w", err), model.ProjectRecordAttribute{}
	}
	return nil, attribute
}

func (store *Store) ProjectRecordAttributeSetAs(ctx context.Context, attribute model.ProjectRecordAttribute, principal model.PrincipalRef, author ProjectRecordAuthor) (error, *model.ProjectRecordAttribute) {
	if !typed_id.Valid(typed_id.ProjectRecordAttribute, attribute.Ref.Id) {
		return fmt.Errorf("update project record attribute: ID is invalid"), nil
	}
	if err := normalizeProjectRecordAttribute(&attribute); err != nil {
		return fmt.Errorf("update project record attribute: %w", err), nil
	}
	if author.Principal == nil && author.Agent == nil {
		author.Principal = &principal
	}
	if err := projectRecordAuthorValid(author, attribute.Ref.Schema.Project.Workspace); err != nil {
		return fmt.Errorf("update project record attribute: %w", err), nil
	}
	if err, available := store.ProjectGet(ctx, attribute.Ref.Schema.Project, principal); err != nil || available == nil {
		return err, nil
	}
	var targetSchema any
	if attribute.TargetSchema != nil {
		targetSchema = attribute.TargetSchema.Id
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project record attribute update: %w", err), nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	authorPrincipal, authorAgent := projectRecordAuthorValues(author)
	result, err := transaction.ExecContext(ctx, `UPDATE gatehouse_project_record_attributes SET label = `+placeholder(1)+`, description = `+placeholder(2)+`, type = `+placeholder(3)+`, target_schema = `+placeholder(4)+`, cardinality = `+placeholder(5)+`, uniqueness = `+placeholder(6)+`, display = `+placeholder(7)+`, author_principal = `+placeholder(8)+`, author_agent = `+placeholder(9)+` WHERE workspace = `+placeholder(10)+` AND project = `+placeholder(11)+` AND schema = `+placeholder(12)+` AND id = `+placeholder(13), attribute.Label, attribute.Description, attribute.Type, targetSchema, attribute.Cardinality, attribute.Uniqueness, attribute.Display, authorPrincipal, authorAgent, attribute.Ref.Schema.Project.Workspace.Id, attribute.Ref.Schema.Project.Id, attribute.Ref.Schema.Id, attribute.Ref.Id)
	if err != nil {
		return fmt.Errorf("update project record attribute: %w", err), nil
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return err, nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_record_attribute.update", ResourceKind: ActivityResourceKindProjectRecordAttribute, ResourceProjectRecordAttribute: &attribute.Ref.Id}, []string{ActivityTopicProjectRecordAttribute(attribute.Ref)}); err != nil {
		return fmt.Errorf("append project record attribute update activity: %w", err), nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project record attribute update: %w", err), nil
	}
	return store.ProjectRecordAttributeGet(ctx, attribute.Ref, principal)
}

func (store *Store) ProjectRecordAttributeRemove(ctx context.Context, reference model.ProjectRecordAttributeRef, principal model.PrincipalRef) (error, bool) {
	if !typed_id.Valid(typed_id.ProjectRecordAttribute, reference.Id) {
		return fmt.Errorf("remove project record attribute: ID is invalid"), false
	}
	if err, available := store.ProjectGet(ctx, reference.Schema.Project, principal); err != nil || available == nil {
		return err, false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project record attribute removal: %w", err), false
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `DELETE FROM gatehouse_project_record_attributes WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND schema = `+placeholder(3)+` AND id = `+placeholder(4), reference.Schema.Project.Workspace.Id, reference.Schema.Project.Id, reference.Schema.Id, reference.Id)
	if err != nil {
		return fmt.Errorf("remove project record attribute: %w", err), false
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return err, false
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_record_attribute.remove", ResourceKind: ActivityResourceKindProjectRecordAttribute, ResourceProjectRecordAttribute: &reference.Id}, []string{ActivityTopicProjectRecordSchema(reference.Schema)}); err != nil {
		return fmt.Errorf("append project record attribute removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project record attribute removal: %w", err), false
	}
	return nil, true
}

func (store *Store) ProjectRecordsGet(ctx context.Context, schema model.ProjectRecordSchemaRef, principal model.PrincipalRef, limit int, cursor string) (error, []model.ProjectRecord) {
	if !typed_id.Valid(typed_id.ProjectRecordSchema, schema.Id) {
		return fmt.Errorf("get project records: schema ID is invalid"), nil
	}
	if limit < 1 || limit > 100 {
		return fmt.Errorf("get project records: limit must be between 1 and 100"), nil
	}
	if cursor != "" && !typed_id.Valid(typed_id.ProjectRecord, cursor) {
		return fmt.Errorf("get project records: cursor is invalid"), nil
	}
	if err, available := store.ProjectGet(ctx, schema.Project, principal); err != nil {
		return err, nil
	} else if available == nil {
		return nil, []model.ProjectRecord{}
	}
	placeholder := keychainPlaceholder(store.kind)
	query := `SELECT id, author_principal, author_agent, created_at FROM gatehouse_project_records WHERE workspace = ` + placeholder(1) + ` AND project = ` + placeholder(2) + ` AND schema = ` + placeholder(3)
	arguments := []any{schema.Project.Workspace.Id, schema.Project.Id, schema.Id}
	if cursor != "" {
		query += ` AND id < ` + placeholder(4)
		arguments = append(arguments, cursor)
	}
	query += ` ORDER BY id DESC LIMIT ` + placeholder(len(arguments)+1)
	arguments = append(arguments, limit)
	rows, err := store.QueryContext(ctx, query, arguments...)
	if err != nil {
		return fmt.Errorf("get project records: %w", err), nil
	}
	defer rows.Close()
	result := []model.ProjectRecord{}
	for rows.Next() {
		record, err := scanProjectRecord(rows, schema)
		if err != nil {
			return err, nil
		}
		result = append(result, record)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project records: %w", err), nil
	}
	return nil, result
}

// ProjectRecordCardsGet returns collection-card values and one shallow level of reference display data.
func (store *Store) ProjectRecordCardsGet(ctx context.Context, schema model.ProjectRecordSchemaRef, principal model.PrincipalRef, limit int, cursor string) (error, []ProjectRecordCard) {
	err, records := store.ProjectRecordsGet(ctx, schema, principal, limit, cursor)
	if err != nil || len(records) == 0 {
		return err, []ProjectRecordCard{}
	}
	err, attributes := store.ProjectRecordAttributesGet(ctx, schema, principal)
	if err != nil {
		return err, nil
	}
	attributeByID := make(map[string]model.ProjectRecordAttribute, len(attributes))
	for _, attribute := range attributes {
		attributeByID[attribute.Ref.Id] = attribute
	}
	cards := make([]ProjectRecordCard, len(records))
	cardByRecord := make(map[string]int, len(records))
	for index, record := range records {
		cards[index] = ProjectRecordCard{Record: record, Values: []ProjectRecordCardValue{}}
		cardByRecord[record.Ref.Id] = index
	}
	placeholder := keychainPlaceholder(store.kind)
	recordPlaceholders := make([]string, len(records))
	arguments := []any{schema.Project.Workspace.Id, schema.Project.Id, schema.Id}
	for index, record := range records {
		recordPlaceholders[index] = placeholder(len(arguments) + 1)
		arguments = append(arguments, record.Ref.Id)
	}
	rows, err := store.QueryContext(ctx, `SELECT record_values.record, record_values.attribute, record_values.value_type, record_values.value_text, record_values.value_number, record_values.value_boolean, record_values.value_datetime, record_values.value_reference, record_values.sensitive FROM gatehouse_project_record_values AS record_values JOIN gatehouse_project_record_attributes AS attributes ON attributes.workspace = record_values.workspace AND attributes.project = record_values.project AND attributes.schema = record_values.schema AND attributes.id = record_values.attribute WHERE record_values.workspace = `+placeholder(1)+` AND record_values.project = `+placeholder(2)+` AND record_values.schema = `+placeholder(3)+` AND record_values.record IN (`+strings.Join(recordPlaceholders, ", ")+`) AND attributes.display <> 'none' ORDER BY record_values.record, attributes.name, record_values.id`, arguments...)
	if err != nil {
		return fmt.Errorf("get project record card values: %w", err), nil
	}
	for rows.Next() {
		recordID, value, err := scanProjectRecordCardValue(rows)
		if err != nil {
			return err, nil
		}
		cards[cardByRecord[recordID]].Values = append(cards[cardByRecord[recordID]].Values, value)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project record card values: %w", err), nil
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close project record card values: %w", err), nil
	}
	inputs := make([]projectRecordReferenceInput, 0, len(cards))
	for _, card := range cards {
		for _, value := range card.Values {
			inputs = append(inputs, projectRecordReferenceInput{Attribute: value.Attribute, Value: value.Value})
		}
	}
	err, references := store.projectRecordReferenceDisplaysGet(ctx, schema.Project, attributeByID, inputs)
	if err != nil {
		return err, nil
	}
	for cardIndex := range cards {
		for valueIndex := range cards[cardIndex].Values {
			value := &cards[cardIndex].Values[valueIndex]
			attribute := attributeByID[value.Attribute]
			if attribute.Type != projectRecordValueTypeRecord {
				continue
			}
			if recordID, ok := value.Value.(string); ok {
				value.Reference = references[recordID]
			}
		}
	}
	return nil, cards
}

// ProjectRecordValueReferenceDisplaysGet returns one shallow reference display projection per value ID.
func (store *Store) ProjectRecordValueReferenceDisplaysGet(ctx context.Context, record model.ProjectRecordRef, principal model.PrincipalRef, values []model.ProjectRecordValue) (error, map[string]*ProjectRecordReferenceDisplay) {
	if len(values) == 0 {
		return nil, map[string]*ProjectRecordReferenceDisplay{}
	}
	err, attributes := store.ProjectRecordAttributesGet(ctx, record.Schema, principal)
	if err != nil {
		return err, nil
	}
	attributeByID := make(map[string]model.ProjectRecordAttribute, len(attributes))
	for _, attribute := range attributes {
		attributeByID[attribute.Ref.Id] = attribute
	}
	inputs := make([]projectRecordReferenceInput, 0, len(values))
	for _, value := range values {
		inputs = append(inputs, projectRecordReferenceInput{Attribute: value.Attribute.Id, Value: value.Value})
	}
	err, references := store.projectRecordReferenceDisplaysGet(ctx, record.Schema.Project, attributeByID, inputs)
	if err != nil {
		return err, nil
	}
	result := make(map[string]*ProjectRecordReferenceDisplay, len(values))
	for _, value := range values {
		attribute := attributeByID[value.Attribute.Id]
		if attribute.Type != projectRecordValueTypeRecord {
			continue
		}
		if recordID, ok := value.Value.(string); ok {
			result[value.Ref.Id] = references[recordID]
		}
	}
	return nil, result
}

func (store *Store) projectRecordReferenceDisplaysGet(ctx context.Context, project model.ProjectRef, attributes map[string]model.ProjectRecordAttribute, inputs []projectRecordReferenceInput) (error, map[string]*ProjectRecordReferenceDisplay) {
	references := map[string]*ProjectRecordReferenceDisplay{}
	schemaIDs := map[string]struct{}{}
	for _, input := range inputs {
		attribute, ok := attributes[input.Attribute]
		if !ok || attribute.Type != projectRecordValueTypeRecord || attribute.TargetSchema == nil {
			continue
		}
		recordID, ok := input.Value.(string)
		if !ok {
			return fmt.Errorf("get project record reference displays: record reference is invalid"), nil
		}
		if references[recordID] == nil {
			references[recordID] = &ProjectRecordReferenceDisplay{PrimaryValues: []ProjectRecordReferenceDisplayValue{}}
			schemaIDs[attribute.TargetSchema.Id] = struct{}{}
		}
	}
	if len(references) == 0 {
		return nil, references
	}
	placeholder := keychainPlaceholder(store.kind)
	schemaPlaceholders := make([]string, 0, len(schemaIDs))
	schemaArguments := []any{project.Workspace.Id, project.Id}
	for id := range schemaIDs {
		schemaPlaceholders = append(schemaPlaceholders, placeholder(len(schemaArguments)+1))
		schemaArguments = append(schemaArguments, id)
	}
	rows, err := store.QueryContext(ctx, `SELECT id, label FROM gatehouse_project_record_schemas WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND id IN (`+strings.Join(schemaPlaceholders, ", ")+`)`, schemaArguments...)
	if err != nil {
		return fmt.Errorf("get project record reference schemas: %w", err), nil
	}
	schemaLabels := map[string]string{}
	for rows.Next() {
		var id, label string
		if err := rows.Scan(&id, &label); err != nil {
			rows.Close()
			return fmt.Errorf("scan project record reference schema: %w", err), nil
		}
		schemaLabels[id] = label
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("iterate project record reference schemas: %w", err), nil
	}
	for _, input := range inputs {
		attribute, ok := attributes[input.Attribute]
		if !ok || attribute.Type != projectRecordValueTypeRecord || attribute.TargetSchema == nil {
			continue
		}
		recordID, ok := input.Value.(string)
		if ok && references[recordID] != nil {
			references[recordID].SchemaLabel = schemaLabels[attribute.TargetSchema.Id]
		}
	}
	referencePlaceholders := make([]string, 0, len(references))
	referenceArguments := []any{project.Workspace.Id, project.Id}
	for id := range references {
		referencePlaceholders = append(referencePlaceholders, placeholder(len(referenceArguments)+1))
		referenceArguments = append(referenceArguments, id)
	}
	rows, err = store.QueryContext(ctx, `SELECT record_values.record, record_values.value_type, record_values.value_text, record_values.value_number, record_values.value_boolean, record_values.value_datetime, record_values.value_reference, record_values.sensitive FROM gatehouse_project_record_values AS record_values JOIN gatehouse_project_record_attributes AS attributes ON attributes.workspace = record_values.workspace AND attributes.project = record_values.project AND attributes.schema = record_values.schema AND attributes.id = record_values.attribute WHERE record_values.workspace = `+placeholder(1)+` AND record_values.project = `+placeholder(2)+` AND record_values.record IN (`+strings.Join(referencePlaceholders, ", ")+`) AND attributes.display = 'primary' ORDER BY record_values.record, attributes.name, record_values.id`, referenceArguments...)
	if err != nil {
		return fmt.Errorf("get project record reference primary values: %w", err), nil
	}
	defer rows.Close()
	for rows.Next() {
		recordID, value, err := scanProjectRecordReferenceDisplayValue(rows)
		if err != nil {
			return err, nil
		}
		references[recordID].PrimaryValues = append(references[recordID].PrimaryValues, value)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project record reference primary values: %w", err), nil
	}
	return nil, references
}

// ProjectRecordIncomingReferencesGet returns source records that reference target, grouped by source schema and attribute.
func (store *Store) ProjectRecordIncomingReferencesGet(ctx context.Context, target model.ProjectRecordRef, principal model.PrincipalRef, limit int, cursor string) (error, []ProjectRecordIncomingReferenceGroup) {
	if !typed_id.Valid(typed_id.ProjectRecord, target.Id) {
		return fmt.Errorf("get project record incoming references: record ID is invalid"), nil
	}
	if limit < 1 || limit > 100 {
		return fmt.Errorf("get project record incoming references: limit must be between 1 and 100"), nil
	}
	if cursor != "" && !typed_id.Valid(typed_id.ProjectRecordValue, cursor) {
		return fmt.Errorf("get project record incoming references: cursor is invalid"), nil
	}
	if err, available := store.ProjectGet(ctx, target.Schema.Project, principal); err != nil || available == nil {
		return err, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	query := `SELECT values_table.id, values_table.schema, values_table.attribute, values_table.record, schemas.label, attributes.name, attributes.label FROM gatehouse_project_record_values AS values_table JOIN gatehouse_project_record_schemas AS schemas ON schemas.workspace = values_table.workspace AND schemas.project = values_table.project AND schemas.id = values_table.schema JOIN gatehouse_project_record_attributes AS attributes ON attributes.workspace = values_table.workspace AND attributes.project = values_table.project AND attributes.schema = values_table.schema AND attributes.id = values_table.attribute WHERE values_table.workspace = ` + placeholder(1) + ` AND values_table.project = ` + placeholder(2) + ` AND values_table.value_type = 'record' AND values_table.value_reference_schema = ` + placeholder(3) + ` AND values_table.value_reference = ` + placeholder(4)
	arguments := []any{target.Schema.Project.Workspace.Id, target.Schema.Project.Id, target.Schema.Id, target.Id}
	if cursor != "" {
		query += ` AND values_table.id < ` + placeholder(5)
		arguments = append(arguments, cursor)
	}
	query += ` ORDER BY values_table.id DESC LIMIT ` + placeholder(len(arguments)+1)
	arguments = append(arguments, limit)
	rows, err := store.QueryContext(ctx, query, arguments...)
	if err != nil {
		return fmt.Errorf("get project record incoming references: %w", err), nil
	}
	groups := []ProjectRecordIncomingReferenceGroup{}
	groupIndexes := map[string]int{}
	recordIDs := map[string]struct{}{}
	for rows.Next() {
		var valueID, schemaID, attributeID, recordID, schemaLabel, attributeName, attributeLabel string
		if err := rows.Scan(&valueID, &schemaID, &attributeID, &recordID, &schemaLabel, &attributeName, &attributeLabel); err != nil {
			return fmt.Errorf("scan project record incoming reference: %w", err), nil
		}
		key := schemaID + "\x00" + attributeID
		index, ok := groupIndexes[key]
		if !ok {
			index = len(groups)
			groupIndexes[key] = index
			schema := model.ProjectRecordSchemaRef{Project: target.Schema.Project, Id: schemaID}
			groups = append(groups, ProjectRecordIncomingReferenceGroup{Schema: schema, SchemaLabel: schemaLabel, Attribute: model.ProjectRecordAttributeRef{Schema: schema, Id: attributeID}, AttributeName: attributeName, AttributeLabel: attributeLabel, References: []ProjectRecordIncomingReference{}})
		}
		record := model.ProjectRecordRef{Schema: model.ProjectRecordSchemaRef{Project: target.Schema.Project, Id: schemaID}, Id: recordID}
		groups[index].References = append(groups[index].References, ProjectRecordIncomingReference{ValueID: valueID, Record: record, PrimaryValues: []ProjectRecordReferenceDisplayValue{}})
		recordIDs[recordID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate project record incoming references: %w", err), nil
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close project record incoming references: %w", err), nil
	}
	primaryValues := make(map[string][]ProjectRecordReferenceDisplayValue, len(recordIDs))
	if len(recordIDs) != 0 {
		placeholders := make([]string, 0, len(recordIDs))
		arguments := []any{target.Schema.Project.Workspace.Id, target.Schema.Project.Id}
		for id := range recordIDs {
			primaryValues[id] = []ProjectRecordReferenceDisplayValue{}
			placeholders = append(placeholders, placeholder(len(arguments)+1))
			arguments = append(arguments, id)
		}
		rows, err := store.QueryContext(ctx, `SELECT record_values.record, record_values.value_type, record_values.value_text, record_values.value_number, record_values.value_boolean, record_values.value_datetime, record_values.value_reference, record_values.sensitive FROM gatehouse_project_record_values AS record_values JOIN gatehouse_project_record_attributes AS attributes ON attributes.workspace = record_values.workspace AND attributes.project = record_values.project AND attributes.schema = record_values.schema AND attributes.id = record_values.attribute WHERE record_values.workspace = `+placeholder(1)+` AND record_values.project = `+placeholder(2)+` AND record_values.record IN (`+strings.Join(placeholders, ", ")+`) AND attributes.display = 'primary' ORDER BY record_values.record, attributes.name, record_values.id`, arguments...)
		if err != nil {
			return fmt.Errorf("get project record incoming reference primary values: %w", err), nil
		}
		for rows.Next() {
			recordID, primaryValue, err := scanProjectRecordReferenceDisplayValue(rows)
			if err != nil {
				rows.Close()
				return err, nil
			}
			primaryValues[recordID] = append(primaryValues[recordID], primaryValue)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate project record incoming reference primary values: %w", err), nil
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close project record incoming reference primary values: %w", err), nil
		}
	}
	for groupIndex := range groups {
		for referenceIndex := range groups[groupIndex].References {
			reference := &groups[groupIndex].References[referenceIndex]
			reference.PrimaryValues = primaryValues[reference.Record.Id]
		}
	}
	return nil, groups
}

func scanProjectRecord(scanner interface{ Scan(...any) error }, schema model.ProjectRecordSchemaRef) (model.ProjectRecord, error) {
	record := model.ProjectRecord{Ref: model.ProjectRecordRef{Schema: schema}}
	var principalID, agentID sql.NullString
	var createdAt any
	if err := scanner.Scan(&record.Ref.Id, &principalID, &agentID, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return record, err
		}
		return record, fmt.Errorf("scan project record: %w", err)
	}
	record.AuthorPrincipal, record.AuthorAgent = projectRecordAuthorFromValues(schema.Project.Workspace, principalID, agentID)
	record.CreatedAt = projectRecordTimestamp(createdAt)
	return record, nil
}

func (store *Store) ProjectRecordGet(ctx context.Context, reference model.ProjectRecordRef, principal model.PrincipalRef) (error, *model.ProjectRecord) {
	if !typed_id.Valid(typed_id.ProjectRecord, reference.Id) {
		return fmt.Errorf("get project record: ID is invalid"), nil
	}
	if err, available := store.ProjectGet(ctx, reference.Schema.Project, principal); err != nil || available == nil {
		return err, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	record, err := scanProjectRecord(store.QueryRowContext(ctx, `SELECT id, author_principal, author_agent, created_at FROM gatehouse_project_records WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND schema = `+placeholder(3)+` AND id = `+placeholder(4), reference.Schema.Project.Workspace.Id, reference.Schema.Project.Id, reference.Schema.Id, reference.Id), reference.Schema)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return err, nil
	}
	return nil, &record
}

func (store *Store) ProjectRecordCreate(ctx context.Context, record model.ProjectRecord, principal model.PrincipalRef, author ProjectRecordAuthor, values []ProjectRecordValueCreate) (error, model.ProjectRecord, []model.ProjectRecordValue) {
	if !typed_id.Valid(typed_id.ProjectRecord, record.Ref.Id) {
		return fmt.Errorf("create project record: ID is invalid"), model.ProjectRecord{}, nil
	}
	if author.Principal == nil && author.Agent == nil {
		author.Principal = &principal
	}
	if err := projectRecordAuthorValid(author, record.Ref.Schema.Project.Workspace); err != nil {
		return fmt.Errorf("create project record: %w", err), model.ProjectRecord{}, nil
	}
	if err, available := store.ProjectGet(ctx, record.Ref.Schema.Project, principal); err != nil || available == nil {
		if err != nil {
			return err, model.ProjectRecord{}, nil
		}
		return fmt.Errorf("create project record: project is unavailable"), model.ProjectRecord{}, nil
	}
	createdAt, err := typed_id.Timestamp(typed_id.ProjectRecord, record.Ref.Id)
	if err != nil {
		return fmt.Errorf("create project record: ID is invalid"), model.ProjectRecord{}, nil
	}
	record.AuthorPrincipal, record.AuthorAgent = author.Principal, author.Agent
	record.CreatedAt = createdAt.Format("2006-01-02T15:04:05.000Z")
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project record creation: %w", err), model.ProjectRecord{}, nil
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	authorPrincipal, authorAgent := projectRecordAuthorValues(author)
	if _, err := transaction.ExecContext(ctx, `INSERT INTO gatehouse_project_records (workspace, project, schema, id, author_principal, author_agent, created_at) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`)`, record.Ref.Schema.Project.Workspace.Id, record.Ref.Schema.Project.Id, record.Ref.Schema.Id, record.Ref.Id, authorPrincipal, authorAgent, record.CreatedAt); err != nil {
		return fmt.Errorf("insert project record: %w", err), model.ProjectRecord{}, nil
	}
	created, err := store.projectRecordValuesCreate(ctx, transaction, record.Ref, author, values)
	if err != nil {
		return err, model.ProjectRecord{}, nil
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_record.create", ResourceKind: ActivityResourceKindProjectRecord, ResourceProjectRecord: &record.Ref.Id}, []string{ActivityTopicProjectRecord(record.Ref)}); err != nil {
		return fmt.Errorf("append project record creation activity: %w", err), model.ProjectRecord{}, nil
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project record creation: %w", err), model.ProjectRecord{}, nil
	}
	return nil, record, created
}

func (store *Store) ProjectRecordRemove(ctx context.Context, reference model.ProjectRecordRef, principal model.PrincipalRef) (error, bool) {
	if !typed_id.Valid(typed_id.ProjectRecord, reference.Id) {
		return fmt.Errorf("remove project record: ID is invalid"), false
	}
	if err, available := store.ProjectGet(ctx, reference.Schema.Project, principal); err != nil || available == nil {
		return err, false
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project record removal: %w", err), false
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	result, err := transaction.ExecContext(ctx, `DELETE FROM gatehouse_project_records WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND schema = `+placeholder(3)+` AND id = `+placeholder(4), reference.Schema.Project.Workspace.Id, reference.Schema.Project.Id, reference.Schema.Id, reference.Id)
	if err != nil {
		return fmt.Errorf("remove project record: %w", err), false
	}
	changed, err := result.RowsAffected()
	if err != nil || changed == 0 {
		return err, false
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_record.remove", ResourceKind: ActivityResourceKindProjectRecord, ResourceProjectRecord: &reference.Id}, []string{ActivityTopicProject(reference.Schema.Project)}); err != nil {
		return fmt.Errorf("append project record removal activity: %w", err), false
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project record removal: %w", err), false
	}
	return nil, true
}

func (store *Store) projectRecordAttributeGetInTransaction(ctx context.Context, transaction *sql.Tx, reference model.ProjectRecordAttributeRef) (model.ProjectRecordAttribute, error) {
	placeholder := keychainPlaceholder(store.kind)
	attribute, err := scanProjectRecordAttribute(transaction.QueryRowContext(ctx, `SELECT id, name, label, description, type, target_schema, cardinality, uniqueness, display, author_principal, author_agent, created_at FROM gatehouse_project_record_attributes WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND schema = `+placeholder(3)+` AND id = `+placeholder(4), reference.Schema.Project.Workspace.Id, reference.Schema.Project.Id, reference.Schema.Id, reference.Id), reference.Schema)
	if err == sql.ErrNoRows {
		return attribute, fmt.Errorf("project record attribute is unavailable")
	}
	return attribute, err
}

func (store *Store) projectRecordValuesCreate(ctx context.Context, transaction *sql.Tx, record model.ProjectRecordRef, author ProjectRecordAuthor, creates []ProjectRecordValueCreate) ([]model.ProjectRecordValue, error) {
	seen := map[string]struct{}{}
	result := make([]model.ProjectRecordValue, 0, len(creates))
	placeholder := keychainPlaceholder(store.kind)
	authorPrincipal, authorAgent := projectRecordAuthorValues(author)
	for _, create := range creates {
		if create.Attribute.Schema != record.Schema {
			return nil, fmt.Errorf("create project record value: attribute belongs to another schema")
		}
		if create.ID == "" {
			id, err := typed_id.New(typed_id.ProjectRecordValue)
			if err != nil {
				return nil, fmt.Errorf("generate project record value ID: %w", err)
			}
			create.ID = id
		}
		if !typed_id.Valid(typed_id.ProjectRecordValue, create.ID) {
			return nil, fmt.Errorf("create project record value: ID is invalid")
		}
		if _, exists := seen[create.ID]; exists {
			return nil, fmt.Errorf("create project record value: duplicate value ID")
		}
		seen[create.ID] = struct{}{}
		attribute, err := store.projectRecordAttributeGetInTransaction(ctx, transaction, create.Attribute)
		if err != nil {
			return nil, fmt.Errorf("create project record value: %w", err)
		}
		if create.Sensitive && attribute.Uniqueness == "global" {
			return nil, fmt.Errorf("create project record value: sensitive values cannot be globally unique")
		}
		value, err := normalizeProjectRecordValue(attribute, create.Value)
		if err != nil {
			return nil, fmt.Errorf("create project record value: %w", err)
		}
		createdAt, err := typed_id.Timestamp(typed_id.ProjectRecordValue, create.ID)
		if err != nil {
			return nil, fmt.Errorf("create project record value: ID is invalid")
		}
		createdAtText := createdAt.Format("2006-01-02T15:04:05.000Z")
		if _, err := transaction.ExecContext(ctx, `INSERT INTO gatehouse_project_record_values (workspace, project, schema, record, id, attribute, value_type, value_text, value_number, value_boolean, value_datetime, value_reference_schema, value_reference, value_key, attribute_cardinality, attribute_uniqueness, sensitive, author_principal, author_agent, created_at) VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`, `+placeholder(8)+`, `+placeholder(9)+`, `+placeholder(10)+`, `+placeholder(11)+`, `+placeholder(12)+`, `+placeholder(13)+`, `+placeholder(14)+`, `+placeholder(15)+`, `+placeholder(16)+`, `+placeholder(17)+`, `+placeholder(18)+`, `+placeholder(19)+`, `+placeholder(20)+`)`, record.Schema.Project.Workspace.Id, record.Schema.Project.Id, record.Schema.Id, record.Id, create.ID, attribute.Ref.Id, value.typeName, value.text, value.number, value.boolean, value.datetime, value.referenceSchema, value.reference, value.key, attribute.Cardinality, attribute.Uniqueness, create.Sensitive, authorPrincipal, authorAgent, createdAtText); err != nil {
			return nil, fmt.Errorf("insert project record value: %w", err)
		}
		result = append(result, model.ProjectRecordValue{Ref: model.ProjectRecordValueRef{Record: record, Id: create.ID}, Attribute: attribute.Ref, Value: value.value, Sensitive: create.Sensitive, AuthorPrincipal: author.Principal, AuthorAgent: author.Agent, CreatedAt: createdAtText})
	}
	return result, nil
}

func (store *Store) ProjectRecordValuesGet(ctx context.Context, record model.ProjectRecordRef, principal model.PrincipalRef, limit int, cursor string) (error, []model.ProjectRecordValue) {
	if !typed_id.Valid(typed_id.ProjectRecord, record.Id) {
		return fmt.Errorf("get project record values: record ID is invalid"), nil
	}
	if limit < 1 || limit > 100 {
		return fmt.Errorf("get project record values: limit must be between 1 and 100"), nil
	}
	if cursor != "" && !typed_id.Valid(typed_id.ProjectRecordValue, cursor) {
		return fmt.Errorf("get project record values: cursor is invalid"), nil
	}
	if err, available := store.ProjectGet(ctx, record.Schema.Project, principal); err != nil {
		return err, nil
	} else if available == nil {
		return nil, []model.ProjectRecordValue{}
	}
	placeholder := keychainPlaceholder(store.kind)
	query := `SELECT id, attribute, value_type, value_text, value_number, value_boolean, value_datetime, value_reference, sensitive, author_principal, author_agent, created_at FROM gatehouse_project_record_values WHERE workspace = ` + placeholder(1) + ` AND project = ` + placeholder(2) + ` AND schema = ` + placeholder(3) + ` AND record = ` + placeholder(4)
	arguments := []any{record.Schema.Project.Workspace.Id, record.Schema.Project.Id, record.Schema.Id, record.Id}
	if cursor != "" {
		query += ` AND id < ` + placeholder(5)
		arguments = append(arguments, cursor)
	}
	query += ` ORDER BY id DESC LIMIT ` + placeholder(len(arguments)+1)
	arguments = append(arguments, limit)
	rows, err := store.QueryContext(ctx, query, arguments...)
	if err != nil {
		return fmt.Errorf("get project record values: %w", err), nil
	}
	defer rows.Close()
	values := []model.ProjectRecordValue{}
	for rows.Next() {
		value, err := scanProjectRecordValue(rows, record)
		if err != nil {
			return err, nil
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate project record values: %w", err), nil
	}
	return nil, values
}

func scanProjectRecordValue(scanner interface{ Scan(...any) error }, record model.ProjectRecordRef) (model.ProjectRecordValue, error) {
	value := model.ProjectRecordValue{Ref: model.ProjectRecordValueRef{Record: record}}
	var typeName string
	var text, reference, principalID, agentID sql.NullString
	var number sql.NullFloat64
	var boolean sql.NullBool
	var datetime, createdAt any
	if err := scanner.Scan(&value.Ref.Id, &value.Attribute.Id, &typeName, &text, &number, &boolean, &datetime, &reference, &value.Sensitive, &principalID, &agentID, &createdAt); err != nil {
		if err == sql.ErrNoRows {
			return value, err
		}
		return value, fmt.Errorf("scan project record value: %w", err)
	}
	value.Attribute.Schema = record.Schema
	stored, err := projectRecordValueFromStorage(typeName, text, number, boolean, datetime, reference)
	if err != nil {
		return value, err
	}
	value.Value = stored
	value.AuthorPrincipal, value.AuthorAgent = projectRecordAuthorFromValues(record.Schema.Project.Workspace, principalID, agentID)
	value.CreatedAt = projectRecordTimestamp(createdAt)
	return value, nil
}

func scanProjectRecordCardValue(scanner interface{ Scan(...any) error }) (string, ProjectRecordCardValue, error) {
	var recordID, typeName string
	var value ProjectRecordCardValue
	var text, reference sql.NullString
	var number sql.NullFloat64
	var boolean sql.NullBool
	var datetime any
	if err := scanner.Scan(&recordID, &value.Attribute, &typeName, &text, &number, &boolean, &datetime, &reference, &value.Sensitive); err != nil {
		return "", value, fmt.Errorf("scan project record card value: %w", err)
	}
	stored, err := projectRecordValueFromStorage(typeName, text, number, boolean, datetime, reference)
	if err != nil {
		return "", value, err
	}
	value.Value = stored
	return recordID, value, nil
}

func scanProjectRecordReferenceDisplayValue(scanner interface{ Scan(...any) error }) (string, ProjectRecordReferenceDisplayValue, error) {
	var recordID, typeName string
	var value ProjectRecordReferenceDisplayValue
	var text, reference sql.NullString
	var number sql.NullFloat64
	var boolean sql.NullBool
	var datetime any
	if err := scanner.Scan(&recordID, &typeName, &text, &number, &boolean, &datetime, &reference, &value.Sensitive); err != nil {
		return "", value, fmt.Errorf("scan project record reference display value: %w", err)
	}
	stored, err := projectRecordValueFromStorage(typeName, text, number, boolean, datetime, reference)
	if err != nil {
		return "", value, err
	}
	value.Value = stored
	return recordID, value, nil
}

func projectRecordValueFromStorage(typeName string, text sql.NullString, number sql.NullFloat64, boolean sql.NullBool, datetime any, reference sql.NullString) (any, error) {
	switch typeName {
	case projectRecordValueTypeText:
		return text.String, nil
	case projectRecordValueTypeNumber:
		return number.Float64, nil
	case projectRecordValueTypeBoolean:
		return boolean.Bool, nil
	case projectRecordValueTypeDatetime:
		return projectRecordTimestamp(datetime), nil
	case projectRecordValueTypeRecord:
		return reference.String, nil
	default:
		return nil, fmt.Errorf("scan project record value: type is invalid")
	}
}

func (store *Store) ProjectRecordValuesMutate(ctx context.Context, record model.ProjectRecordRef, principal model.PrincipalRef, author ProjectRecordAuthor, mutation ProjectRecordValuesMutation) (error, ProjectRecordValuesMutationResult) {
	if !typed_id.Valid(typed_id.ProjectRecord, record.Id) {
		return fmt.Errorf("mutate project record values: record ID is invalid"), ProjectRecordValuesMutationResult{}
	}
	if author.Principal == nil && author.Agent == nil {
		author.Principal = &principal
	}
	if err := projectRecordAuthorValid(author, record.Schema.Project.Workspace); err != nil {
		return fmt.Errorf("mutate project record values: %w", err), ProjectRecordValuesMutationResult{}
	}
	if err, available := store.ProjectGet(ctx, record.Schema.Project, principal); err != nil || available == nil {
		if err != nil {
			return err, ProjectRecordValuesMutationResult{}
		}
		return fmt.Errorf("mutate project record values: project is unavailable"), ProjectRecordValuesMutationResult{}
	}
	if len(mutation.Create)+len(mutation.Update)+len(mutation.Delete) == 0 {
		return fmt.Errorf("mutate project record values: at least one mutation is required"), ProjectRecordValuesMutationResult{}
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin project record value mutation: %w", err), ProjectRecordValuesMutationResult{}
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	var exists int
	if err := transaction.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_project_records WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND schema = `+placeholder(3)+` AND id = `+placeholder(4), record.Schema.Project.Workspace.Id, record.Schema.Project.Id, record.Schema.Id, record.Id).Scan(&exists); err != nil {
		return fmt.Errorf("get project record for value mutation: %w", err), ProjectRecordValuesMutationResult{}
	}
	if exists != 1 {
		return nil, ProjectRecordValuesMutationResult{}
	}

	remove := map[string]struct{}{}
	for _, id := range mutation.Delete {
		if !typed_id.Valid(typed_id.ProjectRecordValue, id) {
			return fmt.Errorf("mutate project record values: delete ID is invalid"), ProjectRecordValuesMutationResult{}
		}
		if _, duplicate := remove[id]; duplicate {
			return fmt.Errorf("mutate project record values: duplicate value mutation"), ProjectRecordValuesMutationResult{}
		}
		remove[id] = struct{}{}
	}
	creates := append([]ProjectRecordValueCreate{}, mutation.Create...)
	for _, update := range mutation.Update {
		if !typed_id.Valid(typed_id.ProjectRecordValue, update.ID) {
			return fmt.Errorf("mutate project record values: update ID is invalid"), ProjectRecordValuesMutationResult{}
		}
		if _, duplicate := remove[update.ID]; duplicate {
			return fmt.Errorf("mutate project record values: a value cannot be updated and deleted"), ProjectRecordValuesMutationResult{}
		}
		var attributeID string
		var sensitive bool
		err := transaction.QueryRowContext(ctx, `SELECT attribute, sensitive FROM gatehouse_project_record_values WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND schema = `+placeholder(3)+` AND record = `+placeholder(4)+` AND id = `+placeholder(5), record.Schema.Project.Workspace.Id, record.Schema.Project.Id, record.Schema.Id, record.Id, update.ID).Scan(&attributeID, &sensitive)
		if err == sql.ErrNoRows {
			return nil, ProjectRecordValuesMutationResult{}
		}
		if err != nil {
			return fmt.Errorf("get project record value for update: %w", err), ProjectRecordValuesMutationResult{}
		}
		remove[update.ID] = struct{}{}
		if update.Sensitive != nil {
			sensitive = *update.Sensitive
		}
		creates = append(creates, ProjectRecordValueCreate{Attribute: model.ProjectRecordAttributeRef{Schema: record.Schema, Id: attributeID}, Value: update.Value, Sensitive: sensitive})
	}
	removed := make([]string, 0, len(remove))
	for id := range remove {
		result, err := transaction.ExecContext(ctx, `DELETE FROM gatehouse_project_record_values WHERE workspace = `+placeholder(1)+` AND project = `+placeholder(2)+` AND schema = `+placeholder(3)+` AND record = `+placeholder(4)+` AND id = `+placeholder(5), record.Schema.Project.Workspace.Id, record.Schema.Project.Id, record.Schema.Id, record.Id, id)
		if err != nil {
			return fmt.Errorf("delete project record value: %w", err), ProjectRecordValuesMutationResult{}
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete project record value: %w", err), ProjectRecordValuesMutationResult{}
		}
		if changed != 1 {
			return nil, ProjectRecordValuesMutationResult{}
		}
		removed = append(removed, id)
	}
	created, err := store.projectRecordValuesCreate(ctx, transaction, record, author, creates)
	if err != nil {
		return err, ProjectRecordValuesMutationResult{}
	}
	if err, _ := store.ActivityEventAppend(ctx, transaction, model.ActivityEvent{Event: "project_record.update", ResourceKind: ActivityResourceKindProjectRecord, ResourceProjectRecord: &record.Id}, []string{ActivityTopicProjectRecord(record)}); err != nil {
		return fmt.Errorf("append project record update activity: %w", err), ProjectRecordValuesMutationResult{}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit project record value mutation: %w", err), ProjectRecordValuesMutationResult{}
	}
	return nil, ProjectRecordValuesMutationResult{Created: created, Removed: removed}
}
