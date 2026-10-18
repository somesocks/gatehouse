package agent

import (
	"strings"
	"testing"

	"gatehouse/model"
)

func TestTranscriptIncludesMessageRecipientSnapshots(t *testing.T) {
	alias, name := "team", `Review <team> & "friends"`
	event := model.SessionEvent{
		Kind: model.SessionEventKindMessageText,
		Payload: map[string]interface{}{"targets": []model.MessageTarget{
			{Ref: model.MessageTargetRef{Kind: "group", Id: "grp_00000000000000000000000000"}, Alias: &alias, Name: &name},
			{Ref: model.MessageTargetRef{Kind: "agent", Id: "wag_00000000000000000000000000"}},
		}},
	}
	text := transcriptMessageEvent(transcriptMessage{Event: event}, "Review this")
	for _, expected := range []string{`<recipient kind="group"`, `alias="team"`, `Review &lt;team&gt; &amp;`, `<recipient kind="agent"`, `<text>Review this</text>`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("transcript %q is missing %q", text, expected)
		}
	}
	if strings.Contains(text, `Review <team>`) {
		t.Fatalf("recipient label was not XML-escaped: %s", text)
	}
}
