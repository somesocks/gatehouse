# Guest / Member / Admin upgrade

Workspace, project, and session roles are now **Guest / Member / Admin**.
Groups use **Member / Admin**, and system grants confer **System admin** access.

## Existing deployments

Update configuration and API integrations alongside the application release.
The configuration `api_version: v1` and `/api/v1` routes retain their versions,
but role values use the new vocabulary:

| Scope | Previous value | New value |
| --- | --- | --- |
| Workspace, project, session grants | `member` | `guest` |
| Workspace, project, session grants | `contributor` | `member` |
| Workspace, project, session grants | `manager` | `admin` |
| Group memberships | `member` | `member` |
| Group memberships | `manager` | `admin` |
| System grants (stored role) | `manager` | `admin` |

In configuration, update `workspaces[].grants[].role` and
`workspaces[].groups[].members[].role` using the appropriate row above.
Keep configured revisions and enabled states intact. `system_grants` has no
configurable role field. A group's project/session grant uses the resource's
three-role vocabulary, independently of its human membership roles.

**An old resource `member` declaration must become `guest`.** Leaving it as
`member` requests contribution rights in the new release. There are no legacy
role aliases or automatic configuration translations.

V069 automatically translates existing SQLite and PostgreSQL database rows on
startup, before configuration reconciliation. It includes disabled grants and
retired group workspace grants, preserves grant IDs, revisions, enabled states,
and activity references, and retains existing permissions. Group membership
`member` remains `member`. The rename itself does not emit access-change activity
or increment revisions.

The upgraded database uses the new roles; an older application binary does not
understand them. Reverting the release requires restoring the matching previous
database and configuration.

## Role meanings

- **Guest:** can view content they have access to; this is not an anonymous or
  temporary account type.
- **Member:** has the former Contributor capabilities in that resource scope.
- **Admin:** has the former Manager capabilities in that scope.
- **Group member:** receives the group's resource grants. Group admins also
  manage the group; their group role does not independently grant resource admin
  rights.

Workspace admission, additive direct/group grants, and effective last-admin
protections follow the same rules as before the rename.
