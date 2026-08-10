package migrations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/identity"
	"gatehouse/keychain"
	"gatehouse/model"
)

func TestMigrateAppliesVersionedMigrationsOnceInIndexOrder(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{
			{
				Index:       2,
				Description: "record_second_version",
				Builder: staticMigrationBuilder(`
					INSERT INTO gatehouse_test_events (entry) VALUES ('versioned-2');
				`),
			},
			{
				Index:       1,
				Description: "create_events",
				Builder: staticMigrationBuilder(`
					CREATE TABLE gatehouse_test_events (
						entry TEXT NOT NULL
					) STRICT;
					INSERT INTO gatehouse_test_events (entry) VALUES ('versioned-1');
				`),
			},
		},
	}

	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}

	if got, want := eventEntries(t, database), []string{"versioned-1", "versioned-2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event entries = %#v, want %#v", got, want)
	}
	if got, want := historyIndexes(t, database, migrationTypeVersioned), []int64{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("versioned migration indexes = %#v, want %#v", got, want)
	}
	var appliedAt string
	if err := database.QueryRow(`
		SELECT applied_at
		FROM gatehouse_schema_migrations
		WHERE migration_type = ? AND migration_index = ?
	`, migrationTypeVersioned, 1).Scan(&appliedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse("2006-01-02T15:04:05.000Z", appliedAt); err != nil {
		t.Fatalf("migration timestamp = %q: %v", appliedAt, err)
	}
	if _, err := database.Exec(`
		INSERT INTO gatehouse_schema_migrations (
			migration_type, migration_index, description, checksum
		) VALUES ('baseline', 1, 'missing timestamp', zeroblob(32))
	`); err == nil {
		t.Fatal("migration history accepted a missing timestamp")
	}
}

func TestMigratePreparesKeychainsFirstWithoutReplacement(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	state := config.State{Keychains: []config.Keychain{{
		ID:      "default",
		Sources: []config.KeychainPassphraseSource{"env:DEFAULT"},
	}}}
	t.Setenv("DEFAULT", "passphrase")
	err, keyring := keychain.NewKeyring(store, state.Keychains, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Close()
	err, set := Build(configuration, state, keyring)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), store, set); err != nil {
		t.Fatal(err)
	}

	var firstRepeatable int64
	if err := store.QueryRow(`
		SELECT migration_index
		FROM gatehouse_schema_migrations
		WHERE migration_type = 'repeatable'
		ORDER BY installed_rank
		LIMIT 1
	`).Scan(&firstRepeatable); err != nil {
		t.Fatal(err)
	}
	if firstRepeatable != 1 {
		t.Fatalf("first repeatable migration = %d, want 1", firstRepeatable)
	}

	reference := model.KeychainRef{Id: "default", Version: 1}
	err, stored := store.KeychainsGet(context.Background(), []model.KeychainRef{reference})
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored keychains = %#v, want one keychain", stored)
	}
	var firstChecksum []byte
	if err := store.QueryRow(`
		SELECT checksum
		FROM gatehouse_schema_migrations
		WHERE migration_type = 'repeatable' AND migration_index = 1
	`).Scan(&firstChecksum); err != nil {
		t.Fatal(err)
	}

	err, repeatKeyring := keychain.NewKeyring(store, state.Keychains, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer repeatKeyring.Close()
	err, repeat := Build(configuration, state, repeatKeyring)
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), store, repeat); err != nil {
		t.Fatal(err)
	}
	err, repeated := store.KeychainsGet(context.Background(), []model.KeychainRef{reference})
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated) != 1 {
		t.Fatalf("stored keychains after repeat = %#v, want one keychain", repeated)
	}
	if repeated[0].KekKdf != stored[0].KekKdf || repeated[0].Key != stored[0].Key || repeated[0].Enabled != stored[0].Enabled {
		t.Fatalf("stored keychain after repeat = %#v, want %#v", repeated[0], stored[0])
	}
	var secondChecksum []byte
	if err := store.QueryRow(`
		SELECT checksum
		FROM gatehouse_schema_migrations
		WHERE migration_type = 'repeatable' AND migration_index = 1
	`).Scan(&secondChecksum); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(secondChecksum, firstChecksum) {
		t.Fatal("keychain migration checksum did not change after generating new candidates")
	}

	err, keys := repeatKeyring.Get(context.Background(), []model.KeychainRef{reference})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(keys[reference]); got != 16 {
		t.Fatalf("prepared DEK length = %d, want 16", got)
	}
}

