package agent

import "gatehouse/lisp"

const gatehousePolicyModuleID = "native:gatehouse/policy/v1"

var (
	policyAwaitApprovalDocumentation   = capabilityDocumentation{"(policy/await-approval) -> Null", "Waits for approval of the current policy-wrapped operation.", "(policy/await-approval)", "null"}
	policyRequireApprovalDocumentation = capabilityDocumentation{"(policy/require-approval call) -> Function", "Returns a function that waits for approval before invoking call.", "((policy/require-approval session/notes/create) \"Decision\" \"Why this was decided\" \"# Decision\")", "((id . \"example-note-id\") (title . \"Decision\") (description . \"Why this was decided\") (author_id . \"example-principal-id\") (author_name) (created_at . \"2026-01-01T00:00:00.000Z\"))"}
)

const policyRequireApprovalSource = `(fn (call)
  (fn args
    (begin
      (await-approval)
      (apply call args))))`

// NewPolicyModule constructs the policy capability module for one agent evaluation.
func NewPolicyModule(await func() error) lisp.HostModule {
	awaitApproval := document(lisp.Function(func(arguments []lisp.Expr) (error, lisp.Expr) {
		if len(arguments) != 0 {
			return lisp.Errorf("policy/await-approval requires no arguments"), nil
		}
		if await == nil {
			return lisp.Errorf("policy/await-approval is unavailable"), nil
		}
		if err := await(); err != nil {
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
