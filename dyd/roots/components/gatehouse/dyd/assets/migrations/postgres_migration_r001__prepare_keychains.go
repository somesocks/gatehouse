package migrations

import (
	"gatehouse/keychain"
)

func postgresMigrationR001PrepareKeychains(keyring *keychain.Keyring) RepeatableMigration {
	return RepeatableMigration{
		Index:       1,
		Description: "prepare_keychains",
		Builder:     keychainMigrationBuilder(keyring),
	}
}