func TestMigrateReconcilesIdentitiesByRevision(t *testing.T) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(context.Background(), configuration)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	algorithm := "pbkdf2-hmac-sha256-v1"
	state := config.State{Principals: []config.Principal{{
		ID: "alice", Enabled: true, Identities: []config.Identity{{
			ID: "gatehouse:alice", Revision: 1, Enabled: true,
			Verifiers: []config.Verifier{{Algorithm: &algorithm, Sources: []config.PasswordSource{"env:IDENTITY_PASSWORD"}}},
		}},
	}}}
	run := func() {
		err, keyring := keychain.NewKeyring(store, nil, keychain.NewPassphraseSourceResolver())
		if err != nil {
			t.Fatal(err)
		}
		defer keyring.Close()
		err, set := Build(configuration, state, keyring)
		if err != nil {
			t.Fatal(err)
		}
		if err := Run(context.Background(), store, set); err != nil {
			t.Fatal(err)
		}
	}
	identityID := "gatehouse:alice"
	t.Setenv("IDENTITY_PASSWORD", "first password")
	run()
	err, first := store.ActiveIdentityGet(context.Background(), identityID)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || len(first.Verifiers) != 1 {
		t.Fatalf("active identity = %#v, want one verifier", first)
	}
	firstVerifier, ok := first.Verifiers[0].(string)
	if !ok {
		t.Fatalf("identity verifier = %#v, want string", first.Verifiers[0])
	}
	if err, valid := identity.VerifyPassword(firstVerifier, []byte("first password")); err != nil || !valid {
		t.Fatalf("first verifier validation = (%v, %t), want (nil, true)", err, valid)
	}

	t.Setenv("IDENTITY_PASSWORD", "")
	run()
	err, unchanged := store.ActiveIdentityGet(context.Background(), identityID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged == nil || unchanged.Verifiers[0] != firstVerifier {
		t.Fatalf("identity at unchanged revision = %#v, want verifier %q", unchanged, firstVerifier)
	}

	state.Principals[0].Identities[0].Revision = 2
	t.Setenv("IDENTITY_PASSWORD", "second password")
	run()
	err, updated := store.ActiveIdentityGet(context.Background(), identityID)
	if err != nil {
		t.Fatal(err)
	}
	updatedVerifier, ok := updated.Verifiers[0].(string)
	if !ok {
		t.Fatalf("updated identity verifier = %#v, want string", updated.Verifiers[0])
	}
	if err, valid := identity.VerifyPassword(updatedVerifier, []byte("second password")); err != nil || !valid {
		t.Fatalf("updated verifier validation = (%v, %t), want (nil, true)", err, valid)
	}
	if err, valid := identity.VerifyPassword(updatedVerifier, []byte("first password")); err != nil || valid {
		t.Fatalf("updated verifier validation with prior password = (%v, %t), want (nil, false)", err, valid)
	}

	if err := store.IdentitiesUpsertRevisions(context.Background(), []model.Identity{{
		Id: identityID, Principal: model.PrincipalRef{Id: "alice"}, Revision: 3, Verifiers: []interface{}{updatedVerifier}, Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IDENTITY_PASSWORD", "")
	run()
	var revision int
	if err := store.QueryRow(`SELECT revision FROM gatehouse_identities WHERE id = ?`, identityID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 3 {
		t.Fatalf("stored identity revision = %d, want 3", revision)
	}
}

func TestMigrateRejectsChangedVersionedMigration(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "create_events",
			Builder:     staticMigrationBuilder(`CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;`),
		}},
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}

	registry.Versioned[0].Builder = staticMigrationBuilder(`CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;
-- changed`)
	err := migrateSQLite(context.Background(), database, registry)
	if err == nil || !strings.Contains(err.Error(), "different checksum") {
		t.Fatalf("migrate() error = %v, want changed checksum error", err)
	}
}

func TestMigrateRejectsOutOfOrderVersionedMigration(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{{
			Index:       2,
			Description: "create_events",
			Builder:     staticMigrationBuilder(`CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;`),
		}},
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}

	registry.Versioned = append(registry.Versioned, VersionedMigration{
		Index:       1,
		Description: "create_legacy_events",
		Builder:     staticMigrationBuilder(`CREATE TABLE gatehouse_test_legacy_events (entry TEXT NOT NULL) STRICT;`),
	})
	err := migrateSQLite(context.Background(), database, registry)
	if err == nil || !strings.Contains(err.Error(), "would run out of order") {
		t.Fatalf("migrate() error = %v, want out-of-order error", err)
	}
}

