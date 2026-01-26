package grsai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Client is the GRS AI API client
type Client struct {
	Host   string
	APIKey string
	Timeout time.Duration
}

// NewClient creates a new GRS AI client
func NewClient(host, apiKey string, timeout time.Duration) *Client {
	return &Client{
		Host:   strings.TrimRight(host, "/"),
		APIKey: apiKey,
		Timeout: timeout,
	}
}

// NanoBananaRequest represents a Nano Banana image generation request
type NanoBananaRequest struct {
	Model        string   `json:"model"`
	Prompt       string   `json:"prompt"`
	AspectRatio  string   `json:"aspectRatio,omitempty"`
	ImageSize    string   `json:"imageSize,omitempty"`
	URLs         []string `json:"urls,omitempty"`
	WebHook      string   `json:"webHook,omitempty"`
	ShutProgress bool     `json:"shutProgress"`
}

// SoraVideoRequest represents a Sora video generation request
type SoraVideoRequest struct {
	Model        string `json:"model"`
	Prompt       string `json:"prompt"`
	URL          string `json:"url,omitempty"`
	AspectRatio  string `json:"aspectRatio,omitempty"`
	Duration     int    `json:"duration,omitempty"`
	Size         string `json:"size,omitempty"`
	ShutProgress bool   `json:"shutProgress"`
}

