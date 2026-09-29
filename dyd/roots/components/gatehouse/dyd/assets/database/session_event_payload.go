package database

import (
	"encoding/json"
	"fmt"
)

// SessionEventPayloadFrom converts a generated event payload record into the
// generic SessionEvent envelope representation.
func SessionEventPayloadFrom(value any) (map[string]interface{}, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode session event payload: %w", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return nil, fmt.Errorf("decode session event payload: %w", err)
	}
	if payload == nil {
		return nil, fmt.Errorf("encode session event payload: expected an object")
	}
	return payload, nil
}
