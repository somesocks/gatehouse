package httpservice

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"gatehouse/auth"
	"gatehouse/authz"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/typed_id"
)

type systemProviderRequest struct { Alias string `json:"alias"`; Protocol string `json:"protocol"`; Endpoint *string `json:"endpoint"`; Keychain *string `json:"keychain"`; APIKey *string `json:"api_key"`; Credential *string `json:"credential"`; Enabled bool `json:"enabled"`; ExpectedRevision *int `json:"expected_revision"` }
type systemModelRequest struct { Alias string `json:"alias"`; Provider string `json:"provider"`; Model string `json:"model"`; Parameters string `json:"parameters"`; Compaction string `json:"compaction"`; MaxTurns int `json:"max_turns"`; MaxOutputTokens int `json:"max_output_tokens"`; ResultInlineBytes *int `json:"result_inline_bytes"`; Enabled bool `json:"enabled"`; ExpectedRevision *int `json:"expected_revision"` }
type systemStorageRequest struct { Alias string `json:"alias"`; Protocol string `json:"protocol"`; Endpoint *string `json:"endpoint"`; Region *string `json:"region"`; Bucket *string `json:"bucket"`; AccessKeyID *string `json:"access_key_id"`; Keychain *string `json:"keychain"`; SecretAccessKey *string `json:"secret_access_key"`; Enabled bool `json:"enabled"`; ExpectedRevision *int `json:"expected_revision"` }
type systemWorkspaceAgentCreateRequest struct { Alias string `json:"alias"`; Model string `json:"model"`; Label *string `json:"label"`; SystemPrompt *string `json:"system_prompt"`; Prelude *string `json:"prelude"`; RateLimits *config.WorkspaceAgentRateLimits `json:"rate_limits"`; Default bool `json:"default"`; Enabled bool `json:"enabled"` }
type systemWorkspaceAgentUpdateRequest struct { Model string `json:"model"`; Label *string `json:"label"`; SystemPrompt *string `json:"system_prompt"`; Prelude *string `json:"prelude"`; RateLimits *config.WorkspaceAgentRateLimits `json:"rate_limits"`; Default bool `json:"default"`; Enabled bool `json:"enabled"`; ExpectedRevision *int `json:"expected_revision"` }
type systemWorkspaceStorageRequest struct { Priority int `json:"priority"`; Enabled bool `json:"enabled"`; ExpectedRevision *int `json:"expected_revision"` }
type systemWorkspaceAgentSearchResponse struct { Agents []database.SystemWorkspaceAgent `json:"agents"`; NextCursor string `json:"next_cursor,omitempty"` }
type systemWorkspaceStorageSearchResponse struct { Bindings []database.SystemWorkspaceStorageProvider `json:"bindings"`; NextCursor string `json:"next_cursor,omitempty"` }

func systemKeychains(keyring *keychain.Keyring, store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc { return func(w http.ResponseWriter, r *http.Request) { if !systemManage(w,r,store,tokens) { return }; if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }; if keyring == nil { http.Error(w,"keychain administration unavailable",http.StatusServiceUnavailable); return }; err, refs := keyring.Current(r.Context()); if err != nil { http.Error(w,"internal server error",500); return }; writeJSON(w,refs) } }

