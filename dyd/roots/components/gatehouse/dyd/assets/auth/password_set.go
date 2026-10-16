package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gatehouse/typed_id"
)

var passwordSetAssociatedData = []byte("gatehouse identity set-password capability v1")
var ErrPasswordLinkExpired = errors.New("password link has expired")

const PasswordLinkLifetime = 24 * time.Hour

// PasswordSetCapability authorizes only setting one identity's password. Its
// revision is consumed by the conditional password update, not by opening it.
type PasswordSetCapability struct {
	Purpose          string `json:"purpose"`
	Principal        string `json:"principal"`
	Identity         string `json:"identity"`
	IdentityRevision int    `json:"identity_revision"`
	ExpiresAt        int64  `json:"expires_at"`
}

func (tokens *BearerTokens) MintPasswordSet(ctx context.Context, principal, identity string, revision int) (error, string, PasswordSetCapability) {
	capability := PasswordSetCapability{
		Purpose: "identity.set-password", Principal: principal, Identity: identity,
		IdentityRevision: revision, ExpiresAt: time.Now().Add(PasswordLinkLifetime).Unix(),
	}
	if !validPasswordSet(capability) {
		return fmt.Errorf("invalid set-password capability claims"), "", PasswordSetCapability{}
	}
	err, encoded := tokens.seal(ctx, passwordSetAssociatedData, capability)
	return err, encoded, capability
}

func (tokens *BearerTokens) AuthenticatePasswordSet(ctx context.Context, authorization string) (error, PasswordSetCapability) {
	return tokens.authenticatePasswordSetAt(ctx, authorization, time.Now())
}

func (tokens *BearerTokens) authenticatePasswordSetAt(ctx context.Context, authorization string, now time.Time) (error, PasswordSetCapability) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) || len(authorization) == len(prefix) || strings.ContainsAny(authorization[len(prefix):], " \t\r\n") {
		return ErrUnauthenticated, PasswordSetCapability{}
	}
	err, payload := tokens.open(ctx, passwordSetAssociatedData, authorization[len(prefix):])
	if err != nil {
		return err, PasswordSetCapability{}
	}
	var capability PasswordSetCapability
	if err := json.Unmarshal(payload, &capability); err != nil || !validPasswordSet(capability) {
		return ErrUnauthenticated, PasswordSetCapability{}
	}
	if now.Unix() >= capability.ExpiresAt {
		return ErrPasswordLinkExpired, PasswordSetCapability{}
	}
	return nil, capability
}

func validPasswordSet(capability PasswordSetCapability) bool {
	return capability.Purpose == "identity.set-password" &&
		typed_id.Valid(typed_id.Principal, capability.Principal) &&
		typed_id.Valid(typed_id.Identity, capability.Identity) &&
		capability.IdentityRevision >= 0 && capability.ExpiresAt > 0
}
