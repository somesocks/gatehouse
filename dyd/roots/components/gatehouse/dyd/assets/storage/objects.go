package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsv4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"gatehouse/database"
	"gatehouse/keychain"
	"gatehouse/model"
)

type Client struct {
	store   *database.Store
	keyring *keychain.Keyring
}

func NewClient(store *database.Store, keyring *keychain.Keyring) *Client {
	return &Client{store: store, keyring: keyring}
}

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
		if size < 0 {
			return fmt.Errorf("upload S3 storage object: declared size is required")
		}
		err, digest := client.putS3(ctx, provider, source, size)
		if err != nil {
			return err
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
		err, s3Client := client.s3(ctx, provider)
		if err != nil {
			return err, nil
		}
		result, err := s3Client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(provider.Bucket), Key: aws.String(provider.Object.Object)})
		if err != nil {
			return fmt.Errorf("download S3 storage object: %w", err), nil
		}
		return nil, result.Body
	default:
		return fmt.Errorf("download storage object: unsupported provider protocol %q", provider.Protocol), nil
	}
}

func (client *Client) Read(ctx context.Context, id string, offset, length int64) (error, []byte) {
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
		err, s3Client := client.s3(ctx, provider)
		if err != nil {
			return err, nil
		}
		end := offset + length - 1
		if end >= provider.Object.Size {
			end = provider.Object.Size - 1
		}
		result, err := s3Client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(provider.Bucket), Key: aws.String(provider.Object.Object), Range: aws.String(fmt.Sprintf("bytes=%d-%d", offset, end))})
		if err != nil {
			return fmt.Errorf("read S3 storage object: %w", err), nil
		}
		defer result.Body.Close()
		data, err := io.ReadAll(io.LimitReader(result.Body, length))
		if err != nil {
			return fmt.Errorf("read S3 storage object bytes: %w", err), nil
		}
		return nil, data
	default:
		return fmt.Errorf("read storage object: unsupported provider protocol %q", provider.Protocol), nil
	}
}

func (client *Client) s3(ctx context.Context, provider *database.StorageObjectProvider) (error, *awss3.Client) {
	err, credentials := client.s3Credentials(ctx, provider)
	if err != nil {
		return err, nil
	}
	configuration := aws.Config{
		Region: provider.Region,
		Credentials: aws.NewCredentialsCache(aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) {
			return credentials, nil
		})),
	}
	return nil, awss3.NewFromConfig(configuration, func(options *awss3.Options) {
		options.BaseEndpoint = aws.String(provider.Endpoint)
		options.UsePathStyle = true
	})
}

func (client *Client) s3Credentials(ctx context.Context, provider *database.StorageObjectProvider) (error, aws.Credentials) {
	if provider.Keychain == nil || provider.Endpoint == "" || provider.Region == "" || provider.Bucket == "" || provider.AccessKeyID == "" || provider.SecretAccessKey == "" {
		return fmt.Errorf("configure S3 storage provider %q: unavailable", provider.Object.Provider), aws.Credentials{}
	}
	err, encrypted := keychain.ParseKey(provider.SecretAccessKey)
	if err != nil {
		return fmt.Errorf("parse secret access key for storage provider %q: %w", provider.Object.Provider, err), aws.Credentials{}
	}
	err, keys := client.keyring.Get(ctx, []model.KeychainRef{*provider.Keychain})
	if err != nil {
		return fmt.Errorf("get keychain for storage provider %q: %w", provider.Object.Provider, err), aws.Credentials{}
	}
	key, ok := keys[*provider.Keychain]
	if !ok {
		clear(keys)
		return fmt.Errorf("get keychain for storage provider %q: unavailable", provider.Object.Provider), aws.Credentials{}
	}
	decryptedErr, secret := keychain.Open(key, []byte("gh=v1|storage-provider="+provider.Object.Provider), encrypted)
	clear(key)
	clear(keys)
	if decryptedErr != nil {
		return fmt.Errorf("decrypt secret access key for storage provider %q: %w", provider.Object.Provider, decryptedErr), aws.Credentials{}
	}
	defer clear(secret)
	return nil, aws.Credentials{AccessKeyID: provider.AccessKeyID, SecretAccessKey: string(secret)}
}

