package httpservice

import (
	"bytes"
	"encoding/json"
	"fmt"

	"gatehouse/apischema"
	"gatehouse/model"
)

type transcriptEventTreeResponse struct {
	Event    any                            `json:"event"`
	Children []*transcriptEventTreeResponse `json:"children"`
}

func transcriptEventWire(event model.SessionEvent) (error, any) {
	encoded, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal transcript event %q: %w", event.Ref.Id, err), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var input any
	if err := decoder.Decode(&input); err != nil {
		return fmt.Errorf("decode transcript event %q: %w", event.Ref.Id, err), nil
	}
	err, typed := apischema.DecodeTranscriptEvent(input)
	if err != nil {
		return fmt.Errorf("type transcript event %q (%s): %w", event.Ref.Id, event.Kind, err), nil
	}
	err, wire := apischema.EncodeTranscriptEvent(typed)
	if err != nil {
		return fmt.Errorf("encode transcript event %q (%s): %w", event.Ref.Id, event.Kind, err), nil
	}
	return nil, wire
}

func transcriptEventTrees(trees []*sessionEventTreeResponse) (error, []*transcriptEventTreeResponse) {
	encoded := make([]*transcriptEventTreeResponse, 0, len(trees))
	for _, tree := range trees {
		err, event := transcriptEventWire(tree.Event)
		if err != nil {
			return err, nil
		}
		err, children := transcriptEventTrees(tree.Children)
		if err != nil {
			return err, nil
		}
		encoded = append(encoded, &transcriptEventTreeResponse{Event: event, Children: children})
	}
	return nil, encoded
}
