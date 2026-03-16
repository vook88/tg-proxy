package db

import (
	"database/sql"
	"fmt"
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
	DeviceName string
	Active     bool
	CreatedAt  time.Time
}

type DB struct {
	conn *sql.DB
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, err
	}

	if err := migrate(conn); err != nil {
		return nil, err
	}

	return &DB{conn: conn}, nil
}

func migrate(conn *sql.DB) error {
	_, err := conn.Exec(`
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
			device_name TEXT NOT NULL DEFAULT '',
			active      BOOLEAN NOT NULL DEFAULT 0,
			created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
	`)
	return err
}

func (d *DB) CreateUser(telegramID int64, username string) (*User, error) {
	res, err := d.conn.Exec(
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
	err := d.conn.QueryRow(
		"SELECT id, telegram_id, username, status, created_at FROM users WHERE telegram_id = ?",
		telegramID,
	).Scan(&u.ID, &u.TelegramID, &u.Username, &u.Status, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (d *DB) UpdateUserStatus(telegramID int64, status string) error {
	_, err := d.conn.Exec("UPDATE users SET status = ? WHERE telegram_id = ?", status, telegramID)
	return err
}

func (d *DB) CreateSecret(userID int64, hexSecret, deviceName string, active bool) (*Secret, error) {
	res, err := d.conn.Exec(
		"INSERT INTO secrets (user_id, hex_secret, device_name, active) VALUES (?, ?, ?, ?)",
		userID, hexSecret, deviceName, active,
	)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Secret{
		ID:         id,
		UserID:     userID,
		HexSecret:  hexSecret,
		DeviceName: deviceName,
		Active:     active,
		CreatedAt:  time.Now(),
	}, nil
}

func (d *DB) ActivateSecret(id int64) error {
	_, err := d.conn.Exec("UPDATE secrets SET active = 1 WHERE id = ?", id)
	return err
}

func (d *DB) GetSecretsByTelegramID(telegramID int64) ([]Secret, error) {
	rows, err := d.conn.Query(`
		SELECT s.id, s.user_id, s.hex_secret, s.device_name, s.active, s.created_at
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
		if err := rows.Scan(&s.ID, &s.UserID, &s.HexSecret, &s.DeviceName, &s.Active, &s.CreatedAt); err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

func (d *DB) GetAllActiveSecrets() ([]Secret, error) {
	rows, err := d.conn.Query("SELECT id, user_id, hex_secret, device_name, active, created_at FROM secrets WHERE active = 1")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []Secret
	for rows.Next() {
		var s Secret
		if err := rows.Scan(&s.ID, &s.UserID, &s.HexSecret, &s.DeviceName, &s.Active, &s.CreatedAt); err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

func (d *DB) DeactivateUserSecrets(telegramID int64) (int64, error) {
	res, err := d.conn.Exec(`
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
	err := d.conn.QueryRow(`
		SELECT COUNT(*) FROM secrets s
		JOIN users u ON u.id = s.user_id
		WHERE u.telegram_id = ? AND s.active = 1
	`, telegramID).Scan(&count)
	return count, err
}

func (d *DB) ListApprovedUsers() ([]User, map[int64]int, error) {
	rows, err := d.conn.Query("SELECT id, telegram_id, username, status, created_at FROM users WHERE status IN ('approved', 'pending') ORDER BY created_at DESC")
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
	err := d.conn.QueryRow(`
		SELECT s.id, s.user_id, s.hex_secret, s.device_name, s.active, s.created_at
		FROM secrets s
		JOIN users u ON u.id = s.user_id
		WHERE u.telegram_id = ? AND s.active = 0
		ORDER BY s.created_at DESC LIMIT 1
	`, telegramID).Scan(&s.ID, &s.UserID, &s.HexSecret, &s.DeviceName, &s.Active, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (d *DB) DeleteUser(telegramID int64) error {
	_, err := d.conn.Exec("DELETE FROM secrets WHERE user_id = (SELECT id FROM users WHERE telegram_id = ?)", telegramID)
	if err != nil {
		return err
	}
	_, err = d.conn.Exec("DELETE FROM users WHERE telegram_id = ?", telegramID)
	return err
}

func (d *DB) DeleteSecret(id int64) error {
	_, err := d.conn.Exec("DELETE FROM secrets WHERE id = ?", id)
	return err
}

// SecretLabelToUser maps proxy config labels (e.g. "u1") back to username and device.
func (d *DB) SecretLabelToUser() (map[string]string, error) {
	rows, err := d.conn.Query(`
		SELECT s.id, u.username, s.device_name
		FROM secrets s
		JOIN users u ON u.id = s.user_id
		WHERE s.active = 1
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var id int64
		var username, device string
		if err := rows.Scan(&id, &username, &device); err != nil {
			return nil, err
		}
		label := fmt.Sprintf("u%d", id)
		display := "@" + username
		if device != "" {
			display += " (" + device + ")"
		}
		result[label] = display
	}
	return result, rows.Err()
}

func (d *DB) ResolveUser(input string) (int64, error) {
	// Try as numeric ID first.
	if id, err := fmt.Sscanf(input, "%d", new(int64)); err == nil && id == 1 {
		var tid int64
		fmt.Sscanf(input, "%d", &tid)
		if _, err := d.GetUserByTelegramID(tid); err == nil {
			return tid, nil
		}
	}

	// Try as username.
	var telegramID int64
	err := d.conn.QueryRow("SELECT telegram_id FROM users WHERE username = ?", input).Scan(&telegramID)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("user not found")
	}
	return telegramID, err
}
