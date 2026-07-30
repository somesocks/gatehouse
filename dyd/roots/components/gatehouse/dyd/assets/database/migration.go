package database

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"text/template"
)

type Registry struct {
	Init       InitMigration
	Versioned  []VersionedMigration
	Repeatable []RepeatableMigration
}

type InitMigration struct {
	SQL string
}

type VersionedMigration struct {
	Index       int64
	Description string
	SQL         string
}

type RepeatableMigration struct {
	Index       int64
	Description string
	Template    string
}

const (
	migrationTypeVersioned  = "versioned"
	migrationTypeRepeatable = "repeatable"
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
}

func validateRegistry(registry Registry) error {
	if strings.TrimSpace(registry.Init.SQL) == "" {
		return fmt.Errorf("migration registry has no init SQL")
	}
	if strings.Contains(registry.Init.SQL, "{{") || strings.Contains(registry.Init.SQL, "}}") {
		return fmt.Errorf("init migration must not contain template actions")
	}

	versionedIndexes := make(map[int64]struct{}, len(registry.Versioned))
	for _, migration := range registry.Versioned {
		if err := validateMigration(migration.Index, migration.Description, migration.SQL, "versioned"); err != nil {
			return err
		}
		if strings.Contains(migration.SQL, "{{") || strings.Contains(migration.SQL, "}}") {
			return fmt.Errorf("versioned migration %d (%s) must not contain template actions", migration.Index, migration.Description)
		}
		if _, exists := versionedIndexes[migration.Index]; exists {
			return fmt.Errorf("duplicate versioned migration index %d", migration.Index)
		}
		versionedIndexes[migration.Index] = struct{}{}
	}

	repeatableIndexes := make(map[int64]struct{}, len(registry.Repeatable))
	for _, migration := range registry.Repeatable {
		if err := validateMigration(migration.Index, migration.Description, migration.Template, "repeatable"); err != nil {
			return err
		}
		if _, exists := repeatableIndexes[migration.Index]; exists {
			return fmt.Errorf("duplicate repeatable migration index %d", migration.Index)
		}
		repeatableIndexes[migration.Index] = struct{}{}
	}
	return nil
}

func validateMigration(index int64, description, source, kind string) error {
	if index <= 0 {
		return fmt.Errorf("%s migration index must be positive", kind)
	}
	if strings.TrimSpace(description) == "" {
		return fmt.Errorf("%s migration %d has no description", kind, index)
	}
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("%s migration %d (%s) has no SQL", kind, index, description)
	}
	return nil
}

func validateHistory(history []appliedMigration, registry Registry) error {
	versioned := make(map[int64]VersionedMigration, len(registry.Versioned))
	for _, migration := range registry.Versioned {
		versioned[migration.Index] = migration
	}
	repeatable := make(map[int64]RepeatableMigration, len(registry.Repeatable))
	for _, migration := range registry.Repeatable {
		repeatable[migration.Index] = migration
	}

	var highestVersionedIndex int64
	for _, applied := range history {
		switch applied.migrationType {
		case migrationTypeVersioned:
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
			if applied.checksum != sha256.Sum256([]byte(definition.SQL)) {
				return fmt.Errorf("applied versioned migration %d (%s) has a different checksum", applied.index, applied.description)
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
		if migration.Index < highestVersionedIndex && !hasMigration(history, migrationTypeVersioned, migration.Index) {
			return fmt.Errorf("versioned migration %d (%s) would run out of order after migration %d", migration.Index, migration.Description, highestVersionedIndex)
		}
	}
	return nil
}

func nextMigration(history []appliedMigration, registry Registry, values any) (error, resolvedMigration, bool) {
	for _, migration := range sortedVersioned(registry.Versioned) {
		if !hasMigration(history, migrationTypeVersioned, migration.Index) {
			return nil, resolvedMigration{
				migrationType: migrationTypeVersioned,
				index:         migration.Index,
				description:   migration.Description,
				source:        migration.SQL,
			}, true
		}
	}

	for _, migration := range sortedRepeatable(registry.Repeatable) {
		err, rendered := renderTemplate(migration.Description, migration.Template, values)
		if err != nil {
			return fmt.Errorf("render repeatable migration %d (%s): %w", migration.Index, migration.Description, err), resolvedMigration{}, false
		}
		checksum := sha256.Sum256([]byte(rendered))
		if latest, ok := latestMigration(history, migrationTypeRepeatable, migration.Index); !ok || latest.checksum != checksum {
			return nil, resolvedMigration{
				migrationType: migrationTypeRepeatable,
				index:         migration.Index,
				description:   migration.Description,
				source:        rendered,
			}, true
		}
	}

	return nil, resolvedMigration{}, false
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
		return "1"
	}
	return "0"
}
