package verification

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const proofLifetime = 5 * time.Minute

type Config struct {
	URL *url.URL
	Key []byte
}

func New(rawURL, rawKey string) (*Config, error) {
	if rawURL == "" && rawKey == "" {
		return nil, nil
	}
	if rawURL == "" || rawKey == "" {
		return nil, errors.New("VERIFY_URL and VERIFY_SIGNING_KEY must both be set")
	}
	key, err := hex.DecodeString(rawKey)
	if err != nil || len(key) != 32 {
		return nil, errors.New("VERIFY_SIGNING_KEY must be 64 hexadecimal characters")
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
		u.Fragment != "" || u.RawQuery != "" {
		return nil, errors.New("VERIFY_URL must be a public HTTPS URL without a query or fragment")
	}
	return &Config{URL: u, Key: key}, nil
}

func (v *Config) Ticket(kind string, userID, expiresAt int64, nonce string) string {
	payload := fmt.Sprintf("v1.%s.%d.%d.%s", kind, userID, expiresAt, nonce)
	mac := hmac.New(sha256.New, v.Key)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (v *Config) ParseProof(value string, userID int64, now int64) (string, bool) {
	if len(value) > 256 {
		return "", false
	}
	parts := strings.Split(value, ".")
	if len(parts) != 6 || parts[0] != "v1" || parts[1] != "p" ||
		parts[2] != strconv.FormatInt(userID, 10) {
		return "", false
	}
	expiresAt, err := strconv.ParseInt(parts[3], 10, 64)
	if err != nil || strconv.FormatInt(expiresAt, 10) != parts[3] ||
		expiresAt <= now || expiresAt > now+int64((proofLifetime+time.Minute).Seconds()) {
		return "", false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[4])
	if err != nil || len(nonce) != 16 || base64.RawURLEncoding.EncodeToString(nonce) != parts[4] {
		return "", false
	}
	received, err := base64.RawURLEncoding.DecodeString(parts[5])
	if err != nil || len(received) != sha256.Size ||
		base64.RawURLEncoding.EncodeToString(received) != parts[5] {
		return "", false
	}
	mac := hmac.New(sha256.New, v.Key)
	mac.Write([]byte(strings.Join(parts[:5], ".")))
	if !hmac.Equal(received, mac.Sum(nil)) {
		return "", false
	}
	return parts[4], true
}

func (v *Config) ChallengeURL(userID, expiresAt int64, nonce string) string {
	u := *v.URL
	query := u.Query()
	query.Set("challenge", v.Ticket("c", userID, expiresAt, nonce))
	u.RawQuery = query.Encode()
	return u.String()
}
