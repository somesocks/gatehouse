package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"gatehouse/inputform"
	"gatehouse/model"
	"gatehouse/typed_id"
)

var inputCapabilityAssociatedData = []byte("gatehouse input capability v1")
var formInputActions = []string{"read", "patch", "submit", "cancel"}

// InputCapability is an independently sealed, purpose-bound bearer credential.
// The regular login bearer cannot be used as an input capability or vice versa.
type InputCapability struct {
	Purpose      string   `json:"purpose"`
	Workspace    string   `json:"workspace"`
	Session      string   `json:"session"`
	Input        string   `json:"input"`
	Principal    string   `json:"principal"`
	Identity     string   `json:"identity"`
	Actions      []string `json:"actions"`
	Path         []string `json:"path,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
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
	return tokens.authenticateInput(ctx, authorization, validFormInput)
}

func (tokens *BearerTokens) MintFieldInput(ctx context.Context, request model.SessionEventRef, responder Claims, path, capabilities []string) (error, string) {
	capability := InputCapability{
		Purpose: "field", Workspace: request.Session.Workspace.Id, Session: request.Session.Id,
		Input: request.Id, Principal: responder.Principal.Ref.Id, Identity: responder.Identity,
		Actions: []string{"read", "patch"}, Path: path, Capabilities: capabilities,
	}
	if !validFieldInput(capability) {
		return fmt.Errorf("invalid field input capability claims"), ""
	}
	return tokens.seal(ctx, inputCapabilityAssociatedData, capability)
}

func (tokens *BearerTokens) AuthenticateFieldInput(ctx context.Context, authorization string) (error, InputCapability, Claims) {
	return tokens.authenticateInput(ctx, authorization, validFieldInput)
}

func (tokens *BearerTokens) authenticateInput(ctx context.Context, authorization string, valid func(InputCapability) bool) (error, InputCapability, Claims) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) || len(authorization) == len(prefix) || strings.Contains(authorization[len(prefix):], " ") {
		return fmt.Errorf("%w: invalid input authorization", ErrUnauthenticated), InputCapability{}, Claims{}
	}
	err, payload := tokens.open(ctx, inputCapabilityAssociatedData, authorization[len(prefix):])
	if err != nil {
		return err, InputCapability{}, Claims{}
	}
	var capability InputCapability
	if err := json.Unmarshal(payload, &capability); err != nil || !valid(capability) {
		return fmt.Errorf("%w: invalid input capability", ErrUnauthenticated), InputCapability{}, Claims{}
	}
	err, active := tokens.store.ActiveIdentityGetByID(ctx, capability.Identity)
	if err != nil {
		return err, InputCapability{}, Claims{}
	}
	if active == nil || active.Principal.Ref.Id != capability.Principal {
		return fmt.Errorf("%w: input identity is no longer active", ErrUnauthenticated), InputCapability{}, Claims{}
	}
	return nil, capability, Claims{Principal: active.Principal, Identity: active.ID}
}

func validFormInput(capability InputCapability) bool {
	return capability.Purpose == "form" &&
		len(capability.Path) == 0 && len(capability.Capabilities) == 0 &&
		validInputScope(capability) && slices.Equal(capability.Actions, formInputActions)
}

func validFieldInput(capability InputCapability) bool {
	if capability.Purpose != "field" || !validInputScope(capability) || !slices.Equal(capability.Actions, []string{"read", "patch"}) || len(capability.Path) == 0 {
		return false
	}
	for _, segment := range capability.Path {
		if strings.TrimSpace(segment) == "" {
			return false
		}
	}
	seen := map[string]bool{}
	for _, name := range capability.Capabilities {
		if !inputform.ValidCapability(name) || seen[name] {
			return false
		}
		seen[name] = true
	}
	return true
}

func validInputScope(capability InputCapability) bool {
	return typed_id.Valid(typed_id.Workspace, capability.Workspace) &&
		typed_id.Valid(typed_id.Session, capability.Session) &&
		typed_id.Valid(typed_id.SessionEvent, capability.Input) &&
		typed_id.Valid(typed_id.Principal, capability.Principal) &&
		typed_id.Valid(typed_id.Identity, capability.Identity)
}
