package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

type Preference struct {
	Notification bool   `json:"notification"`
	Blocked      bool   `json:"blocked"`
	Name         string `json:"name"`
}

type Store struct {
	dir         string
	db          *sql.DB
	preferences map[string]Preference
	Offset      int64
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, preferences: make(map[string]Preference)}
	dbPath := filepath.Join(dir, "bot.db")
	var err error
	s.db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	s.db.SetMaxOpenConns(1)
	for _, statement := range []string{
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS preferences (user_id INTEGER PRIMARY KEY, name TEXT NOT NULL, notification INTEGER NOT NULL, blocked INTEGER NOT NULL, last_seen INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS messages (admin_message_id INTEGER PRIMARY KEY, sender_id INTEGER NOT NULL, created_at INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS state (key TEXT PRIMARY KEY, value INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS deliveries (update_id INTEGER PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS verified_users (user_id INTEGER PRIMARY KEY, verified_at INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS verification_challenges (user_id INTEGER PRIMARY KEY, nonce TEXT NOT NULL, expires_at INTEGER NOT NULL, prompted_at INTEGER NOT NULL)`,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			s.Close()
			return nil, err
		}
	}
	if err := s.addColumnIfMissing("preferences", "last_seen", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.addColumnIfMissing("messages", "created_at", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.loadData(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) addColumnIfMissing(table, column, definition string) error {
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primary int
		var name, dataType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primary); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	_, err = s.db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

func (s *Store) loadData() error {
	rows, err := s.db.Query(`SELECT user_id,name,notification,blocked FROM preferences`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var p Preference
		if err := rows.Scan(&id, &p.Name, &p.Notification, &p.Blocked); err != nil {
			rows.Close()
			return err
		}
		s.preferences[strconv.FormatInt(id, 10)] = p
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	err = s.db.QueryRow(`SELECT value FROM state WHERE key='offset'`).Scan(&s.Offset)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	return nil
}

func (s *Store) InitUser(id int64, name string) error {
	key := strconv.FormatInt(id, 10)
	p, found := s.preferences[key]
	if !found || p.Name != name {
		p.Name = name
		if err := s.SetPreference(id, p); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`UPDATE preferences SET last_seen=? WHERE user_id=?`, time.Now().Unix(), id)
	return err
}
func (s *Store) Preference(id int64) Preference { return s.preferences[strconv.FormatInt(id, 10)] }
func (s *Store) SetPreference(id int64, p Preference) error {
	_, err := s.db.Exec(`INSERT INTO preferences (user_id,name,notification,blocked) VALUES (?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET name=excluded.name,notification=excluded.notification,blocked=excluded.blocked`, id, p.Name, p.Notification, p.Blocked)
	if err == nil {
		s.preferences[strconv.FormatInt(id, 10)] = p
	}
	return err
}
func (s *Store) Delivered(updateID int64) (bool, error) {
	if updateID <= 0 {
		return false, nil
	}
	var found int64
	err := s.db.QueryRow(`SELECT update_id FROM deliveries WHERE update_id=?`, updateID).Scan(&found)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) MarkDelivered(updateID int64) error {
	if updateID <= 0 {
		return nil
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO deliveries (update_id) VALUES (?)`, updateID)
	return err
}

func (s *Store) Link(updateID, messageID, senderID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO messages (admin_message_id,sender_id,created_at) VALUES (?,?,?) ON CONFLICT(admin_message_id) DO UPDATE SET sender_id=excluded.sender_id,created_at=excluded.created_at`, messageID, senderID, time.Now().Unix()); err != nil {
		return err
	}
	if updateID > 0 {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO deliveries (update_id) VALUES (?)`, updateID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetPreferenceForUpdate(updateID, id int64, p Preference) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO preferences (user_id,name,notification,blocked) VALUES (?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET name=excluded.name,notification=excluded.notification,blocked=excluded.blocked`, id, p.Name, p.Notification, p.Blocked); err != nil {
		return err
	}
	if updateID > 0 {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO deliveries (update_id) VALUES (?)`, updateID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.preferences[strconv.FormatInt(id, 10)] = p
	return nil
}

func (s *Store) Sender(messageID int64) (int64, bool, error) {
	var senderID int64
	err := s.db.QueryRow(`SELECT sender_id FROM messages WHERE admin_message_id=?`, messageID).Scan(&senderID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return senderID, true, nil
}

func (s *Store) AdvanceOffset(next int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO state (key,value) VALUES ('offset',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, next); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM deliveries WHERE update_id < ?`, next); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.Offset = next
	return nil
}

func (s *Store) Directory() string { return s.dir }

func (s *Store) LookupPreference(id int64) (Preference, bool) {
	p, ok := s.preferences[strconv.FormatInt(id, 10)]
	return p, ok
}
