# Cryptography configuration

`crypto.default_kdf` selects the algorithm for newly generated password verifiers
(including authentication dummy verifiers) and wrapped keychain keys:

```yaml
api_version: v1
crypto:
  default_kdf: pbkdf2-hmac-sha256-v1
```

Omitting `crypto` or `default_kdf` uses `pbkdf2-hmac-sha256-v1`. Supported profiles:

| Algorithm | PBKDF2-HMAC-SHA256 iterations | Use |
| --- | ---: | --- |
| `pbkdf2-hmac-sha256-v1` | 600,000 | Default |
| `pbkdf2-hmac-sha256-v0` | 1 | Insecure profile for disposable test fixtures |

Both profiles retain the existing salt and output sizes. The selected algorithm
is recorded in each `gh-ver:` password verifier or `gh-kdf:` keychain KDF URI.
Verification and decryption always follow that stored algorithm, independently
of the current creation default. Existing v1 records retain their meaning.

Changing the default alone does not rewrite password verifiers or rewrap
keychain keys. Configured identities retain revision-based reconciliation;
new or explicitly changed passwords use the current default.

Go fixtures can opt in explicitly when opening their store:

```go
err, store := database.OpenWithCrypto(ctx, databaseConfig, config.Crypto{
    DefaultKDF: "pbkdf2-hmac-sha256-v0",
})
```

The store's policy is shared by keyring creation, identity reconciliation,
password administration/reset, and authentication preparation. `database.Open`
continues to use v1. No process-global override is used.