func TestMigrateRejectsPersistedOutOfOrderVersionedHistory(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{
			{Index: 1, Description: "first", Builder: staticMigrationBuilder(`CREATE TABLE gatehouse_test_first (value TEXT) STRICT;`)},
			{Index: 2, Description: "second", Builder: staticMigrationBuilder(`CREATE TABLE gatehouse_test_second (value TEXT) STRICT;`)},
		},
	}
	if err := migrateSQLite(context.Background(), database, Registry{Init: testSQLiteRegistry(t).Init}); err != nil {
		t.Fatal(err)
	}
	for _, index := range []int{2, 1} {
		migration := registry.Versioned[index-1]
		checksum := sha256.Sum256([]byte(versionedMigrationSource(t, migration)))
		if _, err := database.Exec(`
			INSERT INTO gatehouse_schema_migrations (
				migration_type, migration_index, description, checksum, applied_at
			) VALUES (?, ?, ?, ?, ?)
		`, migrationTypeVersioned, migration.Index, migration.Description, checksum[:], "2026-01-01T00:00:00.000Z"); err != nil {
			t.Fatal(err)
		}
	}

	err := migrateSQLite(context.Background(), database, registry)
	if err == nil || !strings.Contains(err.Error(), "was applied out of order") {
		t.Fatalf("migrate() error = %v, want persisted out-of-order error", err)
	}
}

func TestMigrateAppliesOnlyChangedRepeatablesInIndexOrder(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registryFor := func(values repeatableValues) Registry {
		return Registry{
			Init: testSQLiteRegistry(t).Init,
			Versioned: []VersionedMigration{{
				Index:       1,
				Description: "create_events",
				Builder:     staticMigrationBuilder(`CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;`),
			}},
			Repeatable: []RepeatableMigration{
				{Index: 2, Description: "record_second_value", Builder: templateMigrationBuilder(`INSERT INTO gatehouse_test_events (entry) VALUES ('second:' || {{ sqlLiteral .Second }});`, values)},
				{Index: 1, Description: "record_first_value", Builder: templateMigrationBuilder(`INSERT INTO gatehouse_test_events (entry) VALUES ('first:' || {{ sqlLiteral .First }});`, values)},
			},
		}
	}

	registry := registryFor(repeatableValues{First: "one", Second: "two"})
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}
	registry = registryFor(repeatableValues{First: "o'hare", Second: "two"})
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}
	registry = registryFor(repeatableValues{First: "three", Second: "four"})
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}

	if got, want := eventEntries(t, database), []string{
		"first:one",
		"second:two",
		"first:o'hare",
		"first:three",
		"second:four",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event entries = %#v, want %#v", got, want)
	}
	if got, want := historyIndexes(t, database, migrationTypeRepeatable), []int64{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("repeatable migration indexes = %#v, want %#v", got, want)
	}
	if got, want := historyChecksum(t, database, migrationTypeRepeatable, 1), sha256.Sum256([]byte(migrationSource(t, registry.Repeatable[1]))); got != want {
		t.Fatalf("repeatable migration 1 checksum = %x, want %x", got, want)
	}
	if got, want := historyChecksum(t, database, migrationTypeRepeatable, 2), sha256.Sum256([]byte(migrationSource(t, registry.Repeatable[0]))); got != want {
		t.Fatalf("repeatable migration 2 checksum = %x, want %x", got, want)
	}
}

