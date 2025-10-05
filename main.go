package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"anki-builder/pkg/aislop/gemini"
	"anki-builder/pkg/ankiclient"
	"anki-builder/pkg/config"
)

const shutdownTimeout = 5 * time.Second

var expectedAnkiNoteFields = []string{"Question", "Translation", "Example", "Notes"}

//go:embed prompts/*.txt
var prompts embed.FS

type AnkiCard struct {
	Question    string
	Translation string
	Example     string
	Notes       string
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

//nolint:funlen // I like it this way
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

	log.Print("Anki Card Builder")
	log.Printf("Using AI Provider: gemini")

	chosenLanguage, chosenLanguageCfg, err := chooseLanguage(cfg.LanguageSettings)
	if err != nil {
		return fmt.Errorf("failed to choose language: %w", err)
	}

	queryTmpl, err := findPrompt(chosenLanguageCfg.PromptFile)
	if err != nil {
		return fmt.Errorf("failed to choose prompt: %w", err)
	}

	err = validateAnkiDeck(ctx, ankiClient, chosenLanguageCfg)
	if err != nil {
		return fmt.Errorf("anki deck is not setup correctly: %w", err)
	}

	log.Printf("Using Anki deck '%s' and model '%s'", chosenLanguageCfg.DeckName, chosenLanguageCfg.ModelName)
	log.Printf("Enter %s words or phrases (to exit use Ctrl+C or type 'q', 'quit' or 'exit'):", strings.ToTitle(chosenLanguage[0:1])+chosenLanguage[1:])

	dataChan := make(chan string)
	scanner := bufio.NewScanner(os.Stdin)

loop:
	for {
		go func() {
			fmt.Print("> ") //nolint:forbidigo // need it here for proper prompt
			if !scanner.Scan() {
				dataChan <- "quit"
			}

			dataChan <- trimConsoleInput(scanner.Text())
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

		card, err := generateCard(ctx, cfg, queryTmpl, input)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return errors.New("operation cancelled, shutting down")
			}
			log.Printf("Error generating card: %v", err)
			continue
		}

		err = addCardToAnki(ctx, ankiClient, chosenLanguage, chosenLanguageCfg, card)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Print("Operation cancelled. Shutting down...")
				break loop
			}
			log.Printf("Error adding card to Anki: %v", err)
			continue
		}

		log.Printf("✅ Successfully added card for: \nword: '%s'", card.Question)
	}

	log.Print("Goodbye!")
	return nil
}

func generateCard(ctx context.Context, cfg *config.Config, queryTmpl, input string) (*AnkiCard, error) {
	aiResponse, err := generateWithGemini(ctx, cfg, queryTmpl, input)
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

	card := &AnkiCard{
		Question:    aiResponse.Phrase,
		Translation: translations,
		Example:     strings.Join(aiResponse.Examples, "<br>"),
		Notes:       strings.Join(aiResponse.Notes, "<br>"),
	}

	return card, nil
}

func generateWithGemini(ctx context.Context, cfg *config.Config, queryTmpl, input string) (*AIResponse, error) {
	client := gemini.NewClient(cfg.GeminiAPIKey)
	prompt := buildPrompt(queryTmpl, input)

	responseText, err := client.GenerateContent(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to generate content with Gemini: %w", err)
	}

	return parseAIResponse(responseText)
}

func buildPrompt(queryTmpl, word string) string {
	return fmt.Sprintf(queryTmpl, word)
}

func parseAIResponse(responseText string) (*AIResponse, error) {
	responseText = strings.TrimSpace(responseText)

	// Find JSON boundaries
	start := strings.Index(responseText, "{")
	end := strings.LastIndex(responseText, "}")

	if start == -1 || end == -1 {
		return nil, fmt.Errorf("no JSON found in response: %s", responseText)
	}

	jsonStr := responseText[start : end+1]

	var aiResponse AIResponse
	err := json.Unmarshal([]byte(jsonStr), &aiResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w; body: %s", err, responseText)
	}

	return &aiResponse, nil
}