func systemAgentProviders(store *database.Store, ring *keychain.Keyring, tokens *auth.BearerTokens) http.HandlerFunc { return func(w http.ResponseWriter, r *http.Request) { if !systemManage(w,r,store,tokens) { return }; if r.Method == http.MethodGet { if r.URL.Query().Has("limit") { systemAgentProvidersSearch(w,r,store); return }; err,v:=store.SystemAgentProvidersGet(r.Context()); adminResult(w,err,v); return }; if r.Method != http.MethodPost { w.WriteHeader(405); return }; var q systemProviderRequest; if !adminDecode(w,r,&q) { return }; input,err:=agentProviderInput(r.Context(),ring,q); if err != nil { adminError(w,err); return }; err,v:=store.SystemAgentProviderCreate(r.Context(),input); adminResult(w,err,v) } }
func systemAgentProvider(store *database.Store, ring *keychain.Keyring, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !systemManage(w,r,store,tokens) { return }
		id:=r.PathValue("provider")
		if r.Method == http.MethodGet {
			w.Header().Set("Cache-Control", "no-store")
			err,v:=store.SystemAgentProviderGet(r.Context(),id)
			if err!=nil { adminError(w,err); return }; if v==nil { http.NotFound(w,r); return }; writeJSON(w,v); return
		}
		if r.Method != http.MethodPatch { w.WriteHeader(405); return }
		var q systemProviderRequest
		if !adminDecode(w,r,&q) { return }
		if q.ExpectedRevision==nil { http.Error(w,"expected revision is required",400); return }
		err,current:=store.SystemAgentProviderGet(r.Context(),id)
		if err!=nil { adminError(w,err); return }; if current==nil { http.NotFound(w,r); return }
		input,err:=agentProviderInput(r.Context(),ring,q)
		if err!=nil { adminError(w,err); return }
		// A metadata edit preserves both the live credential and its actual key
		// version, even if a newer keychain version has become current.
		if q.APIKey==nil && q.Credential==nil && current.Keychain!=nil && input.Keychain!=nil && input.Keychain.Id==current.Keychain.Id { input.Keychain=current.Keychain }
		err,v:=store.SystemAgentProviderUpdate(r.Context(),id,*q.ExpectedRevision,input)
		adminResult(w,err,v)
	}
}

