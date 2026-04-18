package embed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/jimschubert/mnemonic/internal/config"
)

// Embedder defines the interface for generating vector embeddings from text.
type Embedder interface {
	// Embed takes a slice of strings and returns a corresponding slice of float32 vectors.
	Embed([]string) ([][]float32, error)
	// EmbedSingle is a convenience method for embedding a single string.
	EmbedSingle(string) ([]float32, error)
	// Available indicates whether the embedder is properly configured and can be used.
	Available() bool
}

type NoopEmbedder struct{}

func (e *NoopEmbedder) Embed([]string) ([][]float32, error) {
	return nil, nil
}

func (e *NoopEmbedder) EmbedSingle(string) ([]float32, error) {
	return nil, nil
}

func (e *NoopEmbedder) Available() bool {
	return false
}

type request struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}
type response struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Model string `json:"model"`
}

// HttpEmbedder implements the Embedder interface by making HTTP requests to an embedding service.
type HttpEmbedder struct {
	config          config.Config
	client          *http.Client
	oncePreflight   sync.Once
	preflightPassed bool
}

// New creates a new HttpEmbedder with the given configuration.
func New(conf config.Config) *HttpEmbedder {
	return &HttpEmbedder{
		config: conf,
		client: &http.Client{Timeout: time.Duration(conf.ClientTimeout()) * time.Second},
	}
}

func (e *HttpEmbedder) Embed(text []string) ([][]float32, error) {
	if err := e.preflight(); err != nil {
		return nil, err
	}

	resp, err := e.doRequest(text...)
	if err != nil {
		return nil, err
	}

	embeddings := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		embeddings[i] = mapToFloat32(d.Embedding)
	}
	return embeddings, nil
}

func (e *HttpEmbedder) EmbedSingle(text string) ([]float32, error) {
	if err := e.preflight(); err != nil {
		return nil, err
	}

	resp, err := e.doRequest(text)
	if err != nil {
		return nil, err
	}
	return mapToFloat32(resp.Data[0].Embedding), nil
}

func (e *HttpEmbedder) Available() bool {
	return e.config.Embeddings.Endpoint != "" && e.config.Embeddings.Model != ""
}

func (e *HttpEmbedder) doRequest(text ...string) (*response, error) {
	r := request{
		Model: e.config.Embeddings.Model,
		Input: text,
	}
	body, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	resp, err := e.client.Post(e.config.Embeddings.Endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() // nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding endpoint returned %d", resp.StatusCode)
	}

	var result response
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding embedding response: %w", err)
	}
	if len(result.Data) == 0 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("embedding response missing data")
	}

	return &result, nil
}

func (e *HttpEmbedder) preflight() error {
	if e.config.Embeddings.SkipPreflight || e.preflightPassed {
		return nil
	}

	var maybeErr error
	e.oncePreflight.Do(func() {
		resp, err := e.doRequest("test")
		if err != nil {
			maybeErr = fmt.Errorf("embedding preflight request failed: %w", err)
		} else {
			dimensions := len(resp.Data[0].Embedding)
			if dimensions != e.config.Index.Dimensions {
				maybeErr = fmt.Errorf("embedding dimensions mismatch: got %d, expected %d", dimensions, e.config.Index.Dimensions)
			}
		}
		if maybeErr == nil {
			e.preflightPassed = true
		}
	})
	return maybeErr
}

func mapToFloat32(input []float64) []float32 {
	output := make([]float32, len(input))
	for i, v := range input {
		output[i] = float32(v)
	}
	return output
}
