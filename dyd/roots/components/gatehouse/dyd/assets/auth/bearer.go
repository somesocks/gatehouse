package auth

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gatehouse/database"
	"gatehouse/identity"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/storage"
	"gatehouse/typed_id"
)

var bearerAssociatedData = []byte("gatehouse bearer token v1")
var storageTokenAssociatedData = []byte("gatehouse storage token v1")
var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrUnauthenticated = errors.New("unauthenticated")

type Claims struct {
	Principal model.Principal `json:"principal"`
	Identity  string          `json:"identity"`
}

type StorageToken struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	ExpiresAt string `json:"expires_at"`
}

type BearerTokens struct {
	store         *database.Store
	keyring       *keychain.Keyring
	keychainID    string
	dummyVerifier string
}

func Prepare(ctx context.Context, store *database.Store, keyring *keychain.Keyring, keychainID string) (error, *BearerTokens) {
	err, dummyVerifier := identity.NewDummyPasswordVerifier()
	if err != nil {
		return fmt.Errorf("create dummy password verifier: %w", err), nil
	}
	return nil, &BearerTokens{store: store, keyring: keyring, keychainID: keychainID, dummyVerifier: dummyVerifier}
}

func (tokens *BearerTokens) StorageClient() *storage.Client {
	return storage.NewClient(tokens.store, tokens.keyring)
}

func (tokens *BearerTokens) Login(ctx context.Context, identityKey string, password []byte) (error, string) {
	err, active := tokens.store.ActiveIdentityGetByKey(ctx, identityKey)
	if err != nil {
		return err, ""
	}
	var valid bool
	var verified bool
	if active != nil {
		valid, verified = verifyPassword(active.Verifiers, password)
	}
	if !verified {
		_, _ = identity.VerifyPassword(tokens.dummyVerifier, password)
	}
	if active == nil || !valid {
		return ErrInvalidCredentials, ""
	}
	return tokens.Mint(ctx, Claims{Principal: active.Principal, Identity: active.ID})
}

func (tokens *BearerTokens) Mint(ctx context.Context, claims Claims) (error, string) {
	if claims.Principal.Ref.Id == "" || claims.Identity == "" {
		return fmt.Errorf("bearer claims must include principal and identity"), ""
	}
	err, current := tokens.store.KeychainsGetCurrent(ctx, []string{tokens.keychainID})
	if err != nil {
		return err, ""
	}
	if len(current) != 1 {
		return fmt.Errorf("get current bearer keychain %q: not found", tokens.keychainID), ""
	}
	keyReference := current[0].Ref
	payload, err := json.Marshal(claims)
	if err != nil {
		return fmt.Errorf("encode bearer claims: %w", err), ""
	}
	err, keys := tokens.keyring.Get(ctx, []model.KeychainRef{keyReference})
	if err != nil {
		return err, ""
	}
	key, ok := keys[keyReference]
	if !ok {
		return fmt.Errorf("get current bearer keychain %q: unavailable", tokens.keychainID), ""
	}
	defer clear(key)
	err, encrypted := keychain.Seal(rand.Reader, key, bearerAssociatedData, payload)
	if err != nil {
		return fmt.Errorf("seal bearer token: %w", err), ""
	}
	encrypted.Key = &keyReference
	return nil, encrypted.String()
}

func (tokens *BearerTokens) Authenticate(ctx context.Context, authorization string) (error, Claims) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authorization, prefix) || strings.Contains(authorization[len(prefix):], " ") {
		return fmt.Errorf("%w: invalid bearer authorization", ErrUnauthenticated), Claims{}
	}
	err, encrypted := keychain.ParseResource(strings.TrimPrefix(authorization, prefix))
	if err != nil {
		return fmt.Errorf("%w: parse bearer token: %v", ErrUnauthenticated, err), Claims{}
	}
	if encrypted.Key == nil || encrypted.Key.Id != tokens.keychainID {
		return fmt.Errorf("%w: bearer token uses another keychain", ErrUnauthenticated), Claims{}
	}
	err, stored := tokens.store.KeychainsGet(ctx, []model.KeychainRef{*encrypted.Key})
	if err != nil {
		return err, Claims{}
	}
	if len(stored) != 1 || !stored[0].Enabled {
		return fmt.Errorf("%w: bearer token uses an unavailable keychain version", ErrUnauthenticated), Claims{}
	}
	err, keys := tokens.keyring.Get(ctx, []model.KeychainRef{*encrypted.Key})
	if err != nil {
		return err, Claims{}
	}
	key, ok := keys[*encrypted.Key]
	if !ok {
		return fmt.Errorf("get bearer keychain %q version %d: unavailable", encrypted.Key.Id, encrypted.Key.Version), Claims{}
	}
	defer clear(key)
	err, payload := keychain.Open(key, bearerAssociatedData, encrypted)
	if err != nil {
		return fmt.Errorf("%w: open bearer token: %v", ErrUnauthenticated, err), Claims{}
	}
	var claims Claims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return fmt.Errorf("%w: decode bearer claims: %v", ErrUnauthenticated, err), Claims{}
	}
	if claims.Principal.Ref.Id == "" || claims.Identity == "" {
		return fmt.Errorf("%w: bearer claims must include principal and identity", ErrUnauthenticated), Claims{}
	}
	err, active := tokens.store.ActiveIdentityGetByID(ctx, claims.Identity)
	if err != nil {
		return err, Claims{}
	}
	if active == nil || active.Principal.Ref != claims.Principal.Ref {
		return fmt.Errorf("%w: bearer token identity is no longer active", ErrUnauthenticated), Claims{}
	}
	return nil, claims
}

