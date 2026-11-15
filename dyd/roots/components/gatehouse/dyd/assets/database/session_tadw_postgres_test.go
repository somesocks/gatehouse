package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gatehouse/config"
	"gatehouse/model"
)

// Execute the production PostgreSQL recursive walk in SQLite, whose recursive
// subset supports this query. Only the final preview projection uses SQLite's
// existing join instead of PostgreSQL LATERAL. This checks traversal semantics,
// not PostgreSQL parsing, planning, or lock behavior.
func postgresWalkOfflineSQL() string {
	return tadwPostgresHistoryWalkSQL + `
 SELECT r.id,r.parent,r.created_at,r.depth,r.root_id,r.kind,
        p.id,p.parent,p.created_at,p.kind
 FROM (SELECT * FROM branch_walk LIMIT -1) AS r
 LEFT JOIN gatehouse_session_events AS p
   ON r.id<>r.root_id AND r.kind<>'agent.request'
  AND p.workspace=$1 AND p.session=$2
  AND p.id IN (
   SELECT c.id FROM gatehouse_session_events AS c
   WHERE c.workspace=$1 AND c.session=$2 AND c.parent=r.id
   ORDER BY c.created_at ASC,c.id ASC LIMIT 3
  )`
}

func TestPostgresTADWRecursiveWalkOffline(t *testing.T) {
	random := rand.New(rand.NewPCG(72, 71))
	for trial := 0; trial < 30; trial++ {
		store := tadwCursorStore(t)
		var nodes []tadwCursorFixtureNode
		anchor := 11
		if trial == 0 {
			nodes = tadwCursorBoundary(t, store)
		} else {
			for id := 1; id <= 100; id++ {
				parent := 0
				if id > 1 && random.IntN(5) != 0 {
					parent = random.IntN(id-1) + 1
				}
				nodes = append(nodes, tadwCursorFixtureNode{id, parent, "message.text", tadwCursorAt(random.IntN(8))})
			}
			next := 100
			for owner := 1; owner <= 100; owner++ {
				for count := random.IntN(5); count > 0; count-- {
					next++
					request := next
					nodes = append(nodes, tadwCursorFixtureNode{request, owner, "agent.request", tadwCursorAt(random.IntN(8))})
					if random.IntN(3) != 0 {
						next++
						nodes = append(nodes, tadwCursorFixtureNode{next, request, "agent.success", tadwCursorAt(random.IntN(8))})
					}
				}
			}
			tadwCursorInsert(t, store.DB, tadwCursorScope(), nodes...)
			anchor = random.IntN(100) + 1
		}
		want := tadwCursorOracle(nodes, anchor)
		for _, limit := range []int{0, 1, 4, len(want), len(want) + 1} {
			tx, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, tadwCursorRef(anchor))
			if err != nil {
				t.Fatal(err)
			}
			// Supply the unmodified recursive walk through a SQLite projection.
			// Driver tests separately check lazy start and consumption bounds.
			rows, err := tx.QueryContext(context.Background(), postgresWalkOfflineSQL(), tadwCursorScope().Workspace.Id, tadwCursorScope().Id, tadwCursorID(anchor))
			if err != nil {
				t.Fatal(err)
			}
			cursor.seeds, cursor.initialized = rows, true
			var got []string
			for len(got) < limit && cursor.Next() {
				got = append(got, cursor.Event().Event.Ref.Id)
			}
			if err := cursor.Err(); err != nil {
				t.Fatal(err)
			}
			cursor.Close()
			tx.Rollback()
			if !slices.Equal(got, want[:min(limit, len(want))]) {
				t.Fatalf("trial=%d limit=%d got=%v want=%v", trial, limit, got, want)
			}
		}
	}
}

func TestPostgresTADWQueryBounds(t *testing.T) {
	for _, query := range []string{tadwPostgresHistorySeedsSQL, tadwPostgresHistoryChildrenSQL, tadwPostgresHistoryEventSQL} {
		for _, sqlite := range []string{"INDEXED BY", ":workspace", ":session", "LIMIT -1"} {
			if strings.Contains(query, sqlite) {
				t.Fatal("SQLite syntax in PostgreSQL query", sqlite)
			}
		}
	}
	for _, part := range []string{"WITH RECURSIVE branch_walk", "LEFT JOIN LATERAL", "LIMIT 3"} {
		if !strings.Contains(tadwPostgresHistorySeedsSQL, part) {
			t.Fatal("missing bounded history clause", part)
		}
	}
	if strings.Count(tadwPostgresHistoryWalkSQL, "FROM branch_walk AS r") != 1 {
		t.Fatal("recursive term must have one recursive reference")
	}
	if strings.Count(tadwPostgresHistoryChildrenSQL, "::text") != tadwHistoryFrontierKeys || !strings.Contains(tadwPostgresHistoryChildrenSQL, "LIMIT 2") {
		t.Fatal("terminal frontier must be fixed and bounded")
	}
}

