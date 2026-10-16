# Workspace group administration

Sysadmins manage groups under **System → Workspaces → [workspace] → Groups**.
This area requires `system.manage`; workspace manager status alone does not
grant access. The workspace-facing Groups page continues to show the user's
enabled memberships.

The groups directory supports name/alias search, pagination, and enabled or
disabled filtering. Create a group with a workspace-unique alias and optional
display name. Edit its alias, display name, or availability from its detail page.
Edits retain the group's `grp_…` ID and therefore its memberships and grants.

## Members

Add existing principals using the principal picker. A membership has no role;
roles belong to grants. **Remove membership** disables that membership while
retaining its identity. Use **Enable membership** to restore it. Adding an
already-present membership, including a disabled one, returns a conflict.

## Workspace grants

Each workspace grant gives one role—Member, Contributor, or Manager—to the
group's enabled members. Create grants, edit their roles, or revoke/re-enable
them individually. Revocation disables a grant; it does not delete it. Another
grant for the same group and role, even disabled, is a duplicate.

Disabling a group suspends all access inherited through it, including access to
projects and chats. Re-enabling it restores access through enabled memberships
and grants. Other direct or group-derived permissions continue to apply.

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
- `POST /groups/{group}/members` accepts `{"principal":"prn_…"}`.
- `PATCH /groups/{group}/members/{principal}` accepts `enabled` and
  `expected_revision`.
- `GET /groups/{group}/grants` returns all of the group's workspace `grants`,
  including disabled ones. There can be at most one per role.
- `POST /groups/{group}/grants` accepts `{"role":"contributor"}`.
- `PATCH /grants/{grant}` accepts `role` and/or `enabled`, plus
  `expected_revision`. Omitted fields are preserved.

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
    groups:
      - alias: developers
        name: Developers
        revision: 3
        members:
          - principal: alice
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

PostgreSQL group reconciliation resolves aliases and IDs inside its execution
transaction. It briefly blocks writes to the group and membership tables while
resolving identities and applying changes; ordinary reads remain available.
Memberships and activity records therefore use the actual group identity even
if an admin created or renamed a group after the migration SQL was prepared.
