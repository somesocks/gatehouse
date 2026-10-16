# Principal and login administration

System managers create accounts under **System → Principals → Create principal**.
The creation form accepts a display name, optional account alias, and enabled
status. It does not accept login credentials. After creating the principal, use
**Add username/password** on its detail page to add a login. Each principal can
have multiple login identities.

For each username/password login, managers can edit the username, set a new
password, enable or disable the login, or delete it. Usernames are case-sensitive
and globally unique, including disabled logins. They cannot contain whitespace,
control characters, or colons. Login usernames are separate from principal
aliases.

Passwords are write-only and hashed using Gatehouse's existing password-verifier
format. Setting a password replaces the identity's previous password verifiers;
existing authenticated sessions remain active. Disabling the principal or login
stops its sessions from authenticating while it is disabled. Deleting a login
physically removes its identity and verifiers, makes its old sessions unusable,
and frees its username. Reusing the username creates a new identity ID.

Identity activity is retained under the owning principal. Its historical
identity ID has no foreign key to the deleted login.

## API

All endpoints require `system.manage` and return the current principal, including
identity metadata but no passwords or verifier values.

1. `POST /api/v1/system/principals`

   ```json
   {"name":"Example account","alias":"example","enabled":true}
   ```

   Account metadata is optional; `enabled` defaults to `true`. The result has no
   identities. Credential fields are rejected.

2. `POST /api/v1/system/principals/{principal}/identities`

   ```json
   {"username":"example-login","password":"a new password","enabled":true}
   ```

   Username and password are required; `enabled` defaults to `true`.

3. `PATCH /api/v1/system/principals/{principal}/identities/{identity}`

   ```json
   {"expected_revision":1,"username":"renamed-login"}
   ```

   Send any combination of `username`, `password`, and `enabled`. Omitted fields
   are preserved. `expected_revision` must match the identity's current revision.

4. `DELETE /api/v1/system/principals/{principal}/identities/{identity}`

   ```json
   {"expected_revision":2}
   ```

Invalid inputs return `400`, missing principals or identities (including an
identity under a different principal) return `404`, and duplicate aliases,
duplicate usernames, or stale revisions return `409`.

Configuration provisioning remains revision-based. Updating a configured
identity in the panel increments its revision. Deleting an identity does not
remove its configuration declaration; later reconciliation can recreate it.
