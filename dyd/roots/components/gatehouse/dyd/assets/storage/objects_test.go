package storage_test

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"gatehouse/config"
	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/migrations"
	"gatehouse/model"
	"gatehouse/storage"
)

func TestClientStoresS3Objects(t *testing.T) {
	var mutex sync.Mutex
	var object []byte
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/gatehouse/object" {
			http.NotFound(response, request)
			return
		}
		if !strings.HasPrefix(request.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			http.Error(response, "missing AWS signature", http.StatusForbidden)
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		switch request.Method {
		case http.MethodPut:
			if request.Header.Get("Content-Encoding") != "aws-chunked" || request.Header.Get("X-Amz-Content-Sha256") != "STREAMING-AWS4-HMAC-SHA256-PAYLOAD" {
				http.Error(response, "missing streaming signature", http.StatusForbidden)
				return
			}
			var err error
			object, err = readStreamingObject(request.Body)
			if err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			response.WriteHeader(http.StatusOK)
		case http.MethodHead:
			response.Header().Set("Content-Length", strconv.Itoa(len(object)))
			response.WriteHeader(http.StatusOK)
		case http.MethodGet:
			contents := object
			if request.Header.Get("Range") == "bytes=6-10" {
				contents = object[6:11]
				response.WriteHeader(http.StatusPartialContent)
			}
			_, _ = response.Write(contents)
		default:
			response.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	ctx := context.Background()
	err, store := database.Open(ctx, config.DatabaseConfig{Kind: config.DatabaseKindEphemeral})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	t.Setenv("S3_TEST_KEYCHAIN", "test passphrase")
	t.Setenv("S3_TEST_SECRET", "test secret")
	keychains := []config.Keychain{{ID: "storage", Sources: []config.KeychainPassphraseSource{"env:S3_TEST_KEYCHAIN"}}}
	err, keyring := keychain.NewKeyring(store, keychains, keychain.NewPassphraseSourceResolver())
	if err != nil {
		t.Fatal(err)
	}
	defer keyring.Close()
	state := config.State{
		Keychains:  keychains,
		Workspaces: []config.Workspace{{Alias: "engineering", Enabled: true}},
		StorageProviders: []config.StorageProvider{{
			ID: "s3", Revision: 1, Protocol: "s3", Endpoint: &server.URL, Region: stringPointer("us-east-1"), Bucket: stringPointer("gatehouse"), AccessKeyID: stringPointer("access-key"), Keychain: stringPointer("storage"), SecretKeySources: []config.StorageProviderSecretKeySource{"env:S3_TEST_SECRET"}, Enabled: true,
		}},
		WorkspaceStorageProviders: []config.WorkspaceStorageProvider{{WorkspaceID: "engineering", Provider: "s3", Priority: 1, Enabled: true}},
	}
	err, set := migrations.Build(config.DatabaseConfig{Kind: config.DatabaseKindEphemeral}, state, keyring)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.Run(ctx, store, set); err != nil {
		t.Fatal(err)
	}
	workspace := workspaceRef(t, ctx, store, "engineering")
	if _, err := store.ExecContext(ctx, `INSERT INTO gatehouse_principals (id, enabled) VALUES ('alice', TRUE); INSERT INTO gatehouse_sessions (workspace, id, author_principal, enabled, created_at) VALUES (?, 'session', 'alice', TRUE, '2026-01-01 00:00:00');`, workspace.Id); err != nil {
		t.Fatal(err)
	}
	file := model.SessionFile{Ref: model.SessionFileRef{Session: model.SessionRef{Workspace: workspace, Id: "session"}, Id: "file"}, Name: "report.txt"}
	err, _, objectID := store.SessionFileCreate(ctx, file, "object")
	if err != nil {
		t.Fatal(err)
	}
	client := storage.NewClient(store, keyring)
	if err := client.Put(ctx, objectID, &chunkReader{data: []byte("hello, S3 world"), limit: 3}, int64(len("hello, S3 world"))); err != nil {
		t.Fatal(err)
	}
	if err := client.Finish(ctx, objectID); err != nil {
		t.Fatal(err)
	}
	err, content := client.Get(ctx, objectID)
	if err != nil {
		t.Fatal(err)
	}
	if content == nil {
		t.Fatal("S3 object is unavailable")
	}
	defer content.Close()
	bytes, err := io.ReadAll(content)
	if err != nil || string(bytes) != "hello, S3 world" {
		t.Fatalf("S3 object = (%q, %v)", bytes, err)
	}
	err, bytes = client.Read(ctx, objectID, 6, 5)
	if err != nil || string(bytes) != " S3 w" {
		t.Fatalf("S3 range = (%q, %v)", bytes, err)
	}
}

func stringPointer(value string) *string { return &value }

func workspaceRef(t *testing.T, ctx context.Context, store *database.Store, alias string) model.WorkspaceRef {
	t.Helper()
	err, workspace := store.WorkspaceRefGetByAlias(ctx, alias)
	if err != nil {
		t.Fatal(err)
	}
	if workspace == nil {
		t.Fatalf("workspace alias %q was not found", alias)
	}
	return *workspace
}

func readStreamingObject(source io.Reader) ([]byte, error) {
	reader := bufio.NewReader(source)
	var object bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		parts := strings.Split(strings.TrimSuffix(line, "\r\n"), ";chunk-signature=")
		if len(parts) != 2 || len(parts[1]) != 64 {
			return nil, io.ErrUnexpectedEOF
		}
		length, err := strconv.ParseInt(parts[0], 16, 64)
		if err != nil || length < 0 {
			return nil, io.ErrUnexpectedEOF
		}
		if length == 0 {
			if _, err := reader.ReadString('\n'); err != nil {
				return nil, err
			}
			return object.Bytes(), nil
		}
		if _, err := io.CopyN(&object, reader, length); err != nil {
			return nil, err
		}
		if trailer, err := reader.ReadString('\n'); err != nil || trailer != "\r\n" {
			return nil, io.ErrUnexpectedEOF
		}
	}
}

type chunkReader struct {
	data  []byte
	limit int
}

func (reader *chunkReader) Read(destination []byte) (int, error) {
	if len(reader.data) == 0 {
		return 0, io.EOF
	}
	count := min(len(destination), len(reader.data), reader.limit)
	copy(destination, reader.data[:count])
	reader.data = reader.data[count:]
	return count, nil
}
