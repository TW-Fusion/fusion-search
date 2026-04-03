package models

// SearchDepth represents the depth of search
// @Enum basic advanced
type SearchDepth string

const (
	SearchDepthBasic    SearchDepth = "basic"
	SearchDepthAdvanced SearchDepth = "advanced"
)

// Topic represents the search category
// @Enum general news
type Topic string

const (
	TopicGeneral Topic = "general"
	TopicNews    Topic = "news"
)

// TimeRange represents the time filter for search results
// @Enum day week month year
type TimeRange string

const (
	TimeRangeDay   TimeRange = "day"
	TimeRangeWeek  TimeRange = "week"
	TimeRangeMonth TimeRange = "month"
	TimeRangeYear  TimeRange = "year"
)

// ContentFormat represents the output format for extraction
// @Enum markdown text
type ContentFormat string

const (
	ContentFormatMarkdown ContentFormat = "markdown"
	ContentFormatText     ContentFormat = "text"
)

// SearchRequest represents the request body for search endpoint
// swagger:model SearchRequest
type SearchRequest struct {
	// The search query
	// required: true
	// example: "What is Go programming language?"
	Query string `json:"query" binding:"required"`

	// Search depth level: basic for fast snippets, advanced for full content extraction
	// default: basic
	SearchDepth SearchDepth `json:"search_depth"`

	// Search category
	// default: general
	Topic Topic `json:"topic"`

	// Number of results to return (1-20)
	// minimum: 1
	// maximum: 20
	// default: 5
	MaxResults int `json:"max_results"`

	// Generate an AI answer from search results (requires LLM config)
	// default: false
	IncludeAnswer bool `json:"include_answer"`

	// Include full extracted page content in results
	// default: false
	IncludeRawContent bool `json:"include_raw_content"`

	// Include image search results
	// default: false
	IncludeImages bool `json:"include_images"`

	// Only include results from these domains
	IncludeDomains []string `json:"include_domains"`

	// Exclude results from these domains
	ExcludeDomains []string `json:"exclude_domains"`

	// Filter results by time range
	TimeRange *TimeRange `json:"time_range,omitempty"`
}

// SearchResult represents a single search result
// swagger:model SearchResult
type SearchResult struct {
	// Title of the page
	Title string `json:"title"`

	// URL of the page
	URL string `json:"url"`

	// Short snippet / description
	Content string `json:"content"`

	// Relevance score (0-1)
	Score float64 `json:"score"`

	// Full extracted page content (if requested)
	RawContent *string `json:"raw_content,omitempty"`
}

// ImageResult represents an image search result
// swagger:model ImageResult
type ImageResult struct {
	// Direct image URL
	URL string `json:"url"`

	// Image description or alt text
	Description string `json:"description"`
}

// SearchResponse represents the response from search endpoint
// swagger:model SearchResponse
type SearchResponse struct {
	// The original search query
	Query string `json:"query"`

	// AI-generated answer based on search results (if requested)
	Answer *string `json:"answer,omitempty"`

	// List of search results
	Results []SearchResult `json:"results"`

	// List of image results (if requested)
	Images []ImageResult `json:"images"`

	// Total response time in seconds
	ResponseTime float64 `json:"response_time"`
}

// ExtractRequest represents the request body for extract endpoint
// swagger:model ExtractRequest
type ExtractRequest struct {
	// URLs to extract content from
	// required: true
	// minItems: 1
	// maxItems: 20
	// example: ["https://golang.org/doc/", "https://gin-gonic.com/docs/"]
	URLs []string `json:"urls" binding:"required,min=1,max=20"`

	// Extraction depth
	ExtractDepth string `json:"extract_depth"`

	// Output format: markdown or text
	// default: markdown
	Format ContentFormat `json:"format"`
}

// ExtractResult represents successful extraction result
// swagger:model ExtractResult
type ExtractResult struct {
	// The URL that was extracted
	URL string `json:"url"`

	// Extracted page content
	RawContent string `json:"raw_content"`
}

// FailedResult represents a failed extraction
// swagger:model FailedResult
type FailedResult struct {
	// The URL that failed
	URL string `json:"url"`

	// Error message describing the failure
	Error string `json:"error"`
}

// ExtractResponse represents the response from extract endpoint
// swagger:model ExtractResponse
type ExtractResponse struct {
	// Successfully extracted results
	Results []ExtractResult `json:"results"`

	// URLs that failed to extract
	FailedResults []FailedResult `json:"failed_results"`

	// Total response time in seconds
	ResponseTime float64 `json:"response_time"`
}
