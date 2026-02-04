package jobs

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"nano-backend/internal/config"
	"nano-backend/internal/crypto"
	"nano-backend/internal/database"
	"nano-backend/internal/gemini"
	"nano-backend/internal/grsai"
	"nano-backend/internal/handlers"
	"nano-backend/internal/models"
)

var (
	cfg        *config.Config
	activeJobs sync.Map // map[generationID]bool
)

// StartJobRunner starts the background job runner
func StartJobRunner(c *config.Config) {
	cfg = c

	// Run immediately
	go tick()

	// Run every 3 seconds
	ticker := time.NewTicker(3 * time.Second)
	go func() {
		for range ticker.C {
			tick()
		}
	}()

	log.Printf("[jobs] Job runner started")
}

func tick() {
	generations, err := database.GetPendingGenerations()
	if err != nil {
		log.Printf("[jobs] Error getting pending generations: %v", err)
		return
	}

	for _, g := range generations {
		// Skip if already processing
		if _, ok := activeJobs.Load(g.ID); ok {
			continue
		}

		// Mark as active and process
		activeJobs.Store(g.ID, true)
		go func(gen models.Generation) {
			defer activeJobs.Delete(gen.ID)
			if err := runGeneration(&gen); err != nil {
				log.Printf("[jobs] Error running generation %s: %v", gen.ID, err)
			}
		}(g)
	}
}

func runGeneration(g *models.Generation) error {
	log.Printf("[jobs] Starting generation %s (type=%s, model=%s)", g.ID, g.Type, g.Model)

	// Update status to running
	updates := map[string]interface{}{
		"status": "running",
	}
	if g.StartedAt == nil || *g.StartedAt == 0 {
		updates["startedAt"] = models.Now()
	}
	if err := database.UpdateGeneration(g.ID, updates); err != nil {
		return err
	}

	// Get provider credentials
	providerHost, apiKey, err := getEffectiveProvider(g.UserID)
	if err != nil {
		return updateFailedWithCode(g.ID, err.Error(), models.ErrorCodeAPIError)
	}

	timeoutSeconds := resolveJobTimeoutSeconds(g.Type)
	log.Printf("[jobs] Using timeoutSeconds=%d for generation %s (type=%s)", timeoutSeconds, g.ID, g.Type)

	// Check if using Gemini API (including modelverse.cn)
	isGeminiAPI := strings.Contains(providerHost, "yunwu.ai") || strings.Contains(providerHost, "gemini") || strings.Contains(providerHost, "google") || strings.Contains(providerHost, "modelverse.cn")

	if isGeminiAPI {
		return runGeminiGeneration(g, providerHost, apiKey, timeoutSeconds)
	}

	// Use GRS AI API
	return runGRSAIGeneration(g, providerHost, apiKey, timeoutSeconds)
}

func updateFailed(generationID, errMsg string) error {
	return updateFailedWithCode(generationID, errMsg, "")
}

func updateFailedWithCode(generationID, errMsg string, errorCode models.GenerationErrorCode) error {
	if errorCode == "" {
		errorCode = identifyErrorCode(errMsg)
	}

	log.Printf("[jobs] Generation %s failed: %s (code: %s)", generationID, errMsg, errorCode)
	updates := map[string]interface{}{
		"status":    "failed",
		"error":     errMsg,
		"errorCode": errorCode,
	}
	if elapsed := resolveElapsedSeconds(generationID); elapsed != nil {
		updates["elapsedSeconds"] = *elapsed
	}
	return database.UpdateGeneration(generationID, updates)
}

