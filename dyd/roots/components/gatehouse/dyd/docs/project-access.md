# Project access

Open the **Members** card on the project page to manage project membership.
The dedicated Members page has separate searchable, paginated People and Groups
sections, following the group membership layout. Project managers can add
recipients, edit roles, and remove grants; other project members can view the
roster. Dialogs are used for adding recipients and confirming changes.
Each grant assigns
one role: `member` (read only), `contributor`, or `manager`.

Grants are additive. A person's direct grants and the grants of their enabled
groups all apply. Removing one grant does not remove other direct or inherited
access. Managing a group does not independently confer a project role.

Recipients must be enabled principals with explicit workspace admission, or
enabled groups in the project's workspace. Project sharing never admits someone
to the workspace. The People list shows direct grants; group grants are managed
under Groups rather than materialized as editable people.

## API

Under `/api/v1/workspaces/{workspace}/projects/{project}`:

- `GET /members?kind=principal|group&name=...&cursor=...&limit=50` lists active
  explicit recipients and their grants. Project members may read this roster.
- `GET /principal-options` and `GET /group-options` search eligible recipients;
  they accept the same pagination/search parameters and require project manager
  access.
- `POST /grants` accepts exactly one of `principal` or `group`, plus `role`.
- `PATCH /grants/{grant}` accepts `role` and/or `enabled`, with a required
  `expected_revision`. Revoke a grant with `enabled: false`.

Roster and option responses contain `members`, optional `next_cursor`, and
`can_manage_access`. Each grant has an ID, subject, role, enabled state, and
revision. A subject can have distinct grants for different roles. Re-adding a
disabled role reuses its grant ID and increments its revision.

Mutations re-check manager authorization inside a project-locked transaction.
Stale edits, duplicate role assignments, and removing/demoting the last effective
manager return `409`. The remaining manager must be an enabled workspace-admitted
person, directly or through an enabled group membership. Workspace administration
does not bypass project grants.

Successful changes emit `project_grant.create` or `project_grant.update` activity
on the project and recipient topics, including current group members' topics.
Migration v066 initializes existing project grants at revision 1.
