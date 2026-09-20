package database

import (
	"strings"
	"testing"
)

func TestSessionEventSearchPostgresPredicateIncludesAgentReplies(t *testing.T) {
	predicate := sessionEventSearchTextPredicate("$1")
	if !strings.Contains(predicate, "events.kind IN ('message.text', 'agent.reply')") {
		t.Fatalf("sessionEventSearchTextPredicate() = %q", predicate)
	}
}
