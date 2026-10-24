# ChatGPT / Codex provider credentials

Create an agent provider with protocol **OpenAI Codex / ChatGPT**, choose its
encryption keychain, and save. Its detail page offers **Connect ChatGPT**, opening
`/app/login/openai-codex?provider=<provider-id>` in a new tab with a standalone
centered card. Request a one-time code, open OpenAI in another tab, and approve.
Some accounts/workspaces require device-code login to be enabled in ChatGPT
settings.

The server installs the exchanged access/refresh tokens as versioned JSON sealed
by the selected keychain. The success card shows **Account connected**, the email
when available, and a close-tab action. Tokens never pass through browser responses
or workflow checkpoints. Account ID, optional email, and expiry are stored as
derived public metadata alongside the ciphertext. The hourly [provider maintenance
workflow](provider-maintenance.md) refreshes OAuth credentials. Connected providers
support [Codex subscription inference](codex-inference.md).

The provider detail page supports **Reconnect ChatGPT** and **Disconnect**.
Reconnect preserves the old credential until the new bundle is committed.
Disconnect clears the bundle and account metadata locally; it does not revoke
the session at OpenAI. Both actions advance the provider revision. Provider
activity events refresh open administration views.

## API

Both endpoints require current system-admin authorization, accept JSON POST
bodies, and return `Cache-Control: no-store`:

- `/api/v1/system/agent-providers/openai-codex/login/start`: body
  `{"provider":"apr_…","expected_revision":1}`; returns
  `device_auth_id`, `user_code`, `verification_uri`, `expires_at` (UTC RFC3339),
  and `poll_after_ms`.
- `/api/v1/system/agent-providers/openai-codex/login/poll`: body
  `{"provider":"apr_…","expected_revision":1,"device_auth_id":"…","user_code":"…"}`; returns either
  `{"status":"pending","poll_after_ms":5000}` or
  `{"status":"succeeded","account":{"id":"…","email":"…"},"expires_at":"…","provider":{…}}`.
  Email is optional. Approval causes an authorization-code/PKCE exchange and an
  encrypted, revision-checked save before success is reported.
- `/api/v1/system/agent-providers/<provider-id>/disconnect`: body
  `{"expected_revision":2}`; returns the updated, redacted provider.

Each start/poll validates the target provider, protocol, revision, and keychain.
The final save checks the revision again. Changes, disconnects, and competing
successful logins invalidate old completions with HTTP 409. After an ambiguous
poll failure the browser rereads provider status rather than retrying the code.

The browser owns temporary attempt state in memory and sends non-overlapping
polls, respecting both the initial and subsequent polling intervals. Attempts
expire after 15 minutes. Cancel/navigation aborts requests and clears timers;
reloading starts a fresh login. Gatehouse stores no login-attempt state. Browser
cancellation cannot retract an exchange already accepted by OpenAI.

## Configuration and upgrades

```yaml
agent_providers:
  - alias: chatgpt
    revision: 1
    protocol: openai-codex
    enabled: true
```

The endpoint defaults to the fixed Codex subscription endpoint. Credential
sources are not accepted for this protocol: use login to install tokens. Higher
configuration revisions preserve connected credentials and their actual keychain
version. New and refreshed credentials use the current version of Gatehouse's
fixed `default` system keychain; providers do not select a keychain.

SQLite/PostgreSQL V056 renames operational `api_key` to `credential`, preserves
existing API-key ciphertext unchanged, and permits disconnected Codex providers.
Existing API-key configuration/input remains supported; `credential` is also
accepted for API-key providers, with the same source configuration. Specifying
both `api_key` and `credential` is rejected. API-key plaintext remains a raw key;
only Codex credentials use the new JSON format.

The card displays a selectable code with an icon-only copy control and a relative
expiry. Background polls leave its waiting state steady. Success, failure,
expiry, and cancellation replace the code with completion or recovery actions;
the changing expiry is excluded from live announcements.

All OpenAI API calls are server-to-server, using the Codex public client and
fixed authentication URLs. The external approval tab is ordinary navigation;
no Gatehouse OAuth callback or OpenAI browser CORS support is needed.

Protocol references: [Codex device authentication][codex] and [OpenCode's Codex
adapter][opencode]. Token claims decoded from the trusted exchange response are
upstream account metadata, not Gatehouse authentication claims.

[codex]: https://github.com/openai/codex/blob/rust-v0.115.0/codex-rs/login/src/device_code_auth.rs
[opencode]: https://github.com/anomalyco/opencode/blob/v1.3.0/packages/opencode/src/plugin/codex.ts