// A scripted database/sql driver enforces pgx's no-interleaved-results contract
// without a PostgreSQL service. It verifies actual Store/cursor calls, binding,
// transaction ordering and resource release; it is not a PostgreSQL SQL engine.
type tadwSQLStep struct {
	check   func(string, []driver.NamedValue)
	values  [][]driver.Value
	columns int
	err     error
}
type tadwSQLScript struct {
	t                  *testing.T
	steps              []tadwSQLStep
	active             bool
	name               string
	commits, rollbacks int
	options            driver.TxOptions
}
type tadwSQLConnector struct{ script *tadwSQLScript }

func (c tadwSQLConnector) Connect(context.Context) (driver.Conn, error) {
	return &tadwSQLConn{script: c.script}, nil
}
func (c tadwSQLConnector) Driver() driver.Driver { return tadwSQLDriver{script: c.script} }

type tadwSQLDriver struct{ script *tadwSQLScript }

func (d tadwSQLDriver) Open(string) (driver.Conn, error) { return &tadwSQLConn{script: d.script}, nil }

type tadwSQLConn struct{ script *tadwSQLScript }

func (c *tadwSQLConn) Close() error              { return nil }
func (c *tadwSQLConn) Begin() (driver.Tx, error) { return &tadwSQLTx{c.script}, nil }
func (c *tadwSQLConn) BeginTx(_ context.Context, options driver.TxOptions) (driver.Tx, error) {
	c.script.options = options
	return c.Begin()
}
func (c *tadwSQLConn) Prepare(query string) (driver.Stmt, error) {
	if c.script.active {
		return nil, fmt.Errorf("connection busy at prepare")
	}
	return &tadwSQLStmt{conn: c, query: query}, nil
}
func (c *tadwSQLConn) step(query string, args []driver.NamedValue) (tadwSQLStep, error) {
	c.script.t.Helper()
	if c.script.active {
		return tadwSQLStep{}, fmt.Errorf("connection busy: preceding result was not closed")
	}
	if len(c.script.steps) == 0 {
		c.script.t.Fatalf("unexpected SQL: %s", query)
	}
	step := c.script.steps[0]
	c.script.steps = c.script.steps[1:]
	step.check(query, args)
	return step, step.err
}
func (c *tadwSQLConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	_, err := c.step(query, args)
	return driver.RowsAffected(1), err
}
func (c *tadwSQLConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	step, err := c.step(query, args)
	if err != nil {
		return nil, err
	}
	c.script.active = true
	return &tadwSQLRows{script: c.script, values: step.values, columns: step.columns}, nil
}

type tadwSQLTx struct{ script *tadwSQLScript }

func (tx *tadwSQLTx) Commit() error   { tx.script.commits++; return nil }
func (tx *tadwSQLTx) Rollback() error { tx.script.rollbacks++; return nil }

type tadwSQLStmt struct {
	conn  *tadwSQLConn
	query string
}

func (s *tadwSQLStmt) Close() error  { return nil }
func (s *tadwSQLStmt) NumInput() int { return -1 }
func tadwSQLValues(values []driver.Value) []driver.NamedValue {
	args := make([]driver.NamedValue, len(values))
	for i, value := range values {
		args[i] = driver.NamedValue{Ordinal: i + 1, Value: value}
	}
	return args
}
func (s *tadwSQLStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.conn.ExecContext(context.Background(), s.query, tadwSQLValues(args))
}
func (s *tadwSQLStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.conn.QueryContext(context.Background(), s.query, tadwSQLValues(args))
}

type tadwSQLRows struct {
	script  *tadwSQLScript
	values  [][]driver.Value
	columns int
}

func (r *tadwSQLRows) Columns() []string {
	columns := make([]string, r.columns)
	for i := range columns {
		columns[i] = fmt.Sprint(i)
	}
	return columns
}
func (r *tadwSQLRows) Close() error { r.script.active = false; return nil }
func (r *tadwSQLRows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		return io.EOF
	}
	copy(dest, r.values[0])
	r.values = r.values[1:]
	return nil
}

