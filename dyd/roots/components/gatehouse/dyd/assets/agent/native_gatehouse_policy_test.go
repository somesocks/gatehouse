package agent

import (
	"testing"

	"gatehouse/lisp"
)

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
