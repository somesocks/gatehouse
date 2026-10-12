package database

import (
	"gatehouse/config"
	"strings"
)

// Match the scalar and one-level reference projection used by record cards.
func recordScalarSearchSQL(kind config.DatabaseKind, alias string) string {
	datetime := "CAST(" + alias + ".value_datetime AS TEXT)"
	if kind == config.DatabaseKindPostgres {
		datetime = "to_char(" + alias + `.value_datetime AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')`
	}
	return "CASE " + alias + ".value_type WHEN 'text' THEN " + alias + ".value_text WHEN 'number' THEN CAST(" + alias + ".value_number AS TEXT) WHEN 'boolean' THEN CASE WHEN " + alias + ".value_boolean THEN 'Yes true' ELSE 'No false' END WHEN 'datetime' THEN " + datetime + " ELSE '' END"
}

func (store *Store) recordSearchSQL(name string, args []any) (string, []any) {
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(name)) + "%"
	p := keychainPlaceholder(store.kind)
	like := func(expression string) string {
		args = append(args, pattern)
		return "LOWER(COALESCE(" + expression + ", '')) LIKE LOWER(" + p(len(args)) + `) ESCAPE '\'`
	}
	id := like("gatehouse_project_records.id")
	scalar := like(recordScalarSearchSQL(store.kind, "search_values"))
	file := like("search_files.name")
	reference := like(recordScalarSearchSQL(store.kind, "target_values"))
	query := " AND (" + id + ` OR EXISTS (
		SELECT 1 FROM gatehouse_project_record_values AS search_values
		JOIN gatehouse_project_record_attributes AS search_fields ON search_fields.workspace = search_values.workspace AND search_fields.project = search_values.project AND search_fields.schema = search_values.schema AND search_fields.id = search_values.attribute
		WHERE search_values.workspace = gatehouse_project_records.workspace AND search_values.project = gatehouse_project_records.project AND search_values.schema = gatehouse_project_records.schema AND search_values.record = gatehouse_project_records.id AND search_fields.display <> 'none'
		AND (` + scalar + ` OR EXISTS (
			SELECT 1 FROM gatehouse_project_files AS search_files WHERE search_files.workspace = search_values.workspace AND search_files.project = search_values.project AND search_files.id = search_values.value_reference_file AND search_files.enabled = TRUE AND ` + file + `
		) OR EXISTS (
			SELECT 1 FROM gatehouse_project_record_values AS target_values
			JOIN gatehouse_project_record_attributes AS target_fields ON target_fields.workspace = target_values.workspace AND target_fields.project = target_values.project AND target_fields.schema = target_values.schema AND target_fields.id = target_values.attribute
			WHERE target_values.workspace = search_values.workspace AND target_values.project = search_values.project AND target_values.schema = search_fields.target_schema AND target_values.record = search_values.value_reference AND target_fields.display = 'primary' AND target_fields.type NOT IN ('record', 'file') AND ` + reference + `
		))
	))`
	return query, args
}
