package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Fixtures are from the AWS signing test suite at awslabs/aws-c-auth commit
// 16c7289432839ca7e49132b68ad5a776e7279394, v4 header-signing cases.
func TestAWSSigV4Fixtures(t *testing.T) {
	fixtures := []struct {
		name, target, canonicalRequest, stringToSign, signature string
	}{
		{
			name: "get-vanilla", target: "/",
			canonicalRequest: "GET\n/\n\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			stringToSign: "AWS4-HMAC-SHA256\n20150830T123600Z\n20150830/us-east-1/service/aws4_request\nbb579772317eb040ac9ed261061d46c1f17a8133879d6129b6e1c25292927e63",
			signature: "5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31",
		}, {
			name: "get-utf8", target: "/\u1234",
			canonicalRequest: "GET\n/%E1%88%B4\n\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			stringToSign: "AWS4-HMAC-SHA256\n20150830T123600Z\n20150830/us-east-1/service/aws4_request\n2a0a97d02205e45ce2e994789806b19270cfbbb0921b278ccf58f5249ac42102",
			signature: "8318018e0b0f223aa2bbf98705b62bb787dc9c0e678f255a891fd03141be5d85",
		}, {
			name: "get-vanilla-query-order-encoded", target: "/?Param-3=Value3&Param=Value2&%E1%88%B4=Value1",
			canonicalRequest: "GET\n/\n%E1%88%B4=Value1&Param=Value2&Param-3=Value3\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			stringToSign: "AWS4-HMAC-SHA256\n20150830T123600Z\n20150830/us-east-1/service/aws4_request\n868294f5c38bd141c4972a373a76654f1418a8e4fc18b2e7903ae45e8ae0ec71",
			signature: "371d3713e185cc334048618a97f809c9ffe339c62934c032af5a0e595648fcac",
		},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodGet, "https://example.amazonaws.com"+fixture.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			result, err := signRequest(request, Config{
				Region: "us-east-1", AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: []byte("wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"),
			}, "service", emptySHA256, time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC), false)
			if err != nil {
				t.Fatal(err)
			}
			defer clear(result.key)
			if result.canonicalRequest != fixture.canonicalRequest {
				t.Fatalf("canonical request = %q", result.canonicalRequest)
			}
			if result.stringToSign != fixture.stringToSign {
				t.Fatalf("string to sign = %q", result.stringToSign)
			}
			if result.signature != fixture.signature {
				t.Fatalf("signature = %q", result.signature)
			}
			want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=" + fixture.signature
			if request.Header.Get("Authorization") != want {
				t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
			}
		})
	}
}

func TestSignFixedTime(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://s3.example.test/bucket/object", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Range", "bytes=6-10")
	key, signature, scope, timestamp, err := sign(request, Config{
		Region: "us-east-1", AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: []byte("test-secret"),
	}, emptySHA256, time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	if signature != "f8f36218ee29902d0d2f294e9469351438b81f6bebc9325362cb051bf957d69c" {
		t.Fatalf("signature = %q", signature)
	}
	if scope != "20260102/us-east-1/s3/aws4_request" || timestamp != "20260102T030405Z" {
		t.Fatalf("scope, timestamp = %q, %q", scope, timestamp)
	}
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20260102/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f8f36218ee29902d0d2f294e9469351438b81f6bebc9325362cb051bf957d69c"
	if request.Header.Get("Authorization") != want {
		t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
	}
}

func TestObjectURLUsesAWSPathEncoding(t *testing.T) {
	endpoint, err := objectURL("https://s3.example.test/api+v1//", "bucket", "draft+final @2;v=1/%")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Path != "/api+v1//bucket/draft+final @2;v=1/%" {
		t.Fatalf("Path = %q", endpoint.Path)
	}
	want := "/api%2Bv1//bucket/draft%2Bfinal%20%402%3Bv%3D1/%25"
	if endpoint.RawPath != want || canonicalURI(endpoint) != want {
		t.Fatalf("RawPath, canonical URI = %q, %q", endpoint.RawPath, canonicalURI(endpoint))
	}
	unicode, err := objectURL("https://s3.example.test", "bucket", "snowman-\U0001F642")
	if err != nil {
		t.Fatal(err)
	}
	if canonicalURI(unicode) != "/bucket/snowman-%F0%9F%99%82" {
		t.Fatalf("Unicode canonical URI = %q", canonicalURI(unicode))
	}
}

func TestGetSendsAWSPathEncoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.RequestURI != "/bucket/draft%2Bfinal%20%402%3Bv%3D1/%25" {
			t.Fatalf("RequestURI = %q", request.RequestURI)
		}
		_, _ = response.Write([]byte("ok"))
	}))
	defer server.Close()
	body, err := Get(context.Background(), testConfig(server.URL), "draft+final @2;v=1/%", "")
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
	if _, err := io.ReadAll(body); err != nil {
		t.Fatal(err)
	}
}

func TestOperationTransportDoesNotUseGlobalDefault(t *testing.T) {
	original := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
		panic("operation transport must not use http.DefaultTransport")
	})
	t.Cleanup(func() { http.DefaultTransport = original })
	transport := operationTransport()
	if !transport.DisableKeepAlives || transport.ForceAttemptHTTP2 || transport.Proxy == nil || transport.DialContext == nil {
		t.Fatalf("transport = %+v", transport)
	}
}

func TestGetRangeClosesOperationTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/bucket/object" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Range") != "bytes=6-10" || !request.Close {
			t.Fatalf("range, close = %q, %t", request.Header.Get("Range"), request.Close)
		}
		if !strings.HasPrefix(request.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=access/") {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		_, _ = response.Write([]byte("S3 world"))
	}))
	defer server.Close()
	body, err := Get(context.Background(), testConfig(server.URL), "object", "bytes=6-10")
	if err != nil {
		t.Fatal(err)
	}
	contents, err := io.ReadAll(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if string(contents) != "S3 world" {
		t.Fatalf("contents = %q", contents)
	}
}

func TestPutStreamsPayloadAndClosesOperationTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/bucket/object" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		if !request.Close || request.Header.Get("Content-Encoding") != "aws-chunked" || request.Header.Get("X-Amz-Content-Sha256") != streamingPayloadSHA256 {
			t.Fatalf("request headers = %+v, close = %t", request.Header, request.Close)
		}
		contents, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(contents), "5;chunk-signature=") || !strings.HasSuffix(string(contents), "\r\n\r\n") {
			t.Fatalf("stream = %q", contents)
		}
		response.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	digest, err := Put(context.Background(), testConfig(server.URL), "object", bytes.NewBufferString("hello"), 5)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte("hello"))
	if hex.EncodeToString(digest) != hex.EncodeToString(want[:]) {
		t.Fatalf("digest = %x", digest)
	}
}

func TestGetRejectsUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	_, err := Get(context.Background(), testConfig(server.URL), "object", "")
	if err == nil || !strings.Contains(err.Error(), "unexpected status 403 Forbidden") {
		t.Fatalf("Get() error = %v", err)
	}
}

func TestPutPropagatesSourceFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
	}))
	defer server.Close()
	_, err := Put(context.Background(), testConfig(server.URL), "object", failingReader{}, 1)
	if err == nil || !strings.Contains(err.Error(), "read S3 upload") {
		t.Fatalf("Put() error = %v", err)
	}
}

func testConfig(endpoint string) Config {
	return Config{
		Endpoint: endpoint, Region: "us-east-1", Bucket: "bucket", AccessKeyID: "access", SecretAccessKey: []byte("secret"),
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTripper roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}
