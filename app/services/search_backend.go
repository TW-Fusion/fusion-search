package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/logging"

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
}

func NewSearXNGBackend(cfg *config.AppConfig, httpClient *http.Client) *SearXNGBackend {
	return &SearXNGBackend{
		baseURL:    cfg.Search.SearxngURL,
		httpClient: httpClient,
		logger:     logging.GetLogger(),
		cfg:        cfg,
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

	// Perform web search
	results, err := s.webSearch(ctx, effectiveQuery, opts.MaxResults, categories, opts.TimeRange)
	if err != nil {
		return nil, err
	}

	images := []RawImageResult{}
	if opts.IncludeImages {
		imgResults, err := s.imageSearch(ctx, query, 5)
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
		return nil, fmt.Errorf("search request failed with status: %d", resp.StatusCode)
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
		return nil, fmt.Errorf("image search request failed with status: %d", resp.StatusCode)
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
	logger *zap.SugaredLogger
}

func NewDuckDuckGoBackend() *DuckDuckGoBackend {
	return &DuckDuckGoBackend{
		logger: logging.GetLogger(),
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

	results, err := d.webSearch(ctx, effectiveQuery, opts.MaxResults, opts.TimeRange)
	if err != nil {
		return nil, err
	}

	images := []RawImageResult{}
	if opts.IncludeImages {
		imgResults, err := d.imageSearch(ctx, query, 5)
		if err != nil {
			d.logger.Warnw("image_search_failed", "error", err)
		} else {
			images = imgResults
		}
	}

	return &BackendSearchResponse{
		Results: results,
		Images:  images,
	}, nil
}

func (d *DuckDuckGoBackend) webSearch(ctx context.Context, query string, maxResults int, timeRange *string) ([]RawSearchResult, error) {
	// DuckDuckGo search is not directly available in Go without external libraries
	// This is a placeholder - in production, you'd use a proper DDGS library or API
	d.logger.Warn("duckduckgo_backend_not_fully_implemented")
	return []RawSearchResult{}, nil
}

func (d *DuckDuckGoBackend) imageSearch(ctx context.Context, query string, maxResults int) ([]RawImageResult, error) {
	// Placeholder implementation
	return []RawImageResult{}, nil
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
		primary = NewDuckDuckGoBackend()
	default:
		panic(fmt.Sprintf("Unknown search backend: %s. Supported: searxng, duckduckgo", backendName))
	}

	if cfg.Resilience.BackendFallback && backendName == "searxng" {
		fallback := NewDuckDuckGoBackend()
		return NewFallbackSearchBackend(primary, fallback)
	}

	return primary
}
