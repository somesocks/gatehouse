package database

import (
	"strings"
	"testing"
)

func TestSessionEventSearchPostgresPredicateIncludesAgentSuccesses(t *testing.T) {
	predicate := sessionEventSearchTextPredicate("$1")
	if !strings.Contains(predicate, "events.kind IN ('message.text', 'agent.success')") {
		t.Fatalf("sessionEventSearchTextPredicate() = %q", predicate)
	}
}
