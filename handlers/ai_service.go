package handlers

import "context"

// AIService defines the common interface for AI services (Google AI, Aliyun AI, etc.)
// This allows for easy switching between different AI providers
type AIService interface {
	// GenerateContent generates content using AI with a given prompt
	// Input: ctx context, prompt string, maxOutputTokens int
	// Output: generated text string, error
	GenerateContent(ctx context.Context, prompt string, maxOutputTokens int) (string, error)

	// SummarizeSRT summarizes SRT subtitle content
	// Input: ctx context, srtContent string (SRT file content), chunkChars int (size of each chunk)
	// Output: final summary string, chunk summaries []string, error
	SummarizeSRT(ctx context.Context, srtContent string, chunkChars int) (string, []string, error)

	// SaveSummaryToFile saves the summary to a text file next to the subtitle file
	// Input: srtFilePath string, summary string
	// Output: error
	SaveSummaryToFile(srtFilePath, summary string) error
}

// NewAIService creates an AI service instance based on the provider type
// Input: provider string ("google" or "aliyun"), apiKey string (optional)
// Output: AIService interface
func NewAIService(provider string, apiKey string) AIService {
	switch provider {
	case "google":
		return NewGoogleAIService(apiKey)
	case "aliyun":
		return NewAliyunAIService(apiKey)
	default:
		// Default to Google AI
		return NewGoogleAIService(apiKey)
	}
}
