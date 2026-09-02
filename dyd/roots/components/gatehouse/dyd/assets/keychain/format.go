package keychain

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strconv"

	"gatehouse/model"
)

const (
	kdfScheme        = "gh-kdf"
	encryptionScheme = "gh-enc"
	kdfAlgorithm     = "pbkdf2-hmac-sha256-v1"
	encryptionAlg    = "aes128-gcm-v1"
	saltSize         = 16
	AADID            = "id"
	AADAlias         = "alias"
)

var keyID = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

type KDF struct {
	Salt []byte
}

type Encrypted struct {
	Payload []byte
	Key     *model.KeychainRef
	AAD     string
}

func ParseKDF(value string) (error, KDF) {
	err, parsed := parseURI(value, kdfScheme)
	if err != nil {
		return err, KDF{}
	}
	if err := requireQuery(parsed, url.Values{"alg": []string{kdfAlgorithm}}); err != nil {
		return err, KDF{}
	}
	salt, err := base64.RawURLEncoding.DecodeString(parsed.Opaque)
	if err != nil || len(salt) != saltSize {
		return fmt.Errorf("%s salt must be a %d-byte base64url value", kdfScheme, saltSize), KDF{}
	}
	return nil, KDF{Salt: salt}
}

func (kdf KDF) String() string {
	return kdfScheme + ":" + base64.RawURLEncoding.EncodeToString(kdf.Salt) + "?" + url.Values{
		"alg": []string{kdfAlgorithm},
	}.Encode()
}

func ParseEncrypted(value string) (error, Encrypted) {
	err, parsed := parseURI(value, encryptionScheme)
	if err != nil {
		return err, Encrypted{}
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return fmt.Errorf("parse %s query: %w", encryptionScheme, err), Encrypted{}
	}
	if len(query["alg"]) != 1 || query.Get("alg") != encryptionAlg {
		return fmt.Errorf("%s requires alg=%s", encryptionScheme, encryptionAlg), Encrypted{}
	}
	key, hasKey := query["key"]
	version, hasVersion := query["ver"]
	if hasKey != hasVersion {
		return fmt.Errorf("%s key and ver must be specified together", encryptionScheme), Encrypted{}
	}
	expected := url.Values{"alg": []string{encryptionAlg}}
	var aad string
	if values, ok := query["aad"]; ok {
		if len(values) != 1 || (values[0] != AADID && values[0] != AADAlias) {
			return fmt.Errorf("%s aad must be %q or %q", encryptionScheme, AADID, AADAlias), Encrypted{}
		}
		aad = values[0]
		expected.Set("aad", aad)
	}
	var reference *model.KeychainRef
	if hasKey {
		if len(key) != 1 || !keyID.MatchString(key[0]) {
			return fmt.Errorf("%s key must match %q", encryptionScheme, keyID.String()), Encrypted{}
		}
		if len(version) != 1 {
			return fmt.Errorf("%s ver must be a positive integer", encryptionScheme), Encrypted{}
		}
		parsedVersion, err := strconv.Atoi(version[0])
		if err != nil || parsedVersion <= 0 || strconv.Itoa(parsedVersion) != version[0] {
			return fmt.Errorf("%s ver must be a positive canonical integer", encryptionScheme), Encrypted{}
		}
		reference = &model.KeychainRef{Id: key[0], Version: parsedVersion}
		expected.Set("key", key[0])
		expected.Set("ver", version[0])
	}
	if err := requireQuery(parsed, expected); err != nil {
		return err, Encrypted{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parsed.Opaque)
	if err != nil || len(payload) < encryptionOverhead {
		return fmt.Errorf("%s payload must contain a nonce and authentication tag", encryptionScheme), Encrypted{}
	}
	return nil, Encrypted{Payload: payload, Key: reference, AAD: aad}
}

func ParseKey(value string) (error, Encrypted) {
	err, encrypted := ParseEncrypted(value)
	if err != nil {
		return err, Encrypted{}
	}
	if encrypted.Key != nil {
		return fmt.Errorf("%s key encryption must not select a keychain key", encryptionScheme), Encrypted{}
	}
	return nil, encrypted
}

func ParseResource(value string) (error, Encrypted) {
	err, encrypted := ParseEncrypted(value)
	if err != nil {
		return err, Encrypted{}
	}
	if encrypted.Key == nil {
		return fmt.Errorf("%s resource encryption requires key and ver", encryptionScheme), Encrypted{}
	}
	return nil, encrypted
}

func (encrypted Encrypted) String() string {
	query := url.Values{"alg": []string{encryptionAlg}}
	if encrypted.Key != nil {
		query.Set("key", encrypted.Key.Id)
		query.Set("ver", strconv.Itoa(encrypted.Key.Version))
	}
	if encrypted.AAD != "" {
		query.Set("aad", encrypted.AAD)
	}
	return encryptionScheme + ":" + base64.RawURLEncoding.EncodeToString(encrypted.Payload) + "?" + query.Encode()
}

func parseURI(value, scheme string) (error, *url.URL) {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("parse %s: %w", scheme, err), nil
	}
	if parsed.Scheme != scheme || parsed.Opaque == "" || parsed.Fragment != "" || parsed.User != nil || parsed.Host != "" || parsed.Path != "" {
		return fmt.Errorf("invalid %s URI", scheme), nil
	}
	return nil, parsed
}

func requireQuery(parsed *url.URL, expected url.Values) error {
	if parsed.RawQuery != expected.Encode() {
		return fmt.Errorf("%s query must be canonical", parsed.Scheme)
	}
	return nil
}

func validPassphrase(value []byte) bool {
	return len(value) > 0
}
