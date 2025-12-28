package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/genai"
)

// SummarizeRequest is the expected JSON body for /api/summarize
type SummarizeRequest struct {
	APIKey     string `json:"api_key"`
	Transcript string `json:"transcript" binding:"required"`
	ChunkChars int    `json:"chunk_chars"`
}

// SummarizeResponse is returned to the client
type SummarizeResponse struct {
	Summary string   `json:"summary"`
	Chunks  []string `json:"chunks"`
}

// Test api
func testGenaiAPI(apiKey string) error {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal(err)
	}

	result, _ := client.Models.GenerateContent(
		ctx,
		"gemini-2.5-flash",
		genai.Text("Explain how AI works in a few words"),
		nil,
	)

	var s = result.Text()
	fmt.Println(s)

	return nil
}

// summarizeHandler handles POST /api/summarize
func summarizeHandler(c *gin.Context) {
	var req SummarizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: transcript required"})
		return
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "api_key not provided and GOOGLE_API_KEY not set"})
		return
	}

	chunkChars := req.ChunkChars
	if chunkChars <= 0 {
		chunkChars = 3000
	}

	// split transcript
	chunks := chunkText(req.Transcript, chunkChars)
	summaries := make([]string, 0, len(chunks))

	for i, ch := range chunks {
		prompt := "请把以下文字总结为要点，尽量简洁，保留关键信息：\n\n" + ch
		s, err := callGoogleGenerate(c.Request.Context(), apiKey, prompt, 300)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to summarize chunk", "detail": err.Error(), "chunk": i})
			return
		}
		summaries = append(summaries, s)
		// small delay to avoid rate bursts
		time.Sleep(200 * time.Millisecond)
	}

	// combine intermediate summaries and produce a final summary
	combined := strings.Join(summaries, "\n\n")
	finalPrompt := "以下是每段的摘要，请将它们整合为一份最终总结，提取要点，控制长度在300字以内：\n\n" + combined
	finalSummary, err := callGoogleGenerate(c.Request.Context(), apiKey, finalPrompt, 600)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to produce final summary", "detail": err.Error()})
		return
	}

	resp := SummarizeResponse{Summary: finalSummary, Chunks: summaries}
	c.JSON(http.StatusOK, resp)
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

	// split into sentences by punctuation as a heuristic
	separators := []string{"。", ".", "?", "!", "？", "！", "；", ";", "\n"}
	var parts []string
	cur := ""
	for _, r := range text {
		cur += string(r)
		if len(cur) >= maxChars {
			parts = append(parts, strings.TrimSpace(cur))
			cur = ""
			continue
		}
		for _, sep := range separators {
			if strings.HasSuffix(cur, sep) {
				if len(cur) >= 200 { // avoid too short pieces
					parts = append(parts, strings.TrimSpace(cur))
					cur = ""
				}
				break
			}
		}
	}
	if strings.TrimSpace(cur) != "" {
		parts = append(parts, strings.TrimSpace(cur))
	}
	// if any part still too big, force split
	var final []string
	for _, p := range parts {
		if len(p) <= maxChars {
			final = append(final, p)
			continue
		}
		for i := 0; i < len(p); i += maxChars {
			end := i + maxChars
			if end > len(p) {
				end = len(p)
			}
			final = append(final, strings.TrimSpace(p[i:end]))
		}
	}
	return final
}

// callGoogleGenerate sends a prompt to Google Generative API and returns text output.
func callGoogleGenerate(ctx context.Context, apiKey, prompt string, maxOutputTokens int) (string, error) {
	// Prefer using the official genai SDK when possible (accepts API key via options).
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	return "", errors.New("no generated text found in response")
}