func validateAnkiDeck(ctx context.Context, client *ankiclient.AnkiConnectClient, langCfg *config.LanguageConfig) error {
	if !client.IsAvailable(ctx) {
		return fmt.Errorf("ankiConnect is not available at %s", client.BaseURL)
	}

	modelName := langCfg.ModelName

	models, err := client.GetModelNames(ctx)
	if err != nil {
		return fmt.Errorf("failed to get model names: %w", err)
	}

	if !slices.Contains(models, modelName) {
		return fmt.Errorf("model name '%s' not found in Anki; available models: %v", modelName, models)
	}

	modelFields, err := client.GetModelFieldNames(ctx, modelName)
	if err != nil {
		return fmt.Errorf("failed to get field names for model %s: %w", modelName, err)
	}

	if len(modelFields) != len(expectedAnkiNoteFields) {
		return fmt.Errorf("model %s has %d fields, but %d are required: %v", modelName, len(modelFields), len(expectedAnkiNoteFields), expectedAnkiNoteFields)
	}

	for _, expectedField := range expectedAnkiNoteFields {
		if !slices.Contains(modelFields, expectedField) {
			return fmt.Errorf("model %s is missing required field: %s; available fields: %v", modelName, expectedField, modelFields)
		}
	}

	return nil
}

func addCardToAnki(ctx context.Context, client *ankiclient.AnkiConnectClient, langName string, langCfg *config.LanguageConfig, card *AnkiCard) error {
	if !client.IsAvailable(ctx) {
		return fmt.Errorf("ankiConnect is not available at %s", client.BaseURL)
	}

	modelName := langCfg.ModelName
	deckname := langCfg.DeckName

	fieldMap := map[string]string{
		"Question":    card.Question,
		"Translation": card.Translation,
		"Example":     card.Example,
		"Notes":       card.Notes,
	}

	err := client.AddNote(ctx, deckname, modelName, fieldMap, []string{"auto-generated", langName})
	if err != nil {
		return fmt.Errorf("failed to add note: %w", err)
	}

	return nil
}

func trimConsoleInput(input string) string {
	input = strings.TrimSpace(input)
	if !utf8.ValidString(input) {
		input = strings.ToValidUTF8(input, "")
	}

	return input
}

func chooseLanguage(languageSettings map[string]config.LanguageConfig) (string, *config.LanguageConfig, error) {
	scanner := bufio.NewScanner(os.Stdin)

	if len(languageSettings) == 1 {
		for lang, langCfg := range languageSettings {
			log.Printf("Using %s language with %s model and %s deck", lang, langCfg.ModelName, langCfg.DeckName)
			return strings.ToLower(lang), &langCfg, nil
		}
	}

	languages := make([]string, 0, len(languageSettings))
	for lang := range languageSettings {
		languages = append(languages, lang)
	}
	slices.Sort(languages)

	log.Print("Choose language: this will set a corresponding prompt + an Anki deck")
	log.Print("You can either enter the number or initial letters (prefix match, case insensitive)")
	for i, lang := range languages {
		log.Printf("%d: %s", i+1, lang)
	}

	fmt.Print("> ") //nolint:forbidigo // need it here for proper prompt
	if !scanner.Scan() {
		return "", nil, errors.New("no input received")
	}
	input := strings.TrimSpace(scanner.Text())

	// try to parse as number first
	idx, err := strconv.Atoi(input)
	if err == nil {
		if idx < 1 || idx > len(languages) {
			return "", nil, fmt.Errorf("number out of range, must be between 1 and %d", len(languages))
		}
		lang := languages[idx-1]
		langCfg := languageSettings[lang]
		return strings.ToLower(lang), &langCfg, nil
	}

	// try to match as prefix
	input = strings.ToLower(input)
	for _, lang := range languages {
		langLowercase := strings.ToLower(lang)
		if strings.HasPrefix(langLowercase, input) {
			langCfg := languageSettings[lang]
			return langLowercase, &langCfg, nil
		}
	}

	return "", nil, fmt.Errorf("couldn't match input '%s' to any of the languages: %v", input, languages)
}

func findPrompt(fileName string) (string, error) {
	filePath := path.Join("prompts", fileName)
	content, err := prompts.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read prompt file with filename %s: %w", fileName, err)
	}

	return string(content), nil
}
