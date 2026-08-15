// Package s3 implements the small, stateless S3 surface Gatehouse needs.
package s3

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const streamingChunkSize = 64 * 1024
const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
const streamingPayloadSHA256 = "STREAMING-AWS4-HMAC-SHA256-PAYLOAD"

type Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey []byte
}

// Get fetches an object with an optional inclusive HTTP Range header. The
// returned body owns its operation-local transport and must be closed.
func Get(ctx context.Context, config Config, key, byteRange string) (io.ReadCloser, error) {
	request, err := newRequest(ctx, config, http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	if byteRange != "" {
		request.Header.Set("Range", byteRange)
	}
	keyBytes, _, _, _, err := sign(request, config, emptySHA256, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	defer clear(keyBytes)
	transport := operationTransport()
	response, err := operationClient(transport).Do(request)
	if err != nil {
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("request S3 object: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_ = response.Body.Close()
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("request S3 object: unexpected status %s", response.Status)
	}
	return &responseBody{body: response.Body, transport: transport}, nil
}

// Put streams an object using SigV4 chunked payload signing and returns the
// SHA-256 digest of the decoded source bytes.
func Put(ctx context.Context, config Config, key string, source io.Reader, size int64) ([]byte, error) {
	if size < 0 {
		return nil, fmt.Errorf("upload S3 object: declared size is required")
	}
	stream := &streamingReader{source: source, remaining: size, hash: sha256.New()}
	defer stream.clear()
	request, err := newRequest(ctx, config, http.MethodPut, key, stream)
	if err != nil {
		return nil, err
	}
	request.ContentLength = streamingLength(size)
	request.Header.Set("Content-Encoding", "aws-chunked")
	request.Header.Set("X-Amz-Content-Sha256", streamingPayloadSHA256)
	request.Header.Set("X-Amz-Decoded-Content-Length", strconv.FormatInt(size, 10))
	keyBytes, signature, scope, timestamp, err := sign(request, config, streamingPayloadSHA256, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	stream.signingKey = keyBytes
	stream.timestamp = timestamp
	stream.scope = scope
	stream.previous = signature

	transport := operationTransport()
	defer transport.CloseIdleConnections()
	response, err := operationClient(transport).Do(request)
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("upload S3 object: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("upload S3 object: unexpected status %s", response.Status)
	}
	return stream.hash.Sum(nil), nil
}

func newRequest(ctx context.Context, config Config, method, key string, body io.Reader) (*http.Request, error) {
	endpoint, err := objectURL(config.Endpoint, config.Bucket, key)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("create S3 request: %w", err)
	}
	return request, nil
}

func objectURL(rawEndpoint, bucket, key string) (*url.URL, error) {
	endpoint, err := url.Parse(rawEndpoint)
	if err != nil {
		return nil, fmt.Errorf("parse S3 endpoint: %w", err)
	}
	if (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" {
		return nil, fmt.Errorf("parse S3 endpoint: expected absolute HTTP(S) URL")
	}
	endpoint.Path = strings.TrimSuffix(endpoint.Path, "/") + "/" + bucket + "/" + key
	endpoint.RawPath = awsEscapePath(endpoint.Path)
	return endpoint, nil
}

func operationTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     false,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

func operationClient(transport *http.Transport) *http.Client {
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type responseBody struct {
	body      io.ReadCloser
	transport *http.Transport
	once      sync.Once
	err       error
}

func (body *responseBody) Read(destination []byte) (int, error) {
	return body.body.Read(destination)
}

func (body *responseBody) Close() error {
	body.once.Do(func() {
		body.err = body.body.Close()
		body.transport.CloseIdleConnections()
	})
	return body.err
}

func sign(request *http.Request, config Config, payloadSHA256 string, now time.Time) ([]byte, string, string, string, error) {
	result, err := signRequest(request, config, "s3", payloadSHA256, now, true)
	if err != nil {
		return nil, "", "", "", err
	}
	return result.key, result.signature, result.scope, result.timestamp, nil
}

type signingResult struct {
	key              []byte
	signature        string
	scope            string
	timestamp        string
	canonicalRequest string
	stringToSign     string
}

func signRequest(request *http.Request, config Config, service, payloadSHA256 string, now time.Time, includePayloadHeader bool) (signingResult, error) {
	if config.Region == "" || config.AccessKeyID == "" || len(config.SecretAccessKey) == 0 {
		return signingResult{}, fmt.Errorf("sign request: incomplete credentials")
	}
	now = now.UTC()
	date := now.Format("20060102")
	timestamp := now.Format("20060102T150405Z")
	scope := date + "/" + config.Region + "/" + service + "/aws4_request"
	if includePayloadHeader {
		request.Header.Set("X-Amz-Content-Sha256", payloadSHA256)
	}
	request.Header.Set("X-Amz-Date", timestamp)
	canonicalHeaders, signedHeaders := canonicalHeaders(request)
	canonicalRequest := strings.Join([]string{
		request.Method,
		canonicalURI(request.URL),
		canonicalQuery(request.URL),
		canonicalHeaders,
		signedHeaders,
		payloadSHA256,
	}, "\n")
	canonicalDigest := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", timestamp, scope, hex.EncodeToString(canonicalDigest[:])}, "\n")
	signingKey := signingKey(config.SecretAccessKey, date, config.Region, service)
	signature := hex.EncodeToString(hmacBytes(signingKey, stringToSign))
	request.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+config.AccessKeyID+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
	return signingResult{
		key: signingKey, signature: signature, scope: scope, timestamp: timestamp,
		canonicalRequest: canonicalRequest, stringToSign: stringToSign,
	}, nil
}

func canonicalHeaders(request *http.Request) (string, string) {
	headers := map[string][]string{"host": []string{request.URL.Host}}
	for name, values := range request.Header {
		name = strings.ToLower(name)
		if name == "authorization" {
			continue
		}
		headers[name] = append(headers[name], values...)
	}
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	var builder strings.Builder
	for _, name := range names {
		values := headers[name]
		for index, value := range values {
			values[index] = strings.Join(strings.Fields(value), " ")
		}
		builder.WriteString(name)
		builder.WriteByte(':')
		builder.WriteString(strings.Join(values, ","))
		builder.WriteByte('\n')
	}
	return builder.String(), strings.Join(names, ";")
}

func canonicalURI(endpoint *url.URL) string {
	path := awsEscapePath(endpoint.Path)
	if path == "" {
		return "/"
	}
	return path
}

func canonicalQuery(endpoint *url.URL) string {
	type pair struct{ name, value string }
	values := endpoint.Query()
	pairs := make([]pair, 0)
	for name, entries := range values {
		if len(entries) == 0 {
			pairs = append(pairs, pair{name: awsEscape(name)})
			continue
		}
		for _, value := range entries {
			pairs = append(pairs, pair{name: awsEscape(name), value: awsEscape(value)})
		}
	}
	sort.Slice(pairs, func(left, right int) bool {
		if pairs[left].name == pairs[right].name {
			return pairs[left].value < pairs[right].value
		}
		return pairs[left].name < pairs[right].name
	})
	encoded := make([]string, len(pairs))
	for index, pair := range pairs {
		encoded[index] = pair.name + "=" + pair.value
	}
	return strings.Join(encoded, "&")
}

func awsEscape(value string) string {
	return awsEscapeValue(value, false)
}

func awsEscapePath(value string) string {
	return awsEscapeValue(value, true)
}

func awsEscapeValue(value string, preserveSlash bool) string {
	var builder strings.Builder
	for _, character := range []byte(value) {
		if character == '/' && preserveSlash {
			builder.WriteByte(character)
			continue
		}
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || strings.ContainsRune("-_.~", rune(character)) {
			builder.WriteByte(character)
			continue
		}
		builder.WriteByte('%')
		builder.WriteString(strings.ToUpper(hex.EncodeToString([]byte{character})))
	}
	return builder.String()
}

func signingKey(secret []byte, date, region, service string) []byte {
	prefix := append([]byte("AWS4"), secret...)
	dateKey := hmacBytes(prefix, date)
	clear(prefix)
	regionKey := hmacBytes(dateKey, region)
	clear(dateKey)
	serviceKey := hmacBytes(regionKey, service)
	clear(regionKey)
	result := hmacBytes(serviceKey, "aws4_request")
	clear(serviceKey)
	return result
}

func hmacBytes(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func streamingLength(size int64) int64 {
	var length int64
	for remaining := size; remaining > 0; {
		chunk := min(remaining, int64(streamingChunkSize))
		length += int64(len(strconv.FormatInt(chunk, 16))+len(";chunk-signature=")+sha256.Size*2+len("\r\n")) + chunk + int64(len("\r\n"))
		remaining -= chunk
	}
	return length + int64(len("0;chunk-signature=")+sha256.Size*2+len("\r\n\r\n"))
}

type streamingReader struct {
	source                          io.Reader
	hash                            hash.Hash
	remaining                       int64
	signingKey                      []byte
	timestamp, scope, previous      string
	data                            []byte
	finished                        bool
}

func (reader *streamingReader) Read(destination []byte) (int, error) {
	for len(reader.data) == 0 {
		if reader.finished {
			return 0, io.EOF
		}
		if reader.remaining == 0 {
			reader.data = reader.chunk(nil)
			reader.finished = true
			continue
		}
		length := min(reader.remaining, int64(streamingChunkSize))
		chunk := make([]byte, length)
		if _, err := io.ReadFull(reader.source, chunk); err != nil {
			return 0, fmt.Errorf("read S3 upload: %w", err)
		}
		_, _ = reader.hash.Write(chunk)
		reader.remaining -= length
		reader.data = reader.chunk(chunk)
	}
	count := copy(destination, reader.data)
	reader.data = reader.data[count:]
	return count, nil
}

func (reader *streamingReader) chunk(data []byte) []byte {
	digest := sha256.Sum256(data)
	message := strings.Join([]string{"AWS4-HMAC-SHA256-PAYLOAD", reader.timestamp, reader.scope, reader.previous, emptySHA256, hex.EncodeToString(digest[:])}, "\n")
	signature := hex.EncodeToString(hmacBytes(reader.signingKey, message))
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

func (reader *streamingReader) clear() {
	clear(reader.signingKey)
	clear(reader.data)
	reader.signingKey = nil
	reader.data = nil
}
