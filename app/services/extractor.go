package services

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/logging"

	"go.uber.org/zap"
)

// ExtractionResult represents the result of content extraction
type ExtractionResult struct {
	URL     string  `json:"url"`
	Content *string `json:"content,omitempty"`
	Error   *string `json:"error,omitempty"`
}

func (e *ExtractionResult) Success() bool {
	return e.Content != nil
}

// ContentExtractor handles URL content extraction
type ContentExtractor struct {
	cfg               *config.AppConfig
	httpClient        *http.Client
	domainSemaphores  map[string]chan struct{}
	domainMu          sync.Mutex
	domainConcurrency int
	maxDomains        int
	globalSemaphore   chan struct{}
	logger            *zap.SugaredLogger
}

func NewContentExtractor(cfg *config.AppConfig, httpClient *http.Client) *ContentExtractor {
	globalSemaphore := make(chan struct{}, cfg.Extraction.MaxConcurrent)
	for i := 0; i < cfg.Extraction.MaxConcurrent; i++ {
		globalSemaphore <- struct{}{}
	}

	return &ContentExtractor{
		cfg:               cfg,
		httpClient:        httpClient,
		domainSemaphores:  make(map[string]chan struct{}),
		domainConcurrency: cfg.Extraction.DomainConcurrency,
		maxDomains:        cfg.Extraction.DomainSemaphoreMaxSize,
		globalSemaphore:   globalSemaphore,
		logger:            logging.GetLogger(),
	}
}

func (ce *ContentExtractor) getDomainSemaphore(urlStr string) chan struct{} {
	ce.domainMu.Lock()
	defer ce.domainMu.Unlock()

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return make(chan struct{}, ce.domainConcurrency)
	}

	domain := parsedURL.Host

	if sem, exists := ce.domainSemaphores[domain]; exists {
		return sem
	}

	if len(ce.domainSemaphores) >= ce.maxDomains {
		// Remove oldest entry
		oldestDomain := ""
		for domain := range ce.domainSemaphores {
			oldestDomain = domain
			break
		}
		if oldestDomain != "" {
			delete(ce.domainSemaphores, oldestDomain)
		}
	}

	sem := make(chan struct{}, ce.domainConcurrency)
	for i := 0; i < ce.domainConcurrency; i++ {
		sem <- struct{}{}
	}
	ce.domainSemaphores[domain] = sem
	return sem
}

func (ce *ContentExtractor) getHeaders() http.Header {
	userAgents := ce.cfg.Extraction.UserAgents
	ua := userAgents[rand.Intn(len(userAgents))]

	headers := make(http.Header)
	headers.Set("User-Agent", ua)
	headers.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	headers.Set("Accept-Language", "en-US,en;q=0.9")
	headers.Set("Accept-Encoding", "gzip, deflate, br")
	headers.Set("DNT", "1")
	headers.Set("Connection", "keep-alive")
	headers.Set("Upgrade-Insecure-Requests", "1")

	return headers
}

// ExtractURL extracts content from a single URL
func (ce *ContentExtractor) ExtractURL(ctx context.Context, urlStr string, outputFormat string) *ExtractionResult {
	domainSem := ce.getDomainSemaphore(urlStr)

	select {
	case <-ce.globalSemaphore:
		defer func() { ce.globalSemaphore <- struct{}{} }()
	case <-ctx.Done():
		return &ExtractionResult{
			URL:   urlStr,
			Error: strPtr("context cancelled"),
		}
	}

	select {
	case <-domainSem:
		defer func() { domainSem <- struct{}{} }()
	case <-ctx.Done():
		return &ExtractionResult{
			URL:   urlStr,
			Error: strPtr("context cancelled"),
		}
	}

	return ce.fetchAndExtract(ctx, urlStr, outputFormat)
}

