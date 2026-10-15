# ChatGPT / Codex login check

System managers can open **System → Agent providers → Codex login** in a new tab,
or visit `/app/login/openai-codex`, to exercise subscription login in a standalone
centered card.
Request a one-time code, open OpenAI in another tab, and approve the login there.
Some accounts/workspaces require device-code login to be enabled in ChatGPT
settings.

This milestone validates the OAuth code exchange. The success card shows
**Login verified**, the account email when available, and a close-tab action.
The API returns only account metadata and expiry. Access/refresh/ID tokens are
transient server-side values and are discarded. It does not install a provider
credential or enable subscription inference.

## API

Both endpoints require current system-manager authorization, accept JSON POST
bodies, and return `Cache-Control: no-store`:

- `/api/v1/system/agent-providers/openai-codex/login/start`: body `{}`; returns
  `device_auth_id`, `user_code`, `verification_uri`, `expires_at` (UTC RFC3339),
  and `poll_after_ms`.
- `/api/v1/system/agent-providers/openai-codex/login/poll`: body
  `{"device_auth_id":"…","user_code":"…"}`; returns either
  `{"status":"pending","poll_after_ms":5000}` or
  `{"status":"succeeded","account":{"id":"…","email":"…"},"expires_at":"…"}`.
  Email is optional. Approval causes an authorization-code/PKCE exchange on the
  server before success is reported.

The browser owns temporary attempt state in memory and sends non-overlapping
polls, respecting both the initial and subsequent polling intervals. Attempts
expire after 15 minutes. Cancel/navigation aborts requests and clears timers;
reloading starts a fresh login. Gatehouse stores no login-attempt state. Browser
cancellation cannot retract an exchange already accepted by OpenAI.

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
