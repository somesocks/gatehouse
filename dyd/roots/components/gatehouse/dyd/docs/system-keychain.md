# System keychain

Gatehouse uses the fixed keychain identity `default` for authentication and
storage capabilities, internal sealed values, and new global agent/storage
provider credentials. The HTTP service and provider configuration/API do not
accept a keychain selector. The administrator forms select encryption keys
automatically.

The unlocking sources remain configurable:

```yaml
keychains:
  - id: default
    sources:
      - env:GATEHOUSE_KEYCHAIN
      - "stdin:"
```

This is also the implicit configuration when `keychains` is omitted. An explicit
list must include `default` for system encryption to be available. Other named
keychains can still be loaded to decrypt historical data; a system write never
falls back to one of them when `default` is unavailable.

## Encrypted URLs

New resource ciphertext and capability tokens omit the default keychain ID:

```text
gh-enc:<payload>?alg=aes128-gcm-v1&ver=3
```

The explicit historical representation is still accepted:

```text
gh-enc:<payload>?alg=aes128-gcm-v1&key=default&ver=3
```

Other keychains remain explicit (`key=workspace&ver=3`). Versions are always
required for resource ciphertext and are never inferred from the current key.
Wrapped keys and provider credential values, whose key references are held in
database columns, retain their existing representation without `key` or `ver`.
Those values are not interpreted as versionless system tokens.

## Rotation behavior

New encryption selects the highest enabled version of `default`. Decryption
uses the exact keychain and version recorded with the ciphertext. Reconciliation
of configured provider credentials, manual credential replacement, and Codex
credential refresh all use the current system version. Metadata-only edits and
configuration reconciliation of an already-connected Codex provider preserve
the existing ciphertext's actual key reference.

OAuth refresh selects and unwraps its destination before contacting the provider.
An unavailable destination leaves the existing credential untouched. The selected
key/reference is pinned until the returned bundle is saved, including persistence
retries; retiring that version during the request does not discard the bundle or
relabel its ciphertext as a newer version.

This change neither renames nor rewraps existing keys. Explicit `default`
tokens and existing encrypted values remain readable. Deployments that selected
a different HTTP-service keychain must switch system issuance to `default`;
their old non-default authentication/capability tokens are not accepted as
system tokens. Retain any old named keychains and their unlocking sources while
stored credentials or secrets still reference them. Existing provider selectors
must be removed from configuration; new writes use `default`.

The versioned, explicitly identifiable keychain model remains available for
future workspace-owned encryption. Owner-controlled unlocking and workspace
content sealing are separate capabilities, not provided by the system keychain.
