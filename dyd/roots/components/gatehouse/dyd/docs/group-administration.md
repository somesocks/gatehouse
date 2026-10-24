# Workspace group administration

See [Guest / Member / Admin upgrade](role-names-upgrade.md) when updating an existing deployment.

Workspace admins can create groups from **Workspace → Groups**. Creation
atomically adds the creator as a group admin. Group members can open a group
to view its people and bots; group admins can edit details, manage people and
bots, and appoint or demote other admins.

Sysadmins retain full control under **System → Workspaces → [workspace] → Groups**.
This area requires `system.manage`; workspace admin status alone does not
grant access. Workspace admins do not automatically manage every group.

The workspace groups directory supports name/alias search and pagination. Create
a group with a workspace-unique alias and optional display name. Edit its alias
or display name from its detail page. **Delete group** soft-deletes the group and
returns to the directory. Deleted groups are hidden from all workspace users,
including their admins, and their workspace detail URLs return `404`.
Edits retain the group's `grp_…` ID and therefore its memberships and grants.

## Members

Add already-admitted workspace principals using the workspace-scoped picker.
An enabled direct workspace grant is required to add or restore a human
membership. Each human membership has one role: `member` or `admin`. Admins
have all membership benefits plus group administration rights. The group role
does not change workspace roles or the project/chat roles inherited through
resource grants. Bots have no group management role.

Membership edits cannot take an active group from one functioning admin to
zero, including through system administration. A functioning admin has an
enabled admin membership, an enabled account, and current workspace access.
Promote another active workspace member before stepping down or removing that
last admin. Rejected changes return `409` and leave the membership, revision,
and activity history intact. Membership edits are serialized per group so
simultaneous demotions cannot remove every functioning admin.

Account disabling and workspace-access revocation remain available even when
they leave a group without a functioning admin. Already-inactive admins can
be demoted or removed, and system administrators can appoint a new admin to
repair a group.

**Remove** soft-deletes the membership and removes it from the roster. Adding
the same person again restores the existing membership with the chosen role and
an incremented revision. Adding a current member returns a conflict. Rosters and
pickers show current, enabled people; status filters are not exposed in the UI.

## Workspace admission

Workspace membership and roles are assigned directly to principals. Groups
organize people and bots within a workspace; they never admit people or assign
workspace roles. A direct workspace grant is required for all chat and project
access, including resources with retained direct or group grants. Removing the
last enabled workspace grant suspends that access without deleting resource
grants or group memberships. Restoring admission makes retained paths effective
again.
Issued upload/download capabilities carry their principal and workspace, and
the storage proxy rechecks admission on each transfer request. Background replies
and generated chat titles also revalidate the requesting human before publishing.

Migration V063 preserves currently effective group-derived workspace roles as
direct principal grants, deduplicating overlapping paths and preserving existing
direct grant identities. Disabled access paths do not acquire admission. Legacy
group workspace grants are disabled and immutable; they remain solely because
activity history references their identities. New group workspace grants and
restoration of retired grants are prohibited by the database and administration.
The default root bootstrap also preserves prior revocation or role downgrades;
its direct Admin default cannot silently restore old authority.

Disabling a group suspends all access inherited through it, including access to
projects and chats. Re-enabling it restores access through enabled memberships
and grants. Other direct or group-derived permissions continue to apply.
Restoring a deleted group is a system-administration operation.

## Bots

The **Bots** section lists current workspace bot bindings assigned to the group,
with search and pagination. Add a bot using the
workspace-scoped bot picker. A bot can belong to multiple groups, and several
bots backed by the same model can have different memberships.

**Remove** soft-deletes the association; adding the bot again restores it.
Membership and bot availability have separate enabled states. Membership
changes use independent revisions, and no-op updates retain the current revision.
Adding a current association returns a conflict. Deleted bots and removed
memberships are excluded before pagination.
Bots from another workspace cannot be assigned to the group.

## API

All paths below start with `/api/v1/system/workspaces/{workspace}`:

- `GET /groups?name=…&status=enabled|disabled&limit=50&cursor=…` returns `groups`
  and optional `next_cursor`. Omit `status` to include disabled groups.
- `POST /groups` accepts `alias`, optional `name`, and optional `enabled`.
- `GET /groups/{group}` returns the group, including `revision`.
- `PATCH /groups/{group}` accepts any combination of `alias`, `name`, `enabled`,
  plus `expected_revision`. An empty name clears the display name.
- `GET /groups/{group}/members` supports the same search/filter/pagination
  parameters and returns `members` and optional `next_cursor`.
- `POST /groups/{group}/members` accepts `principal` and optional `role`, which
  defaults to `member` and must be `member` or `admin`.
- `PATCH /groups/{group}/members/{principal}` accepts `enabled` and/or `role`,
  plus `expected_revision`. Member entries include their current `role`.
