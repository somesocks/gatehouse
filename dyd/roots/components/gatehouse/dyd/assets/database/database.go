package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gatehouse/config"
	"gatehouse/identity"
	"gatehouse/typed_id"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"modernc.org/sqlite"
)

type Store struct {
	*sql.DB
	kind         config.DatabaseKind
	databaseURL  string
	postgresPool *pgxpool.Pool
}

const sqliteBusyTimeout = 30_000

var sqliteFunctions struct {
	once sync.Once
	err  error
}

func Open(ctx context.Context, configuration config.DatabaseConfig) (error, *Store) {
	switch configuration.Kind {
	case config.DatabaseKindSQLite:
		if err := prepareSQLitePath(configuration.Path); err != nil {
			return err, nil
		}
		err, database := openSQLite(ctx, configuration.Path)
		if err != nil {
			return err, nil
		}
		return nil, &Store{DB: database, kind: config.DatabaseKindSQLite}
	case config.DatabaseKindEphemeral:
		err, database := openSQLite(ctx, ":memory:")
		if err != nil {
			return err, nil
		}
		return nil, &Store{DB: database, kind: config.DatabaseKindSQLite}
	case config.DatabaseKindPostgres:
		err, database, pool, url := openPostgres(ctx, configuration.URL)
		if err != nil {
			return err, nil
		}
		return nil, &Store{DB: database, kind: config.DatabaseKindPostgres, databaseURL: url, postgresPool: pool}
	default:
		return fmt.Errorf("unsupported database kind %q", configuration.Kind), nil
	}
}

// DatabaseURL returns the resolved database URL when the store uses PostgreSQL.
func (store *Store) DatabaseURL() string {
	return store.databaseURL
}

// PostgresPool returns the native PostgreSQL pool when the store uses PostgreSQL.
func (store *Store) PostgresPool() *pgxpool.Pool {
	return store.postgresPool
}

// Close closes all resources owned by the store.
func (store *Store) Close() error {
	err := store.DB.Close()
	if store.postgresPool != nil {
		store.postgresPool.Close()
	}
	return err
}

func prepareSQLitePath(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create SQLite database directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("create SQLite database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close SQLite database file: %w", err)
	}
	return nil
}

func openSQLite(ctx context.Context, source string) (error, *sql.DB) {
	if err := registerSQLiteFunctions(); err != nil {
		return err, nil
	}
	database, err := sql.Open("sqlite", source)
	if err != nil {
		return fmt.Errorf("open SQLite database: %w", err), nil
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return fmt.Errorf("ping SQLite database: %w", err), nil
	}
	if _, err := database.ExecContext(ctx, fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeout)); err != nil {
		database.Close()
		return fmt.Errorf("set SQLite busy timeout: %w", err), nil
	}
	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		database.Close()
		return fmt.Errorf("enable SQLite foreign keys: %w", err), nil
	}
	return nil, database
}

func openPostgres(ctx context.Context, reference string) (error, *sql.DB, *pgxpool.Pool, string) {
	name, found := strings.CutPrefix(reference, "env:")
	if !found || name == "" {
		return fmt.Errorf("PostgreSQL database URL must be an env:VARIABLE_NAME reference"), nil, nil, ""
	}
	url, found := os.LookupEnv(name)
	if !found || url == "" {
		return fmt.Errorf("PostgreSQL database URL environment variable %q is not set or empty", name), nil, nil, ""
	}

	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return fmt.Errorf("open PostgreSQL database: %w", err), nil, nil, ""
	}
	database := stdlib.OpenDBFromPool(pool)
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		pool.Close()
		return fmt.Errorf("ping PostgreSQL database: %w", err), nil, nil, ""
	}
	return nil, database, pool, url
}

func registerSQLiteFunctions() error {
	sqliteFunctions.once.Do(func() {
		sqliteFunctions.err = sqlite.RegisterScalarFunction("gh_id_new", -1, func(_ *sqlite.FunctionContext, arguments []driver.Value) (driver.Value, error) {
			if len(arguments) != 1 && len(arguments) != 2 {
				return nil, fmt.Errorf("gh_id_new expects a prefix and optional Unix-millisecond timestamp")
			}
			prefix, ok := arguments[0].(string)
			if !ok {
				return nil, fmt.Errorf("gh_id_new prefix must be text")
			}
			if len(arguments) == 1 {
				return typed_id.New(prefix)
			}
			milliseconds, ok := arguments[1].(int64)
			if !ok {
				return nil, fmt.Errorf("gh_id_new timestamp must be an integer number of Unix milliseconds")
			}
			return typed_id.NewAt(prefix, time.UnixMilli(milliseconds).UTC())
		})
		if sqliteFunctions.err != nil {
			return
		}
		sqliteFunctions.err = sqlite.RegisterDeterministicScalarFunction("gh_id_timestamp", 1, func(_ *sqlite.FunctionContext, arguments []driver.Value) (driver.Value, error) {
			value, ok := arguments[0].(string)
			if !ok {
				return nil, fmt.Errorf("gh_id_timestamp ID must be text")
			}
			prefix, _, found := strings.Cut(value, "_")
			if !found {
				return nil, fmt.Errorf("gh_id_timestamp ID has no prefix")
			}
			at, err := typed_id.Timestamp(prefix, value)
			if err != nil {
				return nil, err
			}
			return at.Format("2006-01-02T15:04:05.000Z"), nil
		})
		if sqliteFunctions.err != nil {
			return
		}
		sqliteFunctions.err = sqlite.RegisterScalarFunction("gh_identity_verifiers", 2, func(_ *sqlite.FunctionContext, arguments []driver.Value) (driver.Value, error) {
			identityID, ok := arguments[0].(string)
			if !ok {
				return nil, fmt.Errorf("gh_identity_verifiers identity ID must be text")
			}
			source, ok := arguments[1].(string)
			if !ok {
				return nil, fmt.Errorf("gh_identity_verifiers verifier sources must be text")
			}
			err, verifiers := identity.ResolveVerifiersJSON(identityID, source, identity.NewPasswordSourceResolver())
			if err != nil {
				return nil, err
			}
			return verifiers, nil
		})
	})
	return sqliteFunctions.err
}
