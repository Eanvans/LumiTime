package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"google.golang.org/genai"
)

// GoogleAIService provides AI summarization and content generation capabilities
type GoogleAIService struct {
	apiKey string
}

// NewGoogleAIService creates a new GoogleAI service instance
// If apiKey is empty, it will use the configured API key from config
func NewGoogleAIService(apiKey string) *GoogleAIService {
	if apiKey == "" {
		apiKey = GetGoogleAPIConfig().APIKey
	}
	return &GoogleAIService{
		apiKey: apiKey,
	}
}

// SRTSubtitle represents a single subtitle entry
type SRTSubtitle struct {
	Index     int
	StartTime string
	EndTime   string
	Text      string
}

// GenerateContent generates content using Google Gemini API with a given prompt
// Input: prompt string
// Output: generated text string
func (s *GoogleAIService) GenerateContent(ctx context.Context, prompt string, maxOutputTokens int) (string, error) {
	if s.apiKey == "" {
		return "", errors.New("Google API key not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  s.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create genai client: %w", err)
	}

	temp := float32(0.7)
	generateCfg := &genai.GenerateContentConfig{
		MaxOutputTokens: int32(maxOutputTokens),
		Temperature:     &temp,
	}

	log.Printf("Calling Gemini API with maxOutputTokens: %d, prompt length: %d", maxOutputTokens, len(prompt))

	result, err := client.Models.GenerateContent(
		ctx,
		"gemini-2.5-flash-lite",
		genai.Text(prompt),
		generateCfg,
	)
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	text := result.Text()
	if text == "" {
		return "", errors.New("no generated text found in response")
	}

	log.Printf("Received response length: %d characters", len(text))

	return text, nil
}

// SummarizeSRT summarizes SRT subtitle content
// Input: srtContent string (SRT file content), chunkChars int (size of each chunk, default 10000)
// Output: final summary string, chunk summaries []string
func (s *GoogleAIService) SummarizeSRT(ctx context.Context, srtContent string, chunkChars int) (string, []string, error) {
	// Parse SRT content
	transcript, err := parseSRTFile(srtContent)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse SRT file: %w", err)
	}

	if chunkChars <= 0 {
		chunkChars = 10000
	}

	log.Printf("Parsed transcript length: %d characters", len(transcript))

	// Split transcript into chunks
	chunks := chunkText(transcript, chunkChars)
	summaries := make([]string, 0, len(chunks))

	// Summarize each chunk
	for i, ch := range chunks {
		prompt := "This is a clip from a streamer's live broadcast. To summarize, what topics are being discussed in this segment: \n\n" + ch
		s, err := s.GenerateContent(ctx, prompt, 600)
		if err != nil {
			return "", nil, fmt.Errorf("failed to summarize chunk %d: %w", i, err)
		}
		summaries = append(summaries, s)
		log.Printf("Summarized chunk %d/%d", i+1, len(chunks))

		// Small delay to avoid rate bursts
		time.Sleep(200 * time.Millisecond)
	}

	// Combine intermediate summaries and produce a final summary
	combined := strings.Join(summaries, "\n\n")
	finalPrompt := "Below are summaries of each section. Please consolidate them into a final summary, presenting key points in Chinese and keeping the length within 300 words：\n\n" + combined
	finalSummary, err := s.GenerateContent(ctx, finalPrompt, 600)
	if err != nil {
		return "", summaries, fmt.Errorf("failed to produce final summary: %w", err)
	}

	return finalSummary, summaries, nil
}

// SaveSummaryToFile saves the summary to a text file next to the subtitle file
func (s *GoogleAIService) SaveSummaryToFile(srtFilePath, summary string) error {
	// Generate summary file path (replace .srt with _summary.txt)
	summaryPath := strings.TrimSuffix(srtFilePath, filepath.Ext(srtFilePath)) + "_summary.txt"

	// Write summary to file
	err := os.WriteFile(summaryPath, []byte(summary), 0644)
	if err != nil {
		return fmt.Errorf("failed to write summary file: %w", err)
	}

	log.Printf("💾 Summary saved to: %s", summaryPath)
	return nil
}

// chunkText splits text into chunks of approximately maxChars, respecting sentence boundaries when possible.
func chunkText(text string, maxChars int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if len(text) <= maxChars {
		return []string{text}
	}

	// Split by double newlines (subtitle entries)
	entries := strings.Split(text, "\n\n")

	var chunks []string
	currentChunk := ""

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		// If adding this entry would exceed maxChars, save current chunk and start new one
		if currentChunk != "" && len(currentChunk)+len("\n\n")+len(entry) > maxChars {
			chunks = append(chunks, strings.TrimSpace(currentChunk))
			currentChunk = entry
		} else {
			// Add to current chunk
			if currentChunk != "" {
				currentChunk += "\n\n" + entry
			} else {
				currentChunk = entry
			}
		}
	}

	// Add the last chunk
	if strings.TrimSpace(currentChunk) != "" {
		chunks = append(chunks, strings.TrimSpace(currentChunk))
	}

	return chunks
}

// parseSRTFile parses SRT subtitle content and returns the text transcript with timestamps
func parseSRTFile(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", errors.New("empty SRT content")
	}

	// Split by double newlines to separate subtitle blocks
	blocks := regexp.MustCompile(`\n\s*\n`).Split(content, -1)

	var transcriptParts []string

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		lines := strings.Split(block, "\n")
		if len(lines) < 3 {
			continue
		}

		// Skip the index line (first line)
		// Keep the timestamp line (second line)
		timestamp := strings.TrimSpace(lines[1])

		// Extract the text (third line and beyond)
		textLines := lines[2:]
		text := strings.TrimSpace(strings.Join(textLines, "\n"))

		if text != "" {
			// Format: timestamp + newline + text + double newline
			transcriptParts = append(transcriptParts, timestamp+"\n"+text)
		}
	}

	if len(transcriptParts) == 0 {
		return "", errors.New("no valid subtitles found in SRT file")
	}

	return strings.Join(transcriptParts, "\n\n"), nil
}

// ParseSRTDetailed parses SRT content into structured subtitle entries
func ParseSRTDetailed(content string) ([]SRTSubtitle, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, errors.New("empty SRT content")
	}

	blocks := regexp.MustCompile(`\n\s*\n`).Split(content, -1)
	var subtitles []SRTSubtitle

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		lines := strings.Split(block, "\n")
		if len(lines) < 3 {
			continue
		}

		// Parse index
		index, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil {
			continue
		}

		// Parse timestamp line (e.g., "00:00:01,000 --> 00:00:03,000")
		timestamps := strings.Split(lines[1], "-->")
		if len(timestamps) != 2 {
			continue
		}

		startTime := strings.TrimSpace(timestamps[0])
		endTime := strings.TrimSpace(timestamps[1])

		// Extract text
		textLines := lines[2:]
		text := strings.TrimSpace(strings.Join(textLines, " "))

		subtitles = append(subtitles, SRTSubtitle{
			Index:     index,
			StartTime: startTime,
			EndTime:   endTime,
			Text:      text,
		})
	}

	return subtitles, nil
}
