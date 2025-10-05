# Multi-Language Anki Card Builder

A Go application that generates vocabulary cards for multiple languages using AI (Gemini) and adds them to Anki via AnkiConnect.

## Features

- **Multi-language support** - Configure multiple languages with separate decks, models, and prompts
- Interactive language selection (number or prefix matching)
- Interactive console input for words/phrases
- AI-powered card generation using Google Gemini API
- Generates comprehensive vocabulary cards with:
  - Multiple translations
  - 3-4 example sentences (level varies by language)
  - Language-specific notes (etymology, synonyms, kanji information, usage tips)
- Automatically adds cards to Anki via AnkiConnect
- Strict field validation to ensure Anki deck is properly configured

## Prerequisites

1. **Anki** with **AnkiConnect** addon installed
2. API key for **Google Gemini**
3. **Properly configured Anki note type** with the following fields (in any order):
   - `Question`
   - `Translation`
   - `Example`
   - `Notes` 

## Setup

1. Clone or download this project
2. Copy `config.json.example` to `config.json` or `$XDG_CONFIG_HOME/anki-builder/config.json`:
   ```bash
   cp config.json.example config.json
   ```
3. Edit `config.json` with your settings:

### Configuration Options

- `languages` - A map of language configurations:
  - **Key**: Language name (displayed in language selection)
  - **Value**: Language configuration object:
    - `deckName`: Name of the Anki deck (supports nested decks like "Japanese::Words")
    - `modelName`: Name of the Anki note type/model
    - `promptFile`: Filename of the prompt template in the `prompts/` directory
- `geminiApiKey`: Your Google Gemini API key
- `ankiConnectUrl`: URL where AnkiConnect is listening (default: `http://localhost:8765`)

## Installation

1. Build the application:
   ```bash
   go build -o anki-builder
   ```

## Usage

1. Make sure Anki is running with AnkiConnect enabled
2. Run the application:
   ```bash
   ./anki-builder
   ```
   Or directly with Go:
   ```bash
   go run .
   ```

3. **Select a language** (if you have multiple configured):
   - Enter the number (e.g., `1` for the first language)
   - Or type the language name or prefix (e.g., `jap` for Japanese)
   - If only one language is configured, it will be selected automatically

4. **Enter words or phrases** when prompted
5. Type `quit`, `q`, or `exit` to stop, or press `Ctrl+C`

## AnkiConnect Setup

1. Install the AnkiConnect addon in Anki
2. Ensure AnkiConnect is configured to allow connections from localhost
3. The application will automatically detect your deck's field structure

## Card Structure

The application requires an Anki note type with **exactly 4 fields**:

- **Question**: The input word/phrase in the target language
- **Translation**: Multiple translations (formatted as a bulleted list)
- **Example**: 3-4 example sentences in the target language
- **Notes**: Language-specific notes (etymology, synonyms, kanji info, grammar notes, etc.)

## Adding New Languages

To add support for a new language:

1. Create a new prompt file in the `prompts/` directory (e.g., `spanish.txt`)
2. Add the language configuration to `config.json`:
   ```json
   "Spanish": {
       "deckName": "Spanish",
       "modelName": "Spanish",
       "promptFile": "spanish.txt"
   }
   ```
3. Create a corresponding Anki note type with the required fields
4. Rebuild the application - prompts directory is embedded into the app on compilation, so this is necessary to pick up the new prompt

See `prompts/finnish.txt` and `prompts/japanese.txt` for prompt template examples.

## API Keys

### Google Gemini
1. Go to [Google AI Studio](https://makersuite.google.com/app/apikey)
2. Create a new API key
3. Add it to your `config.json` file

## License

See LICENSE file for details.
