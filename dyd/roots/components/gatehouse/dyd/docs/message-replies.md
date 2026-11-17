# Message replies

`POST /api/v1/workspaces/{workspace}/sessions/{session}/messages` accepts an
optional `parent` event ID alongside the existing text, recipients and attachments:

```json
{
  "text": "A follow-up question",
  "parent": "sev_00000000000000000000000001",
  "targets": [{"kind": "agent", "id": "wag_00000000000000000000000001"}]
}
```

Omitting `parent`, or supplying `null`, creates a root message as before. An empty
or malformed parent ID, or an event unavailable in the URL's workspace/session,
is rejected with HTTP 400. The parent must already exist; messages can reply to
other user messages or agent replies. Parent existence is checked in the same
transaction that creates the message and its agent requests.

A successful submission returns HTTP 202 and the stored message. Its `parent`
uses the existing scoped session-event reference representation. The relationship
is event metadata, not a field inside the message payload.

Replying does not inherit recipients, attachments or permissions from the parent.
Existing recipient selection and session authorization rules still apply. Each
selected agent receives a new request beneath the new message, with durable
workflow delivery committed atomically as for root messages.

This endpoint creates the relationship only; branch navigation and reply controls
in the chat UI are separate.
