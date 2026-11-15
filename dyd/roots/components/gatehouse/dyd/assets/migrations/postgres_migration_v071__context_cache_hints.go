package migrations

func postgresMigrationV071ContextCacheHints() VersionedMigration {
	return VersionedMigration{Index: 71, Description: "context_cache_hints", Builder: staticMigrationBuilder(`
 CREATE TABLE gatehouse_context_cache_hints (
  workspace TEXT NOT NULL,
  session TEXT NOT NULL,
  cache_scope TEXT NOT NULL CHECK(length(trim(cache_scope))>0 AND octet_length(cache_scope)<=1024),
  algorithm TEXT NOT NULL CHECK(length(trim(algorithm))>0 AND octet_length(algorithm)<=1024),
  profile TEXT NOT NULL CHECK(length(trim(profile))>0 AND octet_length(profile)<=1024),
  hint_key TEXT NOT NULL CHECK(length(trim(hint_key))>0 AND octet_length(hint_key)<=1024),
  hint TEXT NOT NULL CHECK(octet_length(hint)<=4096) CHECK(json_typeof(hint::json)='object'),
  last_used_at BIGINT NOT NULL,
  PRIMARY KEY(workspace,session,cache_scope,algorithm,profile,hint_key),
  FOREIGN KEY(workspace,session) REFERENCES gatehouse_sessions(workspace,id) ON DELETE CASCADE
 );
 CREATE INDEX gatehouse_context_cache_hints_recent ON gatehouse_context_cache_hints
 (workspace,session,cache_scope,algorithm,profile,last_used_at DESC,hint_key DESC);
 `)}
}