func tadwScriptStore(t *testing.T) (*Store, *tadwSQLScript) {
	t.Helper()
	script := &tadwSQLScript{t: t}
	db := sql.OpenDB(tadwSQLConnector{script})
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		db.Close()
		if len(script.steps) != 0 || script.active {
			t.Errorf("SQL resources/steps remain: %d active=%t", len(script.steps), script.active)
		}
	})
	return &Store{DB: db, kind: config.DatabaseKindPostgres}, script
}
func tadwSQLContains(t *testing.T, parts ...string) func(string, []driver.NamedValue) {
	return func(query string, args []driver.NamedValue) {
		t.Helper()
		for _, part := range parts {
			if !strings.Contains(query, part) {
				t.Fatalf("missing %q in SQL: %s", part, query)
			}
		}
		for _, arg := range args {
			if arg.Name != "" {
				t.Fatal("PostgreSQL received named SQL argument", arg)
			}
		}
	}
}
func tadwSQLDeclare(t *testing.T, script *tadwSQLScript, name string, parts ...string) func(string, []driver.NamedValue) {
	return func(query string, args []driver.NamedValue) {
		t.Helper()
		if !strings.HasPrefix(query, "DECLARE "+name+" NO SCROLL CURSOR WITHOUT HOLD FOR ") {
			t.Fatal("wrong cursor declaration", query)
		}
		tadwSQLContains(t, parts...)(query, args)
		script.name = name
	}
}
func tadwSQLFetch(t *testing.T, script *tadwSQLScript) func(string, []driver.NamedValue) {
	return func(query string, _ []driver.NamedValue) {
		t.Helper()
		if query != "FETCH FORWARD 1 FROM "+script.name {
			t.Fatal("wrong fetch", query)
		}
	}
}
func tadwSQLClose(t *testing.T, script *tadwSQLScript) func(string, []driver.NamedValue) {
	return func(query string, _ []driver.NamedValue) {
		t.Helper()
		if query != "CLOSE "+script.name {
			t.Fatal("wrong close", query)
		}
	}
}
func tadwSQLEvent(id string, parent any, kind, payload string, at time.Time) []driver.Value {
	return []driver.Value{id, parent, kind, "user", nil, nil, true, nil, nil, payload, nil, at}
}

func TestPostgresTADWHistoryCursorConsumptionBounds(t *testing.T) {
	const owners = 9
	for _, limit := range []int{0, 1, 3, 4, 5, 8, 9, 10} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			store, script := tadwScriptStore(t)
			anchor := tadwCursorRef(100)
			at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
			if limit > 0 {
				script.steps = append(script.steps, tadwSQLStep{check: tadwSQLDeclare(t, script, "gatehouse_tadw_history", "WITH RECURSIVE branch_walk", "LEFT JOIN LATERAL")})
			}
			fetched := 0
			for start := 0; start < min(limit, owners); start += tadwHistoryFrontierOwners {
				// Four owners plus one owner lookahead. A pending lookahead is
				// reused by the next frontier rather than fetched a second time.
				for fetched < min(start+tadwHistoryFrontierOwners+1, owners) {
					id := tadwCursorID(100 - fetched)
					script.steps = append(script.steps, tadwSQLStep{check: tadwSQLFetch(t, script), columns: 10, values: [][]driver.Value{{id, nil, at, int64(0), anchor.Id, "message.text", nil, nil, nil, nil}}})
					fetched++
				}
				if start+tadwHistoryFrontierOwners >= owners {
					script.steps = append(script.steps, tadwSQLStep{check: tadwSQLFetch(t, script), columns: 10})
				}
				for position := start; position < min(start+tadwHistoryFrontierOwners, limit, owners); position++ {
					id := tadwCursorID(100 - position)
					script.steps = append(script.steps, tadwSQLStep{check: func(query string, args []driver.NamedValue) {
						tadwSQLContains(t, "events.id=$3")(query, args)
						if len(args) != 3 || args[2].Value != id {
							t.Fatal("wrong consumed payload", args, id)
						}
					}, columns: 12, values: [][]driver.Value{tadwSQLEvent(id, nil, "message.text", `{}`, at)}})
				}
			}
			if limit > 0 {
				script.steps = append(script.steps, tadwSQLStep{check: tadwSQLClose(t, script)})
			}
			tx, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, anchor)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for len(got) < limit && cursor.Next() {
				got = append(got, cursor.Event().Event.Ref.Id)
			}
			if err := cursor.Err(); err != nil {
				t.Fatal(err)
			}
			if err := cursor.Close(); err != nil {
				t.Fatal(err)
			}
			if len(got) != min(limit, owners) {
				t.Fatal("wrong consumption", got, limit)
			}
			if len(script.steps) != 0 || script.active {
				t.Fatal("cursor did not match bounded consumption", len(script.steps), script.active)
			}
		})
	}
}

