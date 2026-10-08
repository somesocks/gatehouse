# Gatehouse Helm chart

This multi-output root builds the Gatehouse Helm chart and its values schema.
The chart's `config` value is a string containing the Gatehouse YAML document;
the chart passes it through into a ConfigMap mounted at
`/etc/gatehouse/config.yaml`.

## Outputs

- `output=schema` builds the chart's Draft 7 `values.schema.json` with
  `dhall-codegen`. The chart schema types `config` as an opaque string; it does
  not duplicate or validate Gatehouse config fields.
- `output=chart` builds and packages the application chart. Its assets contain
  both the unpacked chart under `dyd/assets/chart` and the versioned archive
  `dyd/assets/gatehouse-<version>.tgz`.

The release root collects the packaged chart and a standalone copy of
`values.schema.json`.

## Installation values

Every install must provide `image.repository` and `config`; the chart supplies
no config defaults. Set `config` to a string containing the raw Gatehouse YAML.
If it starts with `base64:`, the chart strips that prefix and decodes the
remainder. Gatehouse validates the mounted file when the container starts. See
`examples/complete-config-values.yaml` in the chart for a values example with
the full Gatehouse document.

An optional `env` array of `{name, value}` entries creates one Kubernetes
Secret, injected into the Pod with `envFrom`. Values in this array are stored in
Helm release history, so protect access to release data. For PostgreSQL, set
`database.url` to an `env:VARIABLE` reference and provide the matching name in
`env`. The chart does not install or manage PostgreSQL.

S3 `credentials.access_key_id` accepts either a literal access key ID or an
`env:VARIABLE_NAME` reference. `credentials.secret_access_key.sources` accepts
`env:VARIABLE_NAME` sources. Provide referenced variables in the chart's `env`
list. When rotating an environment-backed credential, increment the storage
provider's `revision` so Gatehouse reconciles the new value.

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

For this chart, configure the Gatehouse listener in the config string as
enabled at `0.0.0.0:4283`. Set `services.http.public_base_url` to the externally
reachable origin when using Gatehouse input links. External routing is managed
separately; this chart only creates an internal ClusterIP Service.

`database.kind: ephemeral` is available for disposable test installs, but does
not retain state.
