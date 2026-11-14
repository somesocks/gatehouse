package migrations

func sqliteMigrationV071ContextCacheHints() VersionedMigration {
	return VersionedMigration{
		Index: 71, Description: "context_cache_hints",
		Builder: staticMigrationBuilder(`
   CREATE TABLE gatehouse_context_cache_hints (
    workspace TEXT NOT NULL,
    session TEXT NOT NULL,
    cache_scope TEXT NOT NULL CHECK(length(trim(cache_scope))>0 AND length(CAST(cache_scope AS BLOB))<=1024),
    algorithm TEXT NOT NULL CHECK(length(trim(algorithm))>0 AND length(CAST(algorithm AS BLOB))<=1024),
    profile TEXT NOT NULL CHECK(length(trim(profile))>0 AND length(CAST(profile AS BLOB))<=1024),
    hint_key TEXT NOT NULL CHECK(length(trim(hint_key))>0 AND length(CAST(hint_key AS BLOB))<=1024),
    hint TEXT NOT NULL CHECK(length(CAST(hint AS BLOB))<=4096) CHECK(json_valid(hint)) CHECK(json_type(hint)='object'),
    last_used_at INTEGER NOT NULL,
    PRIMARY KEY(workspace,session,cache_scope,algorithm,profile,hint_key),
    FOREIGN KEY(workspace,session) REFERENCES gatehouse_sessions(workspace,id) ON DELETE CASCADE
   ) STRICT;
   CREATE INDEX gatehouse_context_cache_hints_recent ON gatehouse_context_cache_hints
    (workspace,session,cache_scope,algorithm,profile,last_used_at DESC,hint_key DESC);
  `),
	}
}
