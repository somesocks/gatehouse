package migrations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

type MigrationBuilder func(context.Context, *MigrationSession) (error, string)

type MigrationSession struct {
	queryer interface {
		QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	}
}

func (session *MigrationSession) QueryContext(ctx context.Context, query string, arguments ...any) (*sql.Rows, error) {
	return session.queryer.QueryContext(ctx, query, arguments...)
}

func (session *MigrationSession) RenderTemplate(source string, values any) (error, string) {
	return renderTemplate("migration", source, values)
}

type Registry struct {
	Init       InitMigration
	Baseline   []BaselineMigration
	Versioned  []VersionedMigration
	Repeatable []RepeatableMigration
}

type InitMigration struct {
	Builder MigrationBuilder
}

type MigrationOptions struct {
	SQLite   SQLiteMigrationOptions
	Postgres PostgresMigrationOptions
}

type SQLiteMigrationOptions struct {
	DisableForeignKeys bool
}

type PostgresMigrationOptions struct{}

type BaselineMigration struct {
	Index       int64
	Description string
	Builder     MigrationBuilder
	Options     MigrationOptions
}

type VersionedMigration struct {
	Index       int64
	Description string
	Builder     MigrationBuilder
	Options     MigrationOptions
}

type RepeatableMigration struct {
	Index       int64
	Description string
	Builder     MigrationBuilder
	Options     MigrationOptions
}

const (
	migrationTypeBaseline       = "baseline"
	migrationTypeVersioned      = "versioned"
	migrationTypeRepeatable     = "repeatable"
	compactRepeatableHistorySQL = `
		DELETE FROM gatehouse_schema_migrations AS older
		WHERE older.migration_type = 'repeatable'
			AND EXISTS (
				SELECT 1
				FROM gatehouse_schema_migrations AS newer
				WHERE newer.migration_type = 'repeatable'
					AND newer.migration_index = older.migration_index
					AND newer.installed_rank > older.installed_rank
			);
	`
)

type appliedMigration struct {
	installedRank int64
	migrationType string
	index         int64
	description   string
	checksum      [sha256.Size]byte
}

type resolvedMigration struct {
	migrationType string
	index         int64
	description   string
	source        string
	options       MigrationOptions
}

type migrationCursor struct {
	init       string
	versioned  int
	repeatable int
}

func staticMigrationBuilder(source string) MigrationBuilder {
	return func(context.Context, *MigrationSession) (error, string) {
		return nil, source
	}
}

func templateMigrationBuilder(source string, values any) MigrationBuilder {
	return func(context.Context, *MigrationSession) (error, string) {
		return renderTemplate("migration", source, values)
	}
}

func validateRegistry(registry Registry) error {
	if registry.Init.Builder == nil {
		return fmt.Errorf("migration registry has no init builder")
	}

	baselineIndexes := make(map[int64]struct{}, len(registry.Baseline))
	for _, migration := range registry.Baseline {
		if err := validateMigration(migration.Index, migration.Description, "baseline"); err != nil {
			return err
		}
		if migration.Builder == nil {
			return fmt.Errorf("baseline migration %d (%s) has no builder", migration.Index, migration.Description)
		}
		if _, exists := baselineIndexes[migration.Index]; exists {
			return fmt.Errorf("duplicate baseline migration index %d", migration.Index)
		}
		baselineIndexes[migration.Index] = struct{}{}
	}

	versionedIndexes := make(map[int64]struct{}, len(registry.Versioned))
	for _, migration := range registry.Versioned {
		if err := validateMigration(migration.Index, migration.Description, "versioned"); err != nil {
			return err
		}
		if migration.Builder == nil {
			return fmt.Errorf("versioned migration %d (%s) has no builder", migration.Index, migration.Description)
		}
		if _, exists := versionedIndexes[migration.Index]; exists {
			return fmt.Errorf("duplicate versioned migration index %d", migration.Index)
		}
		versionedIndexes[migration.Index] = struct{}{}
	}

	repeatableIndexes := make(map[int64]struct{}, len(registry.Repeatable))
	for _, migration := range registry.Repeatable {
		if err := validateMigration(migration.Index, migration.Description, "repeatable"); err != nil {
			return err
		}
		if migration.Builder == nil {
			return fmt.Errorf("repeatable migration %d (%s) has no builder", migration.Index, migration.Description)
		}
		if _, exists := repeatableIndexes[migration.Index]; exists {
			return fmt.Errorf("duplicate repeatable migration index %d", migration.Index)
		}
		repeatableIndexes[migration.Index] = struct{}{}
	}
	return nil
}