const s3StreamingChunkSize = 64 * 1024
const s3EmptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func (client *Client) putS3(ctx context.Context, provider *database.StorageObjectProvider, source io.Reader, size int64) (error, []byte) {
	err, credentials := client.s3Credentials(ctx, provider)
	if err != nil {
		return err, nil
	}
	endpoint, err := url.Parse(provider.Endpoint)
	if err != nil {
		return fmt.Errorf("configure S3 storage provider %q: invalid endpoint: %w", provider.Object.Provider, err), nil
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/" + provider.Bucket + "/" + provider.Object.Object
	endpoint.RawPath = ""
	stream := &s3StreamingReader{source: source, remaining: size, hash: sha256.New()}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint.String(), stream)
	if err != nil {
		return fmt.Errorf("create S3 upload request: %w", err), nil
	}
	request.ContentLength = s3StreamingLength(size)
	request.Header.Set("Content-Encoding", "aws-chunked")
	request.Header.Set("X-Amz-Content-Sha256", "STREAMING-AWS4-HMAC-SHA256-PAYLOAD")
	request.Header.Set("X-Amz-Decoded-Content-Length", strconv.FormatInt(size, 10))
	now := time.Now().UTC()
	if err := awsv4.NewSigner().SignHTTP(ctx, credentials, request, "STREAMING-AWS4-HMAC-SHA256-PAYLOAD", "s3", provider.Region, now); err != nil {
		return fmt.Errorf("sign S3 upload request: %w", err), nil
	}
	seed, err := awsv4.GetSignedRequestSignature(request)
	if err != nil {
		return fmt.Errorf("get S3 upload signature: %w", err), nil
	}
	stream.signingKey = s3SigningKey(credentials.SecretAccessKey, now, provider.Region)
	stream.timestamp = now.Format("20060102T150405Z")
	stream.scope = now.Format("20060102") + "/" + provider.Region + "/s3/aws4_request"
	stream.previous = hex.EncodeToString(seed)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("upload S3 storage object: %w", err), nil
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("upload S3 storage object: unexpected status %s", response.Status), nil
	}
	return nil, stream.hash.Sum(nil)
}

func s3StreamingLength(size int64) int64 {
	var length int64
	for remaining := size; remaining > 0; {
		chunk := min(remaining, int64(s3StreamingChunkSize))
		length += int64(len(strconv.FormatInt(chunk, 16))+len(";chunk-signature=")+sha256.Size*2+len("\r\n")) + chunk + int64(len("\r\n"))
		remaining -= chunk
	}
	return length + int64(len("0;chunk-signature=")+sha256.Size*2+len("\r\n\r\n"))
}

type s3StreamingReader struct {
	source                          io.Reader
	hash                            hash.Hash
	remaining                       int64
	signingKey                      []byte
	timestamp, scope, previous      string
	data                            []byte
	finished                        bool
}

func (reader *s3StreamingReader) Read(destination []byte) (int, error) {
	for len(reader.data) == 0 {
		if reader.finished {
			return 0, io.EOF
		}
		if reader.remaining == 0 {
			reader.data = reader.chunk(nil)
			reader.finished = true
			continue
		}
		length := min(reader.remaining, int64(s3StreamingChunkSize))
		chunk := make([]byte, length)
		if _, err := io.ReadFull(reader.source, chunk); err != nil {
			return 0, fmt.Errorf("read S3 storage upload: %w", err)
		}
		_, _ = reader.hash.Write(chunk)
		reader.remaining -= length
		reader.data = reader.chunk(chunk)
	}
	count := copy(destination, reader.data)
	reader.data = reader.data[count:]
	return count, nil
}

func (reader *s3StreamingReader) chunk(data []byte) []byte {
	digest := sha256.Sum256(data)
	message := strings.Join([]string{"AWS4-HMAC-SHA256-PAYLOAD", reader.timestamp, reader.scope, reader.previous, s3EmptySHA256, hex.EncodeToString(digest[:])}, "\n")
	signature := hex.EncodeToString(s3HMAC(reader.signingKey, message))
	reader.previous = signature
	encoded := make([]byte, 0, len(data)+128)
	encoded = append(encoded, strconv.FormatInt(int64(len(data)), 16)...)
	encoded = append(encoded, ";chunk-signature="...)
	encoded = append(encoded, signature...)
	encoded = append(encoded, '\r', '\n')
	encoded = append(encoded, data...)
	encoded = append(encoded, '\r', '\n')
	return encoded
}

func s3SigningKey(secret string, time time.Time, region string) []byte {
	date := s3HMAC([]byte("AWS4"+secret), time.Format("20060102"))
	regionKey := s3HMAC(date, region)
	service := s3HMAC(regionKey, "s3")
	return s3HMAC(service, "aws4_request")
}

func s3HMAC(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
