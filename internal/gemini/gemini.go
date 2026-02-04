package gemini

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// Client is the Gemini 3 Pro API client
type Client struct {
	Host    string
	APIKey  string
	Timeout time.Duration
}

// NewClient creates a new Gemini 3 Pro client
func NewClient(host, apiKey string, timeout time.Duration) *Client {
	return &Client{
		Host:    strings.TrimRight(host, "/"),
		APIKey:  apiKey,
		Timeout: timeout,
	}
}

// ImageGenerationRequest represents a Gemini 3 Pro image generation request
type ImageGenerationRequest struct {
	Contents         []Content        `json:"contents"`
	GenerationConfig GenerationConfig `json:"generationConfig"`
}

// Content represents the content part of the request
type Content struct {
	Role  string `json:"role,omitempty"`
	Parts []Part `json:"parts"`
}

// Part represents a part of the content (used for Request)
type Part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inline_data,omitempty"`
}

// InlineData represents inline image data
type InlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

// GenerationConfig represents the generation configuration
type GenerationConfig struct {
	ResponseModalities []string    `json:"responseModalities"`
	ImageConfig        ImageConfig `json:"imageConfig"`
}

// ImageConfig represents the image configuration
type ImageConfig struct {
	AspectRatio string `json:"aspectRatio"`
	ImageSize   string `json:"imageSize,omitempty"`
}

// --- Response Structures (Using Map for robustness) ---

// ImageGenerationResponse represents the response from Gemini 3 Pro
type ImageGenerationResponse struct {
	Candidates []ResponseCandidate `json:"candidates,omitempty"`
}

// ResponseCandidate represents a generation candidate in the response
type ResponseCandidate struct {
	Content ResponseContent `json:"content,omitempty"`
}

// ResponseContent represents the content in the response
type ResponseContent struct {
	// Use map[string]interface{} to capture any field returned by API
	Parts []map[string]interface{} `json:"parts"`
}

// CreateImageTask creates a Gemini 3 Pro image generation task
func (c *Client) CreateImageTask(prompt, aspectRatio, imageSize string, referenceImages []ReferenceImage) (*ImageGenerationResponse, error) {
	// Build parts array
	parts := []Part{
		{Text: prompt},
	}

	// Add reference images as inline data
	for _, ref := range referenceImages {
		if ref.Data != "" {
			parts = append(parts, Part{
				InlineData: &InlineData{
					MimeType: ref.MimeType,
					Data:     ref.Data,
				},
			})
		}
	}

	// Build request
	req := ImageGenerationRequest{
		Contents: []Content{
			{
				Role:  "user",
				Parts: parts,
			},
		},
		GenerationConfig: GenerationConfig{
			ResponseModalities: []string{"TEXT", "IMAGE"},
			ImageConfig: ImageConfig{
				AspectRatio: aspectRatio,
				ImageSize:   imageSize,
			},
		},
	}

	// Build URL according to API documentation
	url := fmt.Sprintf("%s/v1beta/models/gemini-3-pro-image-preview:generateContent", c.Host)

	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	log.Printf("[gemini] POST %s", url)
	log.Printf("[gemini] Request Body: %s", string(jsonBody))

	startTime := time.Now()

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.APIKey)

	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	log.Printf("[gemini] HTTP timeout set to %s", timeout)
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Printf("[gemini] Request failed after %v: %v", time.Since(startTime), err)
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[gemini] Response Status: %d (took %v)", resp.StatusCode, time.Since(startTime))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API调用失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result ImageGenerationResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("[gemini] Failed to parse response as JSON: %v", err)
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	log.Printf("[gemini] Parsed response successfully, candidates count: %d", len(result.Candidates))

	// Debug log for parts structure
	for i, cand := range result.Candidates {
		log.Printf("[gemini] Candidate %d parts count: %d", i, len(cand.Content.Parts))
		for j, part := range cand.Content.Parts {
			keys := make([]string, 0, len(part))
			for k := range part {
				keys = append(keys, k)
			}
			log.Printf("[gemini]   Part %d keys: %v", j, keys)
		}
	}

	return &result, nil
}

