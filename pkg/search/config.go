package search

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EmbeddingConfigFromEnv reads semantic embedding configuration from environment variables.
//
// Supported variables:
//   - BV_SEMANTIC_EMBEDDER: embedding provider (default: "hash")
//   - BV_SEMANTIC_MODEL: model identifier (provider-specific, optional)
//   - BV_SEMANTIC_DIM: embedding dimension (default: DefaultEmbeddingDim)
func EmbeddingConfigFromEnv() EmbeddingConfig {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv(EnvSemanticEmbedder)))
	cfg := EmbeddingConfig{
		Provider: Provider(provider),
		Model:    strings.TrimSpace(os.Getenv(EnvSemanticModel)),
	}
	if dimStr := os.Getenv(EnvSemanticDim); dimStr != "" {
		if dim, err := strconv.Atoi(dimStr); err == nil {
			cfg.Dim = dim
		}
	}
	if cfg.Provider == "" {
		cfg.Provider = ProviderHash
	}
	return cfg.Normalized()
}

// NewEmbedderFromConfig constructs an Embedder for the given configuration.
func NewEmbedderFromConfig(cfg EmbeddingConfig) (Embedder, error) {
	cfg = cfg.Normalized()
	switch cfg.Provider {
	case "", ProviderHash:
		return NewHashEmbedder(cfg.Dim), nil
	case ProviderPythonSentenceTransformers:
		return nil, fmt.Errorf("semantic embedder %q not implemented (mvp placeholder); set %s=%q for deterministic fallback", cfg.Provider, EnvSemanticEmbedder, ProviderHash)
	case ProviderOpenAI:
		return nil, fmt.Errorf("semantic embedder %q not implemented (placeholder); set %s=%q for deterministic fallback", cfg.Provider, EnvSemanticEmbedder, ProviderHash)
	default:
		return nil, fmt.Errorf("unknown semantic embedder %q; expected %q", cfg.Provider, ProviderHash)
	}
}
