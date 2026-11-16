-- One successor per recursive row: next sibling, or the newest eligible node
-- at the next ancestor. NULL-parent and non-NULL-parent seeks are separate;
-- IS NOT DISTINCT FROM is used only to identify which branch was reached.
-- Split timestamp ties into equality + ID range and strictly older time seeks.
-- Keep parent in NULL-parent ORDER BY clauses to match the thread-order index;
-- PostgreSQL does not otherwise treat IS NULL as a fixed ordering prefix.
WITH RECURSIVE branch_walk(root_id, root_parent, cutoff, depth, id, parent, created_at, kind) AS (
    SELECT a.id, a.parent, a.created_at, 0, n.id, n.parent, n.created_at, n.kind
    FROM gatehouse_session_events AS a
    JOIN gatehouse_session_events AS n
      ON n.workspace=$1 AND n.session=$2 AND n.id=COALESCE(
          (SELECT b.id FROM gatehouse_session_events AS b
           WHERE b.workspace=$1 AND b.session=$2 AND b.parent=a.parent
             AND b.created_at<=a.created_at
           ORDER BY b.created_at DESC,b.id DESC LIMIT 1),
          (SELECT b.id FROM gatehouse_session_events AS b
           WHERE a.parent IS NULL AND b.workspace=$1 AND b.session=$2
             AND b.parent IS NULL AND b.created_at<=a.created_at
           ORDER BY b.parent ASC,b.created_at DESC,b.id DESC LIMIT 1)
      )
    WHERE a.workspace=$1 AND a.session=$2 AND a.id=$3

    UNION ALL

    SELECT CASE WHEN n.parent IS NOT DISTINCT FROM r.root_parent THEN r.root_id ELSE p.id END,
           CASE WHEN n.parent IS NOT DISTINCT FROM r.root_parent THEN r.root_parent ELSE p.parent END,
           CASE WHEN n.parent IS NOT DISTINCT FROM r.root_parent THEN r.cutoff ELSE p.created_at END,
           CASE WHEN n.parent IS NOT DISTINCT FROM r.root_parent THEN r.depth ELSE r.depth+1 END,
           n.id,n.parent,n.created_at,n.kind
    FROM branch_walk AS r
    LEFT JOIN gatehouse_session_events AS p
      ON p.workspace=$1 AND p.session=$2 AND p.id=r.root_parent
    JOIN gatehouse_session_events AS n
      ON n.workspace=$1 AND n.session=$2 AND n.id=COALESCE(
          (SELECT b.id FROM gatehouse_session_events AS b
           WHERE b.workspace=$1 AND b.session=$2 AND b.parent=r.root_parent
             AND b.created_at=r.created_at AND b.id<r.id
           ORDER BY b.id DESC LIMIT 1),
          (SELECT b.id FROM gatehouse_session_events AS b
           WHERE r.root_parent IS NULL AND b.workspace=$1 AND b.session=$2
             AND b.parent IS NULL AND b.created_at=r.created_at AND b.id<r.id
           ORDER BY b.parent ASC,b.created_at DESC,b.id DESC LIMIT 1),
          (SELECT b.id FROM gatehouse_session_events AS b
           WHERE b.workspace=$1 AND b.session=$2 AND b.parent=r.root_parent
             AND b.created_at<r.created_at
           ORDER BY b.created_at DESC,b.id DESC LIMIT 1),
          (SELECT b.id FROM gatehouse_session_events AS b
           WHERE r.root_parent IS NULL AND b.workspace=$1 AND b.session=$2
             AND b.parent IS NULL AND b.created_at<r.created_at
           ORDER BY b.parent ASC,b.created_at DESC,b.id DESC LIMIT 1),
          (SELECT b.id FROM gatehouse_session_events AS b
           WHERE p.id IS NOT NULL AND b.workspace=$1 AND b.session=$2
             AND b.parent=p.parent AND b.created_at<=p.created_at
           ORDER BY b.created_at DESC,b.id DESC LIMIT 1),
          (SELECT b.id FROM gatehouse_session_events AS b
           WHERE p.id IS NOT NULL AND p.parent IS NULL AND b.workspace=$1 AND b.session=$2
             AND b.parent IS NULL AND b.created_at<=p.created_at
           ORDER BY b.parent ASC,b.created_at DESC,b.id DESC LIMIT 1)
      )
)