func systemAgentModels(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc { return func(w http.ResponseWriter,r *http.Request) { if !systemManage(w,r,store,tokens) { return }; if r.Method==http.MethodGet { if r.URL.Query().Has("limit") { systemAgentModelsSearch(w,r,store); return }; err,v:=store.SystemAgentModelsGet(r.Context()); adminResult(w,err,v); return }; if r.Method!=http.MethodPost { w.WriteHeader(405); return }; var q systemModelRequest; if !adminDecode(w,r,&q) { return }; err,v:=store.SystemAgentModelCreate(r.Context(),agentModelInput(q)); adminResult(w,err,v) } }
func systemAgentModel(store *database.Store,tokens *auth.BearerTokens) http.HandlerFunc { return func(w http.ResponseWriter,r *http.Request) { if !systemManage(w,r,store,tokens) { return }; id:=r.PathValue("model"); if r.Method==http.MethodGet { err,v:=store.SystemAgentModelGet(r.Context(),id); if err!=nil { adminError(w,err); return }; if v==nil { http.NotFound(w,r); return }; writeJSON(w,v); return }; if r.Method!=http.MethodPatch { w.WriteHeader(405); return }; var q systemModelRequest; if !adminDecode(w,r,&q)||q.ExpectedRevision==nil { if q.ExpectedRevision==nil { http.Error(w,"expected revision is required",400) }; return }; err,v:=store.SystemAgentModelUpdate(r.Context(),id,*q.ExpectedRevision,agentModelInput(q)); adminResult(w,err,v) } }

func systemStorageProviders(store *database.Store,ring *keychain.Keyring,tokens *auth.BearerTokens) http.HandlerFunc { return func(w http.ResponseWriter,r *http.Request) { if !systemManage(w,r,store,tokens) { return }; if r.Method==http.MethodGet { if r.URL.Query().Has("limit") { systemStorageProvidersSearch(w,r,store); return }; err,v:=store.SystemStorageProvidersGet(r.Context()); adminResult(w,err,v); return }; if r.Method!=http.MethodPost { w.WriteHeader(405); return }; var q systemStorageRequest; if !adminDecode(w,r,&q) { return }; input,err:=storageProviderInput(r.Context(),ring,q); if err!=nil { adminError(w,err); return }; err,v:=store.SystemStorageProviderCreate(r.Context(),input); adminResult(w,err,v) } }
func systemStorageProvider(store *database.Store,ring *keychain.Keyring,tokens *auth.BearerTokens) http.HandlerFunc { return func(w http.ResponseWriter,r *http.Request) { if !systemManage(w,r,store,tokens) { return }; id:=r.PathValue("provider"); if r.Method==http.MethodGet { err,v:=store.SystemStorageProviderGet(r.Context(),id); if err!=nil { adminError(w,err); return }; if v==nil { http.NotFound(w,r); return }; writeJSON(w,v); return }; if r.Method!=http.MethodPatch { w.WriteHeader(405); return }; var q systemStorageRequest; if !adminDecode(w,r,&q)||q.ExpectedRevision==nil { if q.ExpectedRevision==nil { http.Error(w,"expected revision is required",400) }; return }; input,err:=storageProviderInput(r.Context(),ring,q); if err!=nil { adminError(w,err); return }; err,v:=store.SystemStorageProviderUpdate(r.Context(),id,*q.ExpectedRevision,input); adminResult(w,err,v) } }

func systemWorkspaceAgents(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !systemManage(w, r, store, tokens) { return }
		if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
		workspaceID := r.URL.Query().Get("workspace")
		if !typed_id.Valid(typed_id.Workspace, workspaceID) { http.Error(w, "a valid workspace is required", http.StatusBadRequest); return }
		if err, workspace := store.SystemWorkspaceGet(r.Context(), workspaceID); err != nil { adminError(w, err); return } else if workspace == nil { http.NotFound(w, r); return }
		name, cursor, limit, ok := catalogSearchParameters(w, r)
		if !ok { return }
		if cursor != "" && !typed_id.Valid(typed_id.WorkspaceAgent, cursor) { http.Error(w, "invalid binding cursor", http.StatusBadRequest); return }
		err, agents, nextCursor := store.SystemWorkspaceAgentsSearch(r.Context(), database.SystemWorkspaceAgentSearch{WorkspaceID: workspaceID, Name: name, Cursor: cursor, Limit: limit})
		if err != nil { adminError(w, err); return }
		writeJSON(w, systemWorkspaceAgentSearchResponse{Agents: agents, NextCursor: nextCursor})
	}
}
func systemWorkspaceAgentCreate(store *database.Store,tokens *auth.BearerTokens) http.HandlerFunc { return func(w http.ResponseWriter,r *http.Request) { if !systemManage(w,r,store,tokens) { return }; if r.Method != http.MethodPost { w.WriteHeader(405); return }; var q systemWorkspaceAgentCreateRequest; if !adminDecode(w,r,&q) { return }; err,v:=store.SystemWorkspaceAgentCreate(r.Context(),r.PathValue("workspace"),database.SystemWorkspaceAgentInput{Alias:q.Alias,ModelID:q.Model,Label:q.Label,SystemPrompt:q.SystemPrompt,Prelude:q.Prelude,RateLimits:q.RateLimits,Default:q.Default,Enabled:q.Enabled}); adminResult(w,err,v) } }
func systemWorkspaceAgent(store *database.Store,tokens *auth.BearerTokens) http.HandlerFunc { return func(w http.ResponseWriter,r *http.Request) { if !systemManage(w,r,store,tokens) { return }; workspace,bindingID:=r.PathValue("workspace"),r.PathValue("binding"); if r.Method==http.MethodGet { err,v:=store.SystemWorkspaceAgentGet(r.Context(),workspace,bindingID); if err!=nil { adminError(w,err); return }; if v==nil { http.NotFound(w,r); return }; writeJSON(w,v); return }; if r.Method != http.MethodPatch { w.WriteHeader(405); return }; var q systemWorkspaceAgentUpdateRequest; if !adminDecode(w,r,&q) { return }; if q.ExpectedRevision==nil { http.Error(w,"expected revision is required",400); return }; input:=database.SystemWorkspaceAgentInput{ModelID:q.Model,Label:q.Label,SystemPrompt:q.SystemPrompt,Prelude:q.Prelude,RateLimits:q.RateLimits,Default:q.Default,Enabled:q.Enabled}; err,v:=store.SystemWorkspaceAgentUpdate(r.Context(),workspace,bindingID,*q.ExpectedRevision,input); adminResult(w,err,v) } }

