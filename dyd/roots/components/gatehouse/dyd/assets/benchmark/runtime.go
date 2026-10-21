package benchmark

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"gatehouse/agent"
	"gatehouse/benchmarkmodel"
	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"github.com/dbos-inc/dbos-transact-golang/dbos"
	_ "github.com/dbos-inc/dbos-transact-golang/dbos/driver/sqlite"
)

type nativeSelection struct {
	model, prompt, environment string
}

type nativeBinding struct {
	workspace model.WorkspaceRef
	agentID   string
}

type nativeRuntime struct {
	store     *database.Store
	keyring   *keychain.Keyring
	runtime   *agent.SessionEventReplyRuntime
	principal model.PrincipalRef
	bindings  map[nativeSelection]nativeBinding
	shutdown  func() error
}

func newNativeRuntime(ctx context.Context, prepared *Prepared, providers map[string]benchmarkmodel.Provider, models map[string]benchmarkmodel.Model, prompts map[string]benchmarkmodel.SystemPrompt, environments map[string]benchmarkmodel.LispEnvironment, credentials map[string][]byte, progress io.Writer) (resultErr error, result *nativeRuntime) {
	configuration := config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}
	err, store := database.Open(ctx, configuration)
	if err != nil {
		return err, nil
	}
	native := &nativeRuntime{store: store, bindings: map[nativeSelection]nativeBinding{}}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, native.close())
		}
	}()

	// Provision only selected combinations, independently of question/repetition
	// counts. Reuse each binding's workspace so the visible alias stays assistant
	// in every built-in prompt, without repeating infrastructure initialization.
	var selections []nativeSelection
	seen := map[nativeSelection]bool{}
	selectedModels := map[string]bool{}
	selectedProviders := map[string]bool{}
	for _, test := range prepared.configuration.Document.Tests {
		for _, modelAlias := range test.Matrix.Models {
			modelID := prepared.modelIDs[modelAlias]
			selectedModels[modelID] = true
			selectedProviders[models[modelID].ProviderId] = true
			for _, promptAlias := range test.Matrix.SystemPrompts {
				for _, environmentAlias := range test.Matrix.LispEnvironments {
					selection := nativeSelection{modelID, prepared.promptIDs[promptAlias], prepared.environmentIDs[environmentAlias]}
					if !seen[selection] {
						seen[selection] = true
						selections = append(selections, selection)
					}
				}
			}
		}
	}
	chainIDs := []string{config.SystemKeychainID}
	for id := range selectedProviders {
		if chain := providers[id].Keychain; chain != nil {
			chainIDs = append(chainIDs, *chain)
		}
	}
	err, native.keyring = keychain.NewEphemeralKeyring(store, chainIDs)
	if err != nil {
		return err, nil
	}
	principalAlias := "benchmark"
	state := config.State{
		Principals:       []config.Principal{{Alias: principalAlias, Enabled: true}},
		StorageProviders: []config.StorageProvider{{Alias: "embedded", Revision: 1, Protocol: "embedded", Enabled: true}},
	}
	for index := range selections {
		alias := fmt.Sprintf("benchmark-%d", index+1)
		state.Workspaces = append(state.Workspaces, config.Workspace{Alias: alias, Enabled: true})
		state.WorkspaceGrants = append(state.WorkspaceGrants, config.WorkspaceGrant{WorkspaceID: alias, PrincipalID: &principalAlias, Role: "contributor", Revision: 1, Enabled: true})
		state.WorkspaceStorageProviders = append(state.WorkspaceStorageProviders, config.WorkspaceStorageProvider{WorkspaceID: alias, ProviderAlias: "embedded", Revision: 1, Priority: 1, Enabled: true})
	}
	err, set := migrations.Build(configuration, state, native.keyring)
	if err != nil {
		return err, nil
	}
	if err := migrations.Run(ctx, store, set); err != nil {
		return err, nil
	}
	if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_principals WHERE alias = 'benchmark'`).Scan(&native.principal.Id); err != nil {
		return err, nil
	}
	providerIDs := map[string]string{}
	for _, provider := range prepared.Providers {
		if !selectedProviders[provider.Id] {
			continue
		}
		input := database.SystemAgentProviderInput{Alias: provider.Alias, Protocol: provider.Protocol, Endpoint: provider.Endpoint, Enabled: true}
		if provider.Protocol != "builtin" {
			chainID := config.SystemKeychainID
			if provider.Keychain != nil {
				chainID = *provider.Keychain
			}
			reference := model.KeychainRef{Id: chainID, Version: 1}
			err, keys := native.keyring.Get(ctx, []model.KeychainRef{reference})
			if err != nil {
				return err, nil
			}
			err, encrypted := keychain.Seal(rand.Reader, keys[reference], []byte("gh=v1|agent-provider|alias="+provider.Alias), credentials[provider.Id])
			for _, key := range keys {
				clear(key)
			}
			if err != nil {
				return err, nil
			}
			encrypted.AAD = keychain.AADAlias
			input.Keychain, input.EncryptedCredential = &reference, stringPointer(encrypted.String())
		}
		err, stored := store.SystemAgentProviderCreate(ctx, input)
		if err != nil {
			return err, nil
		}
		providerIDs[provider.Id] = stored.ID
	}
	modelIDs := map[string]string{}
	for _, selected := range prepared.Models {
		if !selectedModels[selected.Id] {
			continue
		}
		input, err := runtimeModel(selected)
		if err != nil {
			return err, nil
		}
		input.Alias, input.ProviderID = selected.Alias, providerIDs[selected.ProviderId]
		err, stored := store.SystemAgentModelCreate(ctx, input)
		if err != nil {
			return err, nil
		}
		modelIDs[selected.Id] = stored.ID
	}
	options := agent.SessionEventReplyOptions{SystemPromptReplacements: map[model.WorkspaceAgentRef]string{}}
	for index, selection := range selections {
		binding := nativeBinding{}
		if err := store.QueryRowContext(ctx, `SELECT id FROM gatehouse_workspaces WHERE alias = ?`, state.Workspaces[index].Alias).Scan(&binding.workspace.Id); err != nil {
			return err, nil
		}
		prompt := prompts[selection.prompt]
		input := database.SystemWorkspaceAgentInput{Alias: "assistant", ModelID: modelIDs[selection.model], Prelude: environments[selection.environment].Prelude, Default: true, Enabled: true}
		if prompt.Mode == "append" {
			input.SystemPrompt = prompt.Text
		}
		err, stored := store.SystemWorkspaceAgentCreate(ctx, binding.workspace.Id, input)
		if err != nil {
			return err, nil
		}
		binding.agentID = stored.ID
		native.bindings[selection] = binding
		if prompt.Mode == "replace" && prompt.Text != nil {
			options.SystemPromptReplacements[model.WorkspaceAgentRef{Workspace: binding.workspace, Id: binding.agentID}] = *prompt.Text
		}
	}
	durable, err := dbos.NewContext(context.Background(), dbos.Config{AppName: "gatehouse-benchmark", SQLiteSystemDB: store.DB, Logger: slog.New(slog.NewTextHandler(progress, nil))})
	if err != nil {
		return err, nil
	}
	err, native.runtime = agent.NewSessionEventReplyRuntimeWithOptions(durable, store, native.keyring, options)
	if err != nil {
		return err, nil
	}
	if err := dbos.Launch(durable); err != nil {
		return err, nil
	}
	native.shutdown = func() error { return dbos.Shutdown(durable, 5*time.Second) }
	return nil, native
}

func (native *nativeRuntime) close() error {
	var err error
	if native.shutdown != nil {
		err = native.shutdown()
	}
	if native.keyring != nil {
		native.keyring.Close()
	}
	return errors.Join(err, native.store.Close())
}
