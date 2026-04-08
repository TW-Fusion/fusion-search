package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/logging"
	"github.com/sony/gobreaker"

	"go.uber.org/zap"
)

// RawSearchResult represents a single search result
type RawSearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

// RawImageResult represents an image result
type RawImageResult struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// BackendSearchResponse represents the response from a search backend
type BackendSearchResponse struct {
	Results []RawSearchResult `json:"results"`
	Images  []RawImageResult  `json:"images"`
}

// SearchBackend interface for search providers
type SearchBackend interface {
	Search(ctx context.Context, query string, opts SearchOptions) (*BackendSearchResponse, error)
}

// SearchOptions contains search parameters
type SearchOptions struct {
	MaxResults     int
	Topic          string
	TimeRange      *string
	IncludeDomains []string
	ExcludeDomains []string
	IncludeImages  bool
}

// SearXNGBackend implements SearchBackend for SearXNG
type SearXNGBackend struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.SugaredLogger
	cfg        *config.AppConfig
	breaker    *gobreaker.CircuitBreaker
}

func NewSearXNGBackend(cfg *config.AppConfig, httpClient *http.Client) *SearXNGBackend {
	return &SearXNGBackend{
		baseURL:    cfg.Search.SearxngURL,
		httpClient: httpClient,
		logger:     logging.GetLogger(),
		cfg:        cfg,
		breaker:    newCircuitBreaker("search_backend", cfg),
	}
}

func (s *SearXNGBackend) Search(ctx context.Context, query string, opts SearchOptions) (*BackendSearchResponse, error) {
	effectiveQuery := query
	if len(opts.IncludeDomains) > 0 {
		for _, domain := range opts.IncludeDomains {
			effectiveQuery += fmt.Sprintf(" site:%s", domain)
		}
	}
	if len(opts.ExcludeDomains) > 0 {
		for _, domain := range opts.ExcludeDomains {
			effectiveQuery += fmt.Sprintf(" -site:%s", domain)
		}
	}

	categories := "general"
	if opts.Topic == "news" {
		categories = "news"
	}

	results, err := s.webSearchWithRetry(ctx, effectiveQuery, opts.MaxResults, categories, opts.TimeRange)
	if err != nil {
		return nil, err
	}

	images := []RawImageResult{}
	if opts.IncludeImages {
		imgResults, err := s.imageSearchWithRetry(ctx, query, 5)
		if err != nil {
			s.logger.Warnw("image_search_failed", "error", err)
		} else {
			images = imgResults
		}
	}

	return &BackendSearchResponse{
		Results: results,
		Images:  images,
	}, nil
}

func (s *SearXNGBackend) webSearchWithRetry(ctx context.Context, query string, maxResults int, categories string, timeRange *string) ([]RawSearchResult, error) {
	result, err := s.breaker.Execute(func() (interface{}, error) {
		return retryWithBackoff(ctx, s.cfg.Resilience.RetryMaxAttempts, s.cfg.Resilience.RetryBackoffBase, s.cfg.Resilience.RetryOnStatusCodes, func() ([]RawSearchResult, error) {
			return s.webSearch(ctx, query, maxResults, categories, timeRange)
		})
	})
	if err != nil {
		return nil, err
	}
	return result.([]RawSearchResult), nil
}

func (s *SearXNGBackend) imageSearchWithRetry(ctx context.Context, query string, maxResults int) ([]RawImageResult, error) {
	result, err := s.breaker.Execute(func() (interface{}, error) {
		return retryWithBackoff(ctx, s.cfg.Resilience.RetryMaxAttempts, s.cfg.Resilience.RetryBackoffBase, s.cfg.Resilience.RetryOnStatusCodes, func() ([]RawImageResult, error) {
			return s.imageSearch(ctx, query, maxResults)
		})
	})
	if err != nil {
		return nil, err
	}
	return result.([]RawImageResult), nil
}

