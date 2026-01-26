package jobs

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"nano-backend/internal/config"
	"nano-backend/internal/crypto"
	"nano-backend/internal/database"
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
		return updateFailed(g.ID, err.Error())
	}

	timeoutSeconds := resolveJobTimeoutSeconds(g.Type)
	log.Printf("[jobs] Using timeoutSeconds=%d for generation %s (type=%s)", timeoutSeconds, g.ID, g.Type)
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
			return handleSucceeded(g.ID, g.UserID, taskResp.Result, timeoutSeconds)
		}

		// Save provider task ID
		if err := database.UpdateGeneration(g.ID, map[string]interface{}{
			"providerTaskId": taskResp.ID,
			"progress":       0.0,
		}); err != nil {
			return err
		}

		// Refresh generation
		g, err = database.GetGenerationByID(g.ID)
		if err != nil || g == nil {
			return nil
		}
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
			return updateFailed(g.ID, "缺少任务 ID")
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
			return handleSucceeded(g.ID, g.UserID, result, timeoutSeconds)
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

	return updateFailed(g.ID, "等待结果超时")
}

func handleSucceeded(generationID, userID string, result *grsai.TaskResult, timeoutSeconds int) error {
	url := grsai.ExtractFirstResultURL(result)
	if url == "" {
		return updateFailed(generationID, "未返回结果地址")
	}

	log.Printf("[jobs] Downloading result from: %s", url)

	// Download and store the file
	file, err := fetchAndStoreRemoteFile(userID, "generation-output", url, false, timeoutSeconds)
	if err != nil {
		return updateFailed(generationID, "下载失败："+err.Error())
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

func updateFailed(generationID, errMsg string) error {
	log.Printf("[jobs] Generation %s failed: %s", generationID, errMsg)
	updates := map[string]interface{}{
		"status": "failed",
		"error":  errMsg,
	}
	if elapsed := resolveElapsedSeconds(generationID); elapsed != nil {
		updates["elapsedSeconds"] = *elapsed
	}
	return database.UpdateGeneration(generationID, updates)
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
