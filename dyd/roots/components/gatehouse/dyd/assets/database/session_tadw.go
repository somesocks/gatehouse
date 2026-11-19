package database

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gatehouse/config"
	"gatehouse/model"
	"gatehouse/typed_id"
)

//go:embed session_tadw_seeds.sql
var tadwHistorySeedsSQL string

const tadwHistoryFrontierOwners = 4
const tadwHistoryFrontierKeys = tadwHistoryFrontierOwners * 3

// Require the V070 partial index: a timestamp-ordered thread scan followed by
// kind filtering is not bounded by selected outcomes, even with LIMIT 2.
var tadwHistoryChildrenSQL = func() string {
	var slots []string
	for index := 0; index < tadwHistoryFrontierKeys; index++ {
		slots = append(slots, fmt.Sprintf("(:p%d)", index))
	}
	return `WITH inputs(id) AS (VALUES ` + strings.Join(slots, ",") + `),
 parents(id) AS MATERIALIZED (SELECT id FROM inputs WHERE id IS NOT NULL)
 SELECT e.id,e.parent,e.kind,e.created_at
 FROM parents AS p CROSS JOIN gatehouse_session_events AS e INDEXED BY gatehouse_session_events_agent_terminals
 ON e.workspace=:workspace AND e.session=:session AND e.parent=p.id
 AND e.kind IN ('agent.success','agent.failure')
 AND e.id IN (SELECT c.id FROM gatehouse_session_events AS c INDEXED BY gatehouse_session_events_agent_terminals
 WHERE c.workspace=:workspace AND c.session=:session AND c.parent=p.id
 AND c.kind IN ('agent.success','agent.failure')
 ORDER BY c.created_at ASC,c.id ASC LIMIT 2)`
}()

// An internal agent request speaks as the sender of its parent message. Read
// the saved group snapshot with the event, not a live group or membership, and
// leave the stored request's acting principal unchanged for authorization/audit.
const tadwRequestGroupFromParentSQL = ` FROM gatehouse_session_events AS message
 WHERE message.workspace=events.workspace AND message.session=events.session AND message.id=events.parent
 AND message.kind='message.text' AND message.author_principal=events.author_principal`

const tadwEventFromSQL = `, events.metrics, events.created_at
 FROM gatehouse_session_events AS events
 LEFT JOIN gatehouse_principals AS principals ON principals.id=events.author_principal`

const tadwHistoryEventSQL = `SELECT ` + sessionEventAuthorColumns + `,
 CASE WHEN events.kind='agent.request' THEN COALESCE((
   SELECT json_set(events.payload, '$.author_group', message.payload -> 'author_group')` + tadwRequestGroupFromParentSQL + `
     AND CASE WHEN json_valid(message.payload) THEN json_type(message.payload, '$.author_group')='object' ELSE FALSE END
 ), events.payload) ELSE events.payload END` + tadwEventFromSQL + `
 WHERE events.workspace=:workspace AND events.session=:session AND events.id=:event`

// SessionTADWEventGetInSQLTransaction uses the same event projection as the
// history cursor, including when replay loads only the recorded event IDs.
func (store *Store) SessionTADWEventGetInSQLTransaction(ctx context.Context, tx *sql.Tx, ref model.SessionEventRef) (error, *model.SessionEvent) {
	if tx == nil || ref.Session.Workspace.Id == "" || ref.Session.Id == "" || ref.Id == "" {
		return fmt.Errorf("get TADW event: invalid transaction or event scope"), nil
	}
	query := tadwHistoryEventSQL
	args := []any{sql.Named("workspace", ref.Session.Workspace.Id), sql.Named("session", ref.Session.Id), sql.Named("event", ref.Id)}
	if store.kind == config.DatabaseKindPostgres {
		query = tadwPostgresHistoryEventSQL
		args = []any{ref.Session.Workspace.Id, ref.Session.Id, ref.Id}
	}
	err, event := sessionEventFromRow(tx.QueryRowContext(ctx, query, args...), ref.Session)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return err, nil
	}
	return nil, &event
}

// TADWHistoryEvent is one real event, not a request with an embedded outcome.
// The cursor emits reverse prompt order: reverse the entire accepted event list
// once to obtain parent-first history. OwnerID is diagnostic selection metadata,
// not an instruction to regroup or reorder the returned events.
type TADWHistoryEvent struct {
	Event   model.SessionEvent
	OwnerID string
}