func systemWorkspaceStorageProviders(store *database.Store, tokens *auth.BearerTokens) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !systemManage(w, r, store, tokens) { return }
		if r.Method != http.MethodGet { w.WriteHeader(http.StatusMethodNotAllowed); return }
		workspaceID := r.URL.Query().Get("workspace")
		if !typed_id.Valid(typed_id.Workspace, workspaceID) { http.Error(w, "a valid workspace is required", http.StatusBadRequest); return }
		if err, workspace := store.SystemWorkspaceGet(r.Context(), workspaceID); err != nil { adminError(w, err); return } else if workspace == nil { http.NotFound(w, r); return }
		name, cursor, limit, ok := catalogSearchParameters(w, r)
		if !ok { return }
		if cursor != "" && !typed_id.Valid(typed_id.StorageProvider, cursor) { http.Error(w, "invalid binding cursor", http.StatusBadRequest); return }
		err, bindings, nextCursor := store.SystemWorkspaceStorageProvidersSearch(r.Context(), database.SystemWorkspaceStorageProviderSearch{WorkspaceID: workspaceID, Name: name, Cursor: cursor, Limit: limit})
		if err != nil { adminError(w, err); return }
		writeJSON(w, systemWorkspaceStorageSearchResponse{Bindings: bindings, NextCursor: nextCursor})
	}
}
func systemWorkspaceStorageProvider(store *database.Store,tokens *auth.BearerTokens) http.HandlerFunc { return func(w http.ResponseWriter,r *http.Request) { if !systemManage(w,r,store,tokens) { return }; workspace,provider:=r.PathValue("workspace"),r.PathValue("provider"); if r.Method==http.MethodGet { err,v:=store.SystemWorkspaceStorageProviderGet(r.Context(),workspace,provider); if err!=nil { adminError(w,err); return }; if v==nil { http.NotFound(w,r); return }; writeJSON(w,v); return }; var q systemWorkspaceStorageRequest; if !adminDecode(w,r,&q) { return }; input:=database.SystemWorkspaceStorageProviderInput{Priority:q.Priority,Enabled:q.Enabled}; if r.Method==http.MethodPost { err,v:=store.SystemWorkspaceStorageProviderCreate(r.Context(),workspace,provider,input); adminResult(w,err,v); return }; if r.Method==http.MethodPatch&&q.ExpectedRevision!=nil { err,v:=store.SystemWorkspaceStorageProviderUpdate(r.Context(),workspace,provider,*q.ExpectedRevision,input); adminResult(w,err,v); return }; http.Error(w,"expected revision is required",400) } }

