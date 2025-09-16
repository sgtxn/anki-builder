package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"anki-builder/aislop/gemini"
	"anki-builder/ankiclient"
	"anki-builder/config"
)

const shutdownTimeout = 5 * time.Second

//go:embed prompt.txt
var promptTemplate string

type FinnishCard struct {
	Finnish        string
	Translation    string
	FinnishExample string
	Notes          string
}

type AIResponse struct {
	Phrase       string   `json:"phrase"`
	Translations []string `json:"translations"`
	Examples     []string `json:"examples"`
	Notes        []string `json:"notes"`
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.SetFlags(0)

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	ankiClient := ankiclient.NewAnkiConnectClient(cfg.AnkiConnectURL)
	if !ankiClient.IsAvailable(ctx) {
		return fmt.Errorf("AnkiConnect is not available at %s", ankiClient.BaseURL)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Printf("\nReceived %v signal. Shutting down...", sig)
		cancel()

		time.Sleep(shutdownTimeout)
		log.Printf("Slept for %s, force exit triggered", shutdownTimeout)
		os.Exit(1)
	}()

	log.Print("Finnish Anki Card Builder")
	log.Printf("Using AI Provider: gemini")
	log.Print("Enter Finnish words or phrases (to exit use Ctrl+C or type 'q', 'quit' or 'exit'):")

	scanner := bufio.NewScanner(os.Stdin)
	dataChan := make(chan string)

loop:
	for {
		go func() {
			fmt.Print("> ") //nolint:forbidigo // need it here for proper prompt
			if !scanner.Scan() {
				dataChan <- "quit"
			}

			input := strings.TrimSpace(scanner.Text())
			dataChan <- input
		}()

		var input string
		// Check if context was cancelled
		select {
		case <-ctx.Done():
			break loop
		case input = <-dataChan:
		}

		if input == "quit" || input == "q" || input == "exit" {
			break loop
		}

		if input == "" {
			continue
		}

		log.Printf("Processing: %s", input)

		card, err := generateCard(ctx, cfg, input)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return errors.New("operation cancelled, shutting down")
			}
			log.Printf("Error generating card: %v", err)
			continue
		}

		err = addCardToAnki(ctx, ankiClient, cfg.AnkiDeckName, card)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Print("Operation cancelled. Shutting down...")
				break loop
			}
			log.Printf("Error adding card to Anki: %v", err)
			continue
		}

		log.Printf("✅ Successfully added card for: \nword: '%s'", card.Finnish)
	}

	log.Print("Goodbye!")
	return nil
}

func generateCard(ctx context.Context, cfg *config.Config, input string) (*FinnishCard, error) {
	aiResponse, err := generateWithGemini(ctx, cfg, input)
	if err != nil {
		return nil, fmt.Errorf("failed to query AI provider: %w", err)
	}

	var translations string
	for i, tl := range aiResponse.Translations {
		tl = "- " + strings.ToLower(tl)
		if i > 0 {
			translations += "<br>"
		}
		translations += tl
	}

	card := &FinnishCard{
		Finnish:        aiResponse.Phrase,
		Translation:    translations,
		FinnishExample: strings.Join(aiResponse.Examples, "<br>"),
		Notes:          strings.Join(aiResponse.Notes, "<br>"),
	}

	return card, nil
}

func generateWithGemini(ctx context.Context, cfg *config.Config, input string) (*AIResponse, error) {
	client := gemini.NewClient(cfg.GeminiAPIKey)

	prompt := buildPrompt(input)

	responseText, err := client.GenerateContent(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content with Gemini: %w", err)
	}

	return parseAIResponse(responseText)
}

func buildPrompt(input string) string {
	return fmt.Sprintf(promptTemplate, input)
}

func parseAIResponse(responseText string) (*AIResponse, error) {
	// Try to extract JSON from the response
	responseText = strings.TrimSpace(responseText)

	// Find JSON boundaries
	start := strings.Index(responseText, "{")
	end := strings.LastIndex(responseText, "}")

	if start == -1 || end == -1 {
		return nil, errors.New("no JSON found in response")
	}

	jsonStr := responseText[start : end+1]

	var aiResponse AIResponse
	err := json.Unmarshal([]byte(jsonStr), &aiResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w; body: %s", err, jsonStr)
	}

	return &aiResponse, nil
}

func addCardToAnki(ctx context.Context, client *ankiclient.AnkiConnectClient, deckname string, card *FinnishCard) error {
	if !client.IsAvailable(ctx) {
		return fmt.Errorf("AnkiConnect is not available at %s", client.BaseURL)
	}

	models, err := client.GetModelNames(ctx)
	if err != nil {
		return fmt.Errorf("failed to get model names: %w", err)
	}

	modelName := "Basic"
	for _, model := range models {
		if strings.Contains(strings.ToLower(model), "finnish") {
			modelName = model
			break
		}
	}

	fields, err := client.GetModelFieldNames(ctx, modelName)
	if err != nil {
		log.Printf("Warning: Could not get field names for model %s: %v", modelName, err)
		// Fall back to Basic model fields
		fields = []string{"Front", "Back"}
	}

	fieldMap := make(map[string]string)

	hasCustomFields := false
	customFieldNames := []string{"Finnish", "Translation", "Finnish Example", "Notes"}
	for _, customField := range customFieldNames {
		for _, field := range fields {
			if field == customField {
				hasCustomFields = true
				break
			}
		}
		if hasCustomFields {
			break
		}
	}

	if hasCustomFields {
		fieldMap["Finnish"] = card.Finnish
		fieldMap["Translation"] = card.Translation
		fieldMap["Finnish Example"] = card.FinnishExample
		fieldMap["Notes"] = card.Notes
	} else {
		fieldMap["Front"] = card.Finnish
		fieldMap["Back"] = fmt.Sprintf("**Translation:** %s\n\n**Examples:**\n%s\n\n**Notes:**\n%s",
			card.Translation, card.FinnishExample, card.Notes)
	}

	err = client.AddNote(ctx, deckname, modelName, fieldMap, []string{"auto-generated", "finnish"})
	if err != nil {
		return fmt.Errorf("failed to add note: %w", err)
	}

	return nil
}
