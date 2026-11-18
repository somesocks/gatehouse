-- Limits bound branch/preview membership, not execution discovery: cancellation
-- and interactive lookups may walk a selected request's children/tool requests.
WITH
parent_check AS MATERIALIZED (
 SELECT CASE WHEN CAST(@parent AS TEXT) IS NULL OR EXISTS (
  SELECT 1 FROM gatehouse_session_events
  WHERE workspace=@workspace AND session=@session AND id=@parent
 ) THEN 'valid' ELSE 'missing' END AS id
),
branch_candidates AS MATERIALIZED (
 SELECT e.id,e.parent,e.kind,e.created_at
 FROM gatehouse_session_events AS e
 WHERE e.workspace=@workspace AND e.session=@session AND e.parent IS @parent
   AND e.kind IN ('message.text','agent.request','agent.success','agent.failure','input.request','approval.request')
   AND (e.created_at,e.id) > (@after_time,@after_id)
 ORDER BY e.created_at,e.id LIMIT @page_size+1
),
branch_ranked AS MATERIALIZED (
 SELECT *,row_number() OVER (ORDER BY created_at,id) AS slot FROM branch_candidates
),
page AS MATERIALIZED (
 SELECT * FROM branch_ranked WHERE slot<=@page_size
),
previews AS MATERIALIZED (
 SELECT b.id AS owner,c.id,c.parent,c.kind,c.created_at,
        row_number() OVER (PARTITION BY b.id ORDER BY c.created_at,c.id) AS slot
 FROM page AS b CROSS JOIN gatehouse_session_events AS c
 WHERE b.kind NOT IN ('agent.request','input.request','approval.request')
   AND c.workspace=@workspace AND c.session=@session
   AND c.id IN (
    SELECT p.id FROM gatehouse_session_events AS p
    WHERE p.workspace=@workspace AND p.session=@session AND p.parent=b.id
      AND p.kind IN ('message.text','agent.request','agent.success','agent.failure','input.request','approval.request')
    ORDER BY p.created_at,p.id LIMIT 4
   )
),
visible AS MATERIALIZED (
 SELECT id,kind FROM page UNION SELECT id,kind FROM previews WHERE slot<4
),
requests AS MATERIALIZED (
 SELECT v.id,json_extract(e.payload,'$.agent') AS agent
 FROM visible AS v CROSS JOIN gatehouse_session_events AS e
 WHERE v.kind='agent.request'
   AND e.workspace=@workspace AND e.session=@session AND e.id=v.id
),
terminals AS MATERIALIZED (
 SELECT r.id AS owner,c.id,c.parent,c.kind,c.created_at
 FROM requests AS r CROSS JOIN gatehouse_session_events AS c
 WHERE c.workspace=@workspace AND c.session=@session AND c.author_agent=r.agent
   AND c.id IN (
    SELECT t.id FROM gatehouse_session_events AS t
    WHERE t.workspace=@workspace AND t.session=@session AND t.parent=r.id
      AND t.kind IN ('agent.success','agent.failure')
    ORDER BY t.created_at,t.id LIMIT 2
   )
),
cancellations AS MATERIALIZED (
 SELECT r.id AS owner,c.id,c.parent,c.kind,c.created_at
 FROM requests AS r CROSS JOIN gatehouse_session_events AS c
 WHERE c.workspace=@workspace AND c.session=@session
   AND c.id=(
    SELECT x.id FROM gatehouse_session_events AS x
    WHERE x.workspace=@workspace AND x.session=@session AND x.parent=r.id
      AND x.kind='cancel.request'
    ORDER BY x.created_at,x.id LIMIT 1
   )
),
owned_prompts AS MATERIALIZED (
 SELECT r.id AS owner,p.id,p.parent,p.kind,p.created_at
 FROM requests AS r CROSS JOIN gatehouse_session_events AS t CROSS JOIN gatehouse_session_events AS p
 WHERE t.workspace=@workspace AND t.session=@session AND t.parent=r.id
   AND t.kind='tool.request' AND t.author_agent=r.agent
   AND p.workspace=@workspace AND p.session=@session AND p.parent=t.id
   AND p.kind IN ('input.request','approval.request') AND p.author_agent=t.author_agent
),
prompts AS MATERIALIZED (
 SELECT id,parent,kind,created_at FROM owned_prompts
 UNION
 SELECT e.id,e.parent,e.kind,e.created_at
 FROM visible AS v CROSS JOIN gatehouse_session_events AS e
 WHERE v.kind IN ('input.request','approval.request')
   AND e.workspace=@workspace AND e.session=@session AND e.id=v.id
),
resolutions AS MATERIALIZED (
 SELECT p.id AS owner,e.id,e.parent,e.kind,e.created_at
 FROM prompts AS p CROSS JOIN gatehouse_session_events AS e
 WHERE e.workspace=@workspace AND e.session=@session AND e.parent=p.id
   AND ((p.kind='input.request' AND e.kind IN ('input.success','input.failure'))
     OR (p.kind='approval.request' AND e.kind IN ('approval.success','approval.failure')))
),
refs(relation,owner,slot,id,parent,kind,created_at,hydrate) AS MATERIALIZED (
 SELECT 'branch',NULL,slot,id,parent,kind,created_at,1 FROM page
 UNION ALL
 SELECT 'branch-more',NULL,slot,id,parent,kind,created_at,0 FROM branch_ranked WHERE slot>@page_size
 UNION ALL
 SELECT CASE WHEN slot=4 THEN 'preview-more' ELSE 'preview' END,
        owner,slot,id,parent,kind,created_at,CASE WHEN slot<4 THEN 1 ELSE 0 END FROM previews
 UNION ALL
 SELECT 'terminal',owner,0,id,parent,kind,created_at,1 FROM terminals
 UNION ALL
 SELECT 'cancellation',owner,0,id,parent,kind,created_at,1 FROM cancellations
 UNION ALL
 SELECT 'prompt',(SELECT owner FROM owned_prompts AS o WHERE o.id=p.id LIMIT 1),0,id,parent,kind,created_at,1 FROM prompts AS p
 UNION ALL
 SELECT 'resolution',owner,0,id,parent,kind,created_at,1 FROM resolutions
 UNION ALL
 SELECT 'scope',NULL,0,id,NULL,NULL,NULL,0 FROM parent_check
)
SELECT r.relation,r.owner,r.slot,r.id,r.parent,r.kind,r.created_at,
       e.author_principal,a.alias,a.name,a.enabled,e.author_agent,e.author_gateway,e.payload,e.metrics
FROM refs AS r
LEFT JOIN gatehouse_session_events AS e
 ON r.hydrate=1 AND e.workspace=@workspace AND e.session=@session AND e.id=r.id
LEFT JOIN gatehouse_principals AS a ON a.id=e.author_principal
ORDER BY r.relation,r.owner,r.slot,r.created_at,r.id;
