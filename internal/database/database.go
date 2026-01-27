package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"nano-backend/internal/config"
	"nano-backend/internal/crypto"
	"nano-backend/internal/models"

	_ "github.com/glebarez/sqlite"
	"github.com/google/uuid"
)

var (
	db   *sql.DB
	dbMu sync.RWMutex
)

func Init(cfg *config.Config) error {
	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(cfg.StorageDir, 0755); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	dbPath := filepath.Join(cfg.DataDir, "db.sqlite")
	var err error
	db, err = sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Create tables
	if err := createTables(); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	log.Printf("[database] Initialized at %s", dbPath)
	return nil
}

func Close() {
	if db != nil {
		db.Close()
	}
}

func createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS app_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			fileRetentionHours INTEGER NOT NULL,
			referenceHistoryLimit INTEGER NOT NULL,
			imageTimeoutSeconds INTEGER NOT NULL,
			videoTimeoutSeconds INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			username TEXT NOT NULL UNIQUE,
			role TEXT NOT NULL,
			passwordHash TEXT NOT NULL,
			disabled INTEGER NOT NULL DEFAULT 0,
			createdAt INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			userId TEXT NOT NULL,
			createdAt INTEGER NOT NULL,
			expiresAt INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS user_provider (
			userId TEXT PRIMARY KEY,
			providerHost TEXT NOT NULL,
			apiKeyEnc TEXT,
			updatedAt INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS files (
			id TEXT PRIMARY KEY,
			userId TEXT NOT NULL,
			purpose TEXT NOT NULL,
			mimeType TEXT NOT NULL,
			originalName TEXT,
			path TEXT NOT NULL,
			persistent INTEGER NOT NULL,
			publicToken TEXT NOT NULL,
			createdAt INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS generations (
			id TEXT PRIMARY KEY,
			userId TEXT NOT NULL,
			type TEXT NOT NULL,
			prompt TEXT NOT NULL,
			model TEXT NOT NULL,
			status TEXT NOT NULL,
			progress REAL,
			startedAt INTEGER,
			elapsedSeconds INTEGER,
			error TEXT,
			providerTaskId TEXT,
			providerResultUrl TEXT,
			referenceFileIds TEXT,
			imageSize TEXT,
			aspectRatio TEXT,
			favorite INTEGER NOT NULL,
			outputFileId TEXT,
			createdAt INTEGER NOT NULL,
			updatedAt INTEGER NOT NULL,
			duration INTEGER,
			videoSize TEXT,
			runId TEXT,
			nodePosition INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS presets (
			id TEXT PRIMARY KEY,
			userId TEXT NOT NULL,
			name TEXT NOT NULL,
			prompt TEXT NOT NULL,
			createdAt INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS library (
			id TEXT PRIMARY KEY,
			userId TEXT NOT NULL,
			kind TEXT NOT NULL,
			name TEXT NOT NULL,
			fileId TEXT NOT NULL,
			createdAt INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS reference_uploads (
			id TEXT PRIMARY KEY,
			userId TEXT NOT NULL,
			fileId TEXT NOT NULL,
			createdAt INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS video_runs (
			id TEXT PRIMARY KEY,
			userId TEXT NOT NULL,
			name TEXT NOT NULL,
			createdAt INTEGER NOT NULL
		)`,
		/* 影视项目审阅系统表 */
		`CREATE TABLE IF NOT EXISTS review_projects (
			id TEXT PRIMARY KEY,
			userId TEXT NOT NULL,
			name TEXT NOT NULL,
			coverFileId TEXT,
			createdAt INTEGER NOT NULL,
			updatedAt INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS review_episodes (
			id TEXT PRIMARY KEY,
			projectId TEXT NOT NULL,
			userId TEXT NOT NULL,
			name TEXT NOT NULL,
			coverFileId TEXT,
			createdAt INTEGER NOT NULL,
			updatedAt INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS review_storyboards (
			id TEXT PRIMARY KEY,
			episodeId TEXT NOT NULL,
			userId TEXT NOT NULL,
			imageFileId TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			feedback TEXT,
			sortOrder INTEGER NOT NULL,
			createdAt INTEGER NOT NULL,
			updatedAt INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_userId ON sessions(userId)`,
		`CREATE INDEX IF NOT EXISTS idx_generations_userId ON generations(userId)`,
		`CREATE INDEX IF NOT EXISTS idx_files_userId ON files(userId)`,
		`CREATE INDEX IF NOT EXISTS idx_presets_userId ON presets(userId)`,
		`CREATE INDEX IF NOT EXISTS idx_library_userId ON library(userId)`,
		`CREATE INDEX IF NOT EXISTS idx_reference_uploads_userId ON reference_uploads(userId)`,
		`CREATE INDEX IF NOT EXISTS idx_video_runs_userId ON video_runs(userId)`,
		/* 影视项目审阅系统索引 */
		`CREATE INDEX IF NOT EXISTS idx_review_projects_userId ON review_projects(userId)`,
		`CREATE INDEX IF NOT EXISTS idx_review_episodes_projectId ON review_episodes(projectId)`,
		`CREATE INDEX IF NOT EXISTS idx_review_episodes_userId ON review_episodes(userId)`,
		`CREATE INDEX IF NOT EXISTS idx_review_storyboards_episodeId ON review_storyboards(episodeId)`,
		`CREATE INDEX IF NOT EXISTS idx_review_storyboards_userId ON review_storyboards(userId)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return fmt.Errorf("failed to execute query: %w", err)
		}
	}

	// Migration: Add disabled column to users table if it doesn't exist
	_, err := db.Exec("ALTER TABLE users ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		// Ignore error if column already exists (SQLite doesn't have IF NOT EXISTS for columns)
		log.Printf("[database] Note: disabled column migration: %v", err)
	}

	// Migration: Add referenceHistoryLimit column to settings table if it doesn't exist
	_, err = db.Exec("ALTER TABLE settings ADD COLUMN referenceHistoryLimit INTEGER NOT NULL DEFAULT 50")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		log.Printf("[database] Note: referenceHistoryLimit column migration: %v", err)
	}

	// Migration: Add imageTimeoutSeconds column to settings table if it doesn't exist
	_, err = db.Exec("ALTER TABLE settings ADD COLUMN imageTimeoutSeconds INTEGER NOT NULL DEFAULT 600")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		log.Printf("[database] Note: imageTimeoutSeconds column migration: %v", err)
	}

	// Migration: Add videoTimeoutSeconds column to settings table if it doesn't exist
	_, err = db.Exec("ALTER TABLE settings ADD COLUMN videoTimeoutSeconds INTEGER NOT NULL DEFAULT 600")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		log.Printf("[database] Note: videoTimeoutSeconds column migration: %v", err)
	}

	// Migration: Add startedAt/elapsedSeconds columns to generations table if they don't exist
	_, err = db.Exec("ALTER TABLE generations ADD COLUMN startedAt INTEGER")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		log.Printf("[database] Note: startedAt column migration: %v", err)
	}

	_, err = db.Exec("ALTER TABLE generations ADD COLUMN elapsedSeconds INTEGER")
	if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
		log.Printf("[database] Note: elapsedSeconds column migration: %v", err)
	}

	return nil
}

// EnsureInitialAdmin creates the initial admin user if no users exist
// or updates the password hash if the admin exists but has an old format hash
func EnsureInitialAdmin(cfg *config.Config) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return err
	}

	// 如果数据库为空，创建默认管理员
	if count == 0 {
		passwordHash, err := crypto.HashPassword(cfg.InitAdminPassword)
		if err != nil {
			return err
		}

		id := uuid.New().String()
		now := models.Now()

		_, err = db.Exec(
			"INSERT INTO users (id, username, role, passwordHash, createdAt) VALUES (?, ?, ?, ?, ?)",
			id, cfg.InitAdminUsername, "admin", passwordHash, now,
		)
		if err != nil {
			return err
		}

		log.Printf("[init] Created initial admin user: %s", cfg.InitAdminUsername)
		return nil
	}

	// 如果已有用户，检查默认管理员是否存在且密码哈希格式是否正确
	var existingUser struct {
		ID           string
		PasswordHash string
	}
	err := db.QueryRow(
		"SELECT id, passwordHash FROM users WHERE LOWER(username) = LOWER(?)",
		cfg.InitAdminUsername,
	).Scan(&existingUser.ID, &existingUser.PasswordHash)

	if err == sql.ErrNoRows {
		// 默认管理员不存在，创建它
		passwordHash, err := crypto.HashPassword(cfg.InitAdminPassword)
		if err != nil {
			return err
		}

		id := uuid.New().String()
		now := models.Now()

		_, err = db.Exec(
			"INSERT INTO users (id, username, role, passwordHash, createdAt) VALUES (?, ?, ?, ?, ?)",
			id, cfg.InitAdminUsername, "admin", passwordHash, now,
		)
		if err != nil {
			return err
		}

		log.Printf("[init] Created initial admin user: %s", cfg.InitAdminUsername)
		return nil
	}
	if err != nil {
		return err
	}

	// 检查密码哈希格式，如果不是 scrypt 格式，则更新
	if !strings.HasPrefix(existingUser.PasswordHash, "scrypt:") {
		log.Printf("[init] Admin user %s has old password hash format, updating...", cfg.InitAdminUsername)
		passwordHash, err := crypto.HashPassword(cfg.InitAdminPassword)
		if err != nil {
			return err
		}

		_, err = db.Exec(
			"UPDATE users SET passwordHash = ? WHERE id = ?",
			passwordHash, existingUser.ID,
		)
		if err != nil {
			return err
		}

		log.Printf("[init] Updated password hash for admin user: %s", cfg.InitAdminUsername)
	}

	return nil
}

