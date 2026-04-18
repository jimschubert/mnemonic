package embed

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alecthomas/assert/v2"
	"github.com/jimschubert/mnemonic/internal/config"
)

func TestNoopEmbedder_Embed(t *testing.T) {
	e := &NoopEmbedder{}
	result, err := e.Embed([]string{"test"})
	assert.NoError(t, err)
	assert.Zero(t, result)
}

func TestNoopEmbedder_EmbedSingle(t *testing.T) {
	e := &NoopEmbedder{}
	result, err := e.EmbedSingle("test")
	assert.NoError(t, err)
	assert.Zero(t, result)
}

func TestNoopEmbedder_Available(t *testing.T) {
	e := &NoopEmbedder{}
	assert.Equal(t, false, e.Available())
}

func TestHttpEmbedder_Available(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		model    string
		expected bool
	}{
		{
			name:     "both endpoint and model present",
			endpoint: "http://localhost:1234/v1/embeddings",
			model:    "test-model",
			expected: true,
		},
		{
			name:     "only endpoint present",
			endpoint: "http://localhost:1234/v1/embeddings",
			model:    "",
			expected: false,
		},
		{
			name:     "only model present",
			endpoint: "",
			model:    "test-model",
			expected: false,
		},
		{
			name:     "neither present",
			endpoint: "",
			model:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := config.Config{
				Embeddings: config.Embeddings{
					Endpoint: tt.endpoint,
					Model:    tt.model,
				},
			}
			e := New(conf)
			assert.Equal(t, tt.expected, e.Available())
		})
	}
}

func TestHttpEmbedder_New(t *testing.T) {
	conf := config.Config{
		ClientTimeoutSec: 10,
		Embeddings: config.Embeddings{
			Endpoint: "http://localhost:1234/v1/embeddings",
			Model:    "test-model",
		},
	}
	e := New(conf)

	assert.NotZero(t, e)
	assert.NotZero(t, e.client)
	assert.Equal(t, conf.Embeddings, e.config.Embeddings)
}

func TestHttpEmbedder_Embed(t *testing.T) {
	tests := []struct {
		name          string
		inputs        []string
		serverHandler func(w http.ResponseWriter, r *http.Request)
		wantErr       bool
		errMsg        string
		validateResp  func(t *testing.T, resp [][]float32)
	}{
		{
			name:   "successful embedding of multiple texts",
			inputs: []string{"hello", "world"},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := response{
					Data: []struct {
						Embedding []float64 `json:"embedding"`
					}{
						{Embedding: []float64{0.1, 0.2, 0.3}},
						{Embedding: []float64{0.4, 0.5, 0.6}},
					},
				}
				_ = json.NewEncoder(w).Encode(resp) // nolint:errcheck
			},
			wantErr: false,
			validateResp: func(t *testing.T, resp [][]float32) {
				assert.Equal(t, 2, len(resp))
				assert.Equal(t, []float32{0.1, 0.2, 0.3}, resp[0])
				assert.Equal(t, []float32{0.4, 0.5, 0.6}, resp[1])
			},
		},
		{
			name:   "server returns 500",
			inputs: []string{"test"},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr: true,
			errMsg:  "embedding endpoint returned 500",
		},
		{
			name:   "server returns invalid JSON",
			inputs: []string{"test"},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte("invalid json"))
			},
			wantErr: true,
			errMsg:  "decoding embedding response",
		},
		{
			name:   "server returns empty embeddings",
			inputs: []string{"test"},
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := response{
					Data: []struct {
						Embedding []float64 `json:"embedding"`
					}{},
				}
				_ = json.NewEncoder(w).Encode(resp) // nolint:errcheck
			},
			wantErr: true,
			errMsg:  "embedding response missing data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverHandler))
			defer server.Close()

			conf := config.Config{
				ClientTimeoutSec: 5,
				Embeddings: config.Embeddings{
					Endpoint:      server.URL,
					Model:         "test-model",
					SkipPreflight: true,
				},
				Index: config.Index{
					Dimensions: 3,
				},
			}

			e := New(conf)
			resp, err := e.Embed(tt.inputs)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
				assert.NotZero(t, resp)
			}
		})
	}
}

