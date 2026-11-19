package database

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed session_tadw_postgres_walk.sql
var tadwPostgresHistoryWalkSQL string

// Each recursive owner has at most three preview rows. The correlated LIMIT
// keeps preview membership bounded without an outer sort or session-wide join.
var tadwPostgresHistorySeedsSQL = tadwPostgresHistoryWalkSQL + `
 SELECT r.id,r.parent,r.created_at,r.depth,r.root_id,r.kind,
        p.id,p.parent,p.created_at,p.kind
 FROM branch_walk AS r
 LEFT JOIN LATERAL (
  SELECT c.id,c.parent,c.created_at,c.kind FROM gatehouse_session_events AS c
  WHERE r.id<>r.root_id AND r.kind<>'agent.request'
    AND c.workspace=$1 AND c.session=$2 AND c.parent=r.id
  ORDER BY c.created_at ASC,c.id ASC LIMIT 3
 ) AS p ON TRUE`

var tadwPostgresHistoryChildrenSQL = func() string {
	var slots []string
	for index := 0; index < tadwHistoryFrontierKeys; index++ {
		slots = append(slots, fmt.Sprintf("($%d::text)", index+3))
	}
	return `WITH parents(id) AS (VALUES ` + strings.Join(slots, ",") + `)
 SELECT c.id,c.parent,c.kind,c.created_at FROM parents AS p
 CROSS JOIN LATERAL (
  SELECT e.id,e.parent,e.kind,e.created_at FROM gatehouse_session_events AS e
  WHERE p.id IS NOT NULL AND e.workspace=$1 AND e.session=$2 AND e.parent=p.id
    AND e.kind IN ('agent.success','agent.failure')
  ORDER BY e.created_at ASC,e.id ASC LIMIT 2
 ) AS c`
}()

const tadwPostgresHistoryEventSQL = `SELECT ` + sessionEventAuthorColumns + `,
 CASE WHEN events.kind='agent.request' THEN COALESCE((
   SELECT jsonb_set(events.payload, '{author_group}', message.payload -> 'author_group')` + tadwRequestGroupFromParentSQL + `
     AND jsonb_typeof(message.payload -> 'author_group')='object'
 ), events.payload) ELSE events.payload END` + tadwEventFromSQL + `
 WHERE events.workspace=$1 AND events.session=$2 AND events.id=$3`
