package agent

import (
	"fmt"

	"github.com/dbos-inc/dbos-transact-golang/dbos"

	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/lisp"
	"gatehouse/model"
)

const gatehouseGroupSecretsModuleID = "native:gatehouse/group/secrets/v1"

var groupSecretListDocumentation = capabilityDocumentation{
	"(group/secrets/list) -> List",
	"Lists credential metadata from groups that both the executing bot and acting principal currently belong to in this workspace. Includes id, group_id, optional group_name, description, author_id, optional author_name, revision, created_at, and updated_at. Values are not included.",
	"(group/secrets/list)",
	"((id . \"gsc_0123456789abcdefghjkmnpqrs\") (group_id . \"grp_example\") (group_name . \"Operations\") (description . \"Deployment token\"))",
}

var groupSecretReadDocumentation = capabilityDocumentation{
	"(group/secrets/read id) -> Bytes",
	"Reads a group credential by gsc_ ID only when both the executing bot and acting principal are current enabled members of its group. Returns secret-tainted Bytes. Losing either membership takes effect on the next operation.",
	"(group/secrets/read \"gsc_0123456789abcdefghjkmnpqrs\")",
	"#<secret>",
}

func groupSecretMetadataValue(secret database.GroupSecretSummary) lisp.Expr {
	optional := func(value *string) lisp.Expr {
		if value == nil {
			return lisp.Null()
		}
		return lisp.String(*value)
	}
	return lisp.List(
		lisp.Pair("id", lisp.String(secret.Ref.Id)),
		lisp.Pair("group_id", lisp.String(secret.Ref.Group.Id)),
		lisp.Pair("group_name", optional(secret.GroupName)),
		lisp.Pair("description", lisp.String(secret.Description)),
		lisp.Pair("author_id", lisp.String(secret.AuthorPrincipal.Id)),
		lisp.Pair("author_name", optional(secret.AuthorName)),
		lisp.Pair("revision", lisp.Integer(int64(secret.Revision))),
		lisp.Pair("created_at", lisp.String(secret.CreatedAt)),
		lisp.Pair("updated_at", lisp.String(secret.UpdatedAt)),
	)
}

func newGroupSecretsModule(list func() (error, []database.GroupSecretSummary), read SessionSecretRead) lisp.HostModule {
	listFunction := func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("group/secrets/list requires no arguments"), nil
		}
		values := []lisp.Expr{}
		if list != nil {
			err, secrets := list()
			if err != nil {
				return lisp.Errorf("group/secrets/list is unavailable"), nil
			}
			for _, secret := range secrets {
				values = append(values, groupSecretMetadataValue(secret))
			}
		}
		return nil, lisp.List(values...)
	}
	readFunction := unavailableSecretRead("group/secrets/read")
	if read != nil {
		readFunction = sessionSecretReadFunction(read, "group/secrets/read")
	}
	return lisp.HostModule{ID: gatehouseGroupSecretsModuleID, Exports: []lisp.HostExport{
		{Name: "list", Value: document(lisp.FunctionNonLeaky(listFunction), groupSecretListDocumentation)},
		{Name: "read", Value: document(lisp.Function(readFunction), groupSecretReadDocumentation)},
	}}
}

func (runtime *SessionEventReplyRuntime) groupSecretsModule(ctx dbos.Context, session model.SessionRef, principal model.PrincipalRef, agent model.WorkspaceAgentRef) lisp.HostModule {
	return newGroupSecretsModule(func() (error, []database.GroupSecretSummary) {
		return runtime.store.GroupSecretsForBotGet(ctx, session.Workspace, principal, agent)
	}, func(id string) (error, []byte) {
		err, detail := runtime.store.GroupSecretForBotGet(ctx, session.Workspace, principal, agent, id)
		if err != nil {
			return err, nil
		}
		err, encrypted := keychain.ParseResource(detail.Secret.Ciphertext)
		if err != nil {
			return fmt.Errorf("parse group secret: %w", err), nil
		}
		err, keys := runtime.keyring.Get(ctx, []model.KeychainRef{*encrypted.Key})
		if err != nil {
			return err, nil
		}
		key := keys[*encrypted.Key]
		defer clear(key)
		clear(keys)
		err, value := keychain.Open(key, database.GroupSecretAssociatedData(detail.Secret.Ref), encrypted)
		return err, value
	})
}
