package handlers

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

// AliyunAIService provides AI summarization and content generation capabilities using Alibaba Cloud DashScope API
type AliyunAIService struct {
	apiKey string
	client *openai.Client
}

// NewAliyunAIService creates a new AliyunAI service instance
// If apiKey is empty, it will use DASHSCOPE_API_KEY environment variable
func NewAliyunAIService(apiKey string) *AliyunAIService {
	if apiKey == "" {
		apiKey = GetAlibabaAPIConfig().APIKey
	}

	if apiKey == "" {
		log.Println("Warning: DASHSCOPE_API_KEY not configured")
	}

	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL("https://dashscope.aliyuncs.com/compatible-mode/v1"),
	)

	return &AliyunAIService{
		apiKey: apiKey,
		client: &client,
	}
}

// GenerateContent generates content using Alibaba Cloud Qwen API with a given prompt
// Input: prompt string, maxOutputTokens int
// Output: generated text string
//
// Available models:
// - qwen-plus: 通用模型，平衡性能和成本
// - qwen-turbo: 快速响应，适合实时场景
// - qwen-max: 最强大的模型，适合复杂任务
func (s *AliyunAIService) GenerateContent(ctx context.Context, prompt string, maxOutputTokens int) (string, error) {
	if s.apiKey == "" {
		return "", errors.New("Aliyun API key not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	log.Printf("Calling Qwen API with maxOutputTokens: %d, prompt length: %d", maxOutputTokens, len(prompt))

	chatCompletion, err := s.client.Chat.Completions.New(
		ctx, openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(prompt),
			},
			Model: "qwen-flash",
		},
	)

	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if len(chatCompletion.Choices) == 0 {
		return "", errors.New("no choices returned from API")
	}

	text := chatCompletion.Choices[0].Message.Content
	if text == "" {
		return "", errors.New("no generated text found in response")
	}

	log.Printf("Received response length: %d characters", len(text))

	return text, nil
}

// GenerateContentWithModel generates content using a specified Qwen model
// Input: prompt string, maxOutputTokens int, model string
// Output: generated text string
func (s *AliyunAIService) GenerateContentWithModel(ctx context.Context, prompt string, maxOutputTokens int, model string) (string, error) {
	if s.apiKey == "" {
		return "", errors.New("Aliyun API key not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	log.Printf("Calling Qwen API (%s) with maxOutputTokens: %d, prompt length: %d", model, maxOutputTokens, len(prompt))

	chatCompletion, err := s.client.Chat.Completions.New(
		ctx, openai.ChatCompletionNewParams{
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.UserMessage(prompt),
			},
			Model: model,
		},
	)

	if err != nil {
		return "", fmt.Errorf("failed to generate content: %w", err)
	}

	if len(chatCompletion.Choices) == 0 {
		return "", errors.New("no choices returned from API")
	}

	text := chatCompletion.Choices[0].Message.Content
	if text == "" {
		return "", errors.New("no generated text found in response")
	}

	log.Printf("Received response length: %d characters", len(text))

	return text, nil
}

// SummarizeSRT summarizes SRT subtitle content
// Input: srtContent string (SRT file content), chunkChars int (size of each chunk, default 10000)
// Output: final summary string, chunk summaries []string
func (s *AliyunAIService) SummarizeSRT(ctx context.Context, srtContent string, chunkChars int) (string, []string, error) {
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
		prompt := "his is a clip from a streamer's live broadcast. To summarize, what topics are being discussed in this segment: \n\n" + ch
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
	finalPrompt := "Here are summaries of each section. Please consolidate them into a final summary, presenting key points in Chinese and keeping the length within 300 words: \n\n" + combined
	finalSummary, err := s.GenerateContent(ctx, finalPrompt, 600)
	if err != nil {
		return "", summaries, fmt.Errorf("failed to produce final summary: %w", err)
	}

	return finalSummary, summaries, nil
}

// SaveSummaryToFile saves the summary to a text file next to the subtitle file
func (s *AliyunAIService) SaveSummaryToFile(srtFilePath, summary string) error {
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

// ChatCompletion provides a more flexible chat completion interface
// Input: messages []openai.ChatCompletionMessageParamUnion, model string, maxTokens int
// Output: response text string
func (s *AliyunAIService) ChatCompletion(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, model string, maxTokens int) (string, error) {
	if s.apiKey == "" {
		return "", errors.New("Aliyun API key not configured")
	}

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	if model == "" {
		model = "qwen-plus"
	}

	if maxTokens <= 0 {
		maxTokens = 2000
	}

	chatCompletion, err := s.client.Chat.Completions.New(
		ctx, openai.ChatCompletionNewParams{
			Messages: messages,
			Model:    model,
		},
	)

	if err != nil {
		return "", fmt.Errorf("failed to complete chat: %w", err)
	}

	if len(chatCompletion.Choices) == 0 {
		return "", errors.New("no choices returned from API")
	}

	return chatCompletion.Choices[0].Message.Content, nil
}

// StreamingChatCompletion provides streaming chat completion (for real-time response scenarios)
// This method returns a channel that yields response chunks as they arrive
func (s *AliyunAIService) StreamingChatCompletion(ctx context.Context, messages []openai.ChatCompletionMessageParamUnion, model string, maxTokens int) (<-chan string, <-chan error) {
	resultChan := make(chan string, 10)
	errorChan := make(chan error, 1)

	go func() {
		defer close(resultChan)
		defer close(errorChan)

		if s.apiKey == "" {
			errorChan <- errors.New("Aliyun API key not configured")
			return
		}

		ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()

		if model == "" {
			model = "qwen-plus"
		}

		if maxTokens <= 0 {
			maxTokens = 2000
		}

		stream := s.client.Chat.Completions.NewStreaming(
			ctx, openai.ChatCompletionNewParams{
				Messages: messages,
				Model:    model,
			},
		)

		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				select {
				case resultChan <- chunk.Choices[0].Delta.Content:
				case <-ctx.Done():
					errorChan <- ctx.Err()
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			errorChan <- fmt.Errorf("streaming error: %w", err)
		}
	}()

	return resultChan, errorChan
}
