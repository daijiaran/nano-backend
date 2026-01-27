package models

import (
	"time"
)

// --- 数据库表模型 (GORM Tags) ---

type User struct {
	ID           string `gorm:"primaryKey" json:"id"`
	Username     string `gorm:"uniqueIndex;not null" json:"username"`
	Role         string `json:"role"`
	PasswordHash string `json:"-"`
	Disabled     bool   `json:"disabled"`
	CreatedAt    int64  `json:"createdAt"`
}

type Session struct {
	Token     string `gorm:"primaryKey" json:"token"`
	UserID    string `gorm:"index" json:"userId"`
	CreatedAt int64  `json:"createdAt"`
	ExpiresAt int64  `json:"expiresAt"`
}

type UserProvider struct {
	UserID       string `gorm:"primaryKey" json:"userId"`
	ProviderHost string `json:"providerHost"`
	APIKeyEnc    string `json:"-"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type File struct {
	ID           string `gorm:"primaryKey" json:"id"`
	UserID       string `gorm:"index" json:"userId"`
	Purpose      string `json:"purpose"`
	MimeType     string `json:"mimeType"`
	OriginalName string `json:"originalName,omitempty"`
	Path         string `json:"-"`
	Persistent   bool   `json:"persistent"`
	PublicToken  string `gorm:"uniqueIndex" json:"-"`
	CreatedAt    int64  `json:"createdAt"`
}

type Generation struct {
	ID                string   `gorm:"primaryKey" json:"id"`
	UserID            string   `gorm:"index" json:"userId"`
	Type              string   `json:"type"`
	Prompt            string   `json:"prompt"`
	Model             string   `json:"model"`
	Status            string   `json:"status"`
	Progress          *float64 `json:"progress,omitempty"`
	StartedAt         *int64   `json:"startedAt,omitempty"`
	ElapsedSeconds    *int64   `json:"elapsedSeconds,omitempty"`
	Error             *string  `json:"error,omitempty"`
	ProviderTaskID    *string  `json:"-"`
	ProviderResultURL *string  `json:"-"`
	ReferenceFileIDs  []string `gorm:"serializer:json" json:"referenceFileIds"`
	ImageSize         *string  `json:"imageSize,omitempty"`
	AspectRatio       *string  `json:"aspectRatio,omitempty"`
	Favorite          bool     `json:"favorite"`
	OutputFileID      *string  `json:"-"`
	Duration          *int     `json:"duration,omitempty"`
	VideoSize         *string  `json:"videoSize,omitempty"`
	RunID             *string  `gorm:"index" json:"runId,omitempty"`
	NodePosition      *int     `json:"nodePosition,omitempty"`
	CreatedAt         int64    `json:"createdAt"`
	UpdatedAt         int64    `json:"updatedAt"`
}

type Preset struct {
	ID        string `gorm:"primaryKey" json:"id"`
	UserID    string `gorm:"index" json:"userId"`
	Name      string `json:"name"`
	Prompt    string `json:"prompt"`
	CreatedAt int64  `json:"createdAt"`
}

type LibraryItem struct {
	ID        string `gorm:"primaryKey" json:"id"`
	UserID    string `gorm:"index" json:"userId"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	FileID    string `json:"fileId"`
	CreatedAt int64  `json:"createdAt"`
}

type ReferenceUpload struct {
	ID        string `gorm:"primaryKey" json:"id"`
	UserID    string `gorm:"index" json:"userId"`
	FileID    string `json:"fileId"`
	CreatedAt int64  `json:"createdAt"`
}

