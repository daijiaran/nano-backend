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

// Part represents a part of the content
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

// ImageGenerationResponse represents the response from Gemini 3 Pro
type ImageGenerationResponse struct {
	Candidates []Candidate `json:"candidates,omitempty"`
}

// Candidate represents a generation candidate
type Candidate struct {
	Content Content `json:"content,omitempty"`
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

	// Build URL with API key
	url := fmt.Sprintf("%s/v1beta/models/gemini-3-pro-image-preview:generateContent?key=%s", c.Host, c.APIKey)

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
	log.Printf("[gemini] Response Body: %s", string(respBody))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API调用失败 (HTTP %d): %s", resp.StatusCode, string(respBody))
	}

	var result ImageGenerationResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("[gemini] Failed to parse response as JSON: %v", err)
		log.Printf("[gemini] Response body length: %d bytes", len(respBody))
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	log.Printf("[gemini] Parsed response successfully, candidates count: %d", len(result.Candidates))
	for i, candidate := range result.Candidates {
		log.Printf("[gemini] Candidate %d: parts count = %d", i, len(candidate.Content.Parts))
		for j, part := range candidate.Content.Parts {
			if part.InlineData != nil {
				log.Printf("[gemini]   Part %d: inlineData with mime_type=%s, data_length=%d", j, part.InlineData.MimeType, len(part.InlineData.Data))
			} else if part.Text != "" {
				log.Printf("[gemini]   Part %d: text with length=%d", j, len(part.Text))
			}
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
			if part.InlineData != nil {
				log.Printf("[gemini] ExtractImageURLs: candidate %d part %d has inline_data, mime_type=%s, data_length=%d",
					i, j, part.InlineData.MimeType, len(part.InlineData.Data))
				if part.InlineData.Data != "" {
					// Convert base64 to data URL
					dataURL := fmt.Sprintf("data:%s;base64,%s", part.InlineData.MimeType, part.InlineData.Data)
					urls = append(urls, dataURL)
					log.Printf("[gemini] ExtractImageURLs: added data URL, total urls = %d", len(urls))
				}
			} else if part.Text != "" {
				log.Printf("[gemini] ExtractImageURLs: candidate %d part %d has text, length=%d", i, j, len(part.Text))
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
