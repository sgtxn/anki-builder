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

const (
	aiRequestTimeout = 30 * time.Second
	geminiBaseURL    = "https://generativelanguage.googleapis.com/v1beta/models"
)

var errRateLimitReached = errors.New("rate limit reached")

type Client struct {
	apiKey string
	models []model
}

type model struct {
	Name   string
	CanUse bool
}

func New(apiKey string, modelNames []string) *Client {
	models := make([]model, 0, len(modelNames))
	for _, name := range modelNames {
		models = append(models, model{Name: name, CanUse: true})
	}
	return &Client{
		apiKey: apiKey,
		models: models,
	}
}

func (g *Client) GenerateContent(ctx context.Context, prompt string) (string, error) {
	var previousModelFailed bool

	for i, model := range g.models {
		if !model.CanUse {
			continue
		}

		if previousModelFailed {
			log.Printf("Retrying with model '%s'...", model.Name)
		}

		result, err := g.doRequest(ctx, model.Name, prompt)
		if err != nil {
			if errors.Is(err, errRateLimitReached) {
				log.Printf("Model '%s' has reached rate limit for today, marking as unusable", model.Name)
				g.models[i].CanUse = false
				continue
			}

			previousModelFailed = true
			log.Printf("Request with model '%s' failed: %v", model.Name, err)
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
		if resp.StatusCode == http.StatusTooManyRequests {
			return "", errRateLimitReached
		}
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