// TADWHistoryCursor is a lazy history reader for SQLite and PostgreSQL. It buffers at most
// four owners' selected metadata (28 nodes), plus one owner lookahead. Payloads
// are fetched only for events requested by Next. It is not concurrency-safe.
// Close releases cursor resources, never the caller's transaction.
type TADWHistoryCursor struct {
	ctx                 context.Context
	tx                  *sql.Tx
	kind                config.DatabaseKind
	anchor              model.SessionEventRef
	seeds               sessionEventRows
	pending             *tadwSeedRow
	children, events    *sql.Stmt
	selected            []tadwSelectedNode
	position            int
	seen, branchRoots   map[string]bool
	current             TADWHistoryEvent
	err, closeErr       error
	closed, initialized bool
	// Kept per cursor so tests can instrument the exact production queries
	// without registering functions or adding instrumentation to runtime SQL.
	seedQuery, childQuery, eventQuery string
}

// SessionTADWHistoryCursor selects anchor ancestry, eligible siblings, oldest
// three previews, and selected request outcomes. The caller must supply a live
// transaction from this store, and must Close the cursor before ending
// that transaction. No query runs until Next. On PostgreSQL, Close before
// starting another history cursor on the same transaction. PostgreSQL callers must use
// repeatable read for a stable multi-query snapshot, as the normal TADW readers
// do. Visibility begins at the first read, not request creation or construction.
// This API does not perform authorization or activate the TADW runtime path.
func (store *Store) SessionTADWHistoryCursor(ctx context.Context, tx *sql.Tx, anchor model.SessionEventRef) (error, *TADWHistoryCursor) {
	if tx == nil || anchor.Session.Workspace.Id == "" || anchor.Session.Id == "" || !typed_id.Valid(typed_id.SessionEvent, anchor.Id) {
		return fmt.Errorf("open TADW history cursor: invalid transaction or anchor scope"), nil
	}
	if err := ctx.Err(); err != nil {
		return err, nil
	}
	cursor := &TADWHistoryCursor{
		ctx: ctx, tx: tx, kind: store.kind, anchor: anchor, seen: make(map[string]bool), branchRoots: make(map[string]bool),
		seedQuery: tadwHistorySeedsSQL, childQuery: tadwHistoryChildrenSQL, eventQuery: tadwHistoryEventSQL,
	}
	if store.kind == config.DatabaseKindPostgres {
		cursor.seedQuery, cursor.childQuery, cursor.eventQuery = tadwPostgresHistorySeedsSQL, tadwPostgresHistoryChildrenSQL, tadwPostgresHistoryEventSQL
	}
	return nil, cursor
}

func (cursor *TADWHistoryCursor) args() []any {
	return []any{cursor.argument("workspace", cursor.anchor.Session.Workspace.Id), cursor.argument("session", cursor.anchor.Session.Id)}
}

func (cursor *TADWHistoryCursor) argument(name string, value any) any {
	if cursor.kind == config.DatabaseKindPostgres {
		return value
	}
	return sql.Named(name, value)
}

// Next advances one event. After exhaustion, cancellation or failure it closes
// the cursor automatically. Early consumers must call Close themselves.
func (cursor *TADWHistoryCursor) Next() bool {
	if cursor.closed {
		return false
	}
	event, ok, err := cursor.next()
	if err != nil {
		cursor.err = fmt.Errorf("iterate TADW history: %w", err)
	}
	if err != nil || !ok {
		cursor.current = TADWHistoryEvent{}
		cursor.Close()
		return false
	}
	cursor.current = event
	return true
}

func (cursor *TADWHistoryCursor) Event() TADWHistoryEvent { return cursor.current }
func (cursor *TADWHistoryCursor) Err() error              { return cursor.err }

func (cursor *TADWHistoryCursor) Close() error {
	if cursor.closed {
		return cursor.closeErr
	}
	cursor.closed = true
	var failures []error
	if cursor.seeds != nil {
		failures = append(failures, cursor.seeds.Close())
	}
	for _, statement := range []*sql.Stmt{cursor.children, cursor.events} {
		if statement != nil {
			failures = append(failures, statement.Close())
		}
	}
	cursor.selected, cursor.pending, cursor.seen, cursor.branchRoots = nil, nil, nil, nil
	cursor.current = TADWHistoryEvent{}
	cursor.closeErr = errors.Join(failures...)
	cursor.err = errors.Join(cursor.err, cursor.closeErr)
	return cursor.closeErr
}