func validateMigration(index int64, description, kind string) error {
	if index <= 0 {
		return fmt.Errorf("%s migration index must be positive", kind)
	}
	if strings.TrimSpace(description) == "" {
		return fmt.Errorf("%s migration %d has no description", kind, index)
	}
	return nil
}

func validateHistory(history []appliedMigration, registry Registry) error {
	baseline := make(map[int64]BaselineMigration, len(registry.Baseline))
	for _, migration := range registry.Baseline {
		baseline[migration.Index] = migration
	}
	versioned := make(map[int64]VersionedMigration, len(registry.Versioned))
	for _, migration := range registry.Versioned {
		versioned[migration.Index] = migration
	}
	repeatable := make(map[int64]RepeatableMigration, len(registry.Repeatable))
	for _, migration := range registry.Repeatable {
		repeatable[migration.Index] = migration
	}
	var baselineIndex int64
	var highestVersionedIndex int64
	for position, applied := range history {
		switch applied.migrationType {
		case migrationTypeBaseline:
			if position != 0 {
				return fmt.Errorf("baseline migration %d (%s) was not the first migration", applied.index, applied.description)
			}
			if baselineIndex != 0 {
				return fmt.Errorf("multiple baseline migrations were applied")
			}
			definition, ok := baseline[applied.index]
			if !ok {
				return fmt.Errorf("applied baseline migration %d (%s) is not defined", applied.index, applied.description)
			}
			if applied.description != definition.Description {
				return fmt.Errorf("applied baseline migration %d has a different description", applied.index)
			}
			baselineIndex = applied.index
		case migrationTypeVersioned:
			if baselineIndex != 0 && applied.index <= baselineIndex {
				return fmt.Errorf("versioned migration %d (%s) was applied at or below baseline %d", applied.index, applied.description, baselineIndex)
			}
			if applied.index < highestVersionedIndex {
				return fmt.Errorf("versioned migration %d (%s) was applied out of order after migration %d", applied.index, applied.description, highestVersionedIndex)
			}
			definition, ok := versioned[applied.index]
			if !ok {
				return fmt.Errorf("applied versioned migration %d (%s) is not defined", applied.index, applied.description)
			}
			if applied.description != definition.Description {
				return fmt.Errorf("applied versioned migration %d has a different description", applied.index)
			}
			if applied.index > highestVersionedIndex {
				highestVersionedIndex = applied.index
			}
		case migrationTypeRepeatable:
			if _, ok := repeatable[applied.index]; !ok {
				return fmt.Errorf("applied repeatable migration %d (%s) is not defined", applied.index, applied.description)
			}
		default:
			return fmt.Errorf("migration history rank %d has an unsupported type %q", applied.installedRank, applied.migrationType)
		}
	}

	for _, migration := range registry.Versioned {
		if migration.Index > baselineIndex && migration.Index < highestVersionedIndex && !hasMigration(history, migrationTypeVersioned, migration.Index) {
			return fmt.Errorf("versioned migration %d (%s) would run out of order after migration %d", migration.Index, migration.Description, highestVersionedIndex)
		}
	}
	return nil
}

