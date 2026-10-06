# Gatehouse Helm chart

This multi-output root builds the Gatehouse Helm chart and its values schema.
The chart's `config` value is the complete Gatehouse configuration document; the
chart renders it into a ConfigMap mounted at `/etc/gatehouse/config.yaml`.

## Outputs

- `output=schema` builds the chart's Draft 7 `values.schema.json`. It composes
  the Gatehouse configuration schema with chart-specific values and requires
  an explicit database and HTTP listener configuration.
- `output=chart` builds and packages the application chart. Its assets contain
  both the unpacked chart under `dyd/assets/chart` and the versioned archive
  `dyd/assets/gatehouse-<version>.tgz`.

The release root collects the packaged chart and a standalone copy of
`values.schema.json`.

## Installation values

Every install must provide `image.repository` and the complete `config`
document. Helm validates `config` against the Gatehouse configuration schema,
then the chart renders it into a ConfigMap mounted at
`/etc/gatehouse/config.yaml`.

An optional `env` array of `{name, value}` entries creates one Kubernetes
Secret, injected into the Pod with `envFrom`. Values in this array are stored in
Helm release history, so protect access to release data. For PostgreSQL, set
`database.url` to an `env:VARIABLE` reference and provide the matching name in
`env`. The chart does not install or manage PostgreSQL.

The `volumes` array describes any number of volume sources and their mount
paths. A `persistentVolumeClaim.create` source creates a PVC; setting
`persistentVolumeClaim.claimName` references an existing claim. Other sources,
including `emptyDir`, ConfigMaps, and Secrets, are passed through to the Pod.
Volume selection is independent of `database.kind`. When using SQLite, point
`database.path` inside a mounted volume; an `emptyDir` is suitable for tests,
while a PVC retains data beyond the Pod lifetime. SQLite deployments should
stay at one replica. The Deployment strategy is configurable and defaults to
`Recreate`.

PVCs created by the chart are retained after uninstall by default. Set
`persistentVolumeClaim.create.retain: false` to opt out. A retained claim can
be referenced on a later install by its actual Kubernetes claim name. PVC
access modes are configurable; the selected storage backend must support them.

Gatehouse's web assets are embedded in the executable; mounted volumes do not
replace the built-in UI assets.

The Gatehouse listener must be enabled and set to `0.0.0.0:4283`. Set
`services.http.public_base_url` to the externally reachable origin when using
Gatehouse input links. External routing is managed separately; this chart only
creates an internal ClusterIP Service.

`database.kind: ephemeral` is available for disposable test installs, but does
not retain state.