// ========== User operations ==========

func GetUserByUsername(username string) (*models.User, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var u models.User
	var disabled int
	err := db.QueryRow(
		"SELECT id, username, role, passwordHash, disabled, createdAt FROM users WHERE LOWER(username) = LOWER(?)",
		username,
	).Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &disabled, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.Disabled = disabled != 0
	return &u, nil
}

func GetUserByID(id string) (*models.User, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var u models.User
	var disabled int
	err := db.QueryRow(
		"SELECT id, username, role, passwordHash, disabled, createdAt FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &disabled, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.Disabled = disabled != 0
	return &u, nil
}

func CreateUser(username, password, role string) (*models.User, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	// Check if user exists
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM users WHERE LOWER(username) = LOWER(?)", username).Scan(&count); err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("用户名已存在")
	}

	passwordHash, err := crypto.HashPassword(password)
	if err != nil {
		return nil, err
	}

	id := uuid.New().String()
	now := models.Now()

	_, err = db.Exec(
		"INSERT INTO users (id, username, role, passwordHash, disabled, createdAt) VALUES (?, ?, ?, ?, 0, ?)",
		id, username, role, passwordHash, now,
	)
	if err != nil {
		return nil, err
	}

	return &models.User{
		ID:        id,
		Username:  username,
		Role:      role,
		Disabled:  false,
		CreatedAt: now,
	}, nil
}