func identifyErrorCode(errMsg string) models.GenerationErrorCode {
	lowerMsg := strings.ToLower(errMsg)

	if strings.Contains(lowerMsg, "insufficient quota") ||
		strings.Contains(lowerMsg, "quota failed") ||
		strings.Contains(lowerMsg, "余额不足") ||
		strings.Contains(lowerMsg, "配额") {
		return models.ErrorCodeInsufficientQuota
	}

	if strings.Contains(lowerMsg, "invalid api key") ||
		strings.Contains(lowerMsg, "unauthorized") ||
		strings.Contains(lowerMsg, "401") ||
		strings.Contains(lowerMsg, "authentication failed") {
		return models.ErrorCodeInvalidAPIKey
	}

	if strings.Contains(lowerMsg, "timeout") ||
		strings.Contains(lowerMsg, "超时") ||
		strings.Contains(lowerMsg, "timed out") {
		return models.ErrorCodeTimeout
	}

	if strings.Contains(lowerMsg, "network") ||
		strings.Contains(lowerMsg, "connection") ||
		strings.Contains(lowerMsg, "dns") ||
		strings.Contains(lowerMsg, "dial") {
		return models.ErrorCodeNetworkError
	}

	if strings.Contains(lowerMsg, "invalid request") ||
		strings.Contains(lowerMsg, "invalid url") ||
		strings.Contains(lowerMsg, "bad request") ||
		strings.Contains(lowerMsg, "400") {
		return models.ErrorCodeInvalidRequest
	}

	if strings.Contains(lowerMsg, "api调用失败") ||
		strings.Contains(lowerMsg, "api error") ||
		strings.Contains(lowerMsg, "internal error") ||
		strings.Contains(lowerMsg, "500") ||
		strings.Contains(lowerMsg, "502") ||
		strings.Contains(lowerMsg, "503") {
		return models.ErrorCodeAPIError
	}

	if strings.Contains(lowerMsg, "不支持") ||
		strings.Contains(lowerMsg, "not supported") ||
		strings.Contains(lowerMsg, "unsupported") {
		return models.ErrorCodeUnsupportedFeature
	}

	return models.ErrorCodeUnknown
}

func resolveElapsedSeconds(generationID string) *int64 {
	gen, err := database.GetGenerationByID(generationID)
	if err != nil || gen == nil || gen.StartedAt == nil || *gen.StartedAt == 0 {
		return nil
	}
	elapsed := (models.Now() - *gen.StartedAt) / 1000
	if elapsed < 0 {
		elapsed = 0
	}
	return &elapsed
}

func resolveJobTimeoutSeconds(genType string) int {
	timeoutSeconds := 600
	if settings, _, err := database.GetSettings(); err == nil && settings != nil {
		if genType == "video" {
			timeoutSeconds = settings.VideoTimeoutSeconds
		} else {
			timeoutSeconds = settings.ImageTimeoutSeconds
		}
	}
	if timeoutSeconds < 30 {
		timeoutSeconds = 600
	}
	return timeoutSeconds
}

func getEffectiveProvider(userID string) (string, string, error) {
	provider, err := database.GetUserProvider(userID)
	if err != nil {
		return "", "", err
	}

	host := cfg.DefaultProviderHost
	apiKey := cfg.DefaultProviderAPIKey

	if provider != nil {
		host = provider.ProviderHost
		if provider.APIKeyEnc != "" {
			decrypted, err := crypto.DecryptText(provider.APIKeyEnc, cfg.APIKeyEncryptionSecret)
			if err == nil && decrypted != "" {
				apiKey = decrypted
			}
		}
	}

	if apiKey == "" {
		return "", "", fmt.Errorf("未配置接口密钥，请在接口设置中填写。")
	}

	return host, apiKey, nil
}

func fetchAndStoreRemoteFile(userID, purpose, url string, persistent bool, timeoutSeconds int) (*models.File, error) {
	log.Printf("[jobs] Fetching remote file: %s", url)

	// 增加下载文件的超时时间，支持大文件和多任务并发
	timeout := 120 * time.Second
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("下载远程文件失败：HTTP %d", resp.StatusCode)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("[jobs] Downloaded %d bytes, mimeType=%s", len(buf), mimeType)

	return handlers.SaveBufferToFile(userID, purpose, mimeType, "", buf, persistent)
}

// fileToBase64Data 读取文件并转换为base64 data URL格式
func fileToBase64Data(fileID string) (string, error) {
	file, err := database.GetFileByID(fileID)
	if err != nil || file == nil {
		return "", fmt.Errorf("文件不存在: %s", fileID)
	}

	// 读取文件内容
	buf, err := os.ReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	// 转换为base64 data URL格式
	base64Str := base64.StdEncoding.EncodeToString(buf)
	dataURL := fmt.Sprintf("data:%s;base64,%s", file.MimeType, base64Str)

	return dataURL, nil
}

