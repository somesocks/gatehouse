# Activity Events And Topics

## Purpose

The activity system is a durable, topic-based event log that supports client-side change detection and change management in Gatehouse.

## Activity Events

An activity event is immutable. It records:

- An action, such as `workspace.create`, `project.update`, or `session_note.update`.
- The primary resource targeted by that action.

It does not store the original request or an exact change set, and is therefore not suitable for event sourcing.

Each event has a type-prefixed ULID primary key. The ULID is also the event ordering key for change detection.

## Activity Topics

An event has one or more topics, represented by records in the topics table. A
topic identifies a resource context whose projection may be affected by the
event.

Each topic belongs to one event and has no independent ID. Its primary key is
the pair `(activity, topic)`.

Topics are hierarchical where resource containment is stable. A subscription
matches its topic and all of its descendants. An event emits only the
most-specific topic for each affected branch. A session note update emits:

```text
wsp_.../ses_.../snt_...
```

The following subscriptions all match that topic:

```text
wsp_...
wsp_.../ses_...
wsp_.../ses_.../snt_...
```

Prefix matching respects path boundaries. A `wsp_...` subscription matches
`wsp_...` and `wsp_.../...`, but not another workspace whose ID shares that
text prefix.

## Activity Selectors

An activity selector contains a topic prefix and a non-empty list of event
selectors. An event selector is either an exact event name, such as
`workspace.update`, or a namespace selector ending in `.*`, such as
`workspace.*`. A namespace selector matches every documented event name in
that namespace.

An activity checkpoint belongs to a normalized `(topic, events)` selector. The
event list is sorted and de-duplicated. A matching event advances the selector
when its topic matches the topic prefix and its event name matches any event
selector in the list.

Some events have multiple root contexts. For example, a direct workspace grant
affects both a workspace and a principal. The principal is not a child of the workspace,
so the event emits separate topics:


```text
wsp_...
prn_...
```

## Resource Activity

This section defines the activity emitted for each client-meaningful resource.

Workspace, project, and session grants have immutable `wgr_...`, `pgr_...`,
`sgr_...`, and `syg_...` IDs, respectively.

### Principal

- `principal.create` - emitted when a principal is created.
  - `prn_...`
- `principal.update` - emitted when a principal's name, description, or other mutable metadata changes.
  - `prn_...`

### Identity

- `identity.create` - emitted when an identity for a principal is created.
  - `prn_.../idt_...`
- `identity.update` - emitted when an identity's mutable state changes.
  - `prn_.../idt_...`

### Workspace

- `workspace.create` - emitted when a workspace is created.
  - `wsp_...`
- `workspace.update` - emitted when a workspace's name, description, or other mutable metadata changes.
  - `wsp_...`

### Workspace Group

- `group.create` - emitted when a group is created.
  - `wsp_.../grp_...`
- `group.update` - emitted when a group's mutable state changes.
  - `wsp_.../grp_...`

### Workspace Group Membership

- `group_member.create` - emitted when a principal is added to a group.
  - `wsp_.../grp_...`
  - `prn_.../grp_...`
- `group_member.update` - emitted when a group membership's mutable state changes.
  - `wsp_.../grp_...`
  - `prn_.../grp_...`

### Workspace Grant

- `workspace_grant.create` - emitted when a workspace grant is created.
  - `wsp_.../wgr_...` when the subject is a principal.
  - `prn_.../wgr_...` when the subject is a principal.
  - `wsp_.../grp_.../wgr_...` when the subject is a group.
- `workspace_grant.update` - emitted when a workspace grant's role, enabled state, or revision changes.
  - `wsp_.../wgr_...` when the subject is a principal.
  - `prn_.../wgr_...` when the subject is a principal.
  - `wsp_.../grp_.../wgr_...` when the subject is a group.

### System Grant

- `system_grant.create` - emitted when a principal receives a system manager grant.
  - `sys/syg_...`
  - `prn_.../syg_...`
- `system_grant.update` - emitted when a system manager grant's enabled state or revision changes.
  - `sys/syg_...`
  - `prn_.../syg_...`

`sys/...` topics are available only to enabled system managers. The granted
principal can always observe its own `prn_.../syg_...` topic.

### Keychain Version

- `keychain.create` - emitted when a keychain version is created.
  - `sys/kch/<alias>/<version>`

### Agent Provider

- `agent_provider.create` - emitted when an agent provider is created.
  - `sys/apr_...`
- `agent_provider.update` - emitted when an agent provider's mutable state changes.
  - `sys/apr_...`

### Agent Model

- `agent_model.create` - emitted when an agent model is created.
  - `sys/amd_...`
- `agent_model.update` - emitted when an agent model's mutable state changes.
  - `sys/amd_...`

### Workspace-Agent Binding

- `workspace_agent.create` - emitted when a model is bound to a workspace.
  - `wsp_...`
- `workspace_agent.update` - emitted when a workspace-agent binding's mutable state changes.
  - `wsp_...`

### Storage Provider

