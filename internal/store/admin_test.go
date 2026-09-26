package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestLegacyMigrationAndCleanup(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE preferences (user_id INTEGER PRIMARY KEY, name TEXT NOT NULL, notification INTEGER NOT NULL, blocked INTEGER NOT NULL)`,
		`CREATE TABLE messages (admin_message_id INTEGER PRIMARY KEY, sender_id INTEGER NOT NULL)`,
		`INSERT INTO preferences VALUES (2,'old user',0,1)`,
		`INSERT INTO messages VALUES (10,2)`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := time.Now()
	if _, err := s.db.Exec(`INSERT INTO messages (admin_message_id,sender_id,created_at) VALUES (?,?,?)`, 11, 2, now.Add(-MessageRetention-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO messages (admin_message_id,sender_id,created_at) VALUES (?,?,?)`, 12, 2, now.Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO verification_challenges VALUES (2,'expired',?,0)`, now.Add(-time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	challenges, messages, err := s.Cleanup(now)
	if err != nil || challenges != 1 || messages != 1 {
		t.Fatalf("cleanup: %d %d %v", challenges, messages, err)
	}
	if _, ok, err := s.Sender(10); err != nil || !ok {
		t.Fatalf("legacy mapping removed: %v", err)
	}
	if _, ok, err := s.Sender(12); err != nil || !ok {
		t.Fatalf("recent mapping removed: %v", err)
	}
	if _, ok, err := s.Sender(11); err != nil || ok {
		t.Fatalf("old mapping retained: %v", err)
	}
	if !s.Preference(2).Blocked {
		t.Fatal("legacy ban removed")
	}
}

func TestUserListsAndRevokeVerification(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for i := int64(1); i <= 10; i++ {
		if err := s.InitUser(i, "User"); err != nil {
			t.Fatal(err)
		}
	}
	p := s.Preference(2)
	p.Blocked = true
	if err := s.SetPreference(2, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO verified_users VALUES (2,?)`, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}
	users, more, err := s.Users(0, "all")
	if err != nil || !more || len(users) != PageSize {
		t.Fatalf("first page: %d %v %v", len(users), more, err)
	}
	users, more, err = s.Users(1, "all")
	if err != nil || more || len(users) != 2 {
		t.Fatalf("second page: %d %v %v", len(users), more, err)
	}
	banned, _, err := s.Users(0, "blocked")
	if err != nil || len(banned) != 1 || banned[0].ID != 2 {
		t.Fatalf("banned: %+v %v", banned, err)
	}
	verified, _, err := s.Users(0, "verified")
	if err != nil || len(verified) != 1 || verified[0].ID != 2 {
		t.Fatalf("verified: %+v %v", verified, err)
	}
	if revoked, err := s.RevokeVerification(100, 2); err != nil || !revoked {
		t.Fatalf("revoke: %v %v", revoked, err)
	}
	if yes, err := s.IsVerified(2); err != nil || yes {
		t.Fatalf("still verified: %v %v", yes, err)
	}
	if done, err := s.Delivered(100); err != nil || !done {
		t.Fatalf("revoke marker: %v %v", done, err)
	}
}
