package agent

import (
	"errors"
	"strings"
	"testing"

	"gatehouse/database"
	"gatehouse/lisp"
	"gatehouse/model"
)

func TestGroupSecretCapabilities(t *testing.T) {
	secret := database.GroupSecretSummary{Ref: model.GroupSecretRef{Id: "gsc_00000000000000000000000000", Group: model.GroupRef{Id: "grp_00000000000000000000000000"}}, Description: "Deployment token", AuthorPrincipal: model.PrincipalRef{Id: "prn_00000000000000000000000000"}, Revision: 1, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}
	allowed := true
	modules := []lisp.HostModule{newGroupSecretsModule(func() (error, []database.GroupSecretSummary) {
		if !allowed { return nil, nil }
		return nil, []database.GroupSecretSummary{secret}
	}, func(id string) (error, []byte) {
		if !allowed || id != secret.Ref.Id { return errors.New("unavailable"), nil }
		return nil, []byte("CREDENTIAL_SENTINEL")
	})}
	options := lisp.EvalOptions{HostModules: modules}
	err, result := lisp.Evaluate(`(import (group/secrets @native:gatehouse/group/secrets/v1) (group/secrets/list))`, options)
	if err != nil || lisp.TaintOf(result) != lisp.TaintNone || !strings.Contains(result.String(), `(group_id . "grp_00000000000000000000000000")`) || strings.Contains(result.String(), "CREDENTIAL_SENTINEL") { t.Fatalf("metadata = %v %v", result, err) }
	err, result = lisp.Evaluate(`(import (group/secrets @native:gatehouse/group/secrets/v1) (group/secrets/read "gsc_00000000000000000000000000"))`, options)
	if err != nil || !lisp.IsSecret(result) || strings.Contains(result.String(), "CREDENTIAL_SENTINEL") { t.Fatalf("credential was not secret-tainted: %v %v", result, err) }
	if err, value := lisp.RequireBytes(result); err != nil || string(value) != "CREDENTIAL_SENTINEL" { t.Fatal("authorized tool could not use credential bytes") }
	allowed = false
	if err, _ := lisp.Evaluate(`(import (group/secrets @native:gatehouse/group/secrets/v1) (group/secrets/read "gsc_00000000000000000000000000"))`, options); err == nil { t.Fatal("read callback was not re-evaluated after revocation") }
	for _, code := range []string{`(group/secrets/list "extra")`, `(group/secrets/read)`, `(group/secrets/read "")`} {
		if err, _ := lisp.Evaluate(`(import (group/secrets @native:gatehouse/group/secrets/v1) `+code+`)`, options); err == nil { t.Fatalf("invalid call accepted: %s", code) }
	}
}
