package agent

import (
	"testing"

	"gatehouse/lisp"
)

func TestAgentPreludeRequiresApprovalForSessionNoteCreate(t *testing.T) {
	err, result := lisp.Evaluate(`(error/value (error/catch (session/notes/create "Decision" "" "# Decision")))`, lisp.EvalOptions{
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
	if err != nil || result.String() != `"policy/await-approval is unavailable"` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestAgentPreludeRequiresApprovalForProjectNoteCreate(t *testing.T) {
	err, result := lisp.Evaluate(`(error/value (error/catch (project/notes/create "Decision" "" "# Decision")))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, &ProjectNotes{Create: func(string, string, string) (error, ProjectNote) {
				t.Fatal("project note create was called without approval")
				return nil, ProjectNote{}
			}}),
			NewSessionModule(nil, nil, nil),
			NewPolicyModule(nil),
		},
	})
	if err != nil || result.String() != `"policy/await-approval is unavailable"` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestAgentPreludeRequiresApprovalForProjectInfoSet(t *testing.T) {
	err, result := lisp.Evaluate(`(error/value (error/catch (project/info/set "Roadmap" "Current priorities")))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(&ProjectInfo{CreatedAt: "2026-01-01T00:00:00.000Z", Set: func(string, string) (error, ProjectInfo) {
				t.Fatal("project info set was called without approval")
				return nil, ProjectInfo{}
			}}, nil, nil),
			NewSessionModule(nil, nil, nil),
			NewPolicyModule(nil),
		},
	})
	if err != nil || result.String() != `"policy/await-approval is unavailable"` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestAgentPreludeHidesPolicyWrappers(t *testing.T) {
	approvals := 0
	err, result := lisp.Evaluate(`(error/value (error/catch ((policy/require-approval "Add numbers" +) 1 2)))`, lisp.EvalOptions{
		Prelude: agentPrelude,
		HostModules: []lisp.HostModule{
			NewProjectModule(nil, nil, nil),
			NewSessionModule(nil, nil, nil),
			NewPolicyModule(func(string) error { approvals++; return nil }),
		},
	})
	if err != nil || result.String() == `3` || approvals != 0 {
		t.Fatalf("Evaluate() = (%s, %v), approvals = %d", result, err, approvals)
	}
}

func TestGatehousePolicyModuleAwaitApprovalIsUnavailable(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (policy @native:gatehouse/policy/v1)
  (list
    (error/value (error/catch (policy/await-approval "Await an action")))
    (error/value (error/catch ((policy/require-approval "Add numbers" +) 1 2)))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewPolicyModule(nil)},
	})
	if err != nil || result.String() != `("policy/await-approval is unavailable" "policy/await-approval is unavailable")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}

func TestGatehousePolicyModuleAwaitApprovalUsesCallback(t *testing.T) {
	descriptions := []string{}
	err, result := lisp.Evaluate(`(import
  (policy @native:gatehouse/policy/v1)
  (list
    (policy/await-approval "Await an action")
    ((policy/require-approval "Add numbers" +) 1 2)))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{
			NewPolicyModule(func(description string) error {
				descriptions = append(descriptions, description)
				return nil
			}),
		},
	})
	if err != nil || result.String() != `(null 3)` || len(descriptions) != 2 || descriptions[0] != "Await an action" || descriptions[1] != "Add numbers" {
		t.Fatalf("Evaluate() = (%s, %v), descriptions = %#v", result, err, descriptions)
	}
}

func TestGatehousePolicyModuleRequiresNonBlankDescription(t *testing.T) {
	err, result := lisp.Evaluate(`(import
  (policy @native:gatehouse/policy/v1)
  (error/value (error/catch (policy/await-approval " "))))`, lisp.EvalOptions{
		HostModules: []lisp.HostModule{NewPolicyModule(func(string) error { t.Fatal("approval callback was called with blank description"); return nil })},
	})
	if err != nil || result.String() != `"policy/await-approval description must not be blank"` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}
