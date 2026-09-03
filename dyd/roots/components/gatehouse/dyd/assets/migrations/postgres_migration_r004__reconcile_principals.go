package migrations

import (
	"context"

	"gatehouse/config"
	"gatehouse/typed_id"
)

func postgresMigrationR004ReconcilePrincipals(state config.State) RepeatableMigration {
	return RepeatableMigration{
		Index:       4,
		Description: "reconcile_principals",
		Builder:     postgresMigrationR004ReconcilePrincipalsBuilder(state.Principals),
	}
}

type postgresMigrationR004PrincipalValue struct {
	ID       string
	Alias    string
	Name     any
	Revision int
	Enabled  bool
}

func postgresMigrationR004ReconcilePrincipalsBuilder(principals []config.Principal) MigrationBuilder {
	return func(ctx context.Context, session *MigrationSession) (error, string) {
		existing, err := principalIDsByAlias(ctx, session)
		if err != nil {
			return err, ""
		}
		values := make([]postgresMigrationR004PrincipalValue, 0, len(principals))
		for _, principal := range principals {
			id := existing[principal.Alias]
			if id == "" {
				id, err = typed_id.New(typed_id.Principal)
				if err != nil {
					return err, ""
				}
			}
			if principal.Revision == 0 {
				principal.Revision = config.DefaultPrincipalRevision
			}
			var name any
			if principal.Name != nil {
				name = *principal.Name
			}
			values = append(values, postgresMigrationR004PrincipalValue{ID: id, Alias: principal.Alias, Name: name, Revision: principal.Revision, Enabled: principal.Enabled})
		}
		return session.RenderTemplate(`
			SELECT 1;
			{{ range . }}
			INSERT INTO gatehouse_principals (id, alias, name, revision, enabled)
			VALUES ({{ sqlLiteral .ID }}, {{ sqlLiteral .Alias }}, {{ sqlLiteral .Name }}, {{ sqlLiteral .Revision }}, {{ sqlBool .Enabled }})
			ON CONFLICT (alias) DO UPDATE SET name = excluded.name, revision = excluded.revision, enabled = excluded.enabled
			WHERE gatehouse_principals.revision < excluded.revision;
			{{ end }}
		`, values)
	}
}