// ExtractURLs extracts content from multiple URLs concurrently
func (ce *ContentExtractor) ExtractURLs(ctx context.Context, urls []string, outputFormat string) []*ExtractionResult {
	results := make([]*ExtractionResult, len(urls))
	var wg sync.WaitGroup

	for i, urlStr := range urls {
		wg.Add(1)
		go func(idx int, u string) {
			defer wg.Done()
			results[idx] = ce.ExtractURL(ctx, u, outputFormat)
		}(i, urlStr)
	}

	wg.Wait()
	return results
}

func (ce *ContentExtractor) fetchAndExtract(ctx context.Context, urlStr string, outputFormat string) *ExtractionResult {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(ce.cfg.Extraction.Timeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return &ExtractionResult{
			URL:   urlStr,
			Error: strPtr(fmt.Sprintf("failed to create request: %v", err)),
		}
	}

	for key, values := range ce.getHeaders() {
		for _, value := range values {
			req.Header.Set(key, value)
		}
	}

	resp, err := ce.httpClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			return &ExtractionResult{
				URL:   urlStr,
				Error: strPtr(fmt.Sprintf("Timeout after %ds", ce.cfg.Extraction.Timeout)),
			}
		}
		return &ExtractionResult{
			URL:   urlStr,
			Error: strPtr(fmt.Sprintf("HTTP error: %v", err)),
		}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &ExtractionResult{
			URL:   urlStr,
			Error: strPtr(fmt.Sprintf("HTTP %d", resp.StatusCode)),
		}
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") && !strings.Contains(contentType, "application/xhtml") {
		return &ExtractionResult{
			URL:   urlStr,
			Error: strPtr(fmt.Sprintf("Non-HTML content type: %s", contentType)),
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &ExtractionResult{
			URL:   urlStr,
			Error: strPtr(fmt.Sprintf("Failed to read body: %v", err)),
		}
	}

	html := string(body)

	// Try trafilatura-like extraction (simplified HTML to text)
	extracted := ce.extractSimple(html, urlStr, outputFormat)

	if extracted == "" || len(strings.TrimSpace(extracted)) < 50 {
		ce.logger.Infow("extraction_fallback", "url", urlStr)
		extracted = ce.extractFallback(html, urlStr, outputFormat)
	}

	if extracted == "" {
		return &ExtractionResult{
			URL:   urlStr,
			Error: strPtr("No content could be extracted"),
		}
	}

	maxLen := ce.cfg.Extraction.MaxContentLength
	if len(extracted) > maxLen {
		extracted = extracted[:maxLen] + "\n\n[Content truncated]"
	}

	return &ExtractionResult{
		URL:     urlStr,
		Content: &extracted,
	}
}

func (ce *ContentExtractor) extractSimple(html, urlStr, outputFormat string) string {
	// Simple HTML to text extraction
	// Remove script and style elements
	re := regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</\1>`)
	html = re.ReplaceAllString(html, "")

	// Remove all tags
	re = regexp.MustCompile(`(?s)<[^>]+>`)
	text := re.ReplaceAllString(html, " ")

	// Normalize whitespace
	re = regexp.MustCompile(`\s+`)
	text = re.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	if len(text) < 50 {
		return ""
	}

	if outputFormat == "markdown" {
		return ce.toMarkdown(text, html, urlStr)
	}

	return text
}

func (ce *ContentExtractor) extractFallback(html, urlStr, outputFormat string) string {
	return ce.extractSimple(html, urlStr, outputFormat)
}

func (ce *ContentExtractor) toMarkdown(text, html, urlStr string) string {
	// Extract title
	title := ""
	re := regexp.MustCompile(`(?i)<title[^>]*>(.*?)</title>`)
	if matches := re.FindStringSubmatch(html); len(matches) > 1 {
		title = strings.TrimSpace(matches[1])
	}

	lines := []string{}
	if title != "" {
		lines = append(lines, fmt.Sprintf("# %s\n", title))
	}
	lines = append(lines, fmt.Sprintf("Source: %s\n", urlStr))
	lines = append(lines, "---\n")
	lines = append(lines, text)

	return strings.Join(lines, "\n")
}

func strPtr(s string) *string {
	return &s
}