// TaskResult represents the result of a generation task
type TaskResult struct {
	ID       string  `json:"id,omitempty"`
	Status   string  `json:"status,omitempty"`
	Progress float64 `json:"progress,omitempty"`
	Results  []struct {
		URL string `json:"url,omitempty"`
		PID string `json:"pid,omitempty"`
	} `json:"results,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// CreateTaskResponse represents the response from creating a task
type CreateTaskResponse struct {
	ID       string
	Finished bool
	Result   *TaskResult
}

// postJSON makes a POST request with JSON body
func (c *Client) postJSON(endpoint string, body interface{}) (map[string]interface{}, error) {
	url := c.Host + endpoint

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("[grsai] POST %s", url)
	log.Printf("[grsai] Request Body: %s", string(jsonBody))

	startTime := time.Now()

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	// 增加超时时间以支持多个并发任务，特别是视频生成任务可能需要更长时间
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	log.Printf("[grsai] HTTP timeout set to %s for %s", timeout, url)
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[grsai] Request failed after %v: %v", time.Since(startTime), err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[grsai] Response Status: %d (took %v)", resp.StatusCode, time.Since(startTime))
	log.Printf("[grsai] Response Body: %s", string(respBody))

	var result map[string]interface{}
	if len(respBody) > 0 {
		// Check if response is SSE format (starts with "data:")
		respStr := strings.TrimSpace(string(respBody))
		if strings.HasPrefix(respStr, "data:") {
			// Parse SSE stream - find the last valid JSON message
			result = parseSSEResponse(respStr)
			if result != nil {
				log.Printf("[grsai] Parsed SSE response successfully")
			}
		} else {
			if err := json.Unmarshal(respBody, &result); err != nil {
				log.Printf("[grsai] Failed to parse response as JSON: %v", err)
			}
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := "API调用失败"
		if result != nil {
			if m, ok := result["message"].(string); ok && m != "" {
				msg = m
			} else if m, ok := result["error"].(string); ok && m != "" {
				msg = m
			} else if m, ok := result["msg"].(string); ok && m != "" {
				msg = m
			}
		}
		return nil, fmt.Errorf("%s (HTTP %d)", msg, resp.StatusCode)
	}

	return result, nil
}

// CreateNanoBananaTask creates a Nano Banana image generation task
func (c *Client) CreateNanoBananaTask(model, prompt, aspectRatio, imageSize string, urls []string) (*CreateTaskResponse, error) {
	req := NanoBananaRequest{
		Model:        model,
		Prompt:       prompt,
		AspectRatio:  aspectRatio,
		URLs:         urls,
		WebHook:      "-1",     // 使用轮询模式，立即返回id
		ShutProgress: false,
	}

	// 包含imageSize参数（如果提供）
	if imageSize != "" {
		req.ImageSize = imageSize
	}

	log.Printf("[grsai] Creating Nano Banana task: model=%s, aspectRatio=%s, imageSize=%s, urls=%d items",
		model, aspectRatio, imageSize, len(urls))

	result, err := c.postJSON("/v1/draw/nano-banana", req)
	if err != nil {
		return nil, err
	}

	// Check for error in response
	if code, ok := result["code"].(float64); ok && code != 0 {
		msg := "API调用失败"
		if m, ok := result["msg"].(string); ok && m != "" {
			msg = m
		} else if m, ok := result["message"].(string); ok && m != "" {
			msg = m
		}
		return nil, fmt.Errorf("%s", msg)
	}

	// Try to get task ID
	var taskID string
	if data, ok := result["data"].(map[string]interface{}); ok {
		if id, ok := data["id"].(string); ok {
			taskID = id
		}
	}
	if taskID == "" {
		if id, ok := result["id"].(string); ok {
			taskID = id
		}
	}

	// Check if result is immediately available
	if status, ok := result["status"].(string); ok && status != "" {
		if results, ok := result["results"].([]interface{}); ok && len(results) > 0 {
			log.Printf("[grsai] Task completed immediately with status: %s", status)
			taskResult := parseTaskResult(result)
			return &CreateTaskResponse{
				ID:       taskID,
				Finished: true,
				Result:   taskResult,
			}, nil
		}
	}

	if taskID == "" {
		log.Printf("[grsai] Unexpected response format: %v", result)
		return nil, fmt.Errorf("服务返回数据格式异常")
	}

	log.Printf("[grsai] Created Nano Banana task: %s", taskID)
	return &CreateTaskResponse{ID: taskID, Finished: false}, nil
}

// CreateSoraVideoTask creates a Sora video generation task
func (c *Client) CreateSoraVideoTask(model, prompt, refURL, aspectRatio string, duration int, size string) (*CreateTaskResponse, error) {
	req := SoraVideoRequest{
		Model:        model,
		Prompt:       prompt,
		AspectRatio:  aspectRatio,
		Duration:     duration,
		Size:         size,
		ShutProgress: false,
	}

	if refURL != "" {
		req.URL = refURL
	}

	log.Printf("[grsai] Creating Sora video task: model=%s, aspectRatio=%s, duration=%d, size=%s, refURL=%s",
		model, aspectRatio, duration, size, refURL)

	result, err := c.postJSON("/v1/video/sora-video", req)
	if err != nil {
		return nil, err
	}

	// Check for error in response
	if code, ok := result["code"].(float64); ok && code != 0 {
		msg := "API调用失败"
		if m, ok := result["msg"].(string); ok && m != "" {
			msg = m
		} else if m, ok := result["message"].(string); ok && m != "" {
			msg = m
		}
		return nil, fmt.Errorf("%s", msg)
	}

	// Try to get task ID
	var taskID string
	if data, ok := result["data"].(map[string]interface{}); ok {
		if id, ok := data["id"].(string); ok {
			taskID = id
		}
	}
	if taskID == "" {
		if id, ok := result["id"].(string); ok {
			taskID = id
		}
	}

	// Check if result is immediately available
	if status, ok := result["status"].(string); ok && status != "" {
		if results, ok := result["results"].([]interface{}); ok && len(results) > 0 {
			log.Printf("[grsai] Task completed immediately with status: %s", status)
			taskResult := parseTaskResult(result)
			return &CreateTaskResponse{
				ID:       taskID,
				Finished: true,
				Result:   taskResult,
			}, nil
		}
	}

	if taskID == "" {
		log.Printf("[grsai] Unexpected response format: %v", result)
		return nil, fmt.Errorf("服务返回数据格式异常")
	}

	log.Printf("[grsai] Created Sora video task: %s", taskID)
	return &CreateTaskResponse{ID: taskID, Finished: false}, nil
}

// GetTaskResult queries the result of a task
func (c *Client) GetTaskResult(taskID string) (*TaskResult, error) {
	log.Printf("[grsai] Querying task result: %s", taskID)

	result, err := c.postJSON("/v1/draw/result", map[string]string{"id": taskID})
	if err != nil {
		return nil, err
	}

	// Try to get data from nested structure
	data := result
	if d, ok := result["data"].(map[string]interface{}); ok {
		data = d
	}

	return parseTaskResult(data), nil
}

// parseTaskResult parses a map into a TaskResult
func parseTaskResult(data map[string]interface{}) *TaskResult {
	result := &TaskResult{}

	if id, ok := data["id"].(string); ok {
		result.ID = id
	}
	if status, ok := data["status"].(string); ok {
		result.Status = status
	}
	if progress, ok := data["progress"].(float64); ok {
		result.Progress = progress
	}
	if errStr, ok := data["error"].(string); ok {
		result.Error = errStr
	}
	if msg, ok := data["message"].(string); ok {
		result.Message = msg
	}

	if results, ok := data["results"].([]interface{}); ok {
		for _, r := range results {
			if rm, ok := r.(map[string]interface{}); ok {
				item := struct {
					URL string `json:"url,omitempty"`
					PID string `json:"pid,omitempty"`
				}{}
				if url, ok := rm["url"].(string); ok {
					item.URL = url
				}
				if pid, ok := rm["pid"].(string); ok {
					item.PID = pid
				}
				result.Results = append(result.Results, item)
			}
		}
	}

	return result
}

// ExtractFirstResultURL extracts the first result URL from a task result
func ExtractFirstResultURL(result *TaskResult) string {
	if result == nil {
		return ""
	}
	if len(result.Results) > 0 && result.Results[0].URL != "" {
		return result.Results[0].URL
	}
	return ""
}

// parseSSEResponse parses SSE (Server-Sent Events) format response
// It looks for lines starting with "data:" and returns the last valid JSON message
// with status "succeeded" or "failed", or the last valid JSON if no completed status found
func parseSSEResponse(respStr string) map[string]interface{} {
	lines := strings.Split(respStr, "\n")
	var lastResult map[string]interface{}
	var completedResult map[string]interface{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		// Extract JSON part after "data:"
		jsonStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if jsonStr == "" {
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			log.Printf("[grsai] Failed to parse SSE data line: %v", err)
			continue
		}

		lastResult = data

		// Check if this is a completed message (succeeded or failed)
		if status, ok := data["status"].(string); ok {
			if status == "succeeded" || status == "failed" {
				completedResult = data
			}
		}
	}

	// Prefer completed result, otherwise return last valid result
	if completedResult != nil {
		return completedResult
	}
	return lastResult
}
