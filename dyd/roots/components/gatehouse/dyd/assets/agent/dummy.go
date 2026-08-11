package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

const BuiltinModelDummyFixedReply = "dummy.fixed-reply"

func BuiltinReply(model, parameters string) (error, string) {
	if model != BuiltinModelDummyFixedReply {
		return fmt.Errorf("unsupported builtin model %q", model), ""
	}
	var configured struct {
		Text string `json:"text"`
	}
	decoder := json.NewDecoder(strings.NewReader(parameters))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&configured); err != nil || strings.TrimSpace(configured.Text) == "" {
		return fmt.Errorf("invalid parameters for builtin model %q", model), ""
	}
	return nil, configured.Text
}