// runGRSAIGeneration handles GRS AI API generation
func runGRSAIGeneration(g *models.Generation, providerHost, apiKey string, timeoutSeconds int) error {
	client := grsai.NewClient(providerHost, apiKey, time.Duration(timeoutSeconds)*time.Second)

	// Build reference URLs - 将文件转为base64传给API
	refURLs := make([]string, 0)
	for _, fid := range g.ReferenceFileIDs {
		// 读取文件并转为base64（API支持base64格式）
		base64Data, err := fileToBase64Data(fid)
		if err != nil {
			log.Printf("[jobs] Error converting file %s to base64: %v", fid, err)
			continue
		}
		if base64Data != "" {
			refURLs = append(refURLs, base64Data)
		}
	}

	// Submit task if no providerTaskId
	if g.ProviderTaskID == nil || *g.ProviderTaskID == "" {
		var taskResp *grsai.CreateTaskResponse
		var err error

		if g.Type == "image" {
			aspectRatio := "auto"
			if g.AspectRatio != nil {
				aspectRatio = *g.AspectRatio
			}
			imageSize := ""
			if g.ImageSize != nil {
				imageSize = *g.ImageSize
			}

			taskResp, err = client.CreateNanoBananaTask(g.Model, g.Prompt, aspectRatio, imageSize, refURLs)
		} else if g.Type == "video" {
			aspectRatio := "9:16"
			if g.AspectRatio != nil {
				aspectRatio = *g.AspectRatio
			}
			duration := 10
			if g.Duration != nil {
				duration = *g.Duration
			}
			videoSize := "small"
			if g.VideoSize != nil {
				videoSize = *g.VideoSize
			}
			refURL := ""
			if len(refURLs) > 0 {
				refURL = refURLs[0]
			}

			taskResp, err = client.CreateSoraVideoTask(g.Model, g.Prompt, refURL, aspectRatio, duration, videoSize)
		}

		if err != nil {
			return updateFailed(g.ID, err.Error())
		}

		// Check if task completed immediately
		if taskResp.Finished && taskResp.Result != nil {
			return handleGRSAISucceeded(g.ID, g.UserID, taskResp.Result, timeoutSeconds)
		}

		// Save provider task ID
		if err := database.UpdateGeneration(g.ID, map[string]interface{}{
			"providerTaskId": taskResp.ID,
			"progress":       0.0,
		}); err != nil {
			return err
		}

		// Refresh generation
		updatedG, err := database.GetGenerationByID(g.ID)
		if err != nil || updatedG == nil {
			return nil
		}
		g = updatedG
	}

	// Poll for results
	pollSeconds := 2
	maxAttempts := timeoutSeconds / pollSeconds
	if timeoutSeconds%pollSeconds != 0 {
		maxAttempts++
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	for attempts := 0; attempts < maxAttempts; attempts++ {
		// Refresh generation status
		latest, err := database.GetGenerationByID(g.ID)
		if err != nil || latest == nil {
			return nil
		}
		if latest.Status == "succeeded" || latest.Status == "failed" {
			return nil
		}
		if latest.ProviderTaskID == nil || *latest.ProviderTaskID == "" {
			return updateFailedWithCode(g.ID, "缺少任务 ID", models.ErrorCodeInvalidRequest)
		}

		// Query result
		result, err := client.GetTaskResult(*latest.ProviderTaskID)
		if err != nil {
			// Transient error, log every 10 attempts
			if attempts%10 == 0 {
				log.Printf("[jobs] Error querying task result (attempt %d): %v", attempts, err)
				database.UpdateGeneration(g.ID, map[string]interface{}{
					"error": err.Error(),
				})
			}
			time.Sleep(2 * time.Second)
			continue
		}

		// Update progress
		if result.Progress > 0 {
			database.UpdateGeneration(g.ID, map[string]interface{}{
				"progress": result.Progress,
			})
		}

		// Check status
		if result.Status == "succeeded" {
			return handleGRSAISucceeded(g.ID, g.UserID, result, timeoutSeconds)
		}

		if result.Status == "failed" {
			errMsg := "任务执行失败"
			if result.Error != "" {
				errMsg = result.Error
			} else if result.Message != "" {
				errMsg = result.Message
			}
			return updateFailed(g.ID, errMsg)
		}

		time.Sleep(2 * time.Second)
	}

	return updateFailedWithCode(g.ID, "等待结果超时", models.ErrorCodeTimeout)
}

// handleGRSAISucceeded handles successful GRS AI generation
func handleGRSAISucceeded(generationID, userID string, result *grsai.TaskResult, timeoutSeconds int) error {
	url := grsai.ExtractFirstResultURL(result)
	if url == "" {
		return updateFailedWithCode(generationID, "未返回结果地址", models.ErrorCodeAPIError)
	}

	log.Printf("[jobs] Downloading result from: %s", url)

	// Download and store the file
	file, err := fetchAndStoreRemoteFile(userID, "generation-output", url, false, timeoutSeconds)
	if err != nil {
		return updateFailedWithCode(generationID, "下载失败："+err.Error(), models.ErrorCodeNetworkError)
	}

	log.Printf("[jobs] Downloaded and stored file: %s", file.ID)

	updates := map[string]interface{}{
		"status":            "succeeded",
		"progress":          100.0,
		"outputFileId":      file.ID,
		"providerResultUrl": url,
	}
	if elapsed := resolveElapsedSeconds(generationID); elapsed != nil {
		updates["elapsedSeconds"] = *elapsed
	}
	return database.UpdateGeneration(generationID, updates)
}

// runGeminiGeneration handles Gemini 3 Pro API generation
func runGeminiGeneration(g *models.Generation, providerHost, apiKey string, timeoutSeconds int) error {
	// Gemini API only supports image generation
	if g.Type != "image" {
		return updateFailedWithCode(g.ID, "Gemini API 暂不支持视频生成", models.ErrorCodeUnsupportedFeature)
	}

	client := gemini.NewClient(providerHost, apiKey, time.Duration(timeoutSeconds)*time.Second)

	// Build reference images
	referenceImages := make([]gemini.ReferenceImage, 0)
	for _, fid := range g.ReferenceFileIDs {
		file, err := database.GetFileByID(fid)
		if err != nil || file == nil {
			log.Printf("[jobs] Error getting file %s: %v", fid, err)
			continue
		}

		buf, err := os.ReadFile(file.Path)
		if err != nil {
			log.Printf("[jobs] Error reading file %s: %v", fid, err)
			continue
		}

		refImage, err := gemini.FileToReferenceImage(file.MimeType, buf)
		if err != nil {
			log.Printf("[jobs] Error converting file %s to reference image: %v", fid, err)
			continue
		}
		referenceImages = append(referenceImages, refImage)
	}

	// Get aspect ratio and image size
	aspectRatio := "16:9"
	if g.AspectRatio != nil && *g.AspectRatio != "" && *g.AspectRatio != "auto" {
		aspectRatio = *g.AspectRatio
	}

	imageSize := "1K"
	if g.ImageSize != nil && *g.ImageSize != "" {
		imageSize = *g.ImageSize
	}

	log.Printf("[jobs] Calling Gemini API: prompt=%s, aspectRatio=%s, imageSize=%s, refs=%d",
		g.Prompt, aspectRatio, imageSize, len(referenceImages))

	// Call Gemini API
	resp, err := client.CreateImageTask(g.Prompt, aspectRatio, imageSize, referenceImages)
	if err != nil {
		log.Printf("[jobs] Gemini API call failed: %v", err)
		return updateFailed(g.ID, err.Error())
	}

	// Extract image URLs
	imageURLs := gemini.ExtractImageURLs(resp)
	log.Printf("[jobs] Extracted %d image URLs from Gemini response", len(imageURLs))
	if len(imageURLs) == 0 {
		log.Printf("[jobs] No image URLs found in response, marking as failed")
		return updateFailedWithCode(g.ID, "未返回生成结果", models.ErrorCodeAPIError)
	}

	// Store the first image
	firstImageURL := imageURLs[0]
	log.Printf("[jobs] Got Gemini result: %s", firstImageURL)

	// Parse data URL and store file
	parts := strings.SplitN(firstImageURL, ",", 2)
	if len(parts) != 2 {
		return updateFailedWithCode(g.ID, "无效的图片数据格式", models.ErrorCodeInvalidRequest)
	}

	mimeType := strings.TrimPrefix(parts[0], "data:")
	mimeType = strings.TrimSuffix(mimeType, ";base64")

	base64Data := parts[1]
	imageData, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return updateFailedWithCode(g.ID, "解码图片数据失败："+err.Error(), models.ErrorCodeAPIError)
	}

	file, err := handlers.SaveBufferToFile(g.UserID, "generation-output", mimeType, "", imageData, false)
	if err != nil {
		return updateFailedWithCode(g.ID, "保存图片失败："+err.Error(), models.ErrorCodeAPIError)
	}

	log.Printf("[jobs] Stored Gemini result file: %s", file.ID)

	updates := map[string]interface{}{
		"status":            "succeeded",
		"progress":          100.0,
		"outputFileId":      file.ID,
		"providerResultUrl": firstImageURL,
	}
	if elapsed := resolveElapsedSeconds(g.ID); elapsed != nil {
		updates["elapsedSeconds"] = *elapsed
	}
	return database.UpdateGeneration(g.ID, updates)
}
