//nolint:mnd // fuck you
package ankiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const ankiRequestTimeout = 5 * time.Second

type AnkiConnectRequest struct {
	Action  string                 `json:"action"`
	Version int                    `json:"version"`
	Params  map[string]interface{} `json:"params"`
}

type AnkiConnectResponse struct {
	Result interface{} `json:"result"`
	Error  interface{} `json:"error"`
}

type AnkiConnectClient struct {
	BaseURL string
}

func NewAnkiConnectClient(baseURL string) *AnkiConnectClient {
	return &AnkiConnectClient{
		BaseURL: baseURL,
	}
}

func (a *AnkiConnectClient) IsAvailable(ctx context.Context) bool {
	request := AnkiConnectRequest{
		Action:  "version",
		Version: 6,
		Params:  map[string]interface{}{},
	}

	_, err := a.doRequest(ctx, request)
	return err == nil
}

func (a *AnkiConnectClient) AddNote(ctx context.Context, deckName, modelName string, fields map[string]string, tags []string) error {
	note := map[string]interface{}{
		"deckName":  deckName,
		"modelName": modelName,
		"fields":    fields,
		"tags":      tags,
	}

	request := AnkiConnectRequest{
		Action:  "addNote",
		Version: 6,
		Params: map[string]interface{}{
			"note": note,
		},
	}

	_, err := a.doRequest(ctx, request)
	return err
}

func (a *AnkiConnectClient) GetDeckNames(ctx context.Context) ([]string, error) {
	request := AnkiConnectRequest{
		Action:  "deckNames",
		Version: 6,
		Params:  map[string]interface{}{},
	}

	response, err := a.doRequest(ctx, request)
	if err != nil {
		return nil, err
	}

	var decks []string
	if result, ok := response.Result.([]interface{}); ok {
		for _, deck := range result {
			if deckName, ok2 := deck.(string); ok2 {
				decks = append(decks, deckName)
			}
		}
	}

	return decks, nil
}

func (a *AnkiConnectClient) GetModelNames(ctx context.Context) ([]string, error) {
	request := AnkiConnectRequest{
		Action:  "modelNames",
		Version: 6,
		Params:  map[string]interface{}{},
	}

	response, err := a.doRequest(ctx, request)
	if err != nil {
		return nil, err
	}

	var models []string
	if result, ok := response.Result.([]interface{}); ok {
		for _, model := range result {
			if modelName, ok2 := model.(string); ok2 {
				models = append(models, modelName)
			}
		}
	}

	return models, nil
}

func (a *AnkiConnectClient) GetModelFieldNames(ctx context.Context, modelName string) ([]string, error) {
	request := AnkiConnectRequest{
		Action:  "modelFieldNames",
		Version: 6,
		Params: map[string]interface{}{
			"modelName": modelName,
		},
	}

	response, err := a.doRequest(ctx, request)
	if err != nil {
		return nil, err
	}

	var fields []string
	if result, ok := response.Result.([]interface{}); ok {
		for _, field := range result {
			if fieldName, ok2 := field.(string); ok2 {
				fields = append(fields, fieldName)
			}
		}
	}

	return fields, nil
}

func (a *AnkiConnectClient) doRequest(ctx context.Context, request AnkiConnectRequest) (*AnkiConnectResponse, error) {
	time.Sleep(time.Millisecond * 50) // slight delay to avoid AnkiConnect errors

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.BaseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: ankiRequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AnkiConnect request failed with status: %d", resp.StatusCode)
	}

	var response AnkiConnectResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Error != nil {
		return nil, fmt.Errorf("AnkiConnect error: %v", response.Error)
	}

	return &response, nil
}