func (s *SearXNGBackend) webSearch(ctx context.Context, query string, maxResults int, categories string, timeRange *string) ([]RawSearchResult, error) {
	url := fmt.Sprintf("%s/search?q=%s&format=json&categories=%s",
		s.baseURL,
		query,
		categories,
	)

	if timeRange != nil {
		url += fmt.Sprintf("&time_range=%s", *timeRange)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("search request failed with status: %d", resp.StatusCode)}
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	results := []RawSearchResult{}
	if rawResults, ok := data["results"].([]interface{}); ok {
		count := 0
		for _, item := range rawResults {
			if count >= maxResults {
				break
			}
			if result, ok := item.(map[string]interface{}); ok {
				title, _ := result["title"].(string)
				url, _ := result["url"].(string)
				snippet, _ := result["content"].(string)
				rawScore, _ := result["score"].(float64)

				score := rawScore
				if rawScore > 1.0 {
					score = rawScore / 10.0
				}
				if score > 1.0 {
					score = 1.0
				}

				results = append(results, RawSearchResult{
					Title:   title,
					URL:     url,
					Snippet: snippet,
					Score:   score,
				})
				count++
			}
		}
	}

	return results, nil
}

func (s *SearXNGBackend) imageSearch(ctx context.Context, query string, maxResults int) ([]RawImageResult, error) {
	url := fmt.Sprintf("%s/search?q=%s&format=json&categories=images",
		s.baseURL,
		query,
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("image search request failed with status: %d", resp.StatusCode)}
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	images := []RawImageResult{}
	if rawResults, ok := data["results"].([]interface{}); ok {
		count := 0
		for _, item := range rawResults {
			if count >= maxResults {
				break
			}
			if result, ok := item.(map[string]interface{}); ok {
				imgURL, _ := result["img_src"].(string)
				if imgURL == "" {
					imgURL, _ = result["url"].(string)
				}
				description, _ := result["title"].(string)

				if imgURL != "" {
					images = append(images, RawImageResult{
						URL:         imgURL,
						Description: description,
					})
					count++
				}
			}
		}
	}

	return images, nil
}

// DuckDuckGoBackend implements SearchBackend for DuckDuckGo
type DuckDuckGoBackend struct {
	logger     *zap.SugaredLogger
	httpClient *http.Client
	cfg        *config.AppConfig
	breaker    *gobreaker.CircuitBreaker
}

func NewDuckDuckGoBackend(cfg *config.AppConfig, httpClient *http.Client) *DuckDuckGoBackend {
	return &DuckDuckGoBackend{
		logger:     logging.GetLogger(),
		httpClient: httpClient,
		cfg:        cfg,
		breaker:    newCircuitBreaker("duckduckgo_backend", cfg),
	}
}

func (d *DuckDuckGoBackend) Search(ctx context.Context, query string, opts SearchOptions) (*BackendSearchResponse, error) {
	effectiveQuery := query
	if len(opts.IncludeDomains) > 0 {
		for _, domain := range opts.IncludeDomains {
			effectiveQuery += fmt.Sprintf(" site:%s", domain)
		}
	}
	if len(opts.ExcludeDomains) > 0 {
		for _, domain := range opts.ExcludeDomains {
			effectiveQuery += fmt.Sprintf(" -site:%s", domain)
		}
	}

	resultsAny, err := d.breaker.Execute(func() (interface{}, error) {
		return retryWithBackoff(ctx, d.cfg.Resilience.RetryMaxAttempts, d.cfg.Resilience.RetryBackoffBase, d.cfg.Resilience.RetryOnStatusCodes, func() ([]RawSearchResult, error) {
			return d.webSearch(ctx, effectiveQuery, opts.MaxResults, opts.TimeRange)
		})
	})
	if err != nil {
		return nil, err
	}
	results := resultsAny.([]RawSearchResult)

	images := []RawImageResult{}
	if opts.IncludeImages {
		imgAny, err := d.breaker.Execute(func() (interface{}, error) {
			return retryWithBackoff(ctx, d.cfg.Resilience.RetryMaxAttempts, d.cfg.Resilience.RetryBackoffBase, d.cfg.Resilience.RetryOnStatusCodes, func() ([]RawImageResult, error) {
				return d.imageSearch(ctx, query, 5)
			})
		})
		if err != nil {
			d.logger.Warnw("image_search_failed", "error", err)
		} else {
			images = imgAny.([]RawImageResult)
		}
	}

	return &BackendSearchResponse{
		Results: results,
		Images:  images,
	}, nil
}

