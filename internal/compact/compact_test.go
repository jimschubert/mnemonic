package compact

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alecthomas/assert/v2"
)

type testEmbedder struct {
	vectors map[string][]float32
	err     error
}

func (e *testEmbedder) EmbedSingle(text string) ([]float32, error) {
	if e.err != nil {
		return nil, e.err
	}
	if vector, ok := e.vectors[text]; ok {
		return vector, nil
	}
	return []float32{0, 0}, nil
}

func TestCompacterCompact_UsesChatCompletionsRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		cavemanMode         CavemanMode
		input               string
		responseBody        any
		expectedSystem      string
		expectedUserContent string
		expectedOutput      string
	}{
		{
			name:        "sends system and user messages",
			cavemanMode: CavemanOff,
			input:       "original memory",
			responseBody: map[string]any{
				"choices": []any{
					map[string]any{
						"index": 0, "finish_reason": "stop",
						"message": map[string]any{"role": "assistant", "content": "compacted memory"},
					},
				},
			},
			expectedSystem:      "Please shorten the following text to reduce token usage, preserving all key information and details and making zero modification to any code blocks:\n\n",
			expectedUserContent: "Text to compact:\noriginal memory",
			expectedOutput:      "compacted memory",
		},
		{
			name:        "supports caveman prompt as system message",
			cavemanMode: CavemanLite,
			input:       "original memory",
			responseBody: map[string]any{
				"choices": []any{
					map[string]any{
						"index": 0, "finish_reason": "stop",
						"message": map[string]any{"role": "assistant", "content": "compacted memory"},
					},
				},
			},
			expectedSystem:      New(nil, "", "", "", WithCavemanMode(CavemanLite)).getPrompt(),
			expectedUserContent: "Text to compact:\noriginal memory",
			expectedOutput:      "compacted memory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotRequest request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/chat/completions", r.URL.Path)

				body, err := io.ReadAll(r.Body)
				assert.NoError(t, err)
				assert.NoError(t, json.Unmarshal(body, &gotRequest))

				w.Header().Set("Content-Type", "application/json")
				assert.NoError(t, json.NewEncoder(w).Encode(tt.responseBody))
			}))
			defer server.Close()

			compacter := New(&testEmbedder{err: io.EOF}, server.URL, "test-key", "test-model", WithCavemanMode(tt.cavemanMode))
			result, err := compacter.Compact(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedOutput, result)
			assert.Equal(t, "test-model", gotRequest.Model)
			assert.Equal(t, 2, len(gotRequest.Messages))
			assert.Equal(t, tt.expectedSystem, gotRequest.Messages[0].Content)
			assert.Equal(t, "system", gotRequest.Messages[0].Role)
			assert.Equal(t, tt.expectedUserContent, gotRequest.Messages[1].Content)
			assert.Equal(t, "user", gotRequest.Messages[1].Role)
		})
	}
}

func TestCompacterCompact_ChoosesMostSimilarChatChoice(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"index": 0, "finish_reason": "stop",
					"message": map[string]any{"role": "assistant", "content": "less similar"},
				},
				map[string]any{
					"index": 1, "finish_reason": "stop",
					"message": map[string]any{"role": "assistant", "content": "most similar"},
				},
			},
		}))
	}))
	defer server.Close()

	compacter := New(&testEmbedder{
		vectors: map[string][]float32{
			"original":     {1, 0},
			"less similar": {0, 1},
			"most similar": {1, 0},
		},
	}, server.URL, "test-key", "test-model")

	result, err := compacter.Compact("original")
	assert.NoError(t, err)
	assert.Equal(t, "most similar", result)
}

func TestCompacterCompact_ParsesChatMessageContentForms(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		responseBody   any
		expectedOutput string
	}{
		{
			name: "content string",
			responseBody: map[string]any{
				"choices": []any{
					map[string]any{
						"index": 0, "finish_reason": "stop",
						"message": map[string]any{"role": "assistant", "content": "string content"},
					},
				},
			},
			expectedOutput: "string content",
		},
		{
			name: "content part array",
			responseBody: map[string]any{
				"choices": []any{
					map[string]any{
						"index": 0, "finish_reason": "stop", "message": map[string]any{
							"role": "assistant", "content": []any{
								map[string]any{"type": "text", "text": "part one"},
								map[string]any{"type": "text", "text": " + part two"},
							},
						},
					},
				},
			},
			expectedOutput: "part one + part two",
		},
		{
			name: "legacy text fallback",
			responseBody: map[string]any{
				"choices": []any{
					map[string]any{"index": 0, "finish_reason": "stop", "text": "legacy completion text"},
				},
			},
			expectedOutput: "legacy completion text",
		},
		{
			name: "empty choices returns original input",
			responseBody: map[string]any{
				"choices": []any{},
			},
			expectedOutput: "original memory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				assert.NoError(t, json.NewEncoder(w).Encode(tt.responseBody))
			}))
			defer server.Close()

			compacter := New(&testEmbedder{err: io.EOF}, server.URL, "test-key", "test-model")
			result, err := compacter.Compact("original memory")
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedOutput, result)
		})
	}
}
