// Package anthropic provides integration with the Anthropic Claude API.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	apiURL         = "https://api.anthropic.com/v1/messages"
	apiVersion     = "2023-06-01"
	maxRetries     = 3
	retryDelay     = 2 * time.Second
	defaultTimeout = 60 * time.Second
)

// Client is the Anthropic API client.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new Anthropic API client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
	}
}

// Message represents a message in the conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request represents an API request to Claude.
type Request struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	Messages  []Message `json:"messages"`
	System    string    `json:"system,omitempty"`
}

// Response represents an API response from Claude.
type Response struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model        string `json:"model"`
	StopReason   string `json:"stop_reason"`
	StopSequence string `json:"stop_sequence"`
	Usage        struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Send sends a request to the Claude API with retries and backoff.
func (c *Client) Send(ctx context.Context, apiKey, model, systemPrompt, userMessage string) (string, error) {
	req := Request{
		Model:     model,
		MaxTokens: 4096,
		Messages: []Message{
			{
				Role:    "user",
				Content: userMessage,
			},
		},
		System: systemPrompt,
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(retryDelay * time.Duration(attempt)):
			}
		}

		resp, err := c.doRequest(ctx, apiKey, req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Check if we should retry
		if !shouldRetry(err) {
			break
		}
	}

	return "", fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

// doRequest performs a single API request.
func (c *Client) doRequest(ctx context.Context, apiKey string, req Request) (string, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check for errors
	if httpResp.StatusCode != http.StatusOK {
		var errResp ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return "", &APIError{
				StatusCode: httpResp.StatusCode,
				Type:       errResp.Error.Type,
				Message:    errResp.Error.Message,
			}
		}
		return "", &APIError{
			StatusCode: httpResp.StatusCode,
			Message:    string(respBody),
		}
	}

	var resp Response
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract text from content
	if len(resp.Content) > 0 {
		return resp.Content[0].Text, nil
	}

	return "", fmt.Errorf("no content in response")
}

// APIError represents an API error.
type APIError struct {
	StatusCode int
	Type       string
	Message    string
}

func (e *APIError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("API error (%d): %s - %s", e.StatusCode, e.Type, e.Message)
	}
	return fmt.Sprintf("API error (%d): %s", e.StatusCode, e.Message)
}

// shouldRetry determines if an error is retryable.
func shouldRetry(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		// Retry on rate limit (429) and server errors (5xx)
		return apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}
	return false
}
