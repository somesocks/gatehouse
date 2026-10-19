# Workspace-wide bot availability

The workspace-agent binding's `default` flag marks a bot as available to all
workspace members. The administration UI labels it **Available to all workspace
members**. The configuration and API field remains `default`.

Multiple bots in one workspace may have `default: true`, including bots backed
by the same model. Each binding's flag is independent: creating, editing, or
reconciling one binding does not clear another binding's flag or change its
revision. Omitted `default` values in configuration mean `false`.

```yaml
workspaces:
  - alias: engineering
    agents:
      - alias: welcome
        model: assistant
        revision: 1
        default: true
      - alias: general
        model: assistant
        revision: 1
        default: true
```

Only enabled bindings may be default. Administration clears a binding's own
default flag when disabling it. Configuration should set `default: false` when
setting `enabled: false`.

The flag does not automatically select a recipient, add a bot to a chat, or send
messages. Chat interaction continues to use participants and explicit message
targets. Group memberships remain separate associations.

Configuration uses the existing higher-revision-wins policy for each binding.
Equal or lower configuration revisions cannot overwrite administration edits,
and undeclared bindings are retained.

SQLite and PostgreSQL migration 62 remove the single-default unique index.
Existing flags, binding IDs, revisions, group memberships, and activity history
are preserved.