// ReferenceImage represents a reference image
type ReferenceImage struct {
	MimeType string
	Data     string // Base64 encoded data (without data URL prefix)
}

// ParseReferenceDataURL parses a data URL string into ReferenceImage
func ParseReferenceDataURL(dataURL string) (ReferenceImage, error) {
	if !strings.HasPrefix(dataURL, "data:") {
		return ReferenceImage{}, fmt.Errorf("invalid data URL format")
	}

	parts := strings.SplitN(dataURL, ",", 2)
	if len(parts) != 2 {
		return ReferenceImage{}, fmt.Errorf("invalid data URL format")
	}

	mimeType := strings.TrimPrefix(parts[0], "data:")
	mimeType = strings.TrimSuffix(mimeType, ";base64")

	data := parts[1]

	return ReferenceImage{
		MimeType: mimeType,
		Data:     data,
	}, nil
}

// helper function to safely get string from map
func getString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, ok := m[key]; ok && val != nil {
			if strVal, ok := val.(string); ok && strVal != "" {
				return strVal
			}
		}
	}
	return ""
}

// helper function to get map from map
func getMap(m map[string]interface{}, keys ...string) map[string]interface{} {
	for _, key := range keys {
		if val, ok := m[key]; ok && val != nil {
			if mapVal, ok := val.(map[string]interface{}); ok {
				return mapVal
			}
		}
	}
	return nil
}

// ExtractImageURLs extracts image URLs from the response
func ExtractImageURLs(response *ImageGenerationResponse) []string {
	var urls []string

	if response == nil {
		log.Printf("[gemini] ExtractImageURLs: response is nil")
		return urls
	}

	log.Printf("[gemini] ExtractImageURLs: candidates count = %d", len(response.Candidates))

	for i, candidate := range response.Candidates {
		log.Printf("[gemini] ExtractImageURLs: candidate %d, parts count = %d", i, len(candidate.Content.Parts))
		for j, part := range candidate.Content.Parts {
			// Try to find inline_data or inlineData
			inlineData := getMap(part, "inline_data", "inlineData")
			if inlineData != nil {
				// Try mime_type or mimeType
				mimeType := getString(inlineData, "mime_type", "mimeType")
				// Try data
				data := getString(inlineData, "data")

				if mimeType != "" && data != "" {
					dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, data)
					urls = append(urls, dataURL)
					log.Printf("[gemini] ExtractImageURLs: added data URL from inline_data (keys found)")
					continue
				} else {
					log.Printf("[gemini] ExtractImageURLs: found inlineData but mimeType or data is empty. mimeType len: %d, data len: %d", len(mimeType), len(data))
				}
			}

			// Try to find text (which might contain base64 image)
			text := getString(part, "text")
			if text != "" {
				log.Printf("[gemini] ExtractImageURLs: candidate %d part %d has text, length=%d", i, j, len(text))

				var mimeType string
				// Check for common image base64 headers
				if strings.HasPrefix(text, "/9j/") {
					mimeType = "image/jpeg"
				} else if strings.HasPrefix(text, "iVBORw0KGgo") {
					mimeType = "image/png"
				} else if strings.HasPrefix(text, "R0lGOD") {
					mimeType = "image/gif"
				} else if strings.HasPrefix(text, "UklGR") {
					mimeType = "image/webp"
				}

				if mimeType != "" {
					log.Printf("[gemini] ExtractImageURLs: found base64 image in text part, mime_type=%s", mimeType)
					dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, text)
					urls = append(urls, dataURL)
				}
			}
		}
	}

	log.Printf("[gemini] ExtractImageURLs: returning %d urls", len(urls))
	return urls
}

// FileToReferenceImage converts a file to ReferenceImage
func FileToReferenceImage(mimeType string, fileData []byte) (ReferenceImage, error) {
	base64Data := base64.StdEncoding.EncodeToString(fileData)
	return ReferenceImage{
		MimeType: mimeType,
		Data:     base64Data,
	}, nil
}
