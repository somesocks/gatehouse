package migrations

const dropWorkflowOutboxesSQL = `
	DROP TABLE gatehouse_agent_tasks__session_event_reply;
	DROP TABLE gatehouse_agent_tasks__session_name;
	DROP TABLE gatehouse_session_approval_decisions;
	CREATE UNIQUE INDEX gatehouse_session_events_approval_response
	ON gatehouse_session_events (workspace, session, parent)
	WHERE kind IN ('approval.success', 'approval.failure');
`