- `GET /groups/{group}/agents` supports the same search/filter/pagination
  parameters, with bot-binding ID cursors, and returns `agents` and optional
  `next_cursor`. Each entry includes `agent`, `alias`, optional `label`,
  `agent_enabled`, `enabled`, and `revision`.
- `POST /groups/{group}/agents` accepts `{"agent":"wag_…"}`.
- `PATCH /groups/{group}/agents/{agent}` accepts `enabled` and
  `expected_revision`, returning `204` on success.
- `GET /groups/{group}/member-options` searches enabled, directly admitted
  workspace principals and returns `principals` and optional `next_cursor`.
- `GET /groups/{group}/agent-options` searches enabled workspace bot bindings.
- Roster searches accept `active=true` to exclude removed memberships and
  disabled principals or bots before applying pagination.
- Group workspace-grant endpoints have been removed. Workspace role changes
  are performed through direct principal grants.

The same detail and membership operations are available under
`/api/v1/workspaces/{workspace}/groups/{group}` to enabled, admitted group
members. Writes and the member/bot option pickers require the group's `admin`
role; the database rechecks authority inside each mutation transaction.
`GET /groups/{group}/agent-options` returns paginated workspace bot identities
and display data. `GET /group-capabilities` reports whether the caller may
create a group. Workspace `GET /groups` returns the caller's memberships,
including `role`, using collection pagination; deleted groups are excluded.
Workspace rosters always exclude removed memberships and disabled accounts or
bots, regardless of query filters. Workspace `POST /groups` requires workspace admin
permission and returns the created group with the creator's `admin` role.

Groups and membership changes use revisions, as grants already do. Changes and
activity recording are atomic. Stale revisions and duplicate aliases,
memberships, or grant roles return `409`; invalid input returns `400`; resources
under another workspace return `404`. Creation returns `201`; membership updates
return `204`; other updates return the updated record. No-op saves leave
revisions unchanged. Access-change activity is published to affected principals
so their clients can notice revocation after losing workspace access.

## Configuration reconciliation

Groups and memberships use the same **higher revision wins** policy as grants.
Each config declaration has an optional positive `revision`, defaulting to `1`:

```yaml
workspaces:
  - alias: engineering
    grants:
      - principal: alice
        role: member
    groups:
      - alias: developers
        name: Developers
        revision: 3
        members:
          - principal: alice
            role: admin
            revision: 2
            enabled: true
        agents:
          - agent: engineering-assistant
            revision: 2
            enabled: true
```

Missing records are created at their configured revision. Existing records are
updated only when the config revision is strictly higher than their stored
revision; that configured revision becomes the stored revision. Equal or lower
config revisions cannot overwrite admin edits. An actual admin edit increments
the stored revision by one. Group metadata and individual memberships have
independent revisions: raising a group revision does not force membership edits.
To deliberately apply config over an admin change, choose a revision above the
current stored value. Undeclared records are retained.

Existing groups and memberships start at revision `1` when upgrading. Renaming a
configured group requires updating its workspace/alias declaration too;
otherwise reconciliation can recreate the old alias. Grant reconciliation keeps
its existing revision-based behavior.

V064 adds membership roles with a default of `member`, preserving existing
resource access without appointing admins automatically. Configuration can
declare `role: admin`; it follows the membership's higher-revision-wins policy.
For an existing membership, increment its configured revision to apply a role
change, or appoint its admin through system administration.
Configuration membership reconciliation uses the same one-to-zero rule. It
evaluates the final roster using current accounts, workspace access, and
membership revisions, so a single update can hand management to another
functioning admin without blocking account disabling or workspace revocation.

Bot membership declarations identify the bot by its workspace-local binding
alias. Their revisions follow the same higher-revision-wins policy, independently
of group metadata and human memberships. They reconcile after workspace bot
bindings, so groups, bots, and memberships can be created together on a fresh
deployment. Undeclared bot memberships are retained. Mutations and reconciliation
atomically record `group_agent.create` / `group_agent.update` activity with both
group and bot identities and notifications for the group's human members.

Membership aliases can refer to bots created through administration as well as
configuration, including disabled bots. An unresolved workspace, group, or bot
fails reconciliation with an error naming all three aliases and the missing
resource. Validation happens inside the migration transaction before membership
or activity writes.

PostgreSQL group reconciliation resolves aliases and IDs inside its execution
transaction. It briefly blocks writes to the group and membership tables while
resolving identities and applying changes; ordinary reads remain available.
Memberships and activity records therefore use the actual group identity even
if an admin created or renamed a group after the migration SQL was prepared.

Human memberships reconcile after direct workspace grants (R016 after R007).
The default root principal is admitted directly. Default configuration does not
synthesize a root group or membership; groups are created explicitly. Workspace grants
must name a `principal`; the former `group` recipient is no longer accepted.
When upgrading an existing configuration, remove group-targeted workspace-grant
declarations; V063 preserves the access already effective in the database. Use
explicit principal grants for subsequent configuration-managed role changes.