func TestPostgresTADWHistoryCursorNameReuse(t *testing.T) {
	store, script := tadwScriptStore(t)
	for range 2 {
		tx, err := store.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		for range 2 {
			script.steps = []tadwSQLStep{
				{check: tadwSQLDeclare(t, script, "gatehouse_tadw_history", "WITH RECURSIVE branch_walk")},
				{check: tadwSQLFetch(t, script), columns: 10},
				{check: tadwSQLClose(t, script)},
			}
			err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, tadwCursorRef(11))
			if err != nil {
				t.Fatal(err)
			}
			if cursor.Next() || cursor.Err() != nil {
				t.Fatal("empty cursor did not close cleanly", cursor.Err())
			}
			if err := cursor.Close(); err != nil {
				t.Fatal(err)
			}
			if len(script.steps) != 0 || script.active {
				t.Fatal("cursor resources remain before name reuse", len(script.steps), script.active)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	if script.commits != 2 {
		t.Fatal("expected two committed read transactions", script.commits)
	}
}

func TestPostgresTADWHistoryCursorInterleavesHydrationAfterFetch(t *testing.T) {
	store, script := tadwScriptStore(t)
	anchor := tadwCursorRef(11)
	at := time.Date(2026, 11, 1, 0, 0, 0, 123456000, time.UTC)
	file := "sfi_00000000000000000000000000"
	script.steps = []tadwSQLStep{
		{check: tadwSQLDeclare(t, script, "gatehouse_tadw_history", "WITH RECURSIVE branch_walk", "LEFT JOIN LATERAL", "LIMIT 3")},
		{check: tadwSQLFetch(t, script), columns: 10, values: [][]driver.Value{{anchor.Id, nil, at, int64(0), anchor.Id, "message.text", nil, nil, nil, nil}}},
		{check: tadwSQLFetch(t, script), columns: 10},
		{check: tadwSQLContains(t, "WHERE events.workspace=$1 AND events.session=$2 AND events.id=$3"), columns: 12, values: [][]driver.Value{tadwSQLEvent(anchor.Id, nil, "message.text", `{"text":"hello","attachments":["`+file+`"]}`, at)}},
		{check: tadwSQLContains(t, "FROM gatehouse_session_files", "files.workspace = $1", "files.id = $3"), columns: 3, values: [][]driver.Value{{"file.txt", "text/plain", int64(9007199254740993)}}},
		{check: tadwSQLClose(t, script)},
	}
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, anchor)
	if err != nil {
		t.Fatal(err)
	}
	if !cursor.Next() {
		t.Fatal(cursor.Err())
	}
	err, event := store.SessionEventFilesHydrateInTransaction(context.Background(), tx, cursor.Event().Event)
	if err != nil {
		t.Fatal(err)
	}
	if event.CreatedAt != at.Format(time.RFC3339Nano) {
		t.Fatal(event.CreatedAt)
	}
	attachment := event.Payload["attachments"].([]interface{})[0].(map[string]interface{})
	if attachment["size"] != int64(9007199254740993) {
		t.Fatal("native file size lost integer precision", attachment)
	}
	if err := cursor.Close(); err != nil {
		t.Fatal(err)
	}
	if cursor.Next() {
		t.Fatal("closed cursor advanced")
	}
}

func TestPostgresTADWToolingCursorBindingAndHydration(t *testing.T) {
	store, script := tadwScriptStore(t)
	tx, err := store.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// Forward and reverse reads reuse the tooling name after the previous
	// cursor closes, as in rollover on the same transaction.
	for _, option := range []TADWToolingReadOptions{{}, {FromEventID: tadwCursorID(20)}, {FromEventID: tadwCursorID(20), Reverse: true}} {
		t.Run(fmt.Sprint(option), func(t *testing.T) {
			request := tadwCursorRef(10)
			thinking := tadwCursorRef(80)
			agent := model.WorkspaceAgentRef{Workspace: request.Session.Workspace, Id: "agent"}
			tool := tadwCursorID(20)
			at := time.Now().UTC()
			thinkingRow := tadwSQLEvent(thinking.Id, request.Id, "thinking.request", `{}`, at)
			thinkingRow[3] = nil
			thinkingRow[6] = nil
			thinkingRow[7] = agent.Id
			script.steps = []tadwSQLStep{
				{check: tadwSQLContains(t, "events.id = $3"), columns: 12, values: [][]driver.Value{tadwSQLEvent(request.Id, nil, "agent.request", `{"agent":"agent"}`, at)}},
				{check: tadwSQLContains(t, "events.id = $3"), columns: 12, values: [][]driver.Value{thinkingRow}},
			}
			if option.FromEventID != "" {
				script.steps = append(script.steps, tadwSQLStep{check: tadwSQLContains(t, "WITH RECURSIVE active", "events.id=$6"), columns: 1, values: [][]driver.Value{{at}}})
			}
			order := "ORDER BY events.created_at ASC,events.id ASC"
			if option.Reverse {
				order = "ORDER BY events.created_at DESC,events.id DESC"
			}
			script.steps = append(script.steps,
				tadwSQLStep{check: tadwSQLDeclare(t, script, "gatehouse_tadw_tooling", "WITH RECURSIVE active", "CAST($1 AS TEXT)", order)},
				tadwSQLStep{check: tadwSQLFetch(t, script), columns: 1, values: [][]driver.Value{{tool}}},
				tadwSQLStep{check: tadwSQLContains(t, "events.id = $3"), columns: 12, values: [][]driver.Value{tadwSQLEvent(tool, request.Id, "tool.request", `{"code":"(+ 1 2)"}`, at)}},
				tadwSQLStep{check: tadwSQLFetch(t, script), columns: 1},
				tadwSQLStep{check: tadwSQLClose(t, script)},
			)
			err, cursor := store.SessionTADWToolingCursor(context.Background(), tx, request, agent, thinking, option)
			if err != nil {
				t.Fatal(err)
			}
			if cursor.RequestEvent().Ref != request || !cursor.Next() || cursor.Event().Ref.Id != tool {
				t.Fatal("wrong tooling event", cursor.Err())
			}
			if cursor.Next() || cursor.Err() != nil {
				t.Fatal(cursor.Err())
			}
			if len(script.steps) != 0 || script.active {
				t.Fatal("cursor resources remain before tooling name reuse", len(script.steps), script.active)
			}
		})
	}
}

func TestPostgresContextCacheHintsTransaction(t *testing.T) {
	for _, fail := range []string{"", "lock", "upsert", "prune"} {
		t.Run(fail, func(t *testing.T) {
			store, script := tadwScriptStore(t)
			scope := cacheHintScope()
			injected := errors.New("injected " + fail)
			script.steps = []tadwSQLStep{
				{check: tadwSQLContains(t, "gatehouse_sessions WHERE workspace=$1 AND id=$2 FOR NO KEY UPDATE"), columns: 1, values: [][]driver.Value{{scope.Session.Id}}},
				{check: tadwSQLContains(t, "VALUES($1,$2,$3,$4,$5,$6,$7,$8)", "last_used_at=GREATEST(gatehouse_context_cache_hints.last_used_at,excluded.last_used_at)")},
				{check: tadwSQLContains(t, "DELETE FROM gatehouse_context_cache_hints", "workspace=$1", "profile=$5", "workspace=$6", "profile=$10", "LIMIT $11")},
			}
			if fail != "" {
				index := map[string]int{"lock": 0, "upsert": 1, "prune": 2}[fail]
				script.steps[index].err = injected
				script.steps = script.steps[:index+1]
			}
			err := store.ContextCacheHintPut(context.Background(), scope, "base", []byte(`{"version":1}`))
			if script.options.Isolation != driver.IsolationLevel(sql.LevelReadCommitted) || script.options.ReadOnly {
				t.Fatal("hint pruning must use read committed after session locking", script.options)
			}
			if fail == "" {
				if err != nil || script.commits != 1 || script.rollbacks != 0 {
					t.Fatal(err, script.commits, script.rollbacks)
				}
			} else if !errors.Is(err, injected) || script.commits != 0 || script.rollbacks != 1 {
				t.Fatal(err, script.commits, script.rollbacks)
			}
		})
	}
}

func TestPostgresContextCacheHintsList(t *testing.T) {
	store, script := tadwScriptStore(t)
	scope := cacheHintScope()
	script.steps = []tadwSQLStep{{check: func(query string, args []driver.NamedValue) {
		tadwSQLContains(t, "workspace=$1 AND session=$2 AND cache_scope=$3 AND algorithm=$4 AND profile=$5", "ORDER BY last_used_at DESC,hint_key DESC LIMIT $6")(query, args)
		var got []any
		for _, arg := range args {
			got = append(got, arg.Value)
		}
		want := append(scope.args(), int64(ContextCacheHintsPerScope))
		if !reflect.DeepEqual(got, want) {
			t.Fatal(got, want)
		}
	}, columns: 3, values: [][]driver.Value{{"base", `{"version":1}`, int64(123)}}}}
	hints, err := store.ContextCacheHintsList(context.Background(), scope)
	if err != nil || len(hints) != 1 || hints[0].Key != "base" || hints[0].LastUsedAt != 123 {
		t.Fatal(hints, err)
	}
}

func TestPostgresTADWTerminalFrontier(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			store, script := tadwScriptStore(t)
			anchor := tadwCursorRef(11)
			at := time.Now().UTC()
			var terminals [][]driver.Value
			for i := 0; i < count; i++ {
				terminals = append(terminals, []driver.Value{tadwCursorID(12 + i), anchor.Id, "agent.success", at})
			}
			script.steps = []tadwSQLStep{
				{check: tadwSQLDeclare(t, script, "gatehouse_tadw_history", "WITH RECURSIVE branch_walk")},
				{check: tadwSQLFetch(t, script), columns: 10, values: [][]driver.Value{{anchor.Id, nil, at, int64(0), anchor.Id, "agent.request", nil, nil, nil, nil}}},
				{check: tadwSQLFetch(t, script), columns: 10},
				{check: func(query string, args []driver.NamedValue) {
					tadwSQLContains(t, "WITH parents(id)", "($3::text)", "($14::text)", "CROSS JOIN LATERAL", "LIMIT 2")(query, args)
					if len(args) != 14 || args[2].Value != anchor.Id {
						t.Fatal("wrong terminal frontier", args)
					}
					for _, arg := range args[3:] {
						if arg.Value != nil {
							t.Fatal("unused frontier key is not NULL", arg)
						}
					}
				}, columns: 4, values: terminals},
			}
			if count < 2 {
				id, kind, parent := anchor.Id, "agent.request", any(nil)
				if count == 1 {
					id, kind, parent = tadwCursorID(12), "agent.success", anchor.Id
				}
				script.steps = append(script.steps, tadwSQLStep{check: tadwSQLContains(t, "events.id=$3"), columns: 12, values: [][]driver.Value{tadwSQLEvent(id, parent, kind, `{}`, at)}})
			}
			script.steps = append(script.steps, tadwSQLStep{check: tadwSQLClose(t, script)})
			tx, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			err, cursor := store.SessionTADWHistoryCursor(context.Background(), tx, anchor)
			if err != nil {
				t.Fatal(err)
			}
			if count == 2 {
				if cursor.Next() || cursor.Err() == nil || !strings.Contains(cursor.Err().Error(), "multiple terminal outcomes") {
					t.Fatal(cursor.Err())
				}
			} else {
				if !cursor.Next() {
					t.Fatal(cursor.Err())
				}
				want := anchor.Id
				if count == 1 {
					want = tadwCursorID(12)
				}
				if cursor.Event().Event.Ref.Id != want {
					t.Fatal("terminal must precede its request", cursor.Event())
				}
				if err := cursor.Close(); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestPostgresTADWRowsReleaseOnFailure(t *testing.T) {
	for _, mode := range []string{"declare", "fetch", "scan", "close", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			store, script := tadwScriptStore(t)
			injected := errors.New("injected cursor error")
			script.steps = []tadwSQLStep{{check: tadwSQLDeclare(t, script, "gatehouse_tadw_history", "SELECT $1")}}
			if mode == "declare" {
				script.steps[0].err = injected
			}
			if mode == "fetch" || mode == "scan" {
				step := tadwSQLStep{check: tadwSQLFetch(t, script), columns: 1, values: [][]driver.Value{{"bad integer"}}}
				if mode == "fetch" {
					step.err = injected
				}
				script.steps = append(script.steps, step)
			}
			if mode != "declare" && mode != "cancel" {
				step := tadwSQLStep{check: tadwSQLClose(t, script)}
				if mode == "close" {
					step.err = injected
				}
				script.steps = append(script.steps, step)
			}
			tx, err := store.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			rows, err := tadwPostgresQuery(ctx, tx, tadwPostgresHistoryCursorName, "SELECT $1", "value")
			if mode == "declare" {
				if !errors.Is(err, injected) {
					t.Fatal(err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "cancel" {
				cancel()
			}
			if mode == "fetch" || mode == "cancel" {
				if rows.Next() || rows.Err() == nil {
					t.Fatal("failed fetch accepted", rows.Err())
				}
			}
			if mode == "scan" {
				if !rows.Next() {
					t.Fatal(rows.Err())
				}
				var value int
				if rows.Scan(&value) == nil {
					t.Fatal("invalid scan accepted")
				}
			}
			closeErr := rows.Close()
			if mode == "close" && !errors.Is(closeErr, injected) {
				t.Fatal(closeErr)
			}
			if second := rows.Close(); second != closeErr {
				t.Fatal("close must be idempotent", second, closeErr)
			}
			if script.active {
				t.Fatal("result retained after failure")
			}
		})
	}
}

func TestPostgresTADWToolingSQLPortableSemantics(t *testing.T) {
	store, request, agent, thinking := tadwToolingFixture(t)
	for _, reverse := range []bool{false, true} {
		for _, from := range []string{"", tadwCursorID(40)} {
			args := []any{sql.Named("request", request.Id), sql.Named("workspace", request.Session.Workspace.Id), sql.Named("session", request.Session.Id), sql.Named("thinking", thinking.Id), sql.Named("agent", agent.Id)}
			query := tadwToolingSelectedSQL
			want := []string{tadwCursorID(10), tadwCursorID(20), tadwCursorID(25), tadwCursorID(40), tadwCursorID(50), tadwCursorID(57)}
			if from != "" {
				args = append(args, sql.Named("from", from), sql.Named("from_at", tadwCursorAt(40)))
				query += ` AND (events.id=@request OR events.created_at>@from_at OR (events.created_at=@from_at AND events.id>=@from))`
				want = []string{tadwCursorID(10), tadwCursorID(40), tadwCursorID(50), tadwCursorID(57)}
			}
			if reverse {
				query += ` ORDER BY events.created_at DESC,events.id DESC`
				slices.Reverse(want)
			} else {
				query += ` ORDER BY events.created_at ASC,events.id ASC`
			}
			query, bound, err := tadwToolingBindings(context.Background(), config.DatabaseKindPostgres, query, args)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := store.Query(query, bound...)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for rows.Next() {
				var id string
				if err := rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				got = append(got, id)
			}
			if err := errors.Join(rows.Err(), rows.Close()); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, want) {
				t.Fatal("bound PostgreSQL tooling query membership/order", got, want)
			}
		}
	}
}

func TestTADWSelectedTreeNativeTimestampOrder(t *testing.T) {
	for _, test := range []struct {
		name, left, right, first string
	}{
		{"fractions", "2026-11-01T00:00:00Z", "2026-11-01T00:00:00.1Z", "newer"},
		{"offsets", "2026-11-01T01:00:00+01:00", "2026-11-01T00:00:00.1Z", "newer"},
		{"equal fractions", "2026-11-01T00:00:00Z", "2026-11-01T00:00:00.000Z", "older"},
		{"equal offsets", "2026-11-01T01:00:00+01:00", "2026-11-01T00:00:00Z", "older"},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				owner := tadwHistoryNode{ID: "owner", Kind: "message.text", CreatedAt: tadwCursorAt(0)}
				previews := []tadwHistoryNode{
					{ID: "older", Parent: sql.NullString{String: "owner", Valid: true}, CreatedAt: test.left},
					{ID: "newer", Parent: sql.NullString{String: "owner", Valid: true}, CreatedAt: test.right},
				}
				if reverse {
					slices.Reverse(previews)
				}
				nodes, err := tadwSelectedTree(tadwHistorySeed{Owner: owner, Previews: previews}, nil)
				if err != nil || len(nodes) != 3 || nodes[0].ID != test.first || nodes[2].ID != "owner" {
					t.Fatal(nodes, err)
				}
			}
		})
	}
}