func ListUsers() ([]models.User, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	rows, err := db.Query("SELECT id, username, role, disabled, createdAt FROM users ORDER BY createdAt DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		var disabled int
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &disabled, &u.CreatedAt); err != nil {
			return nil, err
		}
		u.Disabled = disabled != 0
		users = append(users, u)
	}
	return users, nil
}

// DeleteUser deletes a user and all their sessions
func DeleteUser(userID string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	// Delete user's sessions
	if _, err := db.Exec("DELETE FROM sessions WHERE userId = ?", userID); err != nil {
		return err
	}

	// Delete user
	result, err := db.Exec("DELETE FROM users WHERE id = ?", userID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}

	return nil
}

// UpdateUserDisabled updates the disabled status of a user
func UpdateUserDisabled(userID string, disabled bool) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	disabledInt := 0
	if disabled {
		disabledInt = 1
		// Also delete all sessions for this user if disabling
		if _, err := db.Exec("DELETE FROM sessions WHERE userId = ?", userID); err != nil {
			return err
		}
	}

	result, err := db.Exec("UPDATE users SET disabled = ? WHERE id = ?", disabledInt, userID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("用户不存在")
	}

	return nil
}

// ========== Session operations ==========

func CreateSession(userID string, ttlHours int) (*models.Session, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	token := crypto.RandomToken()
	now := models.Now()
	expiresAt := now + int64(ttlHours)*3600*1000

	_, err := db.Exec(
		"INSERT INTO sessions (token, userId, createdAt, expiresAt) VALUES (?, ?, ?, ?)",
		token, userID, now, expiresAt,
	)
	if err != nil {
		return nil, err
	}

	return &models.Session{
		Token:     token,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}, nil
}

