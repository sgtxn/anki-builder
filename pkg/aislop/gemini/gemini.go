package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

const aiRequestTimeout = 30 * time.Second

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

type Client struct {
	apiKey string
	models []string
}

func NewClient(apiKey string, models []string) *Client {
	return &Client{
		apiKey: apiKey,
		models: models,
	}
}

func (g *Client) GenerateContent(ctx context.Context, prompt string) (string, error) {
	for i, model := range g.models {
		if i > 0 {
			log.Printf("Retrying with model '%s'...", model)
		}

		result, err := g.doRequest(ctx, model, prompt)
		if err != nil {
			log.Printf("Request with model '%s' failed: %v", model, err)
			continue
		}
		return result, nil
	}

	return "", errors.New("all models failed to generate content")
}

func (g *Client) doRequest(ctx context.Context, model string, prompt string) (string, error) {
	geminiURL, err := url.JoinPath(geminiBaseURL, model+":generateContent")
	if err != nil {
		return "", fmt.Errorf("failed to construct URL: %w", err)
	}

	requestBody := RequestBody{
		Contents: []Content{
			{
				Parts: []Part{
					{
						Text: prompt,
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-Goog-Api-Key", g.apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: aiRequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API request failed with status %d and body %s", resp.StatusCode, respBody)
	}

	var response ResponseBody

	err = json.Unmarshal(respBody, &response)
	if err != nil {
		return "", fmt.Errorf("failed to decode response %s: %w", respBody, err)
	}

	if len(response.Candidates) == 0 || len(response.Candidates[0].Content.Parts) == 0 {
		return "", errors.New("no content in response")
	}

	return response.Candidates[0].Content.Parts[0].Text, nil
}
