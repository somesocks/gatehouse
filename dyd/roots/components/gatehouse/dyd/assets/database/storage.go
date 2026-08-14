package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"time"

	"gatehouse/model"
)

const (
	EmbeddedStorageChunkSize = 256 * 1024
	EmbeddedStorageMaxSize   = 64 * 1024 * 1024
)

type StorageObject struct {
	ID       string
	Provider string
	Object   string
	State    string
	SHA256   []byte
	Size     int64
}

type StorageObjectProvider struct {
	Object          StorageObject
	Protocol        string
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	Keychain        *model.KeychainRef
	SecretAccessKey string
}

type SessionFileSummary struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	MediaType   *string `json:"media_type,omitempty"`
	Size        int64   `json:"size"`
	Fingerprint string  `json:"fingerprint"`
}

func (store *Store) SessionFileSnapshots(ctx context.Context, transaction *sql.Tx, session model.SessionRef, ids []string) (error, []SessionFileSummary) {
	if len(ids) == 0 {
		return nil, []SessionFileSummary{}
	}
	placeholder := keychainPlaceholder(store.kind)
	snapshots := make([]SessionFileSummary, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("session file ID must not be blank"), nil
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("session file ID %q is duplicated", id), nil
		}
		seen[id] = struct{}{}
		row := transaction.QueryRowContext(ctx, `
			SELECT files.name, files.media_type, objects.size, objects.sha256
			FROM gatehouse_session_files AS files
			JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object
			WHERE files.workspace = `+placeholder(1)+` AND files.session = `+placeholder(2)+` AND files.id = `+placeholder(3)+` AND objects.state = 'success'
		`, session.Workspace.Id, session.Id, id)
		var snapshot SessionFileSummary
		snapshot.ID = id
		var digest []byte
		var mediaType sql.NullString
		if err := row.Scan(&snapshot.Name, &mediaType, &snapshot.Size, &digest); err != nil {
			if err == sql.ErrNoRows {
				return fmt.Errorf("session file %q is unavailable", id), nil
			}
			return fmt.Errorf("get session file %q: %w", id, err), nil
		}
		if mediaType.Valid {
			snapshot.MediaType = &mediaType.String
		}
		snapshot.Fingerprint = "sha256:" + hex.EncodeToString(digest)
		snapshots = append(snapshots, snapshot)
	}
	return nil, snapshots
}

func (store *Store) SessionFilesGet(ctx context.Context, session model.SessionRef) (error, []SessionFileSummary) {
	placeholder := keychainPlaceholder(store.kind)
	rows, err := store.QueryContext(ctx, `
		SELECT files.id, files.name, files.media_type, objects.size, objects.sha256
		FROM gatehouse_session_files AS files
		JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object
		WHERE files.workspace = `+placeholder(1)+` AND files.session = `+placeholder(2)+` AND objects.state = 'success'
		ORDER BY files.created_at, files.id
	`, session.Workspace.Id, session.Id)
	if err != nil {
		return fmt.Errorf("get session files: %w", err), nil
	}
	defer rows.Close()
	files := []SessionFileSummary{}
	for rows.Next() {
		var file SessionFileSummary
		var mediaType sql.NullString
		var digest []byte
		if err := rows.Scan(&file.ID, &file.Name, &mediaType, &file.Size, &digest); err != nil {
			return fmt.Errorf("scan session file: %w", err), nil
		}
		if mediaType.Valid {
			file.MediaType = &mediaType.String
		}
		file.Fingerprint = "sha256:" + hex.EncodeToString(digest)
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate session files: %w", err), nil
	}
	return nil, files
}

