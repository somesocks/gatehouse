package agent

import "testing"

func TestBuiltinReplyReturnsConfiguredFixedText(t *testing.T) {
	err, reply := BuiltinReply(BuiltinModelDummyFixedReply, `{"text":"Fallback reply."}`)
	if err != nil || reply != "Fallback reply." {
		t.Fatalf("BuiltinReply() = (%q, %v)", reply, err)
	}
}

func TestBuiltinReplyRejectsUnsupportedOrInvalidModels(t *testing.T) {
	for _, test := range []struct {
		model      string
		parameters string
	}{
		{model: "dummy.echo", parameters: `{"text":"Fallback reply."}`},
		{model: BuiltinModelDummyFixedReply, parameters: `{"text":""}`},
		{model: BuiltinModelDummyFixedReply, parameters: `{"text":"Fallback reply.","extra":true}`},
	} {
		if err, _ := BuiltinReply(test.model, test.parameters); err == nil {
			t.Fatalf("BuiltinReply(%q, %q) succeeded", test.model, test.parameters)
		}
	}
}