func nextMigration(ctx context.Context, session *MigrationSession, history []appliedMigration, registry Registry, cursor migrationCursor) (error, resolvedMigration, migrationCursor, bool) {
	baselineIndex := int64(0)
	if applied, ok := latestBaseline(history); ok {
		baselineIndex = applied.index
		migration, ok := baselineByIndex(registry.Baseline, applied.index)
		if !ok {
			return fmt.Errorf("applied baseline migration %d (%s) is not defined", applied.index, applied.description), resolvedMigration{}, cursor, false
		}
		err, source := buildMigration(ctx, session, migrationTypeBaseline, migration.Index, migration.Description, migration.Builder)
		if err != nil {
			return err, resolvedMigration{}, cursor, false
		}
		if applied.checksum != sha256.Sum256([]byte(source)) {
			return fmt.Errorf("applied baseline migration %d (%s) has a different checksum", migration.Index, migration.Description), resolvedMigration{}, cursor, false
		}
	} else if len(history) == 0 {
		baselines := sortedBaseline(registry.Baseline)
		if len(baselines) > 0 {
			migration := baselines[len(baselines)-1]
			err, source := buildMigration(ctx, session, migrationTypeBaseline, migration.Index, migration.Description, migration.Builder)
			if err != nil {
				return err, resolvedMigration{}, cursor, false
			}
			return nil, resolvedMigration{migrationType: migrationTypeBaseline, index: migration.Index, description: migration.Description, source: source, options: migration.Options}, cursor, true
		}
	}

	versioned := sortedVersioned(registry.Versioned)
	for index := cursor.versioned; index < len(versioned); index++ {
		migration := versioned[index]
		if migration.Index <= baselineIndex {
			cursor.versioned = index + 1
			continue
		}
		err, source := buildMigration(ctx, session, migrationTypeVersioned, migration.Index, migration.Description, migration.Builder)
		if err != nil {
			return err, resolvedMigration{}, cursor, false
		}
		if applied, ok := latestMigration(history, migrationTypeVersioned, migration.Index); ok {
			if applied.checksum != sha256.Sum256([]byte(source)) {
				return fmt.Errorf("applied versioned migration %d (%s) has a different checksum", migration.Index, migration.Description), resolvedMigration{}, cursor, false
			}
			cursor.versioned = index + 1
			continue
		}
		cursor.versioned = index + 1
		return nil, resolvedMigration{migrationType: migrationTypeVersioned, index: migration.Index, description: migration.Description, source: source, options: migration.Options}, cursor, true
	}

	repeatable := sortedRepeatable(registry.Repeatable)
	for index := cursor.repeatable; index < len(repeatable); index++ {
		migration := repeatable[index]
		err, source := buildMigration(ctx, session, migrationTypeRepeatable, migration.Index, migration.Description, migration.Builder)
		if err != nil {
			return err, resolvedMigration{}, cursor, false
		}
		checksum := sha256.Sum256([]byte(source))
		if latest, ok := latestMigration(history, migrationTypeRepeatable, migration.Index); !ok || latest.checksum != checksum {
			cursor.repeatable = index + 1
			return nil, resolvedMigration{migrationType: migrationTypeRepeatable, index: migration.Index, description: migration.Description, source: source, options: migration.Options}, cursor, true
		}
		cursor.repeatable = index + 1
	}

	return nil, resolvedMigration{}, cursor, false
}

func migrationRequired(history []appliedMigration, migration resolvedMigration) (error, bool) {
	checksum := sha256.Sum256([]byte(migration.source))
	latest, exists := latestMigration(history, migration.migrationType, migration.index)
	switch migration.migrationType {
	case migrationTypeBaseline:
		// A concurrent runner may have committed history after this baseline was selected.
		if !exists && len(history) != 0 {
			return nil, false
		}
		if !exists {
			return nil, true
		}
		if latest.checksum != checksum {
			return fmt.Errorf("applied baseline migration %d (%s) has a different checksum", migration.index, migration.description), false
		}
		return nil, false
	case migrationTypeVersioned:
		if !exists {
			return nil, true
		}
		if latest.checksum != checksum {
			return fmt.Errorf("applied versioned migration %d (%s) has a different checksum", migration.index, migration.description), false
		}
		return nil, false
	case migrationTypeRepeatable:
		return nil, !exists || latest.checksum != checksum
	default:
		return fmt.Errorf("unsupported migration type %q", migration.migrationType), false
	}
}

