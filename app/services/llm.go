package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/logging"
	"github.com/TW-Fusion/fusion-search/app/models"

	openai "github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

// LLMService handles AI answer generation
type LLMService struct {
	cfg    *config.AppConfig
	client *openai.Client
	logger *zap.SugaredLogger
}

func NewLLMService(cfg *config.AppConfig) *LLMService {
	return &LLMService{
		cfg:    cfg,
		logger: logging.GetLogger(),
	}
}

func (llm *LLMService) Initialize() {
	if !llm.cfg.LLM.Enabled {
		llm.logger.Info("llm_disabled")
		return
	}

	config := openai.DefaultConfig(llm.cfg.LLM.APIKey)
	config.BaseURL = llm.cfg.LLM.BaseURL
	config.HTTPClient = &http.Client{
		Timeout: time.Duration(llm.cfg.LLM.Timeout) * time.Second,
	}

	llm.client = openai.NewClientWithConfig(config)

	llm.logger.Infow("llm_initialized",
		"provider", llm.cfg.LLM.Provider,
		"model", llm.cfg.LLM.Model,
		"base_url", llm.cfg.LLM.BaseURL,
	)
}

func (llm *LLMService) Close() error {
	if llm.client != nil {
		llm.client = nil
	}
	return nil
}

func (llm *LLMService) buildContext(query string, results []models.SearchResult) []openai.ChatCompletionMessage {
	contextParts := []string{}
	charCount := 0
	maxResults := llm.cfg.LLM.MaxContextResults
	maxChars := llm.cfg.LLM.MaxContextChars

	for i, r := range results {
		if i >= maxResults {
			break
		}

		content := r.Content
		if r.RawContent != nil {
			content = *r.RawContent
		}

		remaining := maxChars - charCount
		if remaining <= 0 {
			break
		}

		text := content
		if len(text) > remaining {
			text = text[:remaining] + "..."
		}

		contextParts = append(contextParts,
			fmt.Sprintf("[%d] %s - %s\n%s", i+1, r.Title, r.URL, text))
		charCount += len(text)
	}

	contextBlock := ""
	if len(contextParts) > 0 {
		contextBlock = strings.Join(contextParts, "\n\n")
	}

	systemPrompt := llm.cfg.LLM.SystemPrompt
	if systemPrompt == "" {
		systemPrompt = "You are a helpful search assistant. Answer the user's question concisely based on the provided search results. Cite sources by number [1], [2], etc."
	}

	return []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		},
		{
			Role: openai.ChatMessageRoleUser,
			Content: fmt.Sprintf("Search results for: %s\n\n%s\n\nQuestion: %s",
				query, contextBlock, query),
		},
	}
}

// GenerateAnswer generates a non-streaming answer
func (llm *LLMService) GenerateAnswer(ctx context.Context, query string, results []models.SearchResult) (*string, error) {
	if llm.client == nil {
		return nil, nil
	}

	messages := llm.buildContext(query, results)

	req := openai.ChatCompletionRequest{
		Model:       llm.cfg.LLM.Model,
		Messages:    messages,
		MaxTokens:   llm.cfg.LLM.MaxTokens,
		Temperature: llm.cfg.LLM.Temperature,
	}

	resp, err := llm.client.CreateChatCompletion(ctx, req)
	if err != nil {
		llm.logger.Errorw("llm_answer_failed", "query", query, "error", err)
		return nil, err
	}

	if len(resp.Choices) > 0 {
		answer := resp.Choices[0].Message.Content
		llm.logger.Infow("llm_answer_generated",
			"query", query,
			"tokens", resp.Usage.TotalTokens,
		)
		return &answer, nil
	}

	return nil, fmt.Errorf("no response from LLM")
}

// GenerateAnswerStream generates a streaming answer and returns a channel for chunks
func (llm *LLMService) GenerateAnswerStream(ctx context.Context, query string, results []models.SearchResult) (<-chan string, <-chan error) {
	ch := make(chan string)
	errCh := make(chan error, 1)

	if llm.client == nil {
		close(ch)
		errCh <- nil
		return ch, errCh
	}

	go func() {
		defer close(ch)
		defer close(errCh)

		messages := llm.buildContext(query, results)

		req := openai.ChatCompletionRequest{
			Model:       llm.cfg.LLM.Model,
			Messages:    messages,
			MaxTokens:   llm.cfg.LLM.MaxTokens,
			Temperature: llm.cfg.LLM.Temperature,
			Stream:      true,
		}

		stream, err := llm.client.CreateChatCompletionStream(ctx, req)
		if err != nil {
			llm.logger.Errorw("llm_stream_failed", "query", query, "error", err)
			errCh <- err
			return
		}
		defer stream.Close()

		for {
			response, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				llm.logger.Errorw("llm_stream_failed", "query", query, "error", err)
				errCh <- err
				return
			}

			if len(response.Choices) > 0 {
				delta := response.Choices[0].Delta
				if delta.Content != "" {
					ch <- delta.Content
				}
			}
		}
	}()

	return ch, errCh
}
