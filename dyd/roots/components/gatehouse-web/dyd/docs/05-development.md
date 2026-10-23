# 5. Web package development

[Documentation index](index.md)

## Dryad environments and dependencies

Dependencies and generated schemas are supplied through Dryad variants. Never
install dependencies inside the source assets. `.dyd-ignore` excludes
`node_modules`, `dist`, and the Vitest cache from source preservation.

Run the following commands from the garden root.

## Update dependency manifests and lockfiles

Use a disposable source environment:

```sh
dryad root develop start dyd/roots/components/gatehouse-web --variant=output=source --scope=none --on-exit=save -- sh -c '"$DYD_STEM/dyd/commands/z-update-lockfile"'
```

## Build and check

Build the application, standalone file picker, and test package, then run type
checking and tests:

```sh
dryad root build dyd/roots/components/gatehouse-web --variant=output=build --scope=none --join-stdout --join-stderr
dryad root build dyd/roots/components/gatehouse-web --variant=output=build-session-file-picker --scope=none --join-stdout --join-stderr
dryad root build dyd/roots/components/gatehouse-web --variant=output=tests --scope=none --join-stdout --join-stderr
dryad sprout run dyd/sprouts/components/gatehouse-web --variant=output=tests --scope=none --join-stdout --join-stderr -- --check
dryad sprout run dyd/sprouts/components/gatehouse-web --variant=output=tests --scope=none --join-stdout --join-stderr
```

`--check` runs the package's `svelte-check` command. The test run uses Vitest,
including the [architecture checks](03-architecture.md#architecture-checks) and
[lifecycle regressions](04-lifecycle.md#verification).
