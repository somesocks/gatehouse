package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"gatehouse/model"
	"gatehouse/typed_id"
)

var inputCapabilityAssociatedData = []byte("gatehouse input capability v1")
var formInputActions = []string{"read", "patch", "submit", "cancel"}

// InputCapability is an independently sealed, purpose-bound bearer credential.
// The regular login bearer cannot be used as an input capability or vice versa.
type InputCapability struct {
	Purpose   string   `json:"purpose"`
	Workspace string   `json:"workspace"`
	Session   string   `json:"session"`
	Input     string   `json:"input"`
	Principal string   `json:"principal"`
	Identity  string   `json:"identity"`
	Actions   []string `json:"actions"`
}

func (capability InputCapability) Request() model.SessionEventRef {
	return model.SessionEventRef{Session: model.SessionRef{Workspace: model.WorkspaceRef{Id: capability.Workspace}, Id: capability.Session}, Id: capability.Input}
}

func (tokens *BearerTokens) MintFormInput(ctx context.Context, request model.SessionEventRef, responder Claims) (error, string) {
	capability := InputCapability{
		Purpose: "form", Workspace: request.Session.Workspace.Id, Session: request.Session.Id,
		Input: request.Id, Principal: responder.Principal.Ref.Id, Identity: responder.Identity,
		Actions: formInputActions,
	}
	if !validFormInput(capability) {
		return fmt.Errorf("invalid form input capability claims"), ""
	}
	return tokens.seal(ctx, inputCapabilityAssociatedData, capability)
}

func (tokens *BearerTokens) AuthenticateFormInput(ctx context.Context, authorization string) (error, InputCapability, Claims) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) || len(authorization) == len(prefix) || strings.Contains(authorization[len(prefix):], " ") {
		return fmt.Errorf("%w: invalid input authorization", ErrUnauthenticated), InputCapability{}, Claims{}
	}
	err, payload := tokens.open(ctx, inputCapabilityAssociatedData, authorization[len(prefix):])
	if err != nil {
		return err, InputCapability{}, Claims{}
	}
	var capability InputCapability
	if err := json.Unmarshal(payload, &capability); err != nil || !validFormInput(capability) {
		return fmt.Errorf("%w: invalid form input capability", ErrUnauthenticated), InputCapability{}, Claims{}
	}
	err, active := tokens.store.ActiveIdentityGetByID(ctx, capability.Identity)
	if err != nil {
		return err, InputCapability{}, Claims{}
	}
	if active == nil || active.Principal.Ref.Id != capability.Principal {
		return fmt.Errorf("%w: form input identity is no longer active", ErrUnauthenticated), InputCapability{}, Claims{}
	}
	return nil, capability, Claims{Principal: active.Principal, Identity: active.ID}
}

func validFormInput(capability InputCapability) bool {
	return capability.Purpose == "form" &&
		typed_id.Valid(typed_id.Workspace, capability.Workspace) &&
		typed_id.Valid(typed_id.Session, capability.Session) &&
		typed_id.Valid(typed_id.SessionEvent, capability.Input) &&
		typed_id.Valid(typed_id.Principal, capability.Principal) &&
		typed_id.Valid(typed_id.Identity, capability.Identity) &&
		slices.Equal(capability.Actions, formInputActions)
}
