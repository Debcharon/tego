package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"
)

const challengeLifetime = 10 * time.Minute

func (s *Store) IsVerified(userID int64) (bool, error) {
	var id int64
	err := s.db.QueryRow("SELECT user_id FROM verified_users WHERE user_id=?", userID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) Challenge(userID, now int64) (string, int64, int64, error) {
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

func (s *Store) MarkChallengePrompted(userID int64, nonce string, now int64) error {
	_, err := s.db.Exec("UPDATE verification_challenges SET prompted_at=? WHERE user_id=? AND nonce=?",
		now, userID, nonce)
	return err
}

func (s *Store) ConsumeProof(updateID, userID int64, nonce string, now int64) (bool, error) {
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