func (d *DuckDuckGoBackend) webSearch(ctx context.Context, query string, maxResults int, timeRange *string) ([]RawSearchResult, error) {
	reqURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", neturl.QueryEscape(query))
	if timeRange != nil {
		m := map[string]string{"day": "d", "week": "w", "month": "m", "year": "y"}
		if v, ok := m[*timeRange]; ok {
			reqURL += "&df=" + v
		}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("duckduckgo web request failed with status: %d", resp.StatusCode)}
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	html := string(body)
	itemRe := regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	snippetRe := regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</a>|<div[^>]*class="[^"]*result__snippet[^"]*"[^>]*>(.*?)</div>`)
	tagRe := regexp.MustCompile(`(?s)<[^>]+>`)
	matches := itemRe.FindAllStringSubmatch(html, maxResults)
	snippets := snippetRe.FindAllStringSubmatch(html, maxResults)
	results := make([]RawSearchResult, 0, len(matches))
	for i, m := range matches {
		link := decodeDDGRedirectURL(m[1])
		title := strings.TrimSpace(tagRe.ReplaceAllString(m[2], " "))
		snippet := ""
		if i < len(snippets) {
			snippet = strings.TrimSpace(tagRe.ReplaceAllString(firstNonEmpty(snippets[i][1], snippets[i][2]), " "))
		}
		score := 1.0 - float64(i)*0.05
		if score < 0.1 {
			score = 0.1
		}
		results = append(results, RawSearchResult{Title: title, URL: link, Snippet: snippet, Score: score})
	}
	return results, nil
}

func (d *DuckDuckGoBackend) imageSearch(ctx context.Context, query string, maxResults int) ([]RawImageResult, error) {
	token, err := d.fetchVQD(ctx, query)
	if err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("https://duckduckgo.com/i.js?l=wt-wt&o=json&q=%s&vqd=%s&p=1", neturl.QueryEscape(query), neturl.QueryEscape(token))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Referer", "https://duckduckgo.com/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode, Message: fmt.Sprintf("duckduckgo image request failed with status: %d", resp.StatusCode)}
	}
	var data struct {
		Results []struct {
			Image string `json:"image"`
			Title string `json:"title"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	images := make([]RawImageResult, 0, maxResults)
	for i, item := range data.Results {
		if i >= maxResults {
			break
		}
		if item.Image == "" {
			continue
		}
		images = append(images, RawImageResult{URL: item.Image, Description: item.Title})
	}
	return images, nil
}

func (d *DuckDuckGoBackend) fetchVQD(ctx context.Context, query string) (string, error) {
	reqURL := fmt.Sprintf("https://duckduckgo.com/?q=%s", neturl.QueryEscape(query))
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`vqd=\\?"([^"]+)"|vqd='([^']+)'`)
	m := re.FindStringSubmatch(string(body))
	if len(m) > 1 {
		return firstNonEmpty(m[1], m[2]), nil
	}
	return "", fmt.Errorf("duckduckgo vqd token not found")
}

func decodeDDGRedirectURL(raw string) string {
	u, err := neturl.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query().Get("uddg")
	if q == "" {
		return raw
	}
	decoded, err := neturl.QueryUnescape(q)
	if err != nil {
		return q
	}
	return decoded
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// FallbackSearchBackend implements fallback logic
type FallbackSearchBackend struct {
	primary  SearchBackend
	fallback SearchBackend
	logger   *zap.SugaredLogger
}

func NewFallbackSearchBackend(primary, fallback SearchBackend) *FallbackSearchBackend {
	return &FallbackSearchBackend{
		primary:  primary,
		fallback: fallback,
		logger:   logging.GetLogger(),
	}
}

func (f *FallbackSearchBackend) Search(ctx context.Context, query string, opts SearchOptions) (*BackendSearchResponse, error) {
	result, err := f.primary.Search(ctx, query, opts)
	if err != nil {
		f.logger.Warnw("primary_backend_failed", "error", err, "falling_back", true)
		return f.fallback.Search(ctx, query, opts)
	}
	return result, nil
}

// CreateSearchBackend creates the appropriate search backend based on config
func CreateSearchBackend(cfg *config.AppConfig, httpClient *http.Client) SearchBackend {
	backendName := strings.ToLower(cfg.Search.Backend)

	var primary SearchBackend
	switch backendName {
	case "searxng":
		primary = NewSearXNGBackend(cfg, httpClient)
	case "duckduckgo":
		primary = NewDuckDuckGoBackend(cfg, httpClient)
	default:
		panic(fmt.Sprintf("Unknown search backend: %s. Supported: searxng, duckduckgo", backendName))
	}

	if cfg.Resilience.BackendFallback && backendName == "searxng" {
		fallback := NewDuckDuckGoBackend(cfg, httpClient)
		return NewFallbackSearchBackend(primary, fallback)
	}

	return primary
}
