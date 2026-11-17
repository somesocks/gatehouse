package database_test

import (
	"gatehouse/config"
	"testing"
)

func TestPostgresConversationBranch(t *testing.T) {
	store := newPostgresIntegrationTestStore(t)
	store.SetMaxOpenConns(1)
	testConversationBranch(t, store, config.DatabaseConfig{Kind: config.DatabaseKindPostgres, URL: "env:GATEHOUSE_TEST_POSTGRES_DATABASE_URL"})
}
