-- Initialize schema for local development (runs only when the data directory is empty)
CREATE DATABASE IF NOT EXISTS dce_logs;
USE dce_logs;

CREATE TABLE IF NOT EXISTS users (
  id VARCHAR(64) PRIMARY KEY,
  username VARCHAR(255) NOT NULL UNIQUE,
  password VARCHAR(255) NOT NULL,
  role VARCHAR(32) NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS build_elves (
  build_id VARCHAR(255) PRIMARY KEY,
  elf_filename VARCHAR(255) NOT NULL,
  elf_blob LONGBLOB NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


