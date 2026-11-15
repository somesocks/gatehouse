package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gatehouse/config"
	"gatehouse/model"
)

// Shared hints are best-effort alignment suggestions, never recovery state.
// CacheScope is a caller-owned provider/model/cache-namespace key, not an agent
// binding. Profile must identify the compatible prompt/rendering configuration.
type ContextCacheHintScope struct {
	Session    model.SessionRef
	CacheScope string
	Algorithm  string
	Profile    string
}

const ContextCacheHintsPerScope = 32
const ContextCacheHintMaxBytes = 4096

func (scope ContextCacheHintScope) Validate() error {
	for _, value := range []string{scope.Session.Workspace.Id, scope.Session.Id, scope.CacheScope, scope.Algorithm, scope.Profile} {
		if !validContextCacheHintKey(value) {
			return fmt.Errorf("invalid context cache hint scope")
		}
	}
	return nil
}

func validContextCacheHintKey(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 1024 && utf8.ValidString(value)
}

type ContextCacheHint struct {
	Key        string
	Hint       json.RawMessage
	LastUsedAt int64 // Unix microseconds; ties are ordered by key, descending.
}

func contextCacheHintsListSQL(kind config.DatabaseKind) string {
	p := keychainPlaceholder(kind)
	return `SELECT hint_key,hint,last_used_at FROM gatehouse_context_cache_hints WHERE ` + contextCacheHintsWhereFor(kind, 1) + ` ORDER BY last_used_at DESC,hint_key DESC LIMIT ` + p(6)
}

func contextCacheHintsWhereFor(kind config.DatabaseKind, first int) string {
	p := keychainPlaceholder(kind)
	return "workspace=" + p(first) + " AND session=" + p(first+1) + " AND cache_scope=" + p(first+2) + " AND algorithm=" + p(first+3) + " AND profile=" + p(first+4)
}

func (scope ContextCacheHintScope) args() []any {
	return []any{scope.Session.Workspace.Id, scope.Session.Id, scope.CacheScope, scope.Algorithm, scope.Profile}
}

// ContextCacheHintsList reads at most 32 small objects from one indexed scope.
// It does not search JSON, join events, interpret algorithms, or checkpoint data.
func (store *Store) ContextCacheHintsList(ctx context.Context, scope ContextCacheHintScope) ([]ContextCacheHint, error) {
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := store.QueryContext(ctx, contextCacheHintsListSQL(store.kind), append(scope.args(), ContextCacheHintsPerScope)...)
	if err != nil {
		return nil, fmt.Errorf("list context cache hints: %w", err)
	}
	defer rows.Close()
	var result []ContextCacheHint
	for rows.Next() {
		var hint ContextCacheHint
		var payload string
		if err := rows.Scan(&hint.Key, &payload, &hint.LastUsedAt); err != nil {
			return nil, err
		}
		hint.Hint = json.RawMessage(payload)
		result = append(result, hint)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ContextCacheHintPut publishes/touches one already-used hint and prunes this
// scope in the same transaction. SQLite starts with a write, avoiding a
// deferred read-to-write snapshot upgrade under concurrent publishers.
// PostgreSQL serializes writers on their session row before upsert/prune, so
// concurrent transactions cannot each prune an incomplete view of the scope.
// Authorization and confirming actual provider use are the caller's job.
func (store *Store) ContextCacheHintPut(ctx context.Context, scope ContextCacheHintScope, key string, hint json.RawMessage) error {
	if err := scope.Validate(); err != nil {
		return err
	}
	if !validContextCacheHintKey(key) || len(hint) > ContextCacheHintMaxBytes || !utf8.Valid(hint) || !json.Valid(hint) || !strings.HasPrefix(strings.TrimSpace(string(hint)), "{") {
		return fmt.Errorf("invalid context cache hint: require a nonempty key and JSON object of at most %d bytes", ContextCacheHintMaxBytes)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var options *sql.TxOptions
	if store.kind == config.DatabaseKindPostgres {
		// After waiting for the session lock, pruning must see the preceding
		// publisher's commit even if the server defaults to repeatable read.
		options = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	tx, err := store.BeginTx(ctx, options)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	p := keychainPlaceholder(store.kind)
	maximum := "MAX"
	if store.kind == config.DatabaseKindPostgres {
		var session string
		if err := tx.QueryRowContext(ctx, `SELECT id FROM gatehouse_sessions WHERE workspace=$1 AND id=$2 FOR NO KEY UPDATE`, scope.Session.Workspace.Id, scope.Session.Id).Scan(&session); err != nil {
			return fmt.Errorf("lock context cache hint session: %w", err)
		}
		maximum = "GREATEST"
	}
	args := append(scope.args(), key, string(hint), time.Now().UnixMicro())
	if _, err := tx.ExecContext(ctx, `INSERT INTO gatehouse_context_cache_hints(workspace,session,cache_scope,algorithm,profile,hint_key,hint,last_used_at)
 VALUES(`+p(1)+`,`+p(2)+`,`+p(3)+`,`+p(4)+`,`+p(5)+`,`+p(6)+`,`+p(7)+`,`+p(8)+`) ON CONFLICT(workspace,session,cache_scope,algorithm,profile,hint_key)
 DO UPDATE SET hint=excluded.hint,last_used_at=`+maximum+`(gatehouse_context_cache_hints.last_used_at,excluded.last_used_at)`, args...); err != nil {
		return fmt.Errorf("put context cache hint: %w", err)
	}
	pruneArgs := append(scope.args(), scope.args()...)
	pruneArgs = append(pruneArgs, ContextCacheHintsPerScope)
	if _, err := tx.ExecContext(ctx, `DELETE FROM gatehouse_context_cache_hints WHERE `+contextCacheHintsWhereFor(store.kind, 1)+`
 AND hint_key NOT IN (SELECT hint_key FROM gatehouse_context_cache_hints WHERE `+contextCacheHintsWhereFor(store.kind, 6)+` ORDER BY last_used_at DESC,hint_key DESC LIMIT `+p(11)+`)`, pruneArgs...); err != nil {
		return fmt.Errorf("prune context cache hints: %w", err)
	}
	return tx.Commit()
}