func TestMigrateRejectsEmptyBuiltMigration(t *testing.T) {
	database := openMigrationTestDatabase(t)
	err := migrateSQLite(context.Background(), database, Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "empty_source",
			Builder:     staticMigrationBuilder(""),
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "has no SQL") {
		t.Fatalf("migrate() error = %v, want empty source error", err)
	}
}

func TestMigrateRejectsMissingRepeatableTemplateValues(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Repeatable: []RepeatableMigration{{
			Index:       1,
			Description: "missing_value",
			Builder:     templateMigrationBuilder(`SELECT {{ sqlLiteral .Missing }};`, struct{}{}),
		}},
	}
	err := migrateSQLite(context.Background(), database, registry)
	if err == nil || !strings.Contains(err.Error(), "can't evaluate field Missing") {
		t.Fatalf("migrate() error = %v, want missing template value error", err)
	}
}

func TestMigrateRollsBackFailedMigration(t *testing.T) {
	database := openMigrationTestDatabase(t)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "invalid_sql",
			Builder:     staticMigrationBuilder(`CREATE TABL gatehouse_test_events (entry TEXT NOT NULL) STRICT;`),
		}},
	}
	if err := migrateSQLite(context.Background(), database, registry); err == nil {
		t.Fatal("migrate() succeeded for invalid SQL")
	}

	var tableCount int
	if err := database.QueryRow(`
		SELECT COUNT(*)
		FROM sqlite_master
		WHERE type = 'table' AND name = 'gatehouse_schema_migrations'
	`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 0 {
		t.Fatalf("migration history table count = %d, want 0", tableCount)
	}
}

func TestMigrateCoordinatesConcurrentSQLiteRunners(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gatehouse.db")
	first := openMigrationTestFileDatabase(t, path)
	second := openMigrationTestFileDatabase(t, path)
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{
			{
				Index:       1,
				Description: "create_events",
				Builder:     staticMigrationBuilder(`CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;`),
			},
			{
				Index:       2,
				Description: "record_versioned",
				Builder:     staticMigrationBuilder(`INSERT INTO gatehouse_test_events (entry) VALUES ('versioned');`),
			},
		},
		Repeatable: []RepeatableMigration{{
			Index:       1,
			Description: "record_repeatable",
			Builder:     staticMigrationBuilder(`INSERT INTO gatehouse_test_events (entry) VALUES ('repeatable');`),
		}},
	}

	start := make(chan struct{})
	errors := make(chan error, 2)
	for _, database := range []*sql.DB{first, second} {
		go func(database *sql.DB) {
			<-start
			errors <- migrateSQLite(context.Background(), database, registry)
		}(database)
	}
	close(start)
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}

	if got, want := eventEntries(t, first), []string{"versioned", "repeatable"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("event entries = %#v, want %#v", got, want)
	}
	if got, want := historyIndexes(t, first, migrationTypeVersioned), []int64{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("versioned migration indexes = %#v, want %#v", got, want)
	}
	if got, want := historyIndexes(t, first, migrationTypeRepeatable), []int64{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("repeatable migration indexes = %#v, want %#v", got, want)
	}
}

type repeatableValues struct {
	First  string
	Second string
}