type VideoRun struct {
	ID        string `gorm:"primaryKey" json:"id"`
	UserID    string `gorm:"index" json:"userId"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
}

// --- API 响应与业务逻辑模型 (纯 Go 定义) ---

type ModelInfo struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	SupportsImageSize   bool     `json:"supportsImageSize"`
	SupportsAspectRatio bool     `json:"supportsAspectRatio"`
	Tags                []string `json:"tags"`
}

type GenerationResponse struct {
	ID               string      `json:"id"`
	Type             string      `json:"type"`
	Prompt           string      `json:"prompt"`
	Model            string      `json:"model"`
	Status           string      `json:"status"`
	Progress         *float64    `json:"progress"`
	StartedAt        *int64      `json:"startedAt"`
	ElapsedSeconds   *int64      `json:"elapsedSeconds"`
	Error            *string     `json:"error"`
	Favorite         bool        `json:"favorite"`
	ImageSize        *string     `json:"imageSize"`
	AspectRatio      *string     `json:"aspectRatio"`
	Duration         *int        `json:"duration"`
	VideoSize        *string     `json:"videoSize"`
	ReferenceFileIDs []string    `json:"referenceFileIds"`
	OutputFile       *StoredFile `json:"outputFile"`
	RunID            *string     `json:"runId"`
	NodePosition     *int        `json:"nodePosition"`
	CreatedAt        int64       `json:"createdAt"`
	UpdatedAt        int64       `json:"updatedAt"`
}

type StoredFile struct {
	ID        string `json:"id"`
	MimeType  string `json:"mimeType"`
	CreatedAt int64  `json:"createdAt"`
	Filename  string `json:"filename,omitempty"`
	URL       string `json:"url"`
}

type LibraryItemResponse struct {
	ID        string      `json:"id"`
	Kind      string      `json:"kind"`
	Name      string      `json:"name"`
	CreatedAt int64       `json:"createdAt"`
	File      *StoredFile `json:"file"`
}

type ReferenceUploadResponse struct {
	ID           string      `json:"id"`
	CreatedAt    int64       `json:"createdAt"`
	File         *StoredFile `json:"file"`
	OriginalName string      `json:"originalName,omitempty"`
}

type SanitizedUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled"`
}

type Settings struct {
	FileRetentionHours    int `json:"fileRetentionHours"`
	ReferenceHistoryLimit int `json:"referenceHistoryLimit"`
	ImageTimeoutSeconds   int `json:"imageTimeoutSeconds"`
	VideoTimeoutSeconds   int `json:"videoTimeoutSeconds"`
}

// --- 影视项目审阅系统模型 ---

type ReviewProject struct {
	ID           string `gorm:"primaryKey" json:"id"`
	UserID       string `gorm:"index" json:"userId"` // 创建者
	Name         string `json:"name"`
	CoverFileID  string `json:"coverFileId"`           // 关联 File 表 ID
	EpisodeCount int    `gorm:"-" json:"episodeCount"` // 动态计算或缓存
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type ReviewEpisode struct {
	ID              string `gorm:"primaryKey" json:"id"`
	ProjectID       string `gorm:"index" json:"projectId"`
	UserID          string `gorm:"index" json:"userId"`
	Name            string `json:"name"`
	CoverFileID     string `json:"coverFileId"`
	StoryboardCount int    `gorm:"-" json:"storyboardCount"`
	SortOrder       int    `json:"sortOrder"`
	CreatedAt       int64  `json:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt"`
}

type ReviewStoryboard struct {
	ID          string `gorm:"primaryKey" json:"id"`
	EpisodeID   string `gorm:"index" json:"episodeId"`
	UserID      string `gorm:"index" json:"userId"` // 创建者
	Name        string `json:"name"`                // 分镜名称
	ImageFileID string `json:"imageFileId"`         // 必须有图
	Status      string `json:"status"`              // pending(未审阅), approved(通过), rejected(未通过)
	Feedback    string `json:"feedback"`            // 修改建议
	SortOrder   int    `json:"sortOrder"`           // 用于拖拽排序
	CreatedAt   int64  `json:"createdAt"`
	UpdatedAt   int64  `json:"updatedAt"`
}

// 响应结构体 (用于前端展示)
type ReviewStoryboardResponse struct {
	ReviewStoryboard
	ImageURL string `json:"imageUrl"`
}

// --- 工具函数 ---

func Now() int64 {
	return time.Now().UnixMilli()
}
