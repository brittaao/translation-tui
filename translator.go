package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/genai"
)

const (
	// Model names for Gemini API
	translationModel = "gemini-2.5-flash-lite-preview-09-2025"
	analysisModel    = "gemini-2.5-flash-preview-09-2025"

	// Temperature settings
	translationTemperature = 0.3 // Higher for more natural translation
	analysisTemperature    = 0.0 // Lower for consistent analysis

	// Environment variable
	envAPIKey = "GEMINI_API_KEY"
)

// translationResult represents the result of a translation operation.
type translationResult struct {
	originalSentence string
	translation      string
	wordAnalysis     []wordInfo
	err              error
}

// translationStepResult represents the structured response from the translation API.
type translationStepResult struct {
	InputLanguage       string `json:"input_language"`
	CleanedSentence     string `json:"cleaned_sentence"`
	Translation         string `json:"translation"`
	TranslationLanguage string `json:"translation_language"`
}

// wordAnalysisItem represents a single word analysis from the API.
type wordAnalysisItem struct {
	Word     string `json:"word"`
	Analysis string `json:"analysis"`
}

// wordAnalysisStepResult represents the structured response from the word analysis API.
type wordAnalysisStepResult struct {
	WordAnalysis []wordAnalysisItem `json:"word_analysis"`
}

// translateSentence creates a tea.Cmd that performs translation and word analysis.
func translateSentence(userLang, targetLang, sentence string) tea.Cmd {
	return func() tea.Msg {
		apiKey := os.Getenv(envAPIKey)
		if apiKey == "" {
			return translationResult{
				err: fmt.Errorf("%s environment variable not set", envAPIKey),
			}
		}

		ctx := context.Background()
		client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
		if err != nil {
			return translationResult{
				err: fmt.Errorf("failed to create client: %w", err),
			}
		}

		userLangName := getLanguageName(userLang)
		targetLangName := getLanguageName(targetLang)

		// Step 1: Translation and cleaning
		translationStep, err := performTranslation(ctx, client, sentence, userLangName, targetLangName)
		if err != nil {
			return translationResult{err: err}
		}

		// Determine which sentence is in the foreign language (target language)
		foreignSentence := getForeignSentence(translationStep, targetLangName)

		// Step 2: Word-by-word analysis
		analysisStep, err := performWordAnalysis(ctx, client, foreignSentence, userLangName, targetLangName)
		if err != nil {
			return translationResult{err: err}
		}

		// Process and clean word analysis results
		wordAnalysis := processWordAnalysis(analysisStep)

		return translationResult{
			originalSentence: translationStep.CleanedSentence, // Always the cleaned input (can be in either language)
			translation:      translationStep.Translation,     // Always the translation to opposite language
			wordAnalysis:     wordAnalysis,
			err:              nil,
		}
	}
}

// performTranslation handles the translation step of the process.
func performTranslation(ctx context.Context, client *genai.Client, sentence, userLangName, targetLangName string) (*translationStepResult, error) {
	prompt := buildTranslationPrompt(sentence, userLangName, targetLangName)
	config := buildTranslationConfig(userLangName, targetLangName)

	resp, err := client.Models.GenerateContent(ctx, translationModel, genai.Text(prompt), config)
	if err != nil {
		return nil, fmt.Errorf("translation API error: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from translation API")
	}

	responseText := extractTextFromResponse(resp)
	var result translationStepResult
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse translation JSON: %w", err)
	}

	return &result, nil
}

// performWordAnalysis handles the word analysis step of the process.
func performWordAnalysis(ctx context.Context, client *genai.Client, foreignSentence, userLangName, targetLangName string) (*wordAnalysisStepResult, error) {
	prompt := buildAnalysisPrompt(foreignSentence, userLangName, targetLangName)
	config := buildAnalysisConfig(userLangName, targetLangName)

	resp, err := client.Models.GenerateContent(ctx, analysisModel, genai.Text(prompt), config)
	if err != nil {
		return nil, fmt.Errorf("word analysis API error: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from word analysis API")
	}

	responseText := extractTextFromResponse(resp)
	var result wordAnalysisStepResult
	if err := json.Unmarshal([]byte(responseText), &result); err != nil {
		return nil, fmt.Errorf("failed to parse word analysis JSON: %w", err)
	}

	return &result, nil
}

// buildTranslationPrompt creates the prompt for the translation step.
func buildTranslationPrompt(sentence, userLangName, targetLangName string) string {
	return fmt.Sprintf(`You are a professional translator. Translate the sentence and clean it if needed.

INPUT:
Sentence: "%s"
User's language: %s
Target language: %s

TASK:
1. Clean the input sentence: fix grammar errors, spelling mistakes, punctuation issues, and formatting problems
2. Detect which language the cleaned sentence is in (%s or %s)
3. Translate the cleaned sentence naturally and fluently to the OPPOSITE language
4. The translation MUST be in a different language than the cleaned sentence
5. The translation should be natural and idiomatic, not word-for-word

IMPORTANT:
- The cleaned_sentence and translation MUST be in different languages
- Focus on natural, fluent translation quality
- Fix any errors in the input sentence
- Preserve the meaning and tone`, sentence, userLangName, targetLangName, userLangName, targetLangName)
}