func (store *Store) SessionFileCreate(ctx context.Context, file model.SessionFile, storageObjectID string) (error, model.SessionFile, string) {
	if strings.TrimSpace(file.Ref.Id) == "" || strings.TrimSpace(file.Name) == "" || strings.TrimSpace(storageObjectID) == "" {
		return fmt.Errorf("create session file: IDs and name must not be blank"), model.SessionFile{}, ""
	}
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session file creation: %w", err), model.SessionFile{}, ""
	}
	defer transaction.Rollback()
	placeholder := keychainPlaceholder(store.kind)
	row := transaction.QueryRowContext(ctx, `
		SELECT bindings.provider, providers.protocol
		FROM gatehouse_workspace_storage_providers AS bindings
		JOIN gatehouse_storage_providers AS providers ON providers.id = bindings.provider
		WHERE bindings.workspace = `+placeholder(1)+`
			AND bindings.enabled = TRUE
			AND providers.enabled = TRUE
		ORDER BY bindings.priority DESC, bindings.provider
		LIMIT 1
	`, file.Ref.Session.Workspace.Id)
	var provider, protocol string
	if err := row.Scan(&provider, &protocol); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("create session file: no available storage provider"), model.SessionFile{}, ""
		}
		return fmt.Errorf("select storage provider: %w", err), model.SessionFile{}, ""
	}
	file.CreatedAt = time.Now().UTC().Truncate(time.Millisecond).Format("2006-01-02T15:04:05.000Z")
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_storage_objects (id, provider, object, state, created_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, 'pending', `+placeholder(4)+`)
	`, storageObjectID, provider, storageObjectID, file.CreatedAt); err != nil {
		return fmt.Errorf("insert storage object: %w", err), model.SessionFile{}, ""
	}
	if protocol == "embedded" {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO gatehouse_embedded_storage_objects (id) VALUES (`+placeholder(1)+`)
		`, storageObjectID); err != nil {
			return fmt.Errorf("insert embedded storage object: %w", err), model.SessionFile{}, ""
		}
	}
	var mediaType any
	if file.MediaType != nil {
		mediaType = *file.MediaType
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO gatehouse_session_files (workspace, session, id, storage_object, name, media_type, created_at)
		VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`, `+placeholder(6)+`, `+placeholder(7)+`)
	`, file.Ref.Session.Workspace.Id, file.Ref.Session.Id, file.Ref.Id, storageObjectID, file.Name, mediaType, file.CreatedAt); err != nil {
		return fmt.Errorf("insert session file: %w", err), model.SessionFile{}, ""
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session file creation: %w", err), model.SessionFile{}, ""
	}
	file.StorageObject = model.StorageObjectRef{Id: storageObjectID}
	return nil, file, storageObjectID
}

func (store *Store) StorageObjectPendingGet(ctx context.Context, id string) (error, *StorageObjectProvider) {
	return store.storageObjectGet(ctx, id, "pending")
}

func (store *Store) StorageObjectSuccessGet(ctx context.Context, id string) (error, *StorageObjectProvider) {
	return store.storageObjectGet(ctx, id, "success")
}

func (store *Store) storageObjectGet(ctx context.Context, id, state string) (error, *StorageObjectProvider) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT objects.id, objects.provider, objects.object, objects.state, objects.sha256, objects.size,
			providers.protocol, providers.endpoint, providers.region, providers.bucket, providers.access_key_id,
			providers.keychain_id, providers.keychain_version, providers.secret_access_key
		FROM gatehouse_storage_objects AS objects
		JOIN gatehouse_storage_providers AS providers ON providers.id = objects.provider
		WHERE objects.id = `+placeholder(1)+` AND objects.state = `+placeholder(2), id, state)
	var stored StorageObjectProvider
	var digest []byte
	var size sql.NullInt64
	var endpoint, region, bucket, accessKeyID, keychainID, secretAccessKey sql.NullString
	var keychainVersion sql.NullInt64
	if err := row.Scan(&stored.Object.ID, &stored.Object.Provider, &stored.Object.Object, &stored.Object.State, &digest, &size,
		&stored.Protocol, &endpoint, &region, &bucket, &accessKeyID, &keychainID, &keychainVersion, &secretAccessKey); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get storage object: %w", err), nil
	}
	if size.Valid {
		stored.Object.SHA256 = digest
		stored.Object.Size = size.Int64
	}
	if endpoint.Valid {
		stored.Endpoint = endpoint.String
		stored.Region = region.String
		stored.Bucket = bucket.String
		stored.AccessKeyID = accessKeyID.String
		stored.SecretAccessKey = secretAccessKey.String
		stored.Keychain = &model.KeychainRef{Id: keychainID.String, Version: int(keychainVersion.Int64)}
	}
	return nil, &stored
}

func (store *Store) StorageObjectStoreIntegrity(ctx context.Context, id string, digest []byte, size int64) error {
	if len(digest) != sha256.Size || size < 0 {
		return fmt.Errorf("store storage object integrity: invalid digest or size")
	}
	placeholder := keychainPlaceholder(store.kind)
	result, err := store.ExecContext(ctx, `
		UPDATE gatehouse_storage_objects SET sha256 = `+placeholder(1)+`, size = `+placeholder(2)+`
		WHERE id = `+placeholder(3)+` AND state = 'pending' AND sha256 IS NULL AND size IS NULL
	`, digest, size, id)
	if err != nil {
		return fmt.Errorf("store storage object integrity: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store storage object integrity: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("store storage object integrity: unavailable")
	}
	return nil
}

func (store *Store) StorageObjectMarkSuccess(ctx context.Context, id string) error {
	placeholder := keychainPlaceholder(store.kind)
	result, err := store.ExecContext(ctx, `
		UPDATE gatehouse_storage_objects SET state = 'success'
		WHERE id = `+placeholder(1)+` AND state = 'pending' AND sha256 IS NOT NULL AND size IS NOT NULL
	`, id)
	if err != nil {
		return fmt.Errorf("finish storage object: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("finish storage object: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("finish storage object: unavailable")
	}
	return nil
}

func (store *Store) SessionFileGet(ctx context.Context, file model.SessionFileRef, principal model.PrincipalRef) (error, *model.SessionFile, *StorageObject) {
	err, session := store.SessionGet(ctx, file.Session, principal)
	if err != nil || session == nil {
		return err, nil, nil
	}
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT files.storage_object, files.name, files.media_type, files.created_at, objects.provider, objects.object, objects.state, objects.sha256, objects.size
		FROM gatehouse_session_files AS files
		JOIN gatehouse_storage_objects AS objects ON objects.id = files.storage_object
		WHERE files.workspace = `+placeholder(1)+` AND files.session = `+placeholder(2)+` AND files.id = `+placeholder(3)+`
	`, file.Session.Workspace.Id, file.Session.Id, file.Id)
	stored := model.SessionFile{Ref: file}
	object := &StorageObject{ID: ""}
	var mediaType sql.NullString
	var digest []byte
	var size sql.NullInt64
	if err := row.Scan(&stored.StorageObject.Id, &stored.Name, &mediaType, &stored.CreatedAt, &object.Provider, &object.Object, &object.State, &digest, &size); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return fmt.Errorf("get session file: %w", err), nil, nil
	}
	object.ID = stored.StorageObject.Id
	if mediaType.Valid {
		stored.MediaType = &mediaType.String
	}
	if size.Valid {
		object.SHA256 = digest
		object.Size = size.Int64
	}
	return nil, &stored, object
}