- `storage_provider.create` - emitted when a storage provider is created.
  - `sys/stp_...`
- `storage_provider.update` - emitted when a storage provider's mutable state changes.
  - `sys/stp_...`

### Workspace-Storage-Provider Binding

- `workspace_storage_provider.create` - emitted when a storage provider is bound to a workspace.
  - `wsp_...`
- `workspace_storage_provider.update` - emitted when a workspace-storage-provider binding's mutable state changes.
  - `wsp_...`

### Project

- `project.create` - emitted when a project is created.
  - `wsp_.../prj_...`
- `project.update` - emitted when a project's mutable state changes.
  - `wsp_.../prj_...`

### Project Grant

- `project_grant.create` - emitted when a project grant is created.
  - `wsp_.../prj_.../pgr_...`
  - `prn_.../pgr_...` when the subject is a principal.
  - `wsp_.../grp_.../pgr_...` when the subject is a group.
- `project_grant.update` - emitted when a project grant's role or enabled state changes.
  - `wsp_.../prj_.../pgr_...`
  - `prn_.../pgr_...` when the subject is a principal.
  - `wsp_.../grp_.../pgr_...` when the subject is a group.

### Project File

- `project_file.create` - emitted when a project file is created.
  - `wsp_.../prj_.../pfi_...`
- `project_file.update` - emitted when a project file's backing storage state changes.
  - `wsp_.../prj_.../pfi_...`

### Project Note

- `project_note.create` - emitted when a project note is created.
  - `wsp_.../prj_.../pnt_...`
- `project_note.update` - emitted when a project note is updated.
  - `wsp_.../prj_.../pnt_...`

### Project Secret

- `project_secret.create` - emitted when a project secret is created.
  - `wsp_.../prj_.../psc_...`
- `project_secret.update` - emitted when a project secret's metadata or value changes.
  - `wsp_.../prj_.../psc_...`

### Session

- `session.create` - emitted when a session is created.
  - `wsp_.../ses_...`
  - `wsp_.../prj_...` - if the session is linked to a project.
- `session.update` - emitted when a session's mutable state changes.
  - `wsp_.../ses_...`
  - `wsp_.../prj_...` - if the session is linked to a project.

### Session-Project Association

- `session.project.link` - emitted when a session is attached to a project.
  - `wsp_.../ses_...`
  - `wsp_.../prj_...` - the new project.
- `session.project.unlink` - emitted when a session is detached from a project.
  - `wsp_.../ses_...`
  - `wsp_.../prj_...` - the former project.
- `session.project.move` - emitted when a session is moved between projects.
  - `wsp_.../ses_...`
  - `wsp_.../prj_...` - the former project.
  - `wsp_.../prj_...` - the new project.

### Session Grant

- `session_grant.create` - emitted when a session grant is created.
  - `wsp_.../ses_.../sgr_...`
  - `prn_.../sgr_...` when the subject is a principal.
  - `wsp_.../grp_.../sgr_...` when the subject is a group.
- `session_grant.update` - emitted when a session grant's role or enabled state changes.
  - `wsp_.../ses_.../sgr_...`
  - `prn_.../sgr_...` when the subject is a principal.
  - `wsp_.../grp_.../sgr_...` when the subject is a group.

### Session Event

- `session_event.create` - emitted when a session event is created.
  - `wsp_.../ses_.../sev_...`

### Session File

- `session_file.create` - emitted when a session file is created.
  - `wsp_.../ses_.../sfi_...`
- `session_file.update` - emitted when a session file's backing storage state changes.
  - `wsp_.../ses_.../sfi_...`

### Session Note

- `session_note.create` - emitted when a session note is created.
  - `wsp_.../ses_.../snt_...`
- `session_note.update` - emitted when a session note is updated.
  - `wsp_.../ses_.../snt_...`

### Session Secret

- `session_secret.create` - emitted when a session secret is created.
  - `wsp_.../ses_.../ssc_...`
- `session_secret.update` - emitted when a session secret's metadata or value changes.
  - `wsp_.../ses_.../ssc_...`

### Internal Rows

The following tables are implementation or protocol state. They do not emit
independent client activity:

- `gatehouse_schema_migrations`
- `gatehouse_activity_events`
- `gatehouse_activity_event_topics`
- `gatehouse_agent_contexts`
- `gatehouse_agent_tasks__session_event_reply`
- `gatehouse_agent_tasks__session_name`
- `gatehouse_session_approval_decisions`
- `gatehouse_session_input_drafts`
- `gatehouse_session_input_responses`
- `gatehouse_storage_objects`
- `gatehouse_embedded_storage_objects`
- `gatehouse_embedded_storage_object_chunks`

Client-visible effects of these rows are emitted through their owning resource,
such as a session event, session file, or project file.

## Emission

An activity event and all of its topic rows are written in the same database
transaction as the mutation they describe. The server emits activity for direct
mutations. Reconciliation migrations that write activity in SQL emit the same
topics in their reconciliation transactions.
