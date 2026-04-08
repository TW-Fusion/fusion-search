package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type ServerConfig struct {
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
}

type SearchConfig struct {
	Backend      string `yaml:"backend" json:"backend"`
	SearxngURL   string `yaml:"searxng_url" json:"searxng_url"`
	DDGSURL      string `yaml:"ddgs_url" json:"ddgs_url"`
	GoogleAPIKey string `yaml:"google_api_key" json:"google_api_key"`
	GoogleCX     string `yaml:"google_cx" json:"google_cx"`
}

type ExtractionConfig struct {
	MaxConcurrent          int      `yaml:"max_concurrent" json:"max_concurrent"`
	Timeout                int      `yaml:"timeout" json:"timeout"`
	MaxContentLength       int      `yaml:"max_content_length" json:"max_content_length"`
	DomainConcurrency      int      `yaml:"domain_concurrency" json:"domain_concurrency"`
	DomainSemaphoreMaxSize int      `yaml:"domain_semaphore_max_size" json:"domain_semaphore_max_size"`
	UserAgents             []string `yaml:"user_agents" json:"user_agents"`
}

type CacheConfig struct {
	Enabled    bool   `yaml:"enabled" json:"enabled"`
	RedisURL   string `yaml:"redis_url" json:"redis_url"`
	SearchTTL  int    `yaml:"search_ttl" json:"search_ttl"`
	ExtractTTL int    `yaml:"extract_ttl" json:"extract_ttl"`
}

type ProxyConfig struct {
	Enabled bool   `yaml:"enabled" json:"enabled"`
	URL     string `yaml:"url" json:"url"`
}

type AuthConfig struct {
	Enabled bool     `yaml:"enabled" json:"enabled"`
	APIKeys []string `yaml:"api_keys" json:"api_keys"`
}

type RateLimitConfig struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	DefaultRate string `yaml:"default_rate" json:"default_rate"`
	SearchRate  string `yaml:"search_rate" json:"search_rate"`
	ExtractRate string `yaml:"extract_rate" json:"extract_rate"`
}

type RerankConfig struct {
	Enabled        bool   `yaml:"enabled" json:"enabled"`
	Model          string `yaml:"model" json:"model"`
	TopK           int    `yaml:"top_k" json:"top_k"`
	OnnxModelPath  string `yaml:"onnx_model_path" json:"onnx_model_path"`
	TokenizerPath  string `yaml:"tokenizer_path" json:"tokenizer_path"`
	ORTLibraryPath string `yaml:"ort_library_path" json:"ort_library_path"`
	MaxLength      int    `yaml:"max_length" json:"max_length"`
}

type ResilienceConfig struct {
	CircuitBreakerFailureThreshold int     `yaml:"circuit_breaker_failure_threshold" json:"circuit_breaker_failure_threshold"`
	CircuitBreakerRecoveryTimeout  int     `yaml:"circuit_breaker_recovery_timeout" json:"circuit_breaker_recovery_timeout"`
	RetryMaxAttempts               int     `yaml:"retry_max_attempts" json:"retry_max_attempts"`
	RetryBackoffBase               float64 `yaml:"retry_backoff_base" json:"retry_backoff_base"`
	RetryOnStatusCodes             []int   `yaml:"retry_on_status_codes" json:"retry_on_status_codes"`
	RequestTimeout                 int     `yaml:"request_timeout" json:"request_timeout"`
	BackendFallback                bool    `yaml:"backend_fallback" json:"backend_fallback"`
}

type LoggingConfig struct {
	Format string `yaml:"format" json:"format"`
	Level  string `yaml:"level" json:"level"`
}

type CorsConfig struct {
	AllowOrigins []string `yaml:"allow_origins" json:"allow_origins"`
}

