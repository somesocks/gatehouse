-- A recursive cursor: exhaust each level newest-first, then ascend.
-- Each recursive row has exactly one successor (next sibling OR next level).
-- Includes each branch root once, alongside its eligible siblings.
-- Equal timestamps are included even when sibling.id > branch_root.id.
-- Index sought: (workspace, session, parent, created_at DESC, id DESC).
-- NULL-parent siblings are handled with IS, not =.
-- This is retrieval order. Reverse owner blocks, not their local tree order,
-- when assembling older history first. No outer history sort is permitted.
WITH RECURSIVE branch_walk(root_id, root_parent, cutoff, depth, id, parent, created_at) AS (
    SELECT /* root */ a.id, a.parent, a.created_at, 0,
           /* node */ n.id, n.parent, n.created_at
    FROM gatehouse_session_events AS a
    JOIN gatehouse_session_events AS n
      ON n.workspace = :workspace AND n.session = :session
     AND n.id = (
         SELECT b.id FROM gatehouse_session_events AS b
         WHERE b.workspace = :workspace AND b.session = :session
           AND b.parent IS a.parent AND b.created_at <= a.created_at
         ORDER BY b.created_at DESC, b.id DESC LIMIT 1
     )
    WHERE a.workspace = :workspace AND a.session = :session AND a.id = :anchor

    UNION ALL

    SELECT r.root_id, r.root_parent, r.cutoff, r.depth,
           /* node */ n.id, n.parent, n.created_at
    FROM branch_walk AS r
    JOIN gatehouse_session_events AS n
      ON n.workspace = :workspace AND n.session = :session
     AND n.id = COALESCE(
         -- Explicit equality + ID range avoids scanning a large timestamp tie.
         (SELECT b.id FROM gatehouse_session_events AS b
          WHERE b.workspace = :workspace AND b.session = :session
            AND b.parent IS r.root_parent AND b.created_at = r.created_at
            AND b.id < r.id
          ORDER BY b.id DESC LIMIT 1),
         (SELECT b.id FROM gatehouse_session_events AS b
          WHERE b.workspace = :workspace AND b.session = :session
            AND b.parent IS r.root_parent AND b.created_at < r.created_at
          ORDER BY b.created_at DESC, b.id DESC LIMIT 1)
     )

    UNION ALL

    SELECT /* root */ p.id, p.parent, p.created_at, r.depth + 1,
           /* node */ n.id, n.parent, n.created_at
    FROM branch_walk AS r
    JOIN gatehouse_session_events AS p
      ON p.workspace = :workspace AND p.session = :session AND p.id = r.root_parent
    JOIN gatehouse_session_events AS n
      ON n.workspace = :workspace AND n.session = :session
     AND n.id = (
         SELECT b.id FROM gatehouse_session_events AS b
         WHERE b.workspace = :workspace AND b.session = :session
           AND b.parent IS p.parent AND b.created_at <= p.created_at
         ORDER BY b.created_at DESC, b.id DESC LIMIT 1
     )
    WHERE NOT EXISTS (
        SELECT 1 FROM gatehouse_session_events AS b
        WHERE b.workspace = :workspace AND b.session = :session
          AND b.parent IS r.root_parent AND b.created_at = r.created_at
          AND b.id < r.id
    ) AND NOT EXISTS (
        SELECT 1 FROM gatehouse_session_events AS b
        WHERE b.workspace = :workspace AND b.session = :session
          AND b.parent IS r.root_parent AND b.created_at < r.created_at
    )
)
-- Bounded preview membership only; outcomes are ordinary child nodes selected
-- separately. LIMIT -1 fences the streaming source before relational fanout.
SELECT r.id, r.parent, r.created_at, r.depth, r.root_id, o.kind,
       p.id, p.parent, p.created_at, p.kind
FROM (SELECT * FROM branch_walk LIMIT -1) AS r
CROSS JOIN gatehouse_session_events AS o
  ON o.workspace = :workspace AND o.session = :session AND o.id = r.id
LEFT JOIN gatehouse_session_events AS p
  ON r.id <> r.root_id AND o.kind <> 'agent.request'
 AND p.workspace = :workspace AND p.session = :session
 AND p.id IN (
     SELECT /* selected-preview */ c.id
     FROM gatehouse_session_events AS c
     WHERE r.id <> r.root_id AND o.kind <> 'agent.request'
       AND c.workspace = :workspace AND c.session = :session
       AND c.parent = /* preview-owner */ r.id /* preview-scan */
     ORDER BY c.created_at ASC, c.id ASC LIMIT 3
 )
