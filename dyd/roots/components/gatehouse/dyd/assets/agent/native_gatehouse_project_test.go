package agent

import (
	"testing"

	"gatehouse/lisp"
)

func TestGatehouseProjectModuleIsAvailableWithoutProject(t *testing.T) {
	err, result := lisp.Evaluate(`(list
  (project/info)
  (project/files/list)
  (project/notes/list)
  (error/value (error/catch (project/files/read "guide" 0 1)))
  (error/value (error/catch (project/notes/read "guide" 0 1))))`, lisp.EvalOptions{
		Prelude:     agentPrelude,
		HostModules: []lisp.HostModule{NewProjectModule(nil, nil, nil), NewSessionModule(nil, nil, nil), NewPolicyModule()},
	})
	if err != nil || result.String() != `(null null null "project/files/read is unavailable" "project/notes/read is unavailable")` {
		t.Fatalf("Evaluate() = (%s, %v)", result, err)
	}
}