type LLMConfig struct {
	Enabled           bool    `yaml:"enabled" json:"enabled"`
	Provider          string  `yaml:"provider" json:"provider"`
	BaseURL           string  `yaml:"base_url" json:"base_url"`
	APIKey            string  `yaml:"api_key" json:"api_key"`
	Model             string  `yaml:"model" json:"model"`
	MaxTokens         int     `yaml:"max_tokens" json:"max_tokens"`
	Temperature       float32 `yaml:"temperature" json:"temperature"`
	Timeout           int     `yaml:"timeout" json:"timeout"`
	SystemPrompt      string  `yaml:"system_prompt" json:"system_prompt"`
	MaxContextResults int     `yaml:"max_context_results" json:"max_context_results"`
	MaxContextChars   int     `yaml:"max_context_chars" json:"max_context_chars"`
	AnswerTTL         int     `yaml:"answer_ttl" json:"answer_ttl"`
}

type AppConfig struct {
	Server     ServerConfig     `yaml:"server" json:"server"`
	Search     SearchConfig     `yaml:"search" json:"search"`
	Extraction ExtractionConfig `yaml:"extraction" json:"extraction"`
	Cache      CacheConfig      `yaml:"cache" json:"cache"`
	Proxy      ProxyConfig      `yaml:"proxy" json:"proxy"`
	Auth       AuthConfig       `yaml:"auth" json:"auth"`
	RateLimit  RateLimitConfig  `yaml:"rate_limit" json:"rate_limit"`
	Rerank     RerankConfig     `yaml:"rerank" json:"rerank"`
	Resilience ResilienceConfig `yaml:"resilience" json:"resilience"`
	Logging    LoggingConfig    `yaml:"logging" json:"logging"`
	Cors       CorsConfig       `yaml:"cors" json:"cors"`
	LLM        LLMConfig        `yaml:"llm" json:"llm"`
}

func LoadConfig(configPath string) (*AppConfig, error) {
	if configPath == "" {
		if envPath := os.Getenv("FUSION_SEARCH_CONFIG"); envPath != "" {
			configPath = envPath
		} else {
			configPath = "config.yaml"
		}
	}

	// Try to resolve relative to current file location
	if !filepath.IsAbs(configPath) {
		if cwd, err := os.Getwd(); err == nil {
			configPath = filepath.Join(cwd, configPath)
		}
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		// Return default config if file doesn't exist
		return DefaultConfig(), nil
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Set defaults for zero values
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8000
	}
	if cfg.Search.Backend == "" {
		cfg.Search.Backend = "searxng"
	}
	if cfg.Search.SearxngURL == "" {
		cfg.Search.SearxngURL = "http://searxng:8080"
	}
	if cfg.Search.DDGSURL == "" {
		cfg.Search.DDGSURL = "http://ddgs:9000"
	}
	if cfg.Extraction.MaxConcurrent == 0 {
		cfg.Extraction.MaxConcurrent = 5
	}
	if cfg.Extraction.Timeout == 0 {
		cfg.Extraction.Timeout = 10
	}
	if cfg.Extraction.MaxContentLength == 0 {
		cfg.Extraction.MaxContentLength = 50000
	}
	if cfg.Extraction.DomainConcurrency == 0 {
		cfg.Extraction.DomainConcurrency = 2
	}
	if cfg.Extraction.DomainSemaphoreMaxSize == 0 {
		cfg.Extraction.DomainSemaphoreMaxSize = 1000
	}
	if len(cfg.Extraction.UserAgents) == 0 {
		cfg.Extraction.UserAgents = defaultUserAgents()
	}
	if cfg.Cache.SearchTTL == 0 {
		cfg.Cache.SearchTTL = 3600
	}
	if cfg.Cache.ExtractTTL == 0 {
		cfg.Cache.ExtractTTL = 86400
	}
	if cfg.Resilience.CircuitBreakerFailureThreshold == 0 {
		cfg.Resilience.CircuitBreakerFailureThreshold = 5
	}
	if cfg.Resilience.CircuitBreakerRecoveryTimeout == 0 {
		cfg.Resilience.CircuitBreakerRecoveryTimeout = 30
	}
	if cfg.Resilience.RetryMaxAttempts == 0 {
		cfg.Resilience.RetryMaxAttempts = 3
	}
	if cfg.Resilience.RetryBackoffBase == 0 {
		cfg.Resilience.RetryBackoffBase = 0.5
	}
	if len(cfg.Resilience.RetryOnStatusCodes) == 0 {
		cfg.Resilience.RetryOnStatusCodes = []int{429, 503, 502, 504}
	}
	if cfg.Resilience.RequestTimeout == 0 {
		cfg.Resilience.RequestTimeout = 30
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = "json"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "INFO"
	}
	if cfg.LLM.MaxTokens == 0 {
		cfg.LLM.MaxTokens = 1024
	}
	if cfg.LLM.Temperature == 0 {
		cfg.LLM.Temperature = 0.1
	}
	if cfg.LLM.MaxContextResults == 0 {
		cfg.LLM.MaxContextResults = 5
	}
	if cfg.LLM.MaxContextChars == 0 {
		cfg.LLM.MaxContextChars = 8000
	}
	if cfg.LLM.AnswerTTL == 0 {
		cfg.LLM.AnswerTTL = 3600
	}
	if cfg.LLM.SystemPrompt == "" {
		cfg.LLM.SystemPrompt = "You are a helpful search assistant. Answer the user's question concisely based on the provided search results. Cite sources by number [1], [2], etc."
	}
	if cfg.Rerank.MaxLength == 0 {
		cfg.Rerank.MaxLength = 512
	}

	return &cfg, nil
}