func GetSession(token string) (*models.Session, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var s models.Session
	err := db.QueryRow(
		"SELECT token, userId, createdAt, expiresAt FROM sessions WHERE token = ?",
		token,
	).Scan(&s.Token, &s.UserID, &s.CreatedAt, &s.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func DeleteSession(token string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

func CleanupExpiredSessions() {
	dbMu.Lock()
	defer dbMu.Unlock()

	now := models.Now()
	result, err := db.Exec("DELETE FROM sessions WHERE expiresAt < ?", now)
	if err != nil {
		log.Printf("[cleanup] Error cleaning sessions: %v", err)
		return
	}
	if count, _ := result.RowsAffected(); count > 0 {
		log.Printf("[cleanup] Removed %d expired sessions", count)
	}
}

// ========== Provider operations ==========

func GetUserProvider(userID string) (*models.UserProvider, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var p models.UserProvider
	var apiKeyEnc sql.NullString
	err := db.QueryRow(
		"SELECT userId, providerHost, apiKeyEnc, updatedAt FROM user_provider WHERE userId = ?",
		userID,
	).Scan(&p.UserID, &p.ProviderHost, &apiKeyEnc, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if apiKeyEnc.Valid {
		p.APIKeyEnc = apiKeyEnc.String
	}
	return &p, nil
}

func SetUserProvider(userID, providerHost, apiKey string, cfg *config.Config) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	var apiKeyEnc sql.NullString
	if apiKey != "" {
		encrypted, err := crypto.EncryptText(apiKey, cfg.APIKeyEncryptionSecret)
		if err != nil {
			return err
		}
		apiKeyEnc = sql.NullString{String: encrypted, Valid: true}
	}

	now := models.Now()

	// Try update first
	result, err := db.Exec(
		"UPDATE user_provider SET providerHost = ?, apiKeyEnc = COALESCE(?, apiKeyEnc), updatedAt = ? WHERE userId = ?",
		providerHost, apiKeyEnc, now, userID,
	)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		// Insert new
		_, err = db.Exec(
			"INSERT INTO user_provider (userId, providerHost, apiKeyEnc, updatedAt) VALUES (?, ?, ?, ?)",
			userID, providerHost, apiKeyEnc, now,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// ========== Settings operations ==========

func GetSettings() (*models.Settings, int, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var fileRetentionHours int
	var referenceHistoryLimit int
	var imageTimeoutSeconds int
	var videoTimeoutSeconds int
	err := db.QueryRow("SELECT fileRetentionHours, referenceHistoryLimit, imageTimeoutSeconds, videoTimeoutSeconds FROM settings WHERE id = 1").
		Scan(&fileRetentionHours, &referenceHistoryLimit, &imageTimeoutSeconds, &videoTimeoutSeconds)
	if err == sql.ErrNoRows {
		return &models.Settings{
			FileRetentionHours:    168,
			ReferenceHistoryLimit: 50,
			ImageTimeoutSeconds:   600,
			VideoTimeoutSeconds:   600,
		}, 168, nil
	}
	if err != nil {
		return nil, 0, err
	}
	// 清理损坏的值：如果 < 1，则设为默认值
	if fileRetentionHours < 1 {
		fileRetentionHours = 168
	}
	if referenceHistoryLimit < 1 {
		referenceHistoryLimit = 50
	}
	if imageTimeoutSeconds < 30 {
		imageTimeoutSeconds = 600
	}
	if videoTimeoutSeconds < 30 {
		videoTimeoutSeconds = 600
	}
	return &models.Settings{
		FileRetentionHours:    fileRetentionHours,
		ReferenceHistoryLimit: referenceHistoryLimit,
		ImageTimeoutSeconds:   imageTimeoutSeconds,
		VideoTimeoutSeconds:   videoTimeoutSeconds,
	}, fileRetentionHours, nil
}

func UpdateSettings(fileRetentionHours int, referenceHistoryLimit int, imageTimeoutSeconds int, videoTimeoutSeconds int) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec(
		"INSERT OR REPLACE INTO settings (id, fileRetentionHours, referenceHistoryLimit, imageTimeoutSeconds, videoTimeoutSeconds) VALUES (1, ?, ?, ?, ?)",
		fileRetentionHours,
		referenceHistoryLimit,
		imageTimeoutSeconds,
		videoTimeoutSeconds,
	)
	return err
}

// ========== File operations ==========

func CreateFile(userID, purpose, mimeType, originalName, filePath string, persistent bool) (*models.File, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	id := uuid.New().String()
	publicToken := crypto.RandomToken()
	now := models.Now()

	_, err := db.Exec(
		`INSERT INTO files (id, userId, purpose, mimeType, originalName, path, persistent, publicToken, createdAt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, userID, purpose, mimeType, originalName, filePath, boolToInt(persistent), publicToken, now,
	)
	if err != nil {
		return nil, err
	}

	return &models.File{
		ID:           id,
		UserID:       userID,
		Purpose:      purpose,
		MimeType:     mimeType,
		OriginalName: originalName,
		Path:         filePath,
		Persistent:   persistent,
		PublicToken:  publicToken,
		CreatedAt:    now,
	}, nil
}

func GetFileByID(id string) (*models.File, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var f models.File
	var persistent int
	var originalName sql.NullString
	err := db.QueryRow(
		`SELECT id, userId, purpose, mimeType, originalName, path, persistent, publicToken, createdAt
		FROM files WHERE id = ?`,
		id,
	).Scan(&f.ID, &f.UserID, &f.Purpose, &f.MimeType, &originalName, &f.Path, &persistent, &f.PublicToken, &f.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	f.Persistent = persistent != 0
	if originalName.Valid {
		f.OriginalName = originalName.String
	}
	return &f, nil
}

func DeleteFile(id string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM files WHERE id = ?", id)
	return err
}

func CleanupExpiredFiles(cfg *config.Config) {
	settings, retentionHours, err := GetSettings()
	if err != nil {
		log.Printf("[cleanup] Error getting settings: %v", err)
		retentionHours = cfg.FileRetentionHours
	} else {
		retentionHours = settings.FileRetentionHours
	}

	cutoff := models.Now() - int64(retentionHours)*3600*1000

	dbMu.Lock()
	defer dbMu.Unlock()

	// Get files to delete
	rows, err := db.Query(
		"SELECT id, path FROM files WHERE persistent = 0 AND createdAt < ?",
		cutoff,
	)
	if err != nil {
		log.Printf("[cleanup] Error querying expired files: %v", err)
		return
	}

	var toDelete []struct {
		ID   string
		Path string
	}
	for rows.Next() {
		var id, path string
		if err := rows.Scan(&id, &path); err == nil {
			toDelete = append(toDelete, struct {
				ID   string
				Path string
			}{id, path})
		}
	}
	rows.Close()

	if len(toDelete) == 0 {
		return
	}

	// Delete files
	for _, f := range toDelete {
		os.Remove(f.Path)
	}

	// Delete from database
	for _, f := range toDelete {
		db.Exec("DELETE FROM files WHERE id = ?", f.ID)
	}

	// Clean up generations with missing output files
	db.Exec(
		"UPDATE generations SET outputFileId = NULL WHERE outputFileId IS NOT NULL AND outputFileId NOT IN (SELECT id FROM files)",
	)

	// Clean up library items with missing files
	db.Exec(
		"DELETE FROM library WHERE fileId NOT IN (SELECT id FROM files)",
	)

	log.Printf("[cleanup] Removed %d expired files (retention %dh)", len(toDelete), retentionHours)
}

// ========== Generation operations ==========

func CreateGeneration(g *models.Generation) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	refFileIDs, _ := json.Marshal(g.ReferenceFileIDs)

	_, err := db.Exec(
		`INSERT INTO generations (id, userId, type, prompt, model, status, progress, startedAt, elapsedSeconds, error,
			providerTaskId, providerResultUrl, referenceFileIds, imageSize, aspectRatio,
			favorite, outputFileId, createdAt, updatedAt, duration, videoSize, runId, nodePosition)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.ID, g.UserID, g.Type, g.Prompt, g.Model, g.Status, g.Progress, g.StartedAt, g.ElapsedSeconds, g.Error,
		g.ProviderTaskID, g.ProviderResultURL, string(refFileIDs), g.ImageSize, g.AspectRatio,
		boolToInt(g.Favorite), g.OutputFileID, g.CreatedAt, g.UpdatedAt, g.Duration, g.VideoSize, g.RunID, g.NodePosition,
	)
	return err
}

func GetGenerationByID(id string) (*models.Generation, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	return getGenerationByIDInternal(id)
}

func getGenerationByIDInternal(id string) (*models.Generation, error) {
	var g models.Generation
	var progress, refFileIDs, imageSize, aspectRatio, errorStr, providerTaskID, providerResultURL, outputFileID, videoSize, runID sql.NullString
	var startedAt, elapsedSeconds, duration, nodePosition sql.NullInt64
	var favorite int

	err := db.QueryRow(
		`SELECT id, userId, type, prompt, model, status, progress, startedAt, elapsedSeconds, error,
			providerTaskId, providerResultUrl, referenceFileIds, imageSize, aspectRatio,
			favorite, outputFileId, createdAt, updatedAt, duration, videoSize, runId, nodePosition
		FROM generations WHERE id = ?`,
		id,
	).Scan(&g.ID, &g.UserID, &g.Type, &g.Prompt, &g.Model, &g.Status, &progress, &startedAt, &elapsedSeconds, &errorStr,
		&providerTaskID, &providerResultURL, &refFileIDs, &imageSize, &aspectRatio,
		&favorite, &outputFileID, &g.CreatedAt, &g.UpdatedAt, &duration, &videoSize, &runID, &nodePosition)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	g.Favorite = favorite != 0

	if progress.Valid {
		p, _ := parseFloat(progress.String)
		g.Progress = &p
	}
	if startedAt.Valid {
		ts := startedAt.Int64
		g.StartedAt = &ts
	}
	if elapsedSeconds.Valid {
		sec := elapsedSeconds.Int64
		g.ElapsedSeconds = &sec
	}
	if errorStr.Valid {
		g.Error = &errorStr.String
	}
	if providerTaskID.Valid {
		g.ProviderTaskID = &providerTaskID.String
	}
	if providerResultURL.Valid {
		g.ProviderResultURL = &providerResultURL.String
	}
	if imageSize.Valid {
		g.ImageSize = &imageSize.String
	}
	if aspectRatio.Valid {
		g.AspectRatio = &aspectRatio.String
	}
	if outputFileID.Valid {
		g.OutputFileID = &outputFileID.String
	}
	if duration.Valid {
		d := int(duration.Int64)
		g.Duration = &d
	}
	if videoSize.Valid {
		g.VideoSize = &videoSize.String
	}
	if runID.Valid {
		g.RunID = &runID.String
	}
	if nodePosition.Valid {
		np := int(nodePosition.Int64)
		g.NodePosition = &np
	}

	if refFileIDs.Valid {
		json.Unmarshal([]byte(refFileIDs.String), &g.ReferenceFileIDs)
	}
	if g.ReferenceFileIDs == nil {
		g.ReferenceFileIDs = []string{}
	}

	return &g, nil
}

func ListGenerations(userID, genType string, favoritesOnly bool, limit, offset int) ([]models.Generation, int, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	// Build query
	query := "SELECT id FROM generations WHERE userId = ?"
	args := []interface{}{userID}

	if genType != "" {
		query += " AND type = ?"
		args = append(args, genType)
	}
	if favoritesOnly {
		query += " AND favorite = 1"
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM generations WHERE userId = ?"
	countArgs := []interface{}{userID}
	if genType != "" {
		countQuery += " AND type = ?"
		countArgs = append(countArgs, genType)
	}
	if favoritesOnly {
		countQuery += " AND favorite = 1"
	}

	var total int
	if err := db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get paginated results
	query += " ORDER BY createdAt DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var generations []models.Generation
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, err
		}
		g, err := getGenerationByIDInternal(id)
		if err != nil {
			return nil, 0, err
		}
		if g != nil {
			generations = append(generations, *g)
		}
	}

	return generations, total, nil
}

func UpdateGeneration(id string, updates map[string]interface{}) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	updates["updatedAt"] = models.Now()

	query := "UPDATE generations SET "
	args := []interface{}{}
	first := true

	for key, value := range updates {
		if !first {
			query += ", "
		}
		query += key + " = ?"
		args = append(args, value)
		first = false
	}
	query += " WHERE id = ?"
	args = append(args, id)

	_, err := db.Exec(query, args...)
	return err
}

func DeleteGeneration(id string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM generations WHERE id = ?", id)
	return err
}

func GetPendingGenerations() ([]models.Generation, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	rows, err := db.Query(
		"SELECT id FROM generations WHERE status IN ('queued', 'running')",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var generations []models.Generation
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		g, err := getGenerationByIDInternal(id)
		if err != nil {
			return nil, err
		}
		if g != nil {
			generations = append(generations, *g)
		}
	}
	return generations, nil
}

func GetMaxNodePosition(userID, runID string) (int, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var maxPos sql.NullInt64
	err := db.QueryRow(
		"SELECT MAX(nodePosition) FROM generations WHERE userId = ? AND type = 'video' AND runId = ?",
		userID, runID,
	).Scan(&maxPos)
	if err != nil {
		return -1, err
	}
	if !maxPos.Valid {
		return -1, nil
	}
	return int(maxPos.Int64), nil
}

// ========== Preset operations ==========

func ListPresets(userID string) ([]models.Preset, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	rows, err := db.Query(
		"SELECT id, userId, name, prompt, createdAt FROM presets WHERE userId = ? ORDER BY createdAt DESC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var presets []models.Preset
	for rows.Next() {
		var p models.Preset
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Prompt, &p.CreatedAt); err != nil {
			return nil, err
		}
		presets = append(presets, p)
	}
	return presets, nil
}

func CreatePreset(userID, name, prompt string) (*models.Preset, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	id := uuid.New().String()
	now := models.Now()

	_, err := db.Exec(
		"INSERT INTO presets (id, userId, name, prompt, createdAt) VALUES (?, ?, ?, ?, ?)",
		id, userID, name, prompt, now,
	)
	if err != nil {
		return nil, err
	}

	return &models.Preset{
		ID:        id,
		UserID:    userID,
		Name:      name,
		Prompt:    prompt,
		CreatedAt: now,
	}, nil
}

func DeletePreset(userID, id string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM presets WHERE id = ? AND userId = ?", id, userID)
	return err
}

// ========== Library operations ==========

func ListLibrary(userID, kind string) ([]models.LibraryItem, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	query := "SELECT id, userId, kind, name, fileId, createdAt FROM library WHERE userId = ?"
	args := []interface{}{userID}
	if kind != "" {
		query += " AND kind = ?"
		args = append(args, kind)
	}
	query += " ORDER BY createdAt DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []models.LibraryItem
	for rows.Next() {
		var l models.LibraryItem
		if err := rows.Scan(&l.ID, &l.UserID, &l.Kind, &l.Name, &l.FileID, &l.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, l)
	}
	return items, nil
}

func CreateLibraryItem(userID, kind, name, fileID string) (*models.LibraryItem, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	id := uuid.New().String()
	now := models.Now()

	_, err := db.Exec(
		"INSERT INTO library (id, userId, kind, name, fileId, createdAt) VALUES (?, ?, ?, ?, ?, ?)",
		id, userID, kind, name, fileID, now,
	)
	if err != nil {
		return nil, err
	}

	return &models.LibraryItem{
		ID:        id,
		UserID:    userID,
		Kind:      kind,
		Name:      name,
		FileID:    fileID,
		CreatedAt: now,
	}, nil
}

func GetLibraryItem(userID, id string) (*models.LibraryItem, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var l models.LibraryItem
	err := db.QueryRow(
		"SELECT id, userId, kind, name, fileId, createdAt FROM library WHERE id = ? AND userId = ?",
		id, userID,
	).Scan(&l.ID, &l.UserID, &l.Kind, &l.Name, &l.FileID, &l.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

func DeleteLibraryItem(userID, id string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM library WHERE id = ? AND userId = ?", id, userID)
	return err
}

// ========== Reference Upload operations ==========

func ListReferenceUploads(userID string, limit int) ([]models.ReferenceUpload, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := db.Query(
		"SELECT id, userId, fileId, createdAt FROM reference_uploads WHERE userId = ? ORDER BY createdAt DESC LIMIT ?",
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uploads []models.ReferenceUpload
	for rows.Next() {
		var u models.ReferenceUpload
		if err := rows.Scan(&u.ID, &u.UserID, &u.FileID, &u.CreatedAt); err != nil {
			return nil, err
		}
		uploads = append(uploads, u)
	}
	return uploads, nil
}

func CountReferenceUploads(userID string) (int, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM reference_uploads WHERE userId = ?", userID).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func ListReferenceUploadsToTrim(userID string, keep int) ([]models.ReferenceUpload, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	if keep < 0 {
		keep = 0
	}

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM reference_uploads WHERE userId = ?", userID).Scan(&total); err != nil {
		return nil, err
	}
	if total <= keep {
		return nil, nil
	}

	toDelete := total - keep
	rows, err := db.Query(
		"SELECT id, userId, fileId, createdAt FROM reference_uploads WHERE userId = ? ORDER BY createdAt ASC LIMIT ?",
		userID, toDelete,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uploads []models.ReferenceUpload
	for rows.Next() {
		var u models.ReferenceUpload
		if err := rows.Scan(&u.ID, &u.UserID, &u.FileID, &u.CreatedAt); err != nil {
			return nil, err
		}
		uploads = append(uploads, u)
	}
	return uploads, nil
}

func CreateReferenceUpload(userID, fileID string) (*models.ReferenceUpload, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	id := uuid.New().String()
	now := models.Now()

	_, err := db.Exec(
		"INSERT INTO reference_uploads (id, userId, fileId, createdAt) VALUES (?, ?, ?, ?)",
		id, userID, fileID, now,
	)
	if err != nil {
		return nil, err
	}

	return &models.ReferenceUpload{
		ID:        id,
		UserID:    userID,
		FileID:    fileID,
		CreatedAt: now,
	}, nil
}

func GetReferenceUpload(userID, id string) (*models.ReferenceUpload, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var u models.ReferenceUpload
	err := db.QueryRow(
		"SELECT id, userId, fileId, createdAt FROM reference_uploads WHERE id = ? AND userId = ?",
		id, userID,
	).Scan(&u.ID, &u.UserID, &u.FileID, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func DeleteReferenceUpload(userID, id string) error {
	dbMu.Lock()
	defer dbMu.Unlock()

	_, err := db.Exec("DELETE FROM reference_uploads WHERE id = ? AND userId = ?", id, userID)
	return err
}

// ========== Video Run operations ==========

func ListVideoRuns(userID string) ([]models.VideoRun, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	rows, err := db.Query(
		"SELECT id, userId, name, createdAt FROM video_runs WHERE userId = ? ORDER BY createdAt ASC",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []models.VideoRun
	for rows.Next() {
		var r models.VideoRun
		if err := rows.Scan(&r.ID, &r.UserID, &r.Name, &r.CreatedAt); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}

func CreateVideoRun(userID, name string) (*models.VideoRun, error) {
	dbMu.Lock()
	defer dbMu.Unlock()

	id := uuid.New().String()
	now := models.Now()

	_, err := db.Exec(
		"INSERT INTO video_runs (id, userId, name, createdAt) VALUES (?, ?, ?, ?)",
		id, userID, name, now,
	)
	if err != nil {
		return nil, err
	}

	return &models.VideoRun{
		ID:        id,
		UserID:    userID,
		Name:      name,
		CreatedAt: now,
	}, nil
}

func GetVideoRun(userID, id string) (*models.VideoRun, error) {
	dbMu.RLock()
	defer dbMu.RUnlock()

	var r models.VideoRun
	err := db.QueryRow(
		"SELECT id, userId, name, createdAt FROM video_runs WHERE id = ? AND userId = ?",
		id, userID,
	).Scan(&r.ID, &r.UserID, &r.Name, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ========== Helper functions ==========

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