func (tokens *BearerTokens) MintStorageToken(ctx context.Context, token StorageToken) (error, string) {
	if !typed_id.Valid(typed_id.StorageObject, token.ID) || (token.Action != "put" && token.Action != "get") || token.ExpiresAt == "" {
		return fmt.Errorf("storage token is invalid"), ""
	}
	return tokens.seal(ctx, storageTokenAssociatedData, token)
}

func (tokens *BearerTokens) AuthenticateStorageToken(ctx context.Context, encoded string) (error, StorageToken) {
	err, encrypted := keychain.ParseResource(encoded)
	if err != nil {
		return fmt.Errorf("parse storage token: %w", err), StorageToken{}
	}
	if encrypted.Key.Id != tokens.keychainID {
		return fmt.Errorf("storage token uses another keychain"), StorageToken{}
	}
	err, stored := tokens.store.KeychainsGet(ctx, []model.KeychainRef{*encrypted.Key})
	if err != nil {
		return err, StorageToken{}
	}
	if len(stored) != 1 || !stored[0].Enabled {
		return fmt.Errorf("storage token uses an unavailable keychain version"), StorageToken{}
	}
	err, keys := tokens.keyring.Get(ctx, []model.KeychainRef{*encrypted.Key})
	if err != nil {
		return err, StorageToken{}
	}
	key := keys[*encrypted.Key]
	decryptedErr, payload := keychain.Open(key, storageTokenAssociatedData, encrypted)
	clear(key)
	clear(keys)
	if decryptedErr != nil {
		return fmt.Errorf("open storage token: %w", decryptedErr), StorageToken{}
	}
	var token StorageToken
	if err := json.Unmarshal(payload, &token); err != nil || !typed_id.Valid(typed_id.StorageObject, token.ID) || (token.Action != "put" && token.Action != "get") || token.ExpiresAt == "" {
		return fmt.Errorf("decode storage token: invalid payload"), StorageToken{}
	}
	return nil, token
}

func (tokens *BearerTokens) seal(ctx context.Context, associatedData []byte, value any) (error, string) {
	err, current := tokens.store.KeychainsGetCurrent(ctx, []string{tokens.keychainID})
	if err != nil {
		return err, ""
	}
	if len(current) != 1 {
		return fmt.Errorf("get current keychain %q: not found", tokens.keychainID), ""
	}
	keyReference := current[0].Ref
	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode sealed payload: %w", err), ""
	}
	err, keys := tokens.keyring.Get(ctx, []model.KeychainRef{keyReference})
	if err != nil {
		return err, ""
	}
	key, ok := keys[keyReference]
	if !ok {
		return fmt.Errorf("get current keychain %q: unavailable", tokens.keychainID), ""
	}
	defer clear(key)
	err, encrypted := keychain.Seal(rand.Reader, key, associatedData, payload)
	if err != nil {
		return fmt.Errorf("seal payload: %w", err), ""
	}
	encrypted.Key = &keyReference
	return nil, encrypted.String()
}

func verifyPassword(verifiers []interface{}, password []byte) (bool, bool) {
	verified := false
	for _, verifier := range verifiers {
		value, ok := verifier.(string)
		if !ok || !strings.HasPrefix(value, "gh-ver:") {
			continue
		}
		if err, valid := identity.VerifyPassword(value, password); err == nil {
			verified = true
			if valid {
				return true, true
			}
		}
	}
	return false, verified
}
