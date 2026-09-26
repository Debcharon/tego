package store

import (
	"database/sql"
	"time"
)

const PageSize = 8
const MessageRetention = 180 * 24 * time.Hour

type UserRecord struct {
	ID       int64
	Name     string
	Blocked  bool
	Verified bool
	LastSeen int64
}

type Stats struct {
	Users, Banned, Verified, ReplyableMessages int64
}

func (s *Store) Users(page int, filter string) ([]UserRecord, bool, error) {
	if page < 0 || page > 100000 {
		return nil, false, nil
	}
	query := `SELECT p.user_id,p.name,p.blocked,p.last_seen,verified_users.user_id IS NOT NULL
		FROM preferences p LEFT JOIN verified_users ON verified_users.user_id=p.user_id`
	switch filter {
	case "blocked":
		query += ` WHERE p.blocked=1`
	case "verified":
		query += ` WHERE verified_users.user_id IS NOT NULL`
	}
	query += ` ORDER BY p.last_seen DESC,p.user_id DESC LIMIT ? OFFSET ?`
	rows, err := s.db.Query(query, PageSize+1, page*PageSize)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	users := make([]UserRecord, 0, PageSize)
	for rows.Next() {
		var u UserRecord
		if err := rows.Scan(&u.ID, &u.Name, &u.Blocked, &u.LastSeen, &u.Verified); err != nil {
			return nil, false, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(users) > PageSize {
		return users[:PageSize], true, nil
	}
	return users, false, nil
}

func (s *Store) User(id int64) (UserRecord, bool, error) {
	var u UserRecord
	err := s.db.QueryRow(`SELECT p.user_id,p.name,p.blocked,p.last_seen,verified_users.user_id IS NOT NULL
		FROM preferences p LEFT JOIN verified_users ON verified_users.user_id=p.user_id WHERE p.user_id=?`, id).
		Scan(&u.ID, &u.Name, &u.Blocked, &u.LastSeen, &u.Verified)
	if err == sql.ErrNoRows {
		return UserRecord{}, false, nil
	}
	return u, err == nil, err
}

func (s *Store) Statistics() (Stats, error) {
	var v Stats
	for _, item := range []struct {
		query  string
		target *int64
	}{
		{`SELECT COUNT(*) FROM preferences`, &v.Users},
		{`SELECT COUNT(*) FROM preferences WHERE blocked=1`, &v.Banned},
		{`SELECT COUNT(*) FROM verified_users`, &v.Verified},
		{`SELECT COUNT(*) FROM messages`, &v.ReplyableMessages},
	} {
		if err := s.db.QueryRow(item.query).Scan(item.target); err != nil {
			return Stats{}, err
		}
	}
	return v, nil
}

func (s *Store) RevokeVerification(updateID, userID int64) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM verified_users WHERE user_id=?`, userID)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(`DELETE FROM verification_challenges WHERE user_id=?`, userID); err != nil {
		return false, err
	}
	if updateID > 0 {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO deliveries (update_id) VALUES (?)`, updateID); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return count > 0, nil
}

// Cleanup leaves records migrated from the old schema (created_at=0) intact because their age is unknown.
func (s *Store) Cleanup(now time.Time) (int64, int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	challengeResult, err := tx.Exec(`DELETE FROM verification_challenges WHERE expires_at<=?`, now.Unix())
	if err != nil {
		return 0, 0, err
	}
	messageResult, err := tx.Exec(`DELETE FROM messages WHERE created_at>0 AND created_at<?`, now.Add(-MessageRetention).Unix())
	if err != nil {
		return 0, 0, err
	}
	challenges, err := challengeResult.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	messages, err := messageResult.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return challenges, messages, nil
}
