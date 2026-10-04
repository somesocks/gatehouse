# Release

This root has two output variants:

- `release-artifacts` collects the Linux and macOS binaries for amd64 and arm64
  as plain `gatehouse-<os>-<arch>` files in `dyd/assets`. The universal container
  image's assets, including its OCI layout and image metadata, are collected in
  `dyd/assets/image-gatehouse`. It also includes `gatehouse-config.schema.json`.
- `deploy-shell` depends on `release-artifacts` and starts an interactive shell.
  Its `dyd/assets` directory is prepended to `PATH` for future `z-*` scripts.
  `RELEASE_ASSETS` points to the collected release assets. The shell uses `SHELL`
  when set, otherwise `/bin/sh`, and starts with the `[release-shell]$` prompt.

`z-publish-tag [remote]` creates the annotated `release-<version>` tag if it
doesn't exist, then pushes that tag to `origin` or the specified Git remote. Run
it from the project Git working tree.

Both variants carry the Gatehouse version in `dyd/traits/version`. Publishing
scripts in the deploy shell can read it from `$DYD_STEM/dyd/traits/version`.

Build both variants:

```sh
dryad run build --scope=release
```

The release scope also selects this root for `dryad roots build --scope=release`.

Enter the deploy shell:

```sh
dryad sprout run --scope=none --variant=output=deploy-shell dyd/sprouts/deployments/release
```

Publish the release tag from the project Git working tree:

```sh
dryad run publish-release-tag --scope=release
```
