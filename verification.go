package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const challengeLifetime = 10 * time.Minute
const proofLifetime = 5 * time.Minute
const promptCooldown = 30 * time.Second

type VerificationConfig struct {
	URL *url.URL
	Key []byte
}

func newVerificationConfig(rawURL, rawKey string) (*VerificationConfig, error) {
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
	return &VerificationConfig{URL: u, Key: key}, nil
}

func (v *VerificationConfig) ticket(kind string, userID, expiresAt int64, nonce string) string {
	payload := fmt.Sprintf("v1.%s.%d.%d.%s", kind, userID, expiresAt, nonce)
	mac := hmac.New(sha256.New, v.Key)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (v *VerificationConfig) parseProof(value string, userID int64, now int64) (string, bool) {
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

func (v *VerificationConfig) challengeURL(userID, expiresAt int64, nonce string) string {
	u := *v.URL
	query := u.Query()
	query.Set("challenge", v.ticket("c", userID, expiresAt, nonce))
	u.RawQuery = query.Encode()
	return u.String()
}

func (s *Store) isVerified(userID int64) (bool, error) {
	var id int64
	err := s.db.QueryRow("SELECT user_id FROM verified_users WHERE user_id=?", userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) challenge(userID, now int64) (string, int64, int64, error) {
	var nonce string
	var expiresAt, promptedAt int64
	err := s.db.QueryRow("SELECT nonce,expires_at,prompted_at FROM verification_challenges WHERE user_id=?", userID).
		Scan(&nonce, &expiresAt, &promptedAt)
	if err == nil && expiresAt > now {
		return nonce, expiresAt, promptedAt, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", 0, 0, err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", 0, 0, err
	}
	nonce = base64.RawURLEncoding.EncodeToString(random)
	expiresAt = now + int64(challengeLifetime.Seconds())
	_, err = s.db.Exec("INSERT INTO verification_challenges (user_id,nonce,expires_at,prompted_at) "+
		"VALUES (?,?,?,0) ON CONFLICT(user_id) DO UPDATE SET "+
		"nonce=excluded.nonce,expires_at=excluded.expires_at,prompted_at=0",
		userID, nonce, expiresAt)
	return nonce, expiresAt, 0, err
}

func (s *Store) markChallengePrompted(userID int64, nonce string, now int64) error {
	_, err := s.db.Exec("UPDATE verification_challenges SET prompted_at=? WHERE user_id=? AND nonce=?",
		now, userID, nonce)
	return err
}

func (s *Store) consumeProof(updateID, userID int64, nonce string, now int64) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec("DELETE FROM verification_challenges "+
		"WHERE user_id=? AND nonce=? AND expires_at>?", userID, nonce, now)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count != 1 {
		return false, nil
	}
	if _, err := tx.Exec("INSERT INTO verified_users (user_id,verified_at) VALUES (?,?) "+
		"ON CONFLICT(user_id) DO UPDATE SET verified_at=excluded.verified_at", userID, now); err != nil {
		return false, err
	}
	if updateID > 0 {
		if _, err := tx.Exec("INSERT OR IGNORE INTO deliveries (update_id) VALUES (?)", updateID); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (b *Bot) promptVerification(ctx context.Context, m *Message, force bool) error {
	now := time.Now().Unix()
	nonce, expiresAt, promptedAt, err := b.store.challenge(m.From.ID, now)
	if err != nil {
		return err
	}
	if !force && promptedAt > 0 && now-promptedAt < int64(promptCooldown.Seconds()) {
		return nil
	}
	if err := b.api.sendVerification(ctx, m.Chat.ID, b.text("verification_required"),
		b.text("verification_button"), b.verify.challengeURL(m.From.ID, expiresAt, nonce)); err != nil {
		return err
	}
	return b.store.markChallengePrompted(m.From.ID, nonce, now)
}

func (b *Bot) acceptVerification(ctx context.Context, m *Message, updateID int64) error {
	now := time.Now().Unix()
	nonce, valid := b.verify.parseProof(m.WebAppData.Data, m.From.ID, now)
	if !valid {
		return b.say(ctx, m.Chat.ID, "verification_failed")
	}
	accepted, err := b.store.consumeProof(updateID, m.From.ID, nonce, now)
	if err != nil {
		return err
	}
	if !accepted {
		return b.say(ctx, m.Chat.ID, "verification_failed")
	}
	if err := b.api.clearVerification(ctx, m.Chat.ID, b.text("verification_success")); err != nil {
		log.Printf("verification confirmation to %d failed: %v", m.Chat.ID, err)
		return nil
	}
	return nil
}
