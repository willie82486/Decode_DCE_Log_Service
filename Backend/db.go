package main

import (
	"database/sql"
	"fmt"
	"log"
	_ "github.com/go-sql-driver/mysql"
)

var db *sql.DB

func initDB() error {
	dsn := getenv("MYSQL_DSN", "dce_user:dce_pass@tcp(mariadb:3306)/dce_logs?parseTime=true")
	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	// Reasonable pool settings
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(0)

	// Verify connection
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	// Create tables if not exist
	usersSQL := `
	CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(64) PRIMARY KEY,
		username VARCHAR(255) NOT NULL UNIQUE,
		password VARCHAR(255) NOT NULL,
		role VARCHAR(32) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	if _, err := db.Exec(usersSQL); err != nil {
		return fmt.Errorf("create users: %w", err)
	}

	// Table for build_id -> ELF (blob)
	elfSQL := `
	CREATE TABLE IF NOT EXISTS build_elves (
		build_id VARCHAR(255) PRIMARY KEY,
		elf_filename VARCHAR(255) NOT NULL,
		elf_blob LONGBLOB NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
	`
	if _, err := db.Exec(elfSQL); err != nil {
		return fmt.Errorf("create build_elves: %w", err)
	}

	// Seed default admin user if not exists
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE username = ?", "nvidia").Scan(&count); err != nil {
		return fmt.Errorf("seed admin check: %w", err)
	}
	if count == 0 {
		newID, err := randomIDHex(12)
		if err != nil {
			return fmt.Errorf("seed admin id: %w", err)
		}
		if _, err := db.Exec("INSERT INTO users (id, username, password, role) VALUES (?, ?, ?, ?)",
			newID, "nvidia", "nvidia", "admin"); err != nil {
			return fmt.Errorf("seed admin insert: %w", err)
		}
		log.Printf("Seeded default admin user 'nvidia' with role 'admin'.")
	}
	return nil
}

// storeELF stores/updates ELF by buildID
func storeELF(buildID, elfFileName string, elfBytes []byte) error {
	_, err := db.Exec(`
		INSERT INTO build_elves (build_id, elf_filename, elf_blob)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE elf_filename = VALUES(elf_filename), elf_blob = VALUES(elf_blob)`,
		buildID, elfFileName, elfBytes)
	return err
}

// getELFByBuildID returns filename and bytes
func getELFByBuildID(buildID string) (string, []byte, error) {
	var name string
	var blob []byte
	err := db.QueryRow("SELECT elf_filename, elf_blob FROM build_elves WHERE build_id = ?", buildID).Scan(&name, &blob)
	if err != nil {
		return "", nil, err
	}
	return name, blob, nil
}


