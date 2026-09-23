package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	_ "modernc.org/sqlite"
)

type Config struct {
	Admin int64  `json:"admin"`
	Lang  string `json:"lang"`
}

type Preference struct {
	Notification bool   `json:"notification"`
	Blocked      bool   `json:"blocked"`
	Name         string `json:"name"`
}

type Store struct {
	dir         string
	db          *sql.DB
	Config      Config
	Preferences map[string]Preference
	Offset      int64
}

func loadStore(dir string) (*Store, error) {
	s := &Store{dir: dir, Preferences: make(map[string]Preference)}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &s.Config); err != nil {
		return nil, err
	}
	if s.Config.Admin <= 0 {
		return nil, fmt.Errorf("admin must be a positive Telegram user ID in %s", filepath.Join(dir, "config.json"))
	}
	dbPath := filepath.Join(dir, "bot.db")
	s.db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	s.db.SetMaxOpenConns(1)
	for _, statement := range []string{
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS preferences (user_id INTEGER PRIMARY KEY, name TEXT NOT NULL, notification INTEGER NOT NULL, blocked INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS messages (admin_message_id INTEGER PRIMARY KEY, sender_id INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS state (key TEXT PRIMARY KEY, value INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS deliveries (update_id INTEGER PRIMARY KEY)`,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			s.Close()
			return nil, err
		}
	}
	if err := s.loadData(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

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
		s.Preferences[strconv.FormatInt(id, 10)] = p
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

func (s *Store) initUser(u User) error {
	key := strconv.FormatInt(u.ID, 10)
	p, found := s.Preferences[key]
	name := u.FullName()
	if !found || p.Name != name {
		p.Name = name
		return s.setPreference(u.ID, p)
	}
	return nil
}
func (s *Store) preference(id int64) Preference { return s.Preferences[strconv.FormatInt(id, 10)] }
func (s *Store) setPreference(id int64, p Preference) error {
	_, err := s.db.Exec(`INSERT INTO preferences (user_id,name,notification,blocked) VALUES (?,?,?,?) ON CONFLICT(user_id) DO UPDATE SET name=excluded.name,notification=excluded.notification,blocked=excluded.blocked`, id, p.Name, p.Notification, p.Blocked)
	if err == nil {
		s.Preferences[strconv.FormatInt(id, 10)] = p
	}
	return err
}
func (s *Store) delivered(updateID int64) (bool, error) {
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

func (s *Store) markDelivered(updateID int64) error {
	if updateID <= 0 {
		return nil
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO deliveries (update_id) VALUES (?)`, updateID)
	return err
}

func (s *Store) link(updateID, messageID, senderID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO messages (admin_message_id,sender_id) VALUES (?,?) ON CONFLICT(admin_message_id) DO UPDATE SET sender_id=excluded.sender_id`, messageID, senderID); err != nil {
		return err
	}
	if updateID > 0 {
		if _, err = tx.Exec(`INSERT OR IGNORE INTO deliveries (update_id) VALUES (?)`, updateID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) setPreferenceForUpdate(updateID, id int64, p Preference) error {
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
	s.Preferences[strconv.FormatInt(id, 10)] = p
	return nil
}

func (s *Store) sender(messageID int64) (int64, bool, error) {
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

func (s *Store) advanceOffset(next int64) error {
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