func TestHttpEmbedder_EmbedSingle(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		serverHandler func(w http.ResponseWriter, r *http.Request)
		wantErr       bool
		errMsg        string
		validateResp  func(t *testing.T, resp []float32)
	}{
		{
			name:  "successful single embedding",
			input: "hello world",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := response{
					Data: []struct {
						Embedding []float64 `json:"embedding"`
					}{
						{Embedding: []float64{0.1, 0.2, 0.3}},
					},
				}
				_ = json.NewEncoder(w).Encode(resp) // nolint:errcheck
			},
			wantErr: false,
			validateResp: func(t *testing.T, resp []float32) {
				assert.Equal(t, []float32{0.1, 0.2, 0.3}, resp)
			},
		},
		{
			name:  "server returns 400",
			input: "test",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
			},
			wantErr: true,
			errMsg:  "embedding endpoint returned 400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverHandler))
			defer server.Close()

			conf := config.Config{
				ClientTimeoutSec: 5,
				Embeddings: config.Embeddings{
					Endpoint:      server.URL,
					Model:         "test-model",
					SkipPreflight: true,
				},
				Index: config.Index{
					Dimensions: 3,
				},
			}

			e := New(conf)
			resp, err := e.EmbedSingle(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
				tt.validateResp(t, resp)
			}
		})
	}
}

func TestHttpEmbedder_DoRequest(t *testing.T) {
	tests := []struct {
		name          string
		texts         []string
		model         string
		serverHandler func(w http.ResponseWriter, r *http.Request)
		wantErr       bool
		errMsg        string
		validateReq   func(t *testing.T, body []byte)
	}{
		{
			name:  "request sends correct model and input",
			texts: []string{"hello", "world"},
			model: "my-model",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				defer r.Body.Close() // nolint:errcheck

				var req request
				_ = json.Unmarshal(body, &req) // nolint:errcheck
				assert.Equal(t, "my-model", req.Model)
				assert.Equal(t, []string{"hello", "world"}, req.Input)

				w.Header().Set("Content-Type", "application/json")
				resp := response{
					Data: []struct {
						Embedding []float64 `json:"embedding"`
					}{
						{Embedding: []float64{0.1}},
						{Embedding: []float64{0.2}},
					},
				}
				_ = json.NewEncoder(w).Encode(resp) // nolint:errcheck
			},
			wantErr: false,
		},
		{
			name:  "connection error",
			texts: []string{"test"},
			model: "test",
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				// simulate connection error by returning 5xx
				w.WriteHeader(502)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverHandler))
			defer server.Close()

			conf := config.Config{
				ClientTimeoutSec: 5,
				Embeddings: config.Embeddings{
					Endpoint: func() string {
						if server != nil {
							return server.URL
						}
						return "http://localhost:9999"
					}(),
					Model: tt.model,
				},
			}

			e := New(conf)
			resp, err := e.doRequest(tt.texts...)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotZero(t, resp)
			}
		})
	}
}