func (cursor *TADWHistoryCursor) next() (TADWHistoryEvent, bool, error) {
	if err := cursor.ctx.Err(); err != nil {
		return TADWHistoryEvent{}, false, err
	}
	if !cursor.initialized {
		args := append(cursor.args(), cursor.argument("anchor", cursor.anchor.Id))
		var rows sessionEventRows
		var err error
		if cursor.kind == config.DatabaseKindPostgres {
			rows, err = tadwPostgresQuery(cursor.ctx, cursor.tx, tadwPostgresHistoryCursorName, cursor.seedQuery, args...)
		} else {
			rows, err = cursor.tx.QueryContext(cursor.ctx, cursor.seedQuery, args...)
		}
		if err != nil {
			return TADWHistoryEvent{}, false, err
		}
		cursor.seeds, cursor.initialized = rows, true
	}
	if cursor.position == len(cursor.selected) {
		if err := cursor.fill(); err != nil {
			return TADWHistoryEvent{}, false, err
		}
		if len(cursor.selected) == 0 {
			return TADWHistoryEvent{}, false, nil
		}
	}
	node := cursor.selected[cursor.position]
	if cursor.seen[node.ID] {
		return TADWHistoryEvent{}, false, fmt.Errorf("duplicate/cyclic selected event %s", node.ID)
	}
	if cursor.events == nil {
		statement, err := cursor.tx.PrepareContext(cursor.ctx, cursor.eventQuery)
		if err != nil {
			return TADWHistoryEvent{}, false, err
		}
		cursor.events = statement
	}
	row := cursor.events.QueryRowContext(cursor.ctx, append(cursor.args(), cursor.argument("event", node.ID))...)
	err, event := sessionEventFromRow(row, cursor.anchor.Session)
	if err != nil {
		return TADWHistoryEvent{}, false, err
	}
	if event.Kind != node.Kind || event.CreatedAt != node.CreatedAt || (event.Parent == nil) != !node.Parent.Valid || event.Parent != nil && event.Parent.Id != node.Parent.String {
		return TADWHistoryEvent{}, false, fmt.Errorf("selected event metadata changed for %s", node.ID)
	}
	cursor.position++
	cursor.seen[node.ID] = true
	return TADWHistoryEvent{Event: event, OwnerID: node.OwnerID}, true, nil
}

type tadwHistoryNode struct {
	ID              string
	Parent          sql.NullString
	Kind, CreatedAt string
}

type tadwSelectedNode struct {
	tadwHistoryNode
	OwnerID string
}

type tadwHistorySeed struct {
	Owner    tadwHistoryNode
	RootID   string
	Previews []tadwHistoryNode
}

type tadwSeedRow struct {
	Owner   tadwHistoryNode
	RootID  string
	Preview tadwHistoryNode
}

func (cursor *TADWHistoryCursor) seedRow() (*tadwSeedRow, error) {
	if cursor.pending != nil {
		row := cursor.pending
		cursor.pending = nil
		return row, nil
	}
	if !cursor.seeds.Next() {
		return nil, cursor.seeds.Err()
	}
	var row tadwSeedRow
	var depth int
	var id, parent, created, kind sql.NullString
	if err := cursor.seeds.Scan(&row.Owner.ID, &row.Owner.Parent, &row.Owner.CreatedAt, &depth, &row.RootID, &row.Owner.Kind, &id, &parent, &created, &kind); err != nil {
		return nil, err
	}
	cursor.branchRoots[row.RootID] = true
	if id.Valid {
		if !parent.Valid || !created.Valid || !kind.Valid || parent.String != row.Owner.ID || row.Owner.ID == row.RootID || row.Owner.Kind == model.SessionEventKindAgentRequest {
			return nil, fmt.Errorf("invalid TADW preview for %s", row.Owner.ID)
		}
		row.Preview = tadwHistoryNode{id.String, parent, kind.String, created.String}
	} else if parent.Valid || created.Valid || kind.Valid {
		return nil, fmt.Errorf("partially NULL TADW preview")
	}
	return &row, nil
}

func (cursor *TADWHistoryCursor) seedGroup() (*tadwHistorySeed, error) {
	first, err := cursor.seedRow()
	if err != nil || first == nil {
		return nil, err
	}
	group := tadwHistorySeed{Owner: first.Owner, RootID: first.RootID}
	row := first
	for {
		if row.Owner != group.Owner || row.RootID != group.RootID {
			return nil, fmt.Errorf("inconsistent TADW owner")
		}
		if row.Preview.ID != "" {
			group.Previews = append(group.Previews, row.Preview)
		}
		if len(group.Previews) > 3 {
			return nil, fmt.Errorf("too many TADW previews")
		}
		row, err = cursor.seedRow()
		if err != nil {
			return nil, err
		}
		if row == nil || row.Owner.ID != group.Owner.ID {
			cursor.pending = row
			break
		}
		if first.Preview.ID == "" || row.Preview.ID == "" {
			return nil, fmt.Errorf("duplicate/cyclic TADW owner %s", group.Owner.ID)
		}
	}
	return &group, nil
}