func systemManage(w http.ResponseWriter,r *http.Request,store *database.Store,tokens *auth.BearerTokens) bool { claims,ok:=authenticate(w,r,tokens); return ok&&systemActionAllowed(w,r,store,claims,authz.SystemManage) }
func adminDecode(w http.ResponseWriter,r *http.Request,target any) bool { d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,1<<20)); d.DisallowUnknownFields(); if d.Decode(target)!=nil { http.Error(w,"invalid system administration request",400); return false }; return true }
func adminResult(w http.ResponseWriter,err error,value any) { if err!=nil { adminError(w,err); return }; if value==nil { w.WriteHeader(http.StatusNotFound); return }; writeJSON(w,value) }
func adminError(w http.ResponseWriter,err error) { if errors.Is(err,database.ErrRevisionConflict)||errors.Is(err,database.ErrAgentProviderExists)||errors.Is(err,database.ErrAgentModelExists)||errors.Is(err,database.ErrStorageProviderExists)||errors.Is(err,database.ErrWorkspaceAgentExists)||errors.Is(err,database.ErrWorkspaceStorageProviderExists) { http.Error(w,err.Error(),409); return }; if errors.Is(err,database.ErrAdminDependencyMissing) { http.Error(w,err.Error(),404); return }; http.Error(w,err.Error(),400) }
func agentModelInput(q systemModelRequest) database.SystemAgentModelInput { return database.SystemAgentModelInput{Alias:q.Alias,ProviderID:q.Provider,Model:q.Model,Parameters:q.Parameters,Compaction:q.Compaction,MaxTurns:q.MaxTurns,MaxOutputTokens:q.MaxOutputTokens,ResultInlineBytes:q.ResultInlineBytes,Enabled:q.Enabled} }
func agentProviderInput(ctx context.Context,ring *keychain.Keyring,q systemProviderRequest) (database.SystemAgentProviderInput,error) {
	if q.APIKey != nil && q.Credential != nil { return database.SystemAgentProviderInput{}, fmt.Errorf("specify only credential or api_key") }
	plain := q.Credential
	if plain == nil { plain = q.APIKey }
	if q.Protocol == config.AgentProviderProtocolOpenAICodex && plain != nil { return database.SystemAgentProviderInput{}, fmt.Errorf("openai-codex credentials must be installed through login") }
	key,err:=systemCredential(ctx,ring,q.Keychain,plain,"gh=v1|agent-provider|alias="+q.Alias)
	if err!=nil{return database.SystemAgentProviderInput{},err}
	return database.SystemAgentProviderInput{Alias:q.Alias,Protocol:q.Protocol,Endpoint:q.Endpoint,Keychain:key.ref,EncryptedCredential:credentialText(key),Enabled:q.Enabled},nil
}
func storageProviderInput(ctx context.Context,ring *keychain.Keyring,q systemStorageRequest) (database.SystemStorageProviderInput,error) { key,err:=systemCredential(ctx,ring,q.Keychain,q.SecretAccessKey,"gh=v1|storage-provider|alias="+q.Alias); if err!=nil{return database.SystemStorageProviderInput{},err}; return database.SystemStorageProviderInput{Alias:q.Alias,Protocol:q.Protocol,Endpoint:q.Endpoint,Region:q.Region,Bucket:q.Bucket,AccessKeyID:q.AccessKeyID,Keychain:key.ref,EncryptedSecretAccessKey:key.text,Enabled:q.Enabled},nil }
type sealedCredential struct { ref *model.KeychainRef; text *string }
func credentialText(value sealedCredential) *string { return value.text }
func systemCredential(ctx context.Context,ring *keychain.Keyring,id *string,plain *string,aad string) (sealedCredential,error) { if id==nil { if plain!=nil { return sealedCredential{},fmt.Errorf("keychain is required") }; return sealedCredential{},nil }; if ring==nil { return sealedCredential{},fmt.Errorf("keychain administration unavailable") }; err,refs:=ring.Current(ctx); if err!=nil{return sealedCredential{},err}; var ref *model.KeychainRef; for _,candidate:=range refs { if candidate.Id==*id { copy:=candidate; ref=&copy; break } }; if ref==nil{return sealedCredential{},fmt.Errorf("configured keychain not found")}; if plain==nil{return sealedCredential{ref:ref},nil}; keysErr,keys:=ring.Get(ctx,[]model.KeychainRef{*ref}); if keysErr!=nil{return sealedCredential{},keysErr}; defer clear(keys[*ref]); encryptedErr,encrypted:=keychain.Seal(rand.Reader,keys[*ref],[]byte(aad),[]byte(*plain)); if encryptedErr!=nil{return sealedCredential{},encryptedErr}; encrypted.AAD=keychain.AADAlias; text:=encrypted.String(); return sealedCredential{ref:ref,text:&text},nil }
