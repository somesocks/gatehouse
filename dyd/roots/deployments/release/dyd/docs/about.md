# Release

This root has two output variants:

- `release-artifacts` collects the Linux and macOS binaries for amd64 and arm64
  as plain `gatehouse-<os>-<arch>` files in `dyd/assets`. The universal container
  image's assets, including its OCI layout and image metadata, are collected in
  `dyd/assets/image-gatehouse`.
- `deploy-shell` depends on `release-artifacts` and starts an interactive shell.
  Its `dyd/assets` directory is prepended to `PATH` for future `z-*` scripts.
  `RELEASE_ASSETS` points to the collected release assets. The shell uses `SHELL`
  when set, otherwise `/bin/sh`.

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
