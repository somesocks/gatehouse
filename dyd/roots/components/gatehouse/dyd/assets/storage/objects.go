package storage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"

	"gatehouse/database"
	"gatehouse/diagnostics"
	"gatehouse/keychain"
	"gatehouse/model"
	"gatehouse/s3"
)

func init() {
	diagnostics.Register("storage.read")
	diagnostics.Register("storage.s3.read")
	diagnostics.Register("storage.s3.credentials")
}

type Client struct {
	store   *database.Store
	keyring *keychain.Keyring
}

func NewClient(store *database.Store, keyring *keychain.Keyring) *Client {
	return &Client{store: store, keyring: keyring}
}

// Put uploads source. A negative size streams an unknown-length source.
func (client *Client) Put(ctx context.Context, id string, source io.Reader, size int64) error {
	err, provider := client.store.StorageObjectPendingGet(ctx, id)
	if err != nil {
		return err
	}
	if provider == nil || provider.Object.SHA256 != nil {
		return fmt.Errorf("upload storage object: unavailable")
	}
	switch provider.Protocol {
	case "embedded":
		return client.store.StorageObjectPutEmbedded(ctx, id, source)
	case "s3":
		err, configuration := client.s3Config(ctx, provider)
		if err != nil {
			return err
		}
		defer clear(configuration.SecretAccessKey)
		var digest []byte
		if size < 0 {
			digest, size, err = s3.PutUnknown(ctx, configuration, provider.Object.Object, source)
		} else {
			digest, err = s3.Put(ctx, configuration, provider.Object.Object, source, size)
		}
		if err != nil {
			return fmt.Errorf("upload S3 storage object: %w", err)
		}
		if err := client.store.StorageObjectStoreIntegrity(ctx, id, digest, size); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("upload storage object: unsupported provider protocol %q", provider.Protocol)
	}
}

func (client *Client) Finish(ctx context.Context, id string) error {
	err, provider := client.store.StorageObjectPendingGet(ctx, id)
	if err != nil {
		return err
	}
	if provider == nil {
		return fmt.Errorf("finish storage object: unavailable")
	}
	switch provider.Protocol {
	case "embedded":
		return client.store.StorageObjectFinishEmbedded(ctx, id)
	case "s3":
		if len(provider.Object.SHA256) != sha256.Size || provider.Object.Size < 0 {
			return fmt.Errorf("finish storage object: unavailable")
		}
		return client.store.StorageObjectMarkSuccess(ctx, id)
	default:
		return fmt.Errorf("finish storage object: unsupported provider protocol %q", provider.Protocol)
	}
}

func (client *Client) Get(ctx context.Context, id string) (error, io.ReadCloser) {
	err, provider := client.store.StorageObjectSuccessGet(ctx, id)
	if err != nil {
		return err, nil
	}
	if provider == nil {
		return nil, nil
	}
	switch provider.Protocol {
	case "embedded":
		return client.store.StorageObjectGetEmbedded(ctx, id)
	case "s3":
		err, configuration := client.s3Config(ctx, provider)
		if err != nil {
			return err, nil
		}
		defer clear(configuration.SecretAccessKey)
		content, err := s3.Get(ctx, configuration, provider.Object.Object, "")
		if err != nil {
			return fmt.Errorf("download S3 storage object: %w", err), nil
		}
		return nil, content
	default:
		return fmt.Errorf("download storage object: unsupported provider protocol %q", provider.Protocol), nil
	}
}

func (client *Client) Read(ctx context.Context, id string, offset, length int64) (error, []byte) {
	call, err := diagnostics.Begin("storage.read", id)
	if err != nil {
		return err, nil
	}
	err, data := client.read(ctx, id, offset, length)
	return call.End(err), data
}