func TestHttpEmbedder_Preflight(t *testing.T) {
	tests := []struct {
		name             string
		skipPreflight    bool
		dimensions       int
		serverDimensions int
		serverHandler    func(w http.ResponseWriter, r *http.Request)
		wantErr          bool
		errMsg           string
	}{
		{
			name:             "successful preflight check",
			skipPreflight:    false,
			dimensions:       3,
			serverDimensions: 3,
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := response{
					Data: []struct {
						Embedding []float64 `json:"embedding"`
					}{
						{Embedding: []float64{0.1, 0.2, 0.3}},
					},
				}
				_ = json.NewEncoder(w).Encode(resp) // nolint:errcheck
			},
			wantErr: false,
		},
		{
			name:             "dimension mismatch",
			skipPreflight:    false,
			dimensions:       3,
			serverDimensions: 5,
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				resp := response{
					Data: []struct {
						Embedding []float64 `json:"embedding"`
					}{
						{Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5}},
					},
				}
				_ = json.NewEncoder(w).Encode(resp) // nolint:errcheck
			},
			wantErr: true,
			errMsg:  "embedding dimensions mismatch",
		},
		{
			name:          "skip preflight",
			skipPreflight: true,
			wantErr:       false,
		},
		{
			name:             "preflight request fails",
			skipPreflight:    false,
			dimensions:       3,
			serverDimensions: 3,
			serverHandler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusUnauthorized)
			},
			wantErr: true,
			errMsg:  "embedding preflight request failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.serverHandler != nil {
				server = httptest.NewServer(http.HandlerFunc(tt.serverHandler))
				defer server.Close()
			}

			conf := config.Config{
				ClientTimeoutSec: 5,
				Embeddings: config.Embeddings{
					Endpoint: func() string {
						if server != nil {
							return server.URL
						}
						return "http://localhost:9999"
					}(),
					Model:         "test-model",
					SkipPreflight: tt.skipPreflight,
				},
				Index: config.Index{
					Dimensions: tt.dimensions,
				},
			}

			e := New(conf)
			err := e.preflight()

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}

			// call again to test that it only runs once
			if !tt.wantErr {
				err2 := e.preflight()
				assert.NoError(t, err2)
			}
		})
	}
}

func TestHttpEmbedder_PreflightOnlyRunsOnce(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		resp := response{
			Data: []struct {
				Embedding []float64 `json:"embedding"`
			}{
				{Embedding: []float64{0.1, 0.2, 0.3}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp) // nolint:errcheck
	}))
	defer server.Close()

	conf := config.Config{
		ClientTimeoutSec: 5,
		Embeddings: config.Embeddings{
			Endpoint:      server.URL,
			Model:         "test-model",
			SkipPreflight: false,
		},
		Index: config.Index{
			Dimensions: 3,
		},
	}

	e := New(conf)

	err1 := e.preflight()
	assert.NoError(t, err1)

	err2 := e.preflight()
	assert.NoError(t, err2)

	assert.Equal(t, 1, callCount, "preflight should only be invoked once, despite number of calls")
}

func TestHttpEmbedder_RequestJSONEncoding(t *testing.T) {
	tests := []struct {
		name     string
		texts    []string
		model    string
		expected request
	}{
		{
			name:  "single text",
			texts: []string{"hello"},
			model: "model-1",
			expected: request{
				Model: "model-1",
				Input: []string{"hello"},
			},
		},
		{
			name:  "multiple texts",
			texts: []string{"hello", "world", "test"},
			model: "model-2",
			expected: request{
				Model: "model-2",
				Input: []string{"hello", "world", "test"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				defer r.Body.Close() // nolint:errcheck

				var req request
				err := json.Unmarshal(body, &req)
				assert.NoError(t, err)
				assert.Equal(t, tt.expected.Model, req.Model)
				assert.Equal(t, tt.expected.Input, req.Input)

				w.Header().Set("Content-Type", "application/json")
				resp := response{
					Data: []struct {
						Embedding []float64 `json:"embedding"`
					}{
						{Embedding: []float64{0.1}},
					},
				}
				_ = json.NewEncoder(w).Encode(resp) // nolint:errcheck
			}))
			defer server.Close()

			conf := config.Config{
				ClientTimeoutSec: 5,
				Embeddings: config.Embeddings{
					Endpoint:      server.URL,
					Model:         tt.model,
					SkipPreflight: true,
				},
				Index: config.Index{
					Dimensions: 1,
				},
			}

			e := New(conf)
			_, err := e.doRequest(tt.texts...)
			assert.NoError(t, err)
		})
	}
}
