package migrations

import (
	"gatehouse/config"
	"gatehouse/keychain"
)

func postgresMigrationR012ReconcileStorageProviders(state config.State, keyring *keychain.Keyring) RepeatableMigration {
	return RepeatableMigration{
		Index:       12,
		Description: "reconcile_storage_providers",
		Builder:     storageProviderMigrationBuilder(state.StorageProviders, keyring),
	}
}
