# Chat discovery, invitations, and group participation

Workspace admission requires an enabled direct principal workspace grant. Group
membership and direct chat/project grants cannot substitute for admission.
Workspace-facing chat invitations require both chat-manager access and an
enabled relationship to the invitation target:

- A person must share an enabled group with the inviter in this workspace and
  have workspace access. Disabled people and disabled memberships are excluded.
- A bot must be enabled with an enabled model/provider, and either have
  `default: true` or share an enabled group with the inviter.
- A group must be enabled, and the inviter must have an enabled membership in it.

These checks apply to invitation options, explicit participant grants, automatic
invite-on-send, and restoring disabled grants through POST or PATCH. Mutations
check current eligibility inside their transaction. Knowing an ID, holding a
workspace-management role, or meeting someone in another chat does not establish
an invitation relationship.

Within a chat, participants can address its effective participants independently
of shared groups. A participating group supplies its enabled human and bot
members. Bots participating through several groups or directly are deduplicated.
Removing one path leaves other paths in effect; removing the final path stops
new requests. Each agent round checks bot participation and the requesting
principal's workspace, chat, and project access before starting. A round consists
of one thinking step (including provider retries) and its resulting tool batch.
An admitted round may finish, including publishing a final answer, if access is
removed midway; the next round fails normally instead of starting new work.
Approval and input waits remain part of their admitted tool batch. Individual
tools still enforce their normal resource-level permissions.

The round's admission decision is committed with its thinking-request marker,
so recovery of an already-started round does not recheck chat access midway.
Permission changes do not manufacture cancellation events or trigger a global
request scan. Explicit user cancellation uses the durable cancellation flow.

Bot, model, and provider availability is checked for discovery, new messages,
and live model resolution. Disabling availability retains participation paths so
already-committed outcomes can replay without another provider call.

The participant roster lists explicit people/groups and effective bots. An
inherited bot lists its source groups and roles in `inherited_from`, separately
from its editable direct `grants`. Source group grant IDs and revisions are not
exposed as bot grants, so older clients cannot accidentally change the group's
participation when managing a bot. The Bots UI displays these sources read-only;
manage their participation through the Groups tab or group membership
administration. Sending to an inherited bot never materializes a direct grant.

`GET /api/v1/workspaces/{workspace}/agents` lists only bots discoverable by the
authenticated principal. Transcript events carry hydrated bot-author and
request-target display metadata, including for removed or disabled bots. Only
identities referenced by the authorized event page are loaded. Display metadata
does not make those bots discoverable or invitable elsewhere; participation
queries depend exclusively on current grants and group memberships. Bot authors
use the same display-object pattern as human authors: `author_agent` contains a
`ref`, `enabled`, `alias`, and optional `label`. Queued `agent.request` events
also carry `target_agent` display metadata while retaining the target ID in
`payload.agent`. Legacy stored `payload.agents` lists are projected into
canonical labeled `payload.targets`; stored events and canonical send-time
recipient snapshots are preserved. The chat renderer obtains attribution from
the transcript response alone.

Chat recipient, participant, and invitation views refresh on group, human
membership, bot membership, workspace-bot, and chat-grant activity.