func (cursor *TADWHistoryCursor) fill() error {
	cursor.selected, cursor.position = nil, 0
	var groups []tadwHistorySeed
	var requests []string
	for len(groups) < tadwHistoryFrontierOwners {
		group, err := cursor.seedGroup()
		if err != nil {
			return err
		}
		if group == nil {
			break
		}
		groups = append(groups, *group)
		for _, node := range append([]tadwHistoryNode{group.Owner}, group.Previews...) {
			if node.Kind == model.SessionEventKindAgentRequest {
				requests = append(requests, node.ID)
			}
		}
	}
	children := make(map[string][]tadwHistoryNode)
	if len(requests) > 0 {
		if len(requests) > tadwHistoryFrontierKeys {
			return fmt.Errorf("TADW child frontier exceeded its bound")
		}
		if cursor.children == nil {
			statement, err := cursor.tx.PrepareContext(cursor.ctx, cursor.childQuery)
			if err != nil {
				return err
			}
			cursor.children = statement
		}
		args := cursor.args()
		for index := 0; index < tadwHistoryFrontierKeys; index++ {
			var id any
			if index < len(requests) {
				id = requests[index]
			}
			args = append(args, cursor.argument(fmt.Sprintf("p%d", index), id))
		}
		rows, err := cursor.children.QueryContext(cursor.ctx, args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var node tadwHistoryNode
			if err := rows.Scan(&node.ID, &node.Parent, &node.Kind, &node.CreatedAt); err != nil {
				rows.Close()
				return err
			}
			children[node.Parent.String] = append(children[node.Parent.String], node)
			if len(children[node.Parent.String]) > 1 {
				rows.Close()
				return fmt.Errorf("multiple terminal outcomes for request %s", node.Parent.String)
			}
		}
		err = errors.Join(rows.Err(), rows.Close())
		if err != nil {
			return err
		}
	}
	// A reply on the anchor ancestry already has its own retrieval block.
	// Do not emit it again when expanding its ancestor agent.request.
	for parent, nodes := range children {
		if len(nodes) == 1 && cursor.branchRoots[nodes[0].ID] {
			delete(children, parent)
		}
	}
	for _, group := range groups {
		nodes, err := tadwSelectedTree(group, children)
		if err != nil {
			return err
		}
		cursor.selected = append(cursor.selected, nodes...)
	}
	return nil
}

// Traversal is independent of child-selection policy: selected tools/results
// can use the same adjacency map later, without changing the event output type.
func tadwSelectedTree(group tadwHistorySeed, children map[string][]tadwHistoryNode) ([]tadwSelectedNode, error) {
	parseCreatedAt := func(node tadwHistoryNode) (time.Time, error) {
		at, err := time.Parse(time.RFC3339Nano, node.CreatedAt)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse TADW history event %q created_at: %w", node.ID, err)
		}
		return at, nil
	}
	if _, err := parseCreatedAt(group.Owner); err != nil {
		return nil, err
	}
	type timedNode struct {
		tadwHistoryNode
		at time.Time
	}
	var result []tadwSelectedNode
	seen := make(map[string]bool)
	var walk func(tadwHistoryNode) error
	walk = func(node tadwHistoryNode) error {
		if seen[node.ID] {
			return fmt.Errorf("duplicate/cyclic selected tree event %s", node.ID)
		}
		seen[node.ID] = true
		selected := children[node.ID]
		if node.ID == group.Owner.ID && node.Kind != model.SessionEventKindAgentRequest {
			selected = group.Previews
		}
		ordered := make([]timedNode, len(selected))
		for index, child := range selected {
			at, err := parseCreatedAt(child)
			if err != nil {
				return err
			}
			ordered[index] = timedNode{child, at}
		}
		sort.Slice(ordered, func(a, b int) bool {
			if !ordered[a].at.Equal(ordered[b].at) {
				return ordered[a].at.After(ordered[b].at)
			}
			return ordered[a].ID > ordered[b].ID
		})
		terminals := 0
		for _, child := range ordered {
			if node.Kind == model.SessionEventKindAgentRequest && (child.Kind == model.SessionEventKindAgentSuccess || child.Kind == model.SessionEventKindAgentFailure) {
				terminals++
			}
		}
		if terminals > 1 {
			return fmt.Errorf("multiple terminal outcomes for request %s", node.ID)
		}
		for _, child := range ordered {
			if !child.Parent.Valid || child.Parent.String != node.ID {
				return fmt.Errorf("selected child has wrong parent")
			}
			if err := walk(child.tadwHistoryNode); err != nil {
				return err
			}
		}
		result = append(result, tadwSelectedNode{node, group.Owner.ID})
		return nil
	}
	err := walk(group.Owner)
	return result, err
}