func buildMigration(ctx context.Context, session *MigrationSession, migrationType string, index int64, description string, builder MigrationBuilder) (error, string) {
	err, source := builder(ctx, session)
	if err != nil {
		return fmt.Errorf("build %s migration %d (%s): %w", migrationType, index, description, err), ""
	}
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("%s migration %d (%s) has no SQL", migrationType, index, description), ""
	}
	return nil, source
}

func hasMigration(history []appliedMigration, migrationType string, index int64) bool {
	_, ok := latestMigration(history, migrationType, index)
	return ok
}

func latestMigration(history []appliedMigration, migrationType string, index int64) (appliedMigration, bool) {
	for position := len(history) - 1; position >= 0; position-- {
		migration := history[position]
		if migration.migrationType == migrationType && migration.index == index {
			return migration, true
		}
	}
	return appliedMigration{}, false
}

func latestBaseline(history []appliedMigration) (appliedMigration, bool) {
	for position := len(history) - 1; position >= 0; position-- {
		if history[position].migrationType == migrationTypeBaseline {
			return history[position], true
		}
	}
	return appliedMigration{}, false
}

func baselineByIndex(migrations []BaselineMigration, index int64) (BaselineMigration, bool) {
	for _, migration := range migrations {
		if migration.Index == index {
			return migration, true
		}
	}
	return BaselineMigration{}, false
}

func sortedBaseline(migrations []BaselineMigration) []BaselineMigration {
	sorted := append([]BaselineMigration(nil), migrations...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].Index < sorted[right].Index
	})
	return sorted
}

func sortedVersioned(migrations []VersionedMigration) []VersionedMigration {
	sorted := append([]VersionedMigration(nil), migrations...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].Index < sorted[right].Index
	})
	return sorted
}

func sortedRepeatable(migrations []RepeatableMigration) []RepeatableMigration {
	sorted := append([]RepeatableMigration(nil), migrations...)
	sort.Slice(sorted, func(left, right int) bool {
		return sorted[left].Index < sorted[right].Index
	})
	return sorted
}

func renderTemplate(name, source string, values any) (error, string) {
	template, err := template.New(name).
		Option("missingkey=error").
		Funcs(template.FuncMap{
			"sqlLiteral": func(value any) (string, error) {
				err, literal := sqlLiteral(value)
				return literal, err
			},
			"sqlIdentifier": func(value string) (string, error) {
				err, identifier := sqlIdentifier(value)
				return identifier, err
			},
			"sqlBool": sqlBool,
			"sqlOptionalString": func(value *string) (string, error) {
				if value == nil {
					return "NULL", nil
				}
				err, literal := sqlLiteral(*value)
				return literal, err
			},
		}).
		Parse(source)
	if err != nil {
		return err, ""
	}

	var rendered bytes.Buffer
	if err := template.Execute(&rendered, values); err != nil {
		return err, ""
	}
	return nil, rendered.String()
}

func sqlLiteral(value any) (error, string) {
	switch value := value.(type) {
	case nil:
		return nil, "NULL"
	case string:
		return nil, "'" + strings.ReplaceAll(value, "'", "''") + "'"
	case bool:
		if value {
			return nil, "1"
		}
		return nil, "0"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return nil, fmt.Sprint(value)
	default:
		return fmt.Errorf("unsupported SQL literal type %T", value), ""
	}
}

func sqlIdentifier(value string) (error, string) {
	if value == "" || strings.ContainsRune(value, 0) || strings.Contains(value, ".") {
		return fmt.Errorf("invalid SQL identifier %q", value), ""
	}
	return nil, `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func sqlBool(value bool) string {
	if value {
		return "TRUE"
	}
	return "FALSE"
}