func testSQLiteRegistry(t *testing.T) Registry {
	t.Helper()
	err, keyring := keychain.NewKeyring(nil, nil, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Close()
	err, registry := sqliteMigrations(config.State{}, keyring)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestMigrateBuildsRepeatablesOneAtATime(t *testing.T) {
	database := openMigrationTestDatabase(t)
	laterBuilt := false
	registry := Registry{
		Init: testSQLiteRegistry(t).Init,
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "create_events",
			Builder:     staticMigrationBuilder(`CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;`),
		}},
		Repeatable: []RepeatableMigration{
			{Index: 1, Description: "fail_before_later_builder", Builder: func(context.Context, *MigrationSession) (error, string) {
				return fmt.Errorf("expected builder failure"), ""
			}},
			{Index: 2, Description: "must_not_be_built", Builder: func(context.Context, *MigrationSession) (error, string) {
				laterBuilt = true
				return nil, `SELECT 1;`
			}},
		},
	}
	err := migrateSQLite(context.Background(), database, registry)
	if err == nil || !strings.Contains(err.Error(), "expected builder failure") {
		t.Fatalf("migrate() error = %v, want builder failure", err)
	}
	if laterBuilt {
		t.Fatal("later repeatable builder ran after an earlier builder failed")
	}
}

func TestMigrateBuildsEachMigrationOnce(t *testing.T) {
	database := openMigrationTestDatabase(t)
	initBuilds := 0
	versionedBuilds := 0
	repeatableBuilds := 0
	init := testSQLiteRegistry(t).Init
	initBuilder := init.Builder
	init.Builder = func(ctx context.Context, session *MigrationSession) (error, string) {
		initBuilds++
		return initBuilder(ctx, session)
	}
	registry := Registry{
		Init: init,
		Versioned: []VersionedMigration{{
			Index:       1,
			Description: "create_events",
			Builder: func(context.Context, *MigrationSession) (error, string) {
				versionedBuilds++
				return nil, `CREATE TABLE gatehouse_test_events (entry TEXT NOT NULL) STRICT;`
			},
		}},
		Repeatable: []RepeatableMigration{{
			Index:       1,
			Description: "record_event",
			Builder: func(context.Context, *MigrationSession) (error, string) {
				repeatableBuilds++
				return nil, `INSERT INTO gatehouse_test_events (entry) VALUES ('repeatable');`
			},
		}},
	}
	if err := migrateSQLite(context.Background(), database, registry); err != nil {
		t.Fatal(err)
	}
	if initBuilds != 1 {
		t.Fatalf("init builder calls = %d, want 1", initBuilds)
	}
	if versionedBuilds != 1 {
		t.Fatalf("versioned builder calls = %d, want 1", versionedBuilds)
	}
	if repeatableBuilds != 1 {
		t.Fatalf("repeatable builder calls = %d, want 1", repeatableBuilds)
	}
}

func openMigrationTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return database
}

func openMigrationTestFileDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	return database
}

func eventEntries(t *testing.T, database queryer) []string {
	t.Helper()
	rows, err := database.Query(`SELECT entry FROM gatehouse_test_events ORDER BY rowid`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var entries []string
	for rows.Next() {
		var entry string
		if err := rows.Scan(&entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return entries
}

func historyChecksum(t *testing.T, database queryer, migrationType string, index int64) [sha256.Size]byte {
	t.Helper()
	rows, err := database.Query(`
		SELECT checksum
		FROM gatehouse_schema_migrations
		WHERE migration_type = ? AND migration_index = ?
	`, migrationType, index)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("migration checksum not found")
	}
	var checksum []byte
	if err := rows.Scan(&checksum); err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Fatal("multiple migration checksums found")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(checksum) != sha256.Size {
		t.Fatalf("migration checksum length = %d, want %d", len(checksum), sha256.Size)
	}
	var result [sha256.Size]byte
	copy(result[:], checksum)
	return result
}

func migrationSource(t *testing.T, migration RepeatableMigration) string {
	t.Helper()
	err, source := migration.Builder(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func versionedMigrationSource(t *testing.T, migration VersionedMigration) string {
	t.Helper()
	err, source := migration.Builder(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func historyIndexes(t *testing.T, database queryer, migrationType string) []int64 {
	t.Helper()
	rows, err := database.Query(`
		SELECT migration_index
		FROM gatehouse_schema_migrations
		WHERE migration_type = ?
		ORDER BY installed_rank
	`, migrationType)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var indexes []int64
	for rows.Next() {
		var index int64
		if err := rows.Scan(&index); err != nil {
			t.Fatal(err)
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return indexes
}

type queryer interface {
	Query(string, ...any) (*sql.Rows, error)
}