// buildTranslationConfig creates the configuration for the translation API call.
func buildTranslationConfig(userLangName, targetLangName string) *genai.GenerateContentConfig {
	return &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		Temperature:      genai.Ptr(float32(translationTemperature)),
		ResponseJsonSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"input_language": map[string]any{
					"type":        "string",
					"description": fmt.Sprintf("The language of the input sentence: either '%s' or '%s'", userLangName, targetLangName),
				},
				"cleaned_sentence": map[string]any{
					"type":        "string",
					"description": "The input sentence after cleaning in original input language (fixing grammar, spelling, punctuation, formatting)",
				},
				"translation": map[string]any{
					"type":        "string",
					"description": "Natural, fluent translation to the opposite language",
				},
				"translation_language": map[string]any{
					"type":        "string",
					"description": fmt.Sprintf("The language of the translation: either '%s' or '%s'", userLangName, targetLangName),
				},
			},
			"required": []string{"input_language", "cleaned_sentence", "translation", "translation_language"},
		},
	}
}

// buildAnalysisPrompt creates the prompt for the word analysis step.
func buildAnalysisPrompt(foreignSentence, userLangName, targetLangName string) string {
	return fmt.Sprintf(`Analyze each word from the foreign language sentence.

Foreign language sentence (%s): "%s"
User's language: %s

TASK:
For each word in the foreign language sentence, provide a short, concise analysis in %s.
Include: translation/meaning and brief grammatical explanation in the context of the whole sentence.

IMPORTANT:
- Only analyze actual words
- Keep each analysis short and direct.`, targetLangName, foreignSentence, userLangName, userLangName)
}

// buildAnalysisConfig creates the configuration for the word analysis API call.
func buildAnalysisConfig(userLangName, targetLangName string) *genai.GenerateContentConfig {
	return &genai.GenerateContentConfig{
		ResponseMIMEType: "application/json",
		Temperature:      genai.Ptr(float32(analysisTemperature)),
		ResponseJsonSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"word_analysis": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"word": map[string]any{
								"type":        "string",
								"description": fmt.Sprintf("Exact word from the %s sentence", targetLangName),
							},
							"analysis": map[string]any{
								"type":        "string",
								"description": fmt.Sprintf("Short, concise analysis in %s: translation/meaning and brief grammatical explanation", userLangName),
							},
						},
						"required": []string{"word", "analysis"},
					},
				},
			},
			"required": []string{"word_analysis"},
		},
	}
}

// extractTextFromResponse extracts text content from the API response.
func extractTextFromResponse(resp *genai.GenerateContentResponse) string {
	var text strings.Builder
	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part.Text != "" {
				text.WriteString(part.Text)
			}
		}
	}
	return strings.TrimSpace(text.String())
}

// getForeignSentence determines which sentence is in the foreign language.
func getForeignSentence(step *translationStepResult, targetLangName string) string {
	if step.InputLanguage == targetLangName {
		return step.CleanedSentence
	}
	return step.Translation
}

// processWordAnalysis processes and cleans word analysis results.
func processWordAnalysis(analysis *wordAnalysisStepResult) []wordInfo {
	wordAnalysis := make([]wordInfo, 0, len(analysis.WordAnalysis))
	for _, w := range analysis.WordAnalysis {
		cleanedWord := removePunctuation(w.Word)
		if cleanedWord == "" {
			continue // Skip entries that are only punctuation
		}
		wordAnalysis = append(wordAnalysis, wordInfo{
			WordInTargetLang:       cleanedWord,
			GrammaticalExplanation: w.Analysis,
		})
	}
	return wordAnalysis
}

// removePunctuation removes all punctuation marks from a string, keeping only letters, numbers, and spaces.
func removePunctuation(s string) string {
	var result strings.Builder
	prevWasSpace := false
	for _, r := range s {
		// Keep letters, numbers, and spaces; remove punctuation
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			result.WriteRune(r)
			prevWasSpace = false
		} else if unicode.IsSpace(r) {
			// Only add one space, collapse multiple spaces
			if !prevWasSpace {
				result.WriteRune(' ')
				prevWasSpace = true
			}
		}
	}
	return strings.TrimSpace(result.String())
}

// getLanguageName returns the full name of a language given its code.
// If the code is not recognized, it returns the code itself.
func getLanguageName(code string) string {
	langMap := map[string]string{
		"en": "English",
		"es": "Spanish",
		"fr": "French",
		"it": "Italian",
		"pt": "Portuguese",
		"sr": "Serbian",
		"sv": "Swedish",
		"de": "German",
	}
	if name, ok := langMap[code]; ok {
		return name
	}
	return code
}