func (client *Client) read(ctx context.Context, id string, offset, length int64) (error, []byte) {
	if offset < 0 || length < 1 || length > 64*1024 {
		return fmt.Errorf("read storage object: invalid range"), nil
	}
	err, provider := client.store.StorageObjectSuccessGet(ctx, id)
	if err != nil {
		return err, nil
	}
	if provider == nil {
		return fmt.Errorf("read storage object: unavailable"), nil
	}
	if offset >= provider.Object.Size {
		return nil, []byte{}
	}
	switch provider.Protocol {
	case "embedded":
		contentErr, content := client.store.StorageObjectGetEmbedded(ctx, id)
		if contentErr != nil || content == nil {
			if contentErr != nil {
				return contentErr, nil
			}
			return fmt.Errorf("read storage object: unavailable"), nil
		}
		defer content.Close()
		if _, err := io.CopyN(io.Discard, content, offset); err != nil {
			return fmt.Errorf("skip embedded storage object bytes: %w", err), nil
		}
		data, err := io.ReadAll(io.LimitReader(content, length))
		if err != nil {
			return fmt.Errorf("read embedded storage object bytes: %w", err), nil
		}
		return nil, data
	case "s3":
		call, diagnosticErr := diagnostics.Begin("storage.s3.read", provider.Object.Provider)
		if diagnosticErr != nil {
			return diagnosticErr, nil
		}
		err, configuration := client.s3Config(ctx, provider)
		if err != nil {
			return call.End(err), nil
		}
		defer clear(configuration.SecretAccessKey)
		end := offset + length - 1
		if end >= provider.Object.Size {
			end = provider.Object.Size - 1
		}
		content, err := s3.Get(ctx, configuration, provider.Object.Object, fmt.Sprintf("bytes=%d-%d", offset, end))
		if err != nil {
			return call.End(fmt.Errorf("read S3 storage object: %w", err)), nil
		}
		defer content.Close()
		data, err := io.ReadAll(io.LimitReader(content, length))
		if err != nil {
			return call.End(fmt.Errorf("read S3 storage object bytes: %w", err)), nil
		}
		return call.End(nil), data
	default:
		return fmt.Errorf("read storage object: unsupported provider protocol %q", provider.Protocol), nil
	}
}

func (client *Client) s3Config(ctx context.Context, provider *database.StorageObjectProvider) (error, s3.Config) {
	call, diagnosticErr := diagnostics.Begin("storage.s3.credentials", provider.Object.Provider)
	if diagnosticErr != nil {
		return diagnosticErr, s3.Config{}
	}
	err, configuration := client.s3ConfigRaw(ctx, provider)
	if err := call.End(err); err != nil {
		clear(configuration.SecretAccessKey)
		return err, s3.Config{}
	}
	return nil, configuration
}

func (client *Client) s3ConfigRaw(ctx context.Context, provider *database.StorageObjectProvider) (error, s3.Config) {
	if provider.Keychain == nil || provider.Endpoint == "" || provider.Region == "" || provider.Bucket == "" || provider.AccessKeyID == "" || provider.SecretAccessKey == "" {
		return fmt.Errorf("configure S3 storage provider %q: unavailable", provider.Object.Provider), s3.Config{}
	}
	err, encrypted := keychain.ParseKey(provider.SecretAccessKey)
	if err != nil {
		return fmt.Errorf("parse secret access key for storage provider %q: %w", provider.Object.Provider, err), s3.Config{}
	}
	err, keys := client.keyring.Get(ctx, []model.KeychainRef{*provider.Keychain})
	if err != nil {
		return fmt.Errorf("get keychain for storage provider %q: %w", provider.Object.Provider, err), s3.Config{}
	}
	key, ok := keys[*provider.Keychain]
	if !ok {
		clear(keys)
		return fmt.Errorf("get keychain for storage provider %q: unavailable", provider.Object.Provider), s3.Config{}
	}
	associatedDataErr, associatedData := storageProviderAssociatedData(provider.Object.Provider, provider.ProviderAlias, encrypted.AAD)
	if associatedDataErr != nil {
		clear(key)
		clear(keys)
		return fmt.Errorf("select secret access key AAD for storage provider %q: %w", provider.Object.Provider, associatedDataErr), s3.Config{}
	}
	decryptedErr, secret := keychain.Open(key, associatedData, encrypted)
	clear(key)
	clear(keys)
	if decryptedErr != nil {
		return fmt.Errorf("decrypt secret access key for storage provider %q: %w", provider.Object.Provider, decryptedErr), s3.Config{}
	}
	return nil, s3.Config{
		Endpoint: provider.Endpoint, Region: provider.Region, Bucket: provider.Bucket,
		AccessKeyID: provider.AccessKeyID, SecretAccessKey: secret,
	}
}

func storageProviderAssociatedData(id string, alias *string, selector string) (error, []byte) {
	switch selector {
	case keychain.AADID:
		return nil, []byte("gh=v1|storage-provider|id=" + id)
	case keychain.AADAlias:
		if alias == nil {
			return fmt.Errorf("storage provider alias is unavailable"), nil
		}
		return nil, []byte("gh=v1|storage-provider|alias=" + *alias)
	default:
		return fmt.Errorf("unsupported AAD selector %q", selector), nil
	}
}
