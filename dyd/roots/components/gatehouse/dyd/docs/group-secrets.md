# Group secrets

Group secrets are encrypted credentials owned by a workspace-local group. The
workspace group page includes a Secrets section. Only current group managers
may create, update, or remove credentials. Group members can list metadata;
workspace/system administration does not implicitly confer secret-management
permission.

## API

Under `/api/v1/workspaces/{workspace}/groups/{group}/secrets`:

- `GET` returns `secrets`, optional `next_cursor`, and `can_manage_secrets`.
  Search/pagination use `name`, `cursor`, and `limit` (maximum 100).
- `POST` accepts `description` and `value`.
- `GET /{secret}` returns metadata only.
- `PATCH /{secret}` accepts `description` and/or a replacement `value`, plus
  `expected_revision`. Omitting `value` preserves the existing credential.
- `DELETE /{secret}` accepts `expected_revision` and clears the stored ciphertext.

Values are write-only over HTTP. Responses never include values or ciphertext
and use `Cache-Control: no-store`. Descriptions are limited to 4096 bytes;
values to 1 MiB. Updates re-check manager membership inside the transaction and
reject stale revisions with `409`.

## Bot access

The `native:gatehouse/group/secrets/v1` module is available in both standalone
and project-linked chats:

- `(group/secrets/list)` returns credential metadata, including source group IDs
  and optional names, without values. Overlapping memberships do not duplicate
  credentials.
- `(group/secrets/read id)` returns secret-tainted bytes by `gsc_` ID.

Available credentials come from the **intersection** of the executing bot's and
the acting principal's enabled group memberships in the current workspace. Both
must belong to the credential's owning group. A principal's membership alone
does not authorize a bot, and a bot's membership alone does not authorize the
principal. The identities
come from the current tool request; other conversation participants do not add
their credentials to that execution.

Chat/project access and participation are admitted once per agent round. Group
secret operations do not repeat those broad admission checks or introduce any
polling. Resource-level permissions stay live: the principal must remain enabled
and workspace-admitted, the bot and group must remain enabled, and **both**
credential memberships must remain enabled. Losing either membership immediately
removes the credential from list and read access. The same resolver
is used for list and read. Group secrets do not grant access to project content.

Encryption uses the existing keychain with associated data binding ciphertext
to the workspace, group, and secret ID. Session secrets remain conversation-scoped.
Migration v068 adds the storage and typed activity targets after v067's outbox
removal; activity is emitted as `group_secret.create`, `.update`, and `.remove`.
