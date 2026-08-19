package agent

import (
	"testing"

	"gatehouse/lisp"
)

func TestAgentPreludeRequiresApprovalForSessionNoteCreate(t *testing.T) {
	err, result := lisp.Evaluate(`(list
  (error/value (error/catch (policy/await-approval)))
  (error/value (error/catch ((policy/require-approval +) 1 2)))
  (error/value (error/catch (session/notes/create "Decision" "" "# Decision"))))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, nil),
			NewSessionModule(nil, nil, &SessionNotes{Create: func(string, string, string) (error, ProjectNote) {
				t.Fatal("session note create was called without approval")
				return nil, ProjectNote{}
			}}),
			NewPolicyModule(nil),
		},
	})
	if err != nil || result.String() != `("policy/await-approval is unavailable" "policy/await-approval is unavailable" "policy/await-approval is unavailable")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehousePolicyModuleAwaitApprovalIsUnavailable(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (policy @native:gatehouse/policy/v1)
  (list
    (error/value (error/catch (policy/await-approval)))
    (error/value (error/catch ((policy/require-approval +) 1 2)))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewPolicyModule(nil)},
	})
	if err != nil || result.String() != `("policy/await-approval is unavailable" "policy/await-approval is unavailable")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehousePolicyModuleAwaitApprovalUsesCallback(t *testing.T) {
	awaits := 0
	err, result := lisp.Evaluate(`(list
  (policy/await-approval)
  ((policy/require-approval +) 1 2))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, nil),
			NewSessionModule(nil, nil, nil),
			NewPolicyModule(func() error {
				awaits++
				return nil
			}),
		},
	})
	if err != nil || result.String() != `(null 3)` || awaits != 2 {
		t.Fatalf("Evaluate() = (%s, %v), awaits = %d", result, err, awaits)
	}
}