func DefaultConfig() *AppConfig {
	cfg := &AppConfig{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8000,
		},
		Search: SearchConfig{
			Backend:    "searxng",
			SearxngURL: "http://searxng:8080",
			DDGSURL:    "http://ddgs:9000",
		},
		Extraction: ExtractionConfig{
			MaxConcurrent:          5,
			Timeout:                10,
			MaxContentLength:       50000,
			DomainConcurrency:      2,
			DomainSemaphoreMaxSize: 1000,
			UserAgents:             defaultUserAgents(),
		},
		Cache: CacheConfig{
			Enabled:    true,
			RedisURL:   "redis://localhost:6379",
			SearchTTL:  3600,
			ExtractTTL: 86400,
		},
		Auth: AuthConfig{
			Enabled: false,
		},
		RateLimit: RateLimitConfig{
			Enabled:     false,
			DefaultRate: "60/minute",
			SearchRate:  "30/minute",
			ExtractRate: "30/minute",
		},
		Rerank: RerankConfig{
			Enabled:        false,
			Model:          "ms-marco-MiniLM-L-12-v2",
			TopK:           5,
			OnnxModelPath:  "",
			TokenizerPath:  "",
			ORTLibraryPath: "",
			MaxLength:      512,
		},
		Resilience: ResilienceConfig{
			CircuitBreakerFailureThreshold: 5,
			CircuitBreakerRecoveryTimeout:  30,
			RetryMaxAttempts:               3,
			RetryBackoffBase:               0.5,
			RetryOnStatusCodes:             []int{429, 503, 502, 504},
			RequestTimeout:                 30,
			BackendFallback:                true,
		},
		Logging: LoggingConfig{
			Format: "json",
			Level:  "INFO",
		},
		Cors: CorsConfig{
			AllowOrigins: []string{"*"},
		},
		LLM: LLMConfig{
			Enabled:           false,
			Provider:          "ollama",
			BaseURL:           "http://ollama:11434/v1",
			APIKey:            "ollama",
			Model:             "llama3.1",
			MaxTokens:         1024,
			Temperature:       0.1,
			Timeout:           30,
			SystemPrompt:      "You are a helpful search assistant. Answer the user's question concisely based on the provided search results. Cite sources by number [1], [2], etc.",
			MaxContextResults: 5,
			MaxContextChars:   8000,
			AnswerTTL:         3600,
		},
	}
	return cfg
}

func defaultUserAgents() []string {
	return []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.2 Safari/605.1.15",
	}
}
