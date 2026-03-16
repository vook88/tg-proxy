package main

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type User struct {
	ID         int64
	TelegramID int64
	Username   string
	Status     string // pending, approved, banned
	CreatedAt  time.Time
}

type Secret struct {
	ID         int64
	UserID     int64
	HexSecret  string
	B64Secret  string
	DeviceName string
	Active     bool
	CreatedAt  time.Time
}

type DB struct {
	db *sql.DB
}

func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, err
	}

	if err := migrate(db); err != nil {
		return nil, err
	}

	return &DB{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			telegram_id INTEGER UNIQUE NOT NULL,
			username    TEXT NOT NULL DEFAULT '',
			status      TEXT NOT NULL DEFAULT 'pending',
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS secrets (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id     INTEGER NOT NULL REFERENCES users(id),
			hex_secret  TEXT NOT NULL,
			b64_secret  TEXT NOT NULL,
			device_name TEXT NOT NULL DEFAULT '',
			active      BOOLEAN NOT NULL DEFAULT 0,
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

func (d *DB) CreateUser(telegramID int64, username string) (*User, error) {
	res, err := d.db.Exec(
		"INSERT INTO users (telegram_id, username) VALUES (?, ?) ON CONFLICT(telegram_id) DO UPDATE SET username = ?",
		telegramID, username, username,
	)
	if err != nil {
		return nil, err
	}

	id, _ := res.LastInsertId()
	if id == 0 {
		return d.GetUserByTelegramID(telegramID)
	}

	return &User{
		ID:         id,
		TelegramID: telegramID,
		Username:   username,
		Status:     "pending",
		CreatedAt:  time.Now(),
	}, nil
}

func (d *DB) GetUserByTelegramID(telegramID int64) (*User, error) {
	u := &User{}
	err := d.db.QueryRow(
		"SELECT id, telegram_id, username, status, created_at FROM users WHERE telegram_id = ?",
		telegramID,
	).Scan(&u.ID, &u.TelegramID, &u.Username, &u.Status, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (d *DB) UpdateUserStatus(telegramID int64, status string) error {
	_, err := d.db.Exec("UPDATE users SET status = ? WHERE telegram_id = ?", status, telegramID)
	return err
}

func (d *DB) CreateSecret(userID int64, hexSecret, b64Secret, deviceName string, active bool) (*Secret, error) {
	res, err := d.db.Exec(
		"INSERT INTO secrets (user_id, hex_secret, b64_secret, device_name, active) VALUES (?, ?, ?, ?, ?)",
		userID, hexSecret, b64Secret, deviceName, active,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Secret{
		ID:         id,
		UserID:     userID,
		HexSecret:  hexSecret,
		B64Secret:  b64Secret,
		DeviceName: deviceName,
		Active:     active,
		CreatedAt:  time.Now(),
	}, nil
}

func (d *DB) ActivateSecret(id int64) error {
	_, err := d.db.Exec("UPDATE secrets SET active = 1 WHERE id = ?", id)
	return err
}

func (d *DB) GetSecretsByTelegramID(telegramID int64) ([]Secret, error) {
	rows, err := d.db.Query(`
		SELECT s.id, s.user_id, s.hex_secret, s.b64_secret, s.device_name, s.active, s.created_at
		FROM secrets s
		JOIN users u ON u.id = s.user_id
		WHERE u.telegram_id = ? AND s.active = 1
	`, telegramID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []Secret
	for rows.Next() {
		var s Secret
		if err := rows.Scan(&s.ID, &s.UserID, &s.HexSecret, &s.B64Secret, &s.DeviceName, &s.Active, &s.CreatedAt); err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

func (d *DB) GetAllActiveSecrets() ([]Secret, error) {
	rows, err := d.db.Query("SELECT id, user_id, hex_secret, b64_secret, device_name, active, created_at FROM secrets WHERE active = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []Secret
	for rows.Next() {
		var s Secret
		if err := rows.Scan(&s.ID, &s.UserID, &s.HexSecret, &s.B64Secret, &s.DeviceName, &s.Active, &s.CreatedAt); err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

func (d *DB) DeactivateUserSecrets(telegramID int64) (int64, error) {
	res, err := d.db.Exec(`
		UPDATE secrets SET active = 0
		WHERE user_id = (SELECT id FROM users WHERE telegram_id = ?)
	`, telegramID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (d *DB) CountActiveSecrets(telegramID int64) (int, error) {
	var count int
	err := d.db.QueryRow(`
		SELECT COUNT(*) FROM secrets s
		JOIN users u ON u.id = s.user_id
		WHERE u.telegram_id = ? AND s.active = 1
	`, telegramID).Scan(&count)
	return count, err
}

// ListApprovedUsers returns all approved users with their active secret count.
func (d *DB) ListApprovedUsers() ([]User, map[int64]int, error) {
	rows, err := d.db.Query("SELECT id, telegram_id, username, status, created_at FROM users WHERE status IN ('approved', 'pending') ORDER BY created_at DESC")
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.TelegramID, &u.Username, &u.Status, &u.CreatedAt); err != nil {
			return nil, nil, err
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	counts := make(map[int64]int)
	for _, u := range users {
		c, err := d.CountActiveSecrets(u.TelegramID)
		if err != nil {
			return nil, nil, err
		}
		counts[u.TelegramID] = c
	}

	return users, counts, nil
}

func (d *DB) GetPendingSecretByUser(telegramID int64) (*Secret, error) {
	s := &Secret{}
	err := d.db.QueryRow(`
		SELECT s.id, s.user_id, s.hex_secret, s.b64_secret, s.device_name, s.active, s.created_at
		FROM secrets s
		JOIN users u ON u.id = s.user_id
		WHERE u.telegram_id = ? AND s.active = 0
		ORDER BY s.created_at DESC LIMIT 1
	`, telegramID).Scan(&s.ID, &s.UserID, &s.HexSecret, &s.B64Secret, &s.DeviceName, &s.Active, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (d *DB) DeleteSecret(id int64) error {
	_, err := d.db.Exec("DELETE FROM secrets WHERE id = ?", id)
	return err
}
