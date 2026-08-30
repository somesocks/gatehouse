package agent

import (
	"strings"

	"gatehouse/lisp"
)

const gatehousePolicyModuleID = "native:gatehouse/policy/v1"

var (
	policyAwaitApprovalDocumentation   = capabilityDocumentation{"(policy/await-approval description) -> Null", "Requests action approval before continuing. Description must be a non-blank string shown to the user.", "(policy/await-approval \"Create a session note\")", "null"}
	policyRequireApprovalDocumentation = capabilityDocumentation{"(policy/require-approval description call) -> Function", "Returns a function that requests action approval with description before invoking call.", "((policy/require-approval \"Create a session note\" session/notes/create) \"Decision\" \"Why this was decided\" \"# Decision\")", "((id . \"example-note-id\") (title . \"Decision\") (description . \"Why this was decided\") (author_id . \"example-principal-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
)

const policyRequireApprovalSource = `(fn (description call)
  (fn args
    (begin
      (await-approval description)
      (fn/apply call args))))`

// NewPolicyModule constructs the policy capability module for one agent evaluation.
func NewPolicyModule(requestApproval func(description string) error) lisp.HostModule {
	awaitApproval := document(lisp.Function(func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 1 {
			return lisp.Errorf("policy/await-approval requires one description argument"), nil
		}
		if lisp.TaintOf(arguments[0]) != lisp.TaintNone {
			return lisp.Errorf("policy/await-approval description must not be sensitive"), nil
		}
		err, description := lisp.RequireString(arguments[0])
		if err != nil {
			return err, nil
		}
		description = strings.TrimSpace(description)
		if description == "" {
			return lisp.Errorf("policy/await-approval description must not be blank"), nil
		}
		if requestApproval == nil {
			return lisp.Errorf("policy/await-approval is unavailable"), nil
		}
		if err := requestApproval(description); err != nil {
			return err, nil
		}
		return nil, lisp.Null()
	}), policyAwaitApprovalDocumentation)
	err, requireApproval := lisp.Evaluate(policyRequireApprovalSource, lisp.EvalOptions{Bindings: []lisp.Binding{{Name: "await-approval", Value: awaitApproval}}})
	if err != nil {
		panic(err)
	}
	return lisp.HostModule{ID: gatehousePolicyModuleID, Exports: []lisp.HostExport{
		{Name: "await-approval", Value: awaitApproval},
		{Name: "require-approval", Value: document(requireApproval, policyRequireApprovalDocumentation)},
	}}
}
