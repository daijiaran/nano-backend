package handlers

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"nano-backend/internal/config"
	"nano-backend/internal/crypto"
	"nano-backend/internal/database"
	"nano-backend/internal/fileutil"
	"nano-backend/internal/middleware"
	"nano-backend/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

var cfg *config.Config

func init() {
	cfg = config.Load()
}

// ========== Health Check ==========

func HealthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{"ok": true})
}

// ========== Auth Handlers ==========

func Login(c *fiber.Ctx) error {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		log.Printf("[auth] Login parse error: %v", err)
		return c.Status(400).JSON(fiber.Map{"error": "请求格式错误"})
	}

	log.Printf("[auth] Login attempt for user: %s", body.Username)

	user, err := database.GetUserByUsername(body.Username)
	if err != nil {
		log.Printf("[auth] Database error: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if user == nil {
		log.Printf("[auth] User not found: %s", body.Username)
		return c.Status(401).JSON(fiber.Map{"error": "用户名或密码错误"})
	}

	if !crypto.VerifyPassword(body.Password, user.PasswordHash) {
		log.Printf("[auth] Invalid password for user: %s", body.Username)
		return c.Status(401).JSON(fiber.Map{"error": "用户名或密码错误"})
	}

	// Check if user is disabled
	if user.Disabled {
		log.Printf("[auth] User is disabled: %s", body.Username)
		return c.Status(403).JSON(fiber.Map{"error": "账号已被禁用，请联系管理员"})
	}

	// === 新增互斥登录检查 ===
	// 方案第5点：检查状态。如果已登录且心跳在有效期内，则拒绝。
	// 这里加一个宽限期（例如1分钟），防止因为网络波动导致的误判
	activeTimeout := int64(10 * 60 * 1000) // 10分钟
	if user.IsLoggedIn && (models.Now()-user.LastHeartbeatAt < activeTimeout) {
		log.Printf("[auth] Login rejected: User %s is already logged in", body.Username)
		return c.Status(409).JSON(fiber.Map{
			"error": "该账号已在其他设备登录，请先退出或等待系统自动清理",
		})
	}
	// =====================

	session, err := database.CreateSession(user.ID, cfg.SessionTTLHours)
	if err != nil {
		log.Printf("[auth] Failed to create session: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	// === 更新状态为已登录 ===
	if err := database.UpdateLoginStatus(user.ID, true); err != nil {
		log.Printf("[auth] Failed to update login status: %v", err)
	}

	log.Printf("[auth] Login successful for user: %s", body.Username)

	return c.JSON(fiber.Map{
		"token": session.Token,
		"user": models.SanitizedUser{
			ID:       user.ID,
			Username: user.Username,
			Role:     user.Role,
			Disabled: user.Disabled,
		},
	})
}

func Logout(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	token := middleware.GetToken(c)

	if err := database.DeleteSession(token); err != nil {
		log.Printf("[auth] Logout session error: %v", err)
	}

	// === 方案第3点：将状态置为未登录 ===
	if user != nil {
		if err := database.UpdateLoginStatus(user.ID, false); err != nil {
			log.Printf("[auth] Update status error: %v", err)
		}
	}

	log.Printf("[auth] User logged out")
	return c.JSON(fiber.Map{"ok": true})
}

func GetCurrentUser(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	return c.JSON(user)
}

// Heartbeat 接收前端的保活请求
func Heartbeat(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)

	if err := database.UpdateHeartbeat(user.ID); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

// ========== Models Handler ==========

var supportedModels = []models.ModelInfo{
	{
		ID:                  "nano-banana-fast",
		Name:                "Nano Banana Fast",
		Type:                "image",
		SupportsImageSize:   true, // 所有图片模型都支持分辨率选择
		SupportsAspectRatio: true,
		Tags:                []string{"fast", "1K"},
	},
	{
		ID:                  "nano-banana",
		Name:                "Nano Banana",
		Type:                "image",
		SupportsImageSize:   true, // 所有图片模型都支持分辨率选择
		SupportsAspectRatio: true,
		Tags:                []string{"1K"},
	},
	{
		ID:                  "nano-banana-pro",
		Name:                "Nano Banana Pro",
		Type:                "image",
		SupportsImageSize:   true,
		SupportsAspectRatio: true,
		Tags:                []string{"pro", "1K/2K/4K"},
	},
	{
		ID:                  "nano-banana-pro-vt",
		Name:                "Nano Banana Pro VT",
		Type:                "image",
		SupportsImageSize:   true,
		SupportsAspectRatio: true,
		Tags:                []string{"pro", "vt", "1K/2K/4K"},
	},
	{
		ID:                  "sora-2",
		Name:                "Sora 2",
		Type:                "video",
		SupportsAspectRatio: true,
		Tags:                []string{"video"},
	},
}

func GetModels(c *fiber.Ctx) error {
	return c.JSON(supportedModels)
}

func GetModelByID(modelID string) *models.ModelInfo {
	for _, m := range supportedModels {
		if m.ID == modelID {
			return &m
		}
	}
	return nil
}

// ========== Provider Settings Handlers ==========

func GetProviderSettings(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	provider, err := database.GetUserProvider(user.ID)
	if err != nil {
		log.Printf("[provider] Error getting provider: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	providerHost := cfg.DefaultProviderHost
	hasAPIKey := cfg.DefaultProviderAPIKey != ""

	if provider != nil {
		providerHost = provider.ProviderHost
		hasAPIKey = provider.APIKeyEnc != "" || cfg.DefaultProviderAPIKey != ""
	}

	return c.JSON(fiber.Map{
		"providerHost": providerHost,
		"hasApiKey":    hasAPIKey,
	})
}

func UpdateProviderSettings(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)

	var body struct {
		ProviderHost string `json:"providerHost"`
		APIKey       string `json:"apiKey"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "请求格式错误"})
	}

	providerHost := strings.TrimSpace(body.ProviderHost)
	if providerHost == "" {
		return c.Status(400).JSON(fiber.Map{"error": "服务地址不能为空"})
	}

	if err := database.SetUserProvider(user.ID, providerHost, body.APIKey, cfg); err != nil {
		log.Printf("[provider] Error setting provider: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	log.Printf("[provider] Updated provider settings for user %s", user.Username)

	provider, _ := database.GetUserProvider(user.ID)
	hasAPIKey := false
	if provider != nil && provider.APIKeyEnc != "" {
		hasAPIKey = true
	} else if cfg.DefaultProviderAPIKey != "" {
		hasAPIKey = true
	}

	return c.JSON(fiber.Map{
		"providerHost": providerHost,
		"hasApiKey":    hasAPIKey,
	})
}

// ========== Admin Handlers ==========

func AdminListUsers(c *fiber.Ctx) error {
	users, err := database.ListUsers()
	if err != nil {
		log.Printf("[admin] Error listing users: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	result := make([]fiber.Map, len(users))
	for i, u := range users {
		result[i] = fiber.Map{
			"id":        u.ID,
			"username":  u.Username,
			"role":      u.Role,
			"disabled":  u.Disabled,
			"createdAt": u.CreatedAt,
		}
	}

	return c.JSON(result)
}

func AdminCreateUser(c *fiber.Ctx) error {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "请求格式错误"})
	}

	username := strings.TrimSpace(body.Username)
	if username == "" {
		return c.Status(400).JSON(fiber.Map{"error": "用户名不能为空"})
	}
	if len(body.Password) < 6 {
		return c.Status(400).JSON(fiber.Map{"error": "密码长度不能少于 6 个字符"})
	}

	role := body.Role
	if role == "" {
		role = "user"
	}
	if role != "admin" && role != "user" {
		return c.Status(400).JSON(fiber.Map{"error": "角色不正确"})
	}

	user, err := database.CreateUser(username, body.Password, role)
	if err != nil {
		log.Printf("[admin] Error creating user: %v", err)
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}

	log.Printf("[admin] Created user: %s (role: %s)", user.Username, user.Role)

	return c.JSON(fiber.Map{
		"id":        user.ID,
		"username":  user.Username,
		"role":      user.Role,
		"disabled":  user.Disabled,
		"createdAt": user.CreatedAt,
	})
}

func AdminDeleteUser(c *fiber.Ctx) error {
	currentUser := middleware.GetCurrentUser(c)
	userID := c.Params("id")

	// Prevent self-deletion
	if userID == currentUser.ID {
		return c.Status(400).JSON(fiber.Map{"error": "不能删除自己的账号"})
	}

	// Get user to check if exists and not the last admin
	user, err := database.GetUserByID(userID)
	if err != nil {
		log.Printf("[admin] Error getting user: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "用户不存在"})
	}

	if err := database.DeleteUser(userID); err != nil {
		log.Printf("[admin] Error deleting user: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	log.Printf("[admin] Deleted user: %s", user.Username)

	return c.JSON(fiber.Map{"ok": true})
}

func AdminUpdateUserStatus(c *fiber.Ctx) error {
	currentUser := middleware.GetCurrentUser(c)
	userID := c.Params("id")

	var body struct {
		Disabled bool `json:"disabled"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "请求格式错误"})
	}

	// Prevent self-disabling
	if userID == currentUser.ID && body.Disabled {
		return c.Status(400).JSON(fiber.Map{"error": "不能禁用自己的账号"})
	}

	// Get user to check if exists
	user, err := database.GetUserByID(userID)
	if err != nil {
		log.Printf("[admin] Error getting user: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if user == nil {
		return c.Status(404).JSON(fiber.Map{"error": "用户不存在"})
	}

	if err := database.UpdateUserDisabled(userID, body.Disabled); err != nil {
		log.Printf("[admin] Error updating user status: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	statusText := "启用"
	if body.Disabled {
		statusText = "禁用"
	}
	log.Printf("[admin] Updated user %s status to %s", user.Username, statusText)

	return c.JSON(fiber.Map{
		"id":        user.ID,
		"username":  user.Username,
		"role":      user.Role,
		"disabled":  body.Disabled,
		"createdAt": user.CreatedAt,
	})
}

func AdminGetSettings(c *fiber.Ctx) error {
	settings, _, err := database.GetSettings()
	if err != nil {
		log.Printf("[admin] Error getting settings: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	return c.JSON(settings)
}

func AdminUpdateSettings(c *fiber.Ctx) error {
	var body struct {
		FileRetentionHours    *int `json:"fileRetentionHours"`
		ReferenceHistoryLimit *int `json:"referenceHistoryLimit"`
		ImageTimeoutSeconds   *int `json:"imageTimeoutSeconds"`
		VideoTimeoutSeconds   *int `json:"videoTimeoutSeconds"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "请求格式错误"})
	}

	// 获取当前设置值
	currentSettings, _, err := database.GetSettings()
	if err != nil {
		log.Printf("[admin] Error getting current settings: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	// 使用提供的值或当前值（支持部分更新）
	fileRetentionHours := currentSettings.FileRetentionHours
	if body.FileRetentionHours != nil {
		if *body.FileRetentionHours < 1 {
			return c.Status(400).JSON(fiber.Map{"error": "文件保留时间必须大于等于 1 小时"})
		}
		fileRetentionHours = *body.FileRetentionHours
	}

	referenceHistoryLimit := currentSettings.ReferenceHistoryLimit
	if body.ReferenceHistoryLimit != nil {
		if *body.ReferenceHistoryLimit < 1 {
			return c.Status(400).JSON(fiber.Map{"error": "参考历史限制必须大于等于 1"})
		}
		referenceHistoryLimit = *body.ReferenceHistoryLimit
	}

	imageTimeoutSeconds := currentSettings.ImageTimeoutSeconds
	if body.ImageTimeoutSeconds != nil {
		if *body.ImageTimeoutSeconds < 30 {
			return c.Status(400).JSON(fiber.Map{"error": "图片生成超时时间必须大于等于 30 秒"})
		}
		imageTimeoutSeconds = *body.ImageTimeoutSeconds
	}

	videoTimeoutSeconds := currentSettings.VideoTimeoutSeconds
	if body.VideoTimeoutSeconds != nil {
		if *body.VideoTimeoutSeconds < 30 {
			return c.Status(400).JSON(fiber.Map{"error": "视频生成超时时间必须大于等于 30 秒"})
		}
		videoTimeoutSeconds = *body.VideoTimeoutSeconds
	}

	// 更新设置
	if err := database.UpdateSettings(fileRetentionHours, referenceHistoryLimit, imageTimeoutSeconds, videoTimeoutSeconds); err != nil {
		log.Printf("[admin] Error updating settings: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	log.Printf(
		"[admin] Updated settings: fileRetentionHours=%d, referenceHistoryLimit=%d, imageTimeoutSeconds=%d, videoTimeoutSeconds=%d",
		fileRetentionHours,
		referenceHistoryLimit,
		imageTimeoutSeconds,
		videoTimeoutSeconds,
	)

	return c.JSON(fiber.Map{
		"fileRetentionHours":    fileRetentionHours,
		"referenceHistoryLimit": referenceHistoryLimit,
		"imageTimeoutSeconds":   imageTimeoutSeconds,
		"videoTimeoutSeconds":   videoTimeoutSeconds,
	})
}

// ========== Generation Handlers ==========

func ListGenerations(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	token := middleware.GetToken(c)

	genType := c.Query("type")
	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)
	favoritesOnly := c.Query("favorites") == "1" || c.Query("onlyFavorites") == "1"

	if limit > 200 {
		limit = 200
	}
	if limit < 1 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	generations, total, err := database.ListGenerations(user.ID, genType, favoritesOnly, limit, offset)
	if err != nil {
		log.Printf("[generation] Error listing generations: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	items := make([]models.GenerationResponse, len(generations))
	for i, g := range generations {
		items[i] = toGenerationResponse(&g, token)
	}

	return c.JSON(fiber.Map{
		"items": items,
		"total": total,
	})
}

func GetGeneration(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	id := c.Params("id")
	token := middleware.GetToken(c)

	gen, err := database.GetGenerationByID(id)
	if err != nil {
		log.Printf("[generation] Error getting generation: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if gen == nil || gen.UserID != user.ID {
		return c.Status(404).JSON(fiber.Map{"error": "未找到"})
	}

	return c.JSON(toGenerationResponse(gen, token))
}

func ToggleFavorite(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	id := c.Params("id")
	token := middleware.GetToken(c)

	gen, err := database.GetGenerationByID(id)
	if err != nil {
		log.Printf("[generation] Error getting generation: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if gen == nil || gen.UserID != user.ID {
		return c.Status(404).JSON(fiber.Map{"error": "未找到"})
	}

	newFavorite := !gen.Favorite
	if err := database.UpdateGeneration(id, map[string]interface{}{
		"favorite": boolToInt(newFavorite),
	}); err != nil {
		log.Printf("[generation] Error updating favorite: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	// 重新获取更新后的完整 Generation 对象
	updatedGen, err := database.GetGenerationByID(id)
	if err != nil {
		log.Printf("[generation] Error getting updated generation: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if updatedGen == nil {
		return c.Status(404).JSON(fiber.Map{"error": "未找到"})
	}

	log.Printf("[generation] Toggled favorite for generation %s to %v", id, newFavorite)

	// 返回完整的 Generation 响应对象
	return c.JSON(toGenerationResponse(updatedGen, token))
}

func DeleteGeneration(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	id := c.Params("id")

	gen, err := database.GetGenerationByID(id)
	if err != nil {
		log.Printf("[generation] Error getting generation: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if gen == nil || gen.UserID != user.ID {
		return c.Status(404).JSON(fiber.Map{"error": "未找到"})
	}

	outputFileID := gen.OutputFileID

	if err := database.DeleteGeneration(id); err != nil {
		log.Printf("[generation] Error deleting generation: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	// Delete output file if not used elsewhere
	if outputFileID != nil {
		// For simplicity, we just delete the file
		file, _ := database.GetFileByID(*outputFileID)
		if file != nil {
			fileutil.RemoveWithThumb(file.Path)
			database.DeleteFile(*outputFileID)
		}
	}

	log.Printf("[generation] Deleted generation %s", id)

	return c.JSON(fiber.Map{"ok": true})
}

func GenerateImage(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	token := middleware.GetToken(c)

	// 解析JSON请求体
	var body struct {
		Prompt      string `json:"prompt"`
		Model       string `json:"model"`
		ImageSize   string `json:"imageSize"`
		AspectRatio string `json:"aspectRatio"`
		Batch       int    `json:"batch"`
		// 新的有序参考图列表格式
		ReferenceList []struct {
			Type  string `json:"type"`  // "fileId" 或 "base64"
			Value string `json:"value"` // fileId 或 base64 数据
		} `json:"referenceList"`
		// 兼容旧格式
		ReferenceFileIDs    []string `json:"referenceFileIds"`
		ReferenceBase64List []string `json:"referenceBase64List"`
	}

	if err := c.BodyParser(&body); err != nil {
		log.Printf("[generation] Parse error: %v", err)
		return c.Status(400).JSON(fiber.Map{"error": "请求格式错误"})
	}

	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "提示词不能为空"})
	}

	model := GetModelByID(body.Model)
	if model == nil || model.Type != "image" {
		return c.Status(400).JSON(fiber.Map{"error": "不支持的模型"})
	}

	batchN := body.Batch
	if batchN < 1 {
		batchN = 1
	}
	if batchN > cfg.ImageBatchMax {
		return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("生成数量必须在 1 到 %d 之间", cfg.ImageBatchMax)})
	}

	imageSize := body.ImageSize
	aspectRatio := body.AspectRatio
	if aspectRatio == "" {
		aspectRatio = "auto"
	}

	var refFileIDs []string

	// 优先使用新的有序参考图列表格式
	if len(body.ReferenceList) > 0 {
		for _, ref := range body.ReferenceList {
			if ref.Type == "fileId" && ref.Value != "" {
				// 验证文件权限
				file, err := database.GetFileByID(ref.Value)
				if err != nil || file == nil || file.UserID != user.ID {
					return c.Status(400).JSON(fiber.Map{"error": "无权限访问参考文件"})
				}
				refFileIDs = append(refFileIDs, ref.Value)
			} else if ref.Type == "base64" && ref.Value != "" {
				// 保存 base64 图片并获取 fileId
				savedFile, err := saveBase64ToFile(user.ID, "reference-upload", ref.Value, false)
				if err != nil {
					log.Printf("[generation] Error saving base64 reference: %v", err)
					continue
				}
				refFileIDs = append(refFileIDs, savedFile.ID)
			}
		}
	} else {
		// 兼容旧格式：先添加 fileIds，再添加 base64（顺序不保证）
		refFileIDs = body.ReferenceFileIDs
		if refFileIDs == nil {
			refFileIDs = []string{}
		}

		// Validate existing refs belong to user
		for _, fid := range refFileIDs {
			file, err := database.GetFileByID(fid)
			if err != nil || file == nil || file.UserID != user.ID {
				return c.Status(400).JSON(fiber.Map{"error": "无权限访问参考文件"})
			}
		}

		// 处理base64上传的参考图
		for _, base64Data := range body.ReferenceBase64List {
			if base64Data == "" {
				continue
			}

			savedFile, err := saveBase64ToFile(user.ID, "reference-upload", base64Data, false)
			if err != nil {
				log.Printf("[generation] Error saving base64 reference: %v", err)
				continue
			}
			refFileIDs = append(refFileIDs, savedFile.ID)
		}
	}

	// Cap to 14 references
	if len(refFileIDs) > 14 {
		refFileIDs = refFileIDs[:14]
	}

	createdAt := models.Now()
	created := make([]models.GenerationResponse, 0, batchN)

	for i := 0; i < batchN; i++ {
		gen := &models.Generation{
			ID:               uuid.New().String(),
			UserID:           user.ID,
			Type:             "image",
			Prompt:           prompt,
			Model:            model.ID,
			Status:           "queued",
			ReferenceFileIDs: refFileIDs,
			CreatedAt:        createdAt,
			UpdatedAt:        createdAt,
		}

		if imageSize != "" {
			gen.ImageSize = &imageSize
		}
		gen.AspectRatio = &aspectRatio

		progress := float64(0)
		gen.Progress = &progress

		if err := database.CreateGeneration(gen); err != nil {
			log.Printf("[generation] Error creating generation: %v", err)
			continue
		}

		created = append(created, toGenerationResponse(gen, token))
	}

	log.Printf("[generation] Created %d image generation tasks for user %s", len(created), user.Username)

	return c.JSON(fiber.Map{"created": created})
}

func GenerateVideo(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	token := middleware.GetToken(c)

	// 解析JSON请求体
	var body struct {
		Prompt           string   `json:"prompt"`
		Model            string   `json:"model"`
		AspectRatio      string   `json:"aspectRatio"`
		Duration         int      `json:"duration"`
		VideoSize        string   `json:"videoSize"`
		RunID            string   `json:"runId"`
		ReferenceFileIDs []string `json:"referenceFileIds"`
		ReferenceBase64  string   `json:"referenceBase64"`
	}

	if err := c.BodyParser(&body); err != nil {
		log.Printf("[generation] Parse error: %v", err)
		return c.Status(400).JSON(fiber.Map{"error": "请求格式错误"})
	}

	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "提示词不能为空"})
	}

	model := GetModelByID(body.Model)
	if model == nil || model.Type != "video" {
		return c.Status(400).JSON(fiber.Map{"error": "不支持的模型"})
	}

	aspectRatio := body.AspectRatio
	if aspectRatio == "" {
		aspectRatio = "9:16"
	}

	duration := body.Duration
	if duration < 2 {
		duration = 2
	}
	if duration > 30 {
		duration = 30
	}

	videoSize := body.VideoSize
	if videoSize == "" {
		videoSize = "small"
	}

	var refFileIDs []string

	// 处理已有的参考文件IDs
	for _, fid := range body.ReferenceFileIDs {
		if fid == "" {
			continue
		}
		file, err := database.GetFileByID(fid)
		if err != nil || file == nil || file.UserID != user.ID {
			return c.Status(400).JSON(fiber.Map{"error": "无权限访问参考文件"})
		}
		refFileIDs = append(refFileIDs, fid)
	}

	// 处理base64上传的参考图
	if body.ReferenceBase64 != "" {
		savedFile, err := saveBase64ToFile(user.ID, "reference-upload", body.ReferenceBase64, false)
		if err != nil {
			log.Printf("[generation] Error saving base64 reference: %v", err)
			return c.Status(400).JSON(fiber.Map{"error": "参考图处理失败"})
		}
		refFileIDs = append(refFileIDs, savedFile.ID)
	}

	// Only 1 reference for Sora2
	if len(refFileIDs) > 1 {
		refFileIDs = refFileIDs[:1]
	}

	// Handle run ID
	runID := body.RunID
	if runID != "" {
		run, err := database.GetVideoRun(user.ID, runID)
		if err != nil || run == nil {
			runID = ""
		}
	}

	if runID == "" {
		// Create default run
		run, err := database.CreateVideoRun(user.ID, "默认流程")
		if err != nil {
			log.Printf("[generation] Error creating video run: %v", err)
			return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
		}
		runID = run.ID
	}

	// Get next node position
	maxPos, _ := database.GetMaxNodePosition(user.ID, runID)
	nextPos := maxPos + 1

	createdAt := models.Now()
	gen := &models.Generation{
		ID:               uuid.New().String(),
		UserID:           user.ID,
		Type:             "video",
		Prompt:           prompt,
		Model:            model.ID,
		Status:           "queued",
		ReferenceFileIDs: refFileIDs,
		AspectRatio:      &aspectRatio,
		Duration:         &duration,
		VideoSize:        &videoSize,
		RunID:            &runID,
		NodePosition:     &nextPos,
		CreatedAt:        createdAt,
		UpdatedAt:        createdAt,
	}

	progress := float64(0)
	gen.Progress = &progress

	if err := database.CreateGeneration(gen); err != nil {
		log.Printf("[generation] Error creating generation: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	log.Printf("[generation] Created video generation task for user %s", user.Username)

	return c.JSON(fiber.Map{
		"created": toGenerationResponse(gen, token),
		"runId":   runID,
	})
}

// ========== Video Run Handlers ==========

func ListVideoRuns(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)

	runs, err := database.ListVideoRuns(user.ID)
	if err != nil {
		log.Printf("[video] Error listing runs: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	return c.JSON(runs)
}

func CreateVideoRun(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)

	var body struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "请求格式错误"})
	}

	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = "新流程"
	}

	run, err := database.CreateVideoRun(user.ID, name)
	if err != nil {
		log.Printf("[video] Error creating run: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	log.Printf("[video] Created video run: %s for user %s", name, user.Username)

	return c.JSON(run)
}

// ========== Preset Handlers ==========

func ListPresets(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)

	presets, err := database.ListPresets(user.ID)
	if err != nil {
		log.Printf("[preset] Error listing presets: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	result := make([]fiber.Map, len(presets))
	for i, p := range presets {
		result[i] = fiber.Map{
			"id":        p.ID,
			"name":      p.Name,
			"prompt":    p.Prompt,
			"createdAt": p.CreatedAt,
		}
	}

	return c.JSON(result)
}

func CreatePreset(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)

	var body struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "请求格式错误"})
	}

	name := strings.TrimSpace(body.Name)
	prompt := strings.TrimSpace(body.Prompt)

	if name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "名称不能为空"})
	}
	if prompt == "" {
		return c.Status(400).JSON(fiber.Map{"error": "提示词不能为空"})
	}

	preset, err := database.CreatePreset(user.ID, name, prompt)
	if err != nil {
		log.Printf("[preset] Error creating preset: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	log.Printf("[preset] Created preset: %s for user %s", name, user.Username)

	return c.JSON(preset)
}

func DeletePreset(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	id := c.Params("id")

	if err := database.DeletePreset(user.ID, id); err != nil {
		log.Printf("[preset] Error deleting preset: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	log.Printf("[preset] Deleted preset %s for user %s", id, user.Username)

	return c.JSON(fiber.Map{"ok": true})
}

// ========== Library Handlers ==========

func ListLibrary(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	token := middleware.GetToken(c)
	kind := c.Query("kind")

	items, err := database.ListLibrary(user.ID, kind)
	if err != nil {
		log.Printf("[library] Error listing library: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	result := make([]models.LibraryItemResponse, len(items))
	for i, item := range items {
		result[i] = models.LibraryItemResponse{
			ID:        item.ID,
			Kind:      item.Kind,
			Name:      item.Name,
			CreatedAt: item.CreatedAt,
		}

		file, err := database.GetFileByID(item.FileID)
		if err == nil && file != nil {
			result[i].File = toStoredFile(file, token)
		}
	}

	return c.JSON(result)
}

func CreateLibraryItem(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	token := middleware.GetToken(c)

	name := strings.TrimSpace(c.FormValue("name"))
	kind := strings.TrimSpace(c.FormValue("kind"))

	if name == "" {
		return c.Status(400).JSON(fiber.Map{"error": "名称不能为空"})
	}
	if kind != "role" && kind != "scene" {
		return c.Status(400).JSON(fiber.Map{"error": "类型不正确"})
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "请上传文件"})
	}

	file, err := fh.Open()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "无法读取文件"})
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "无法读取文件"})
	}

	savedFile, err := saveBufferToFile(user.ID, "library-item", fh.Header.Get("Content-Type"), fh.Filename, buf, true)
	if err != nil {
		log.Printf("[library] Error saving file: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	item, err := database.CreateLibraryItem(user.ID, kind, name, savedFile.ID)
	if err != nil {
		log.Printf("[library] Error creating library item: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	log.Printf("[library] Created library item: %s (%s) for user %s", name, kind, user.Username)

	return c.JSON(models.LibraryItemResponse{
		ID:        item.ID,
		Kind:      item.Kind,
		Name:      item.Name,
		CreatedAt: item.CreatedAt,
		File:      toStoredFile(savedFile, token),
	})
}

func DeleteLibraryItem(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	id := c.Params("id")

	item, err := database.GetLibraryItem(user.ID, id)
	if err != nil {
		log.Printf("[library] Error getting library item: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if item == nil {
		return c.Status(404).JSON(fiber.Map{"error": "未找到"})
	}

	if err := database.DeleteLibraryItem(user.ID, id); err != nil {
		log.Printf("[library] Error deleting library item: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	log.Printf("[library] Deleted library item %s for user %s", id, user.Username)

	return c.JSON(fiber.Map{"ok": true})
}

// ========== Reference Upload Handlers ==========

func ListReferenceUploads(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	token := middleware.GetToken(c)

	limit := 0
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}

	settings, _, err := database.GetSettings()
	settingsLimit := 50
	if err == nil && settings != nil && settings.ReferenceHistoryLimit > 0 {
		settingsLimit = settings.ReferenceHistoryLimit
	}
	if limit < 1 {
		limit = settingsLimit
	}
	if limit > settingsLimit {
		limit = settingsLimit
	}

	uploads, err := database.ListReferenceUploads(user.ID, limit)
	if err != nil {
		log.Printf("[reference] Error listing uploads: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	result := make([]models.ReferenceUploadResponse, 0, len(uploads))
	for _, item := range uploads {
		resp := models.ReferenceUploadResponse{
			ID:        item.ID,
			CreatedAt: item.CreatedAt,
		}
		if file, err := database.GetFileByID(item.FileID); err == nil && file != nil {
			resp.File = toStoredFile(file, token)
			resp.OriginalName = file.OriginalName
		}
		result = append(result, resp)
	}

	return c.JSON(result)
}

func CreateReferenceUploads(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	token := middleware.GetToken(c)

	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "请上传文件"})
	}

	files := form.File["files"]
	if len(files) == 0 {
		if single := form.File["file"]; len(single) > 0 {
			files = single
		}
	}
	if len(files) == 0 {
		return c.Status(400).JSON(fiber.Map{"error": "请上传文件"})
	}

	settings, _, _ := database.GetSettings()
	limit := 50
	if settings != nil && settings.ReferenceHistoryLimit > 0 {
		limit = settings.ReferenceHistoryLimit
	}

	var responses []models.ReferenceUploadResponse

	for _, fh := range files {
		file, err := fh.Open()
		if err != nil {
			log.Printf("[reference] Error opening file %s: %v", fh.Filename, err)
			continue
		}
		buf, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			log.Printf("[reference] Error reading file %s: %v", fh.Filename, err)
			continue
		}

		savedFile, err := saveBufferToFile(user.ID, "reference-upload", fh.Header.Get("Content-Type"), fh.Filename, buf, true)
		if err != nil {
			log.Printf("[reference] Error saving upload %s: %v", fh.Filename, err)
			continue
		}

		upload, err := database.CreateReferenceUpload(user.ID, savedFile.ID)
		if err != nil {
			log.Printf("[reference] Error creating upload record for %s: %v", fh.Filename, err)
			continue
		}

		response := models.ReferenceUploadResponse{
			ID:           upload.ID,
			CreatedAt:    upload.CreatedAt,
			File:         toStoredFile(savedFile, token),
			OriginalName: fh.Filename, // 包含原始文件名以便前端匹配
		}
		responses = append(responses, response)
	}

	if err := trimReferenceUploads(user.ID, limit); err != nil {
		log.Printf("[reference] Error trimming uploads: %v", err)
	}

	return c.JSON(responses)
}

func DeleteReferenceUpload(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	id := c.Params("id")

	upload, err := database.GetReferenceUpload(user.ID, id)
	if err != nil {
		log.Printf("[reference] Error getting upload: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}
	if upload == nil {
		return c.Status(404).JSON(fiber.Map{"error": "未找到"})
	}

	if file, err := database.GetFileByID(upload.FileID); err == nil && file != nil {
		fileutil.RemoveWithThumb(file.Path)
		_ = database.DeleteFile(file.ID)
	}

	if err := database.DeleteReferenceUpload(user.ID, id); err != nil {
		log.Printf("[reference] Error deleting upload: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	return c.JSON(fiber.Map{"ok": true})
}

func trimReferenceUploads(userID string, limit int) error {
	if limit < 1 {
		return nil
	}
	toDelete, err := database.ListReferenceUploadsToTrim(userID, limit)
	if err != nil || len(toDelete) == 0 {
		return err
	}

	for _, item := range toDelete {
		if file, err := database.GetFileByID(item.FileID); err == nil && file != nil {
			fileutil.RemoveWithThumb(file.Path)
			_ = database.DeleteFile(file.ID)
		}
		if err := database.DeleteReferenceUpload(userID, item.ID); err != nil {
			log.Printf("[reference] Error deleting old upload %s: %v", item.ID, err)
		}
	}
	return nil
}

// ========== File Handlers ==========

func GetFile(c *fiber.Ctx) error {
	user := middleware.GetCurrentUser(c)
	id := c.Params("id")

	file, err := database.GetFileByID(id)
	if err != nil {
		log.Printf("[file] Error getting file: %v", err)
		return c.Status(500).JSON(fiber.Map{"error": "服务器错误"})
	}

	// 定义哪些用途的文件是公开的 (影视项目的相关图片)
	isPublicAsset := false
	if file != nil {
		switch file.Purpose {
		case "project-cover", "episode-cover", "storyboard-image":
			isPublicAsset = true
		}
	}

	// 逻辑修改：如果是拥有者 OR 是公开资源，则允许访问
	if file == nil || (!isPublicAsset && file.UserID != user.ID) {
		return c.Status(404).JSON(fiber.Map{"error": "未找到"})
	}

	if c.Query("download") == "1" {
		filename := c.Query("filename")
		if filename != "" {
			// 解码 URL 编码的文件名
			if decoded, err := url.QueryUnescape(filename); err == nil {
				filename = decoded
			}
		}
		// 如果没有提供 filename 参数，使用原来的逻辑
		if filename == "" {
			filename = file.OriginalName
			if filename == "" {
				filename = file.ID
			}
		}
		clean := sanitizeDownloadFilename(filename)
		if clean == "" {
			clean = file.ID
		}
		filename = asciiFallbackFilename(clean)
		if filename == "" {
			filename = "file"
		}
		c.Set("Content-Type", file.MimeType)
		return c.Download(file.Path, filename)
	}

	if c.Query("download") != "1" && c.Query("thumb") == "1" && strings.HasPrefix(file.MimeType, "image/") {
		if thumbPath, err := fileutil.EnsureThumbnail(file.Path); err == nil {
			c.Set("Content-Type", fileutil.ThumbMimeType)
			return c.SendFile(thumbPath)
		} else {
			log.Printf("[file] Error generating thumbnail for %s: %v", file.ID, err)
		}
	}

	c.Set("Content-Type", file.MimeType)
	return c.SendFile(file.Path)
}

func GetPublicFile(c *fiber.Ctx) error {
	id := c.Params("id")
	token := c.Query("token")

	file, err := database.GetFileByID(id)
	if err != nil || file == nil {
		return c.Status(404).SendString("")
	}

	if token == "" || token != file.PublicToken {
		return c.Status(404).SendString("")
	}

	c.Set("Content-Type", file.MimeType)
	return c.SendFile(file.Path)
}

// ========== Helper Functions ==========

func toGenerationResponse(g *models.Generation, token string) models.GenerationResponse {
	resp := models.GenerationResponse{
		ID:               g.ID,
		Type:             g.Type,
		Prompt:           g.Prompt,
		Model:            g.Model,
		Status:           g.Status,
		Progress:         g.Progress,
		StartedAt:        g.StartedAt,
		ElapsedSeconds:   g.ElapsedSeconds,
		Error:            g.Error,
		Favorite:         g.Favorite,
		ImageSize:        g.ImageSize,
		AspectRatio:      g.AspectRatio,
		Duration:         g.Duration,
		VideoSize:        g.VideoSize,
		ReferenceFileIDs: g.ReferenceFileIDs,
		RunID:            g.RunID,
		NodePosition:     g.NodePosition,
		CreatedAt:        g.CreatedAt,
		UpdatedAt:        g.UpdatedAt,
	}

	if g.OutputFileID != nil {
		file, err := database.GetFileByID(*g.OutputFileID)
		if err == nil && file != nil {
			resp.OutputFile = toStoredFile(file, token)
		}
	}

	return resp
}

func toStoredFile(f *models.File, token string) *models.StoredFile {
	if f == nil {
		return nil
	}
	return &models.StoredFile{
		ID:        f.ID,
		MimeType:  f.MimeType,
		CreatedAt: f.CreatedAt,
		Filename:  f.OriginalName,
		URL:       buildClientFileURL(f.ID, token, false),
	}
}

func buildClientFileURL(fileID, token string, download bool) string {
	base := cfg.PublicBaseURL
	path := fmt.Sprintf("/api/files/%s", fileID)

	params := url.Values{}
	if token != "" {
		params.Set("token", token)
	}
	if download {
		params.Set("download", "1")
	}

	if len(params) > 0 {
		return fmt.Sprintf("%s%s?%s", base, path, params.Encode())
	}
	return base + path
}

func sanitizeDownloadFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.ReplaceAll(name, "\n", "")
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\t", "")
	name = strings.ReplaceAll(name, `"`, "")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, ";", "_")
	return strings.TrimSpace(name)
}

func asciiFallbackFilename(name string) string {
	if name == "" {
		return "file"
	}
	var b strings.Builder
	for _, r := range name {
		if r >= 0x20 && r <= 0x7e {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	fallback := strings.TrimSpace(b.String())
	if fallback == "" {
		return "file"
	}
	return fallback
}

func BuildPublicFileURL(fileID string) string {
	file, err := database.GetFileByID(fileID)
	if err != nil || file == nil {
		return ""
	}

	base := cfg.PublicBaseURL
	params := url.Values{}
	params.Set("token", file.PublicToken)

	return fmt.Sprintf("%s/public/files/%s?%s", base, fileID, params.Encode())
}

var extByMime = map[string]string{
	"image/png":  "png",
	"image/jpeg": "jpg",
	"image/jpg":  "jpg",
	"image/webp": "webp",
	"image/gif":  "gif",
	"video/mp4":  "mp4",
}

func guessExt(mimeType string) string {
	if ext, ok := extByMime[mimeType]; ok {
		return ext
	}
	return "bin"
}

func saveBufferToFile(userID, purpose, mimeType, originalName string, buf []byte, persistent bool) (*models.File, error) {
	// Ensure storage directory exists
	storageDir := cfg.StorageDir
	dir := filepath.Join(storageDir, fmt.Sprintf("u_%s", userID), purpose)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	// Generate filename
	id := uuid.New().String()
	ext := guessExt(mimeType)
	filename := fmt.Sprintf("%s.%s", id, ext)
	filePath := filepath.Join(dir, filename)

	// Write file
	if err := os.WriteFile(filePath, buf, 0644); err != nil {
		return nil, err
	}

	// Create database record
	file, err := database.CreateFile(userID, purpose, mimeType, originalName, filePath, persistent)
	if err != nil {
		os.Remove(filePath)
		return nil, err
	}

	return file, nil
}

func SaveBufferToFile(userID, purpose, mimeType, originalName string, buf []byte, persistent bool) (*models.File, error) {
	return saveBufferToFile(userID, purpose, mimeType, originalName, buf, persistent)
}

// saveBase64ToFile 将base64编码的图片保存为文件
func saveBase64ToFile(userID, purpose, base64Data string, persistent bool) (*models.File, error) {
	// 解析data URL格式: data:image/png;base64,iVBORw0KG...
	// 或直接是base64字符串
	var mimeType string
	var base64Str string

	if strings.HasPrefix(base64Data, "data:") {
		// 解析data URL
		parts := strings.SplitN(base64Data, ",", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("无效的base64格式")
		}

		// 提取MIME类型
		header := parts[0]
		if strings.Contains(header, ";") {
			mimeType = strings.TrimPrefix(strings.Split(header, ";")[0], "data:")
		}

		base64Str = parts[1]
	} else {
		// 直接是base64字符串，默认为PNG
		mimeType = "image/png"
		base64Str = base64Data
	}

	// 解码base64
	buf, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return nil, fmt.Errorf("base64解码失败: %w", err)
	}

	// 如果没有MIME类型，尝试检测
	if mimeType == "" {
		mimeType = detectMimeType(buf)
	}

	return saveBufferToFile(userID, purpose, mimeType, "", buf, persistent)
}

// detectMimeType 根据文件头检测MIME类型
func detectMimeType(buf []byte) string {
	if len(buf) < 12 {
		return "application/octet-stream"
	}

	// PNG: 89 50 4E 47
	if buf[0] == 0x89 && buf[1] == 0x50 && buf[2] == 0x4E && buf[3] == 0x47 {
		return "image/png"
	}

	// JPEG: FF D8 FF
	if buf[0] == 0xFF && buf[1] == 0xD8 && buf[2] == 0xFF {
		return "image/jpeg"
	}

	// GIF: 47 49 46
	if buf[0] == 0x47 && buf[1] == 0x49 && buf[2] == 0x46 {
		return "image/gif"
	}

	// WebP: RIFF ... WEBP
	if buf[0] == 0x52 && buf[1] == 0x49 && buf[2] == 0x46 && buf[3] == 0x46 &&
		buf[8] == 0x57 && buf[9] == 0x45 && buf[10] == 0x42 && buf[11] == 0x50 {
		return "image/webp"
	}

	return "application/octet-stream"
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