func (store *Store) StorageObjectPutEmbedded(ctx context.Context, id string, source io.Reader) error {
	placeholder := keychainPlaceholder(store.kind)
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin embedded storage upload: %w", err)
	}
	defer transaction.Rollback()
	var embeddedID string
	err = transaction.QueryRowContext(ctx, `
		SELECT objects.object
		FROM gatehouse_storage_objects AS objects
		JOIN gatehouse_embedded_storage_objects AS embedded ON embedded.id = objects.object
		JOIN gatehouse_storage_providers AS providers ON providers.id = objects.provider
		WHERE objects.id = `+placeholder(1)+` AND providers.protocol = 'embedded' AND objects.state = 'pending'
	`, id).Scan(&embeddedID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("upload embedded storage object: unavailable")
		}
		return fmt.Errorf("get embedded storage object: %w", err)
	}
	var count int
	if err := transaction.QueryRowContext(ctx, `SELECT COUNT(*) FROM gatehouse_embedded_storage_object_chunks WHERE embedded_storage_object = `+placeholder(1), embeddedID).Scan(&count); err != nil {
		return fmt.Errorf("count embedded storage chunks: %w", err)
	}
	if count != 0 {
		return fmt.Errorf("upload embedded storage object: already uploaded")
	}
	limited := io.LimitReader(source, EmbeddedStorageMaxSize+1)
	buffer := make([]byte, EmbeddedStorageChunkSize)
	hash := sha256.New()
	var size int64
	for ordinal := 0; ; ordinal++ {
		read, readErr := io.ReadFull(limited, buffer)
		if read > 0 {
			chunk := append([]byte(nil), buffer[:read]...)
			digest := sha256.Sum256(chunk)
			if _, err := hash.Write(chunk); err != nil {
				return fmt.Errorf("hash embedded storage object: %w", err)
			}
			size += int64(read)
			if size > EmbeddedStorageMaxSize {
				return fmt.Errorf("upload embedded storage object: exceeds %d byte limit", EmbeddedStorageMaxSize)
			}
			if _, err := transaction.ExecContext(ctx, `
				INSERT INTO gatehouse_embedded_storage_object_chunks (embedded_storage_object, ordinal, sha256, size, bytes)
				VALUES (`+placeholder(1)+`, `+placeholder(2)+`, `+placeholder(3)+`, `+placeholder(4)+`, `+placeholder(5)+`)
			`, embeddedID, ordinal, digest[:], read, chunk); err != nil {
				return fmt.Errorf("insert embedded storage chunk: %w", err)
			}
		}
		if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read embedded storage upload: %w", readErr)
		}
	}
	if _, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_storage_objects SET sha256 = `+placeholder(1)+`, size = `+placeholder(2)+`
		WHERE id = `+placeholder(3)+` AND state = 'pending'
	`, hash.Sum(nil), size, id); err != nil {
		return fmt.Errorf("store embedded storage integrity: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit embedded storage upload: %w", err)
	}
	return nil
}

func (store *Store) StorageObjectFinishEmbedded(ctx context.Context, id string) error {
	placeholder := keychainPlaceholder(store.kind)
	transaction, err := store.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin storage object finish: %w", err)
	}
	defer transaction.Rollback()
	var embeddedID string
	var storedDigest []byte
	var storedSize int64
	err = transaction.QueryRowContext(ctx, `
		SELECT objects.object, objects.sha256, objects.size
		FROM gatehouse_storage_objects AS objects
		JOIN gatehouse_embedded_storage_objects AS embedded ON embedded.id = objects.object
		JOIN gatehouse_storage_providers AS providers ON providers.id = objects.provider
		WHERE objects.id = `+placeholder(1)+` AND providers.protocol = 'embedded' AND objects.state = 'pending' AND objects.sha256 IS NOT NULL AND objects.size IS NOT NULL
	`, id).Scan(&embeddedID, &storedDigest, &storedSize)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("finish storage object: unavailable")
		}
		return fmt.Errorf("get storage object to finish: %w", err)
	}
	rows, err := transaction.QueryContext(ctx, `
		SELECT sha256, size, bytes FROM gatehouse_embedded_storage_object_chunks
		WHERE embedded_storage_object = `+placeholder(1)+` ORDER BY ordinal
	`, embeddedID)
	if err != nil {
		return fmt.Errorf("get embedded storage chunks to finish: %w", err)
	}
	hash := sha256.New()
	var size int64
	for rows.Next() {
		var digest, bytes []byte
		var chunkSize int64
		if err := rows.Scan(&digest, &chunkSize, &bytes); err != nil {
			rows.Close()
			return fmt.Errorf("read embedded storage chunk to finish: %w", err)
		}
		computed := sha256.Sum256(bytes)
		if string(computed[:]) != string(digest) || int64(len(bytes)) != chunkSize {
			rows.Close()
			return fmt.Errorf("finish storage object: embedded chunk integrity check failed")
		}
		_, _ = hash.Write(bytes)
		size += int64(len(bytes))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate embedded storage chunks to finish: %w", err)
	}
	rows.Close()
	if string(hash.Sum(nil)) != string(storedDigest) || size != storedSize {
		return fmt.Errorf("finish storage object: embedded object integrity check failed")
	}
	result, err := transaction.ExecContext(ctx, `
		UPDATE gatehouse_storage_objects SET state = 'success'
		WHERE id = `+placeholder(1)+` AND state = 'pending' AND sha256 IS NOT NULL AND size IS NOT NULL
	`, id)
	if err != nil {
		return fmt.Errorf("finish storage object: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("finish storage object: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("finish storage object: unavailable")
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit storage object finish: %w", err)
	}
	return nil
}

func (store *Store) StorageObjectGetEmbedded(ctx context.Context, id string) (error, io.ReadCloser) {
	placeholder := keychainPlaceholder(store.kind)
	row := store.QueryRowContext(ctx, `
		SELECT objects.object
		FROM gatehouse_storage_objects AS objects
		JOIN gatehouse_embedded_storage_objects AS embedded ON embedded.id = objects.object
		JOIN gatehouse_storage_providers AS providers ON providers.id = objects.provider
		WHERE objects.id = `+placeholder(1)+` AND providers.protocol = 'embedded' AND objects.state = 'success'
	`, id)
	var embeddedID string
	if err := row.Scan(&embeddedID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return fmt.Errorf("get embedded storage object: %w", err), nil
	}
	rows, err := store.QueryContext(ctx, `
		SELECT bytes FROM gatehouse_embedded_storage_object_chunks
		WHERE embedded_storage_object = `+placeholder(1)+` ORDER BY ordinal
	`, embeddedID)
	if err != nil {
		return fmt.Errorf("get embedded storage chunks: %w", err), nil
	}
	return nil, &embeddedStorageReader{rows: rows}
}

type embeddedStorageReader struct {
	rows *sql.Rows
	data []byte
}

func (reader *embeddedStorageReader) Read(destination []byte) (int, error) {
	for len(reader.data) == 0 {
		if !reader.rows.Next() {
			if err := reader.rows.Err(); err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		if err := reader.rows.Scan(&reader.data); err != nil {
			return 0, err
		}
	}
	read := copy(destination, reader.data)
	reader.data = reader.data[read:]
	return read, nil
}

func (reader *embeddedStorageReader) Close() error {
	return reader.rows.Close()
}
