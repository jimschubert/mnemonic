package compact

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"
)

type embedder interface {
	EmbedSingle(string) ([]float32, error)
}

type request struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens"`
	Temperature float64   `json:"temperature"`
	TopP        float64   `json:"top_p,omitempty"`
	N           int       `json:"n,omitempty"`
	Stop        []string  `json:"stop,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messageContent struct {
	Text string
}

func (c *messageContent) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		c.Text = text
		return nil
	}

	type contentPart struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}

	var parts []contentPart
	if err := json.Unmarshal(data, &parts); err == nil {
		var sb strings.Builder
		for _, part := range parts {
			if part.Type == "" || part.Type == "text" {
				sb.WriteString(part.Text)
			}
		}
		c.Text = sb.String()
		return nil
	}

	return fmt.Errorf("unsupported chat message content shape: %s", string(data))
}

type responseMessage struct {
	Role    string         `json:"role"`
	Content messageContent `json:"content"`
}

type choice struct {
	Text         string          `json:"text"`
	Index        int             `json:"index"`
	FinishReason string          `json:"finish_reason"`
	Message      responseMessage `json:"message"`
}

func (c choice) IsZero() bool {
	return c.Content() == "" && c.Index == 0 && c.FinishReason == ""
}

func (c choice) Content() string {
	if c.Message.Content.Text != "" {
		return c.Message.Content.Text
	}
	return c.Text
}

type response struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Choices []choice `json:"choices"`
}

type Compacter struct {
	embedder    embedder
	baseUrl     string
	apiKey      string
	model       string
	client      *http.Client
	logger      *slog.Logger
	cavemanMode CavemanMode
}

func New(embedder embedder, baseUrl string, apiKey string, model string, opts ...Option) *Compacter {
	c := &Compacter{
		embedder: embedder,
		baseUrl:  baseUrl,
		apiKey:   apiKey,
		model:    model,
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	if c.logger == nil {
		c.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return c
}

//goland:noinspection GoUnhandledErrorResult
func (c *Compacter) Compact(input string) (string, error) {
	url := c.baseUrl + "/chat/completions"
	userPrompt := "Text to compact:\n" + input
	req := request{
		Model: c.model,
		Messages: []message{
			{
				Role:    "system",
				Content: c.getPrompt(),
			},
			{
				Role:    "user",
				Content: userPrompt,
			},
		},
		MaxTokens:   2048,
		Temperature: 0.3,
	}

	b, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}
	r, err := http.NewRequest("POST", url, bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf("creating OpenAI-compatible request: %w", err)
	}
	r.Header.Set("Content-Type", "application/json")

	var token string
	if len(c.apiKey) > 0 {
		token = c.apiKey
	} else {
		token = "abc123"
	}
	r.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(r)
	if err != nil {
		return "", fmt.Errorf("making OpenAI-compatible /completions request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("non-2xx response from /chat/completions endpoint: %d: %s", resp.StatusCode, string(respBody))
	}

	var rr response
	if err := json.Unmarshal(respBody, &rr); err != nil {
		return "", fmt.Errorf("unmarshaling OpenAI-compatible /completions response: %w", err)
	}

	if len(rr.Choices) == 0 {
		c.logger.Warn("no choices returned from /chat/completions endpoint, returning original input")
		return input, nil
	}

	var bestResult choice
	var highestSimilarity float32 = -1
	originalEmbedding, err := c.embedder.EmbedSingle(input)
	if err != nil {
		// nbd just log, we'll just take the last choice instead of the most similar
		c.logger.Warn("error embedding original input, skipping similarity comparison and returning last choice", "err", err)
	} else if len(originalEmbedding) > 0 {
		for _, thisChoice := range rr.Choices {
			choiceText := thisChoice.Content()
			embedding, err := c.embedder.EmbedSingle(choiceText)
			if err != nil {
				// weird that originalEmbedding worked and this didn't
				c.logger.Warn("error embedding choice text, skipping this and remaining similarity comparisons", "err", err)
				break
			}
			similarity, err := cosineSimilarity(originalEmbedding, embedding)
			if err != nil {
				// empty, zero-based, or unbalanced vectors indicates the embedding model is faulty
				c.logger.Warn("error calculating similarity, skipping this and remaining comparisons", "err", err)
				break
			}
			if similarity > highestSimilarity {
				highestSimilarity = similarity
				bestResult = thisChoice
			}
		}
	}

	if bestResult.IsZero() && len(rr.Choices) > 0 {
		c.logger.Debug("falling back to last choice since similarity comparison failed or was not functional")
		bestResult = rr.Choices[len(rr.Choices)-1]
	}

	return bestResult.Content(), nil
}

func (c *Compacter) Close() error {
	defer c.client.CloseIdleConnections()
	if closer, ok := c.embedder.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return fmt.Errorf("closing embedder: %w", err)
		}
	}
	return nil
}

func (c *Compacter) getPrompt() string {
	if c.cavemanMode != CavemanOff {
		// Caveman mode taken with slide modifications from Julius Brussee's caveman mode
		// see: https://juliusbrussee.github.io/caveman/
		// License MIT: https://github.com/JuliusBrussee/caveman/blob/main/LICENSE
		sb := &strings.Builder{}
		sb.WriteString("Respond terse like smart caveman. All technical substance stay. Only fluff die.\n")
		sb.WriteString("## Rules\n")
		sb.WriteString("* Drop articles (a/an/the), filler (just/really/basically/actually/simply), pleasantries (sure/certainly/of course/happy to), and hedging.\n")
		sb.WriteString("* Fragments OK. Use short synonyms (e.g., use \"big\" not \"extensive\", \"fix\" not \"implement a solution for\").\n")
		sb.WriteString("* Technical terms exact. Code blocks unchanged. Errors quoted exact. No wrapping in brackets.\n")
		sb.WriteString("* Pattern: Subject state. Root cause. Required action.\n")
		sb.WriteString("* NO: \"I would be happy to help. It seems your code has a bug because...\"\n")
		sb.WriteString("* YES: \"Motor loud. Bearing dry. Apply oil.\"\n")
		sb.WriteString("## Intensity Levels (Default is " + c.cavemanMode.String() + ")\n")
		sb.WriteString("* off: Do not apply caveman.\n")
		sb.WriteString("* lite: No filler/hedging. Keep articles + full sentences. Professional but tight.\n")
		sb.WriteString("* full: Drop articles, fragments OK, short synonyms. Classic caveman.\n")
		sb.WriteString("* ultra: Abbreviate (DB/auth/config/req/res/fn/impl), strip conjunctions, arrows for causality (X → Y), one word when one word enough.\n")
		sb.WriteString("## Persistence & Boundaries\n")
		sb.WriteString("* ACTIVE EVERY RESPONSE. No revert after many turns. No filler drift.\n")
		sb.WriteString("* Code/commits/PRs: write normally.\n")
		sb.WriteString("* Auto-Clarity: Drop caveman for security warnings, irreversible action confirmations, or multi-step sequences where fragment order risks misread. Resume caveman after clear part done.\n")
		sb.WriteString("\nFinal Instruction: Be concise. No brackets. No wrapping response in quotes.\n")
		sb.WriteString("\n")
		return sb.String()
	}
	return "Please shorten the following text to reduce token usage, preserving all key information and details and making zero modification to any code blocks:\n\n"
}

// cosine similarity between two float32 vectors
// see https://medium.com/advanced-deep-learning/understanding-vector-similarity-b9c10f7506de
// see https://towardsdatascience.com/demystifying-cosine-similarity/
func cosineSimilarity(v1 []float32, v2 []float32) (float32, error) {
	if len(v1) != len(v2) {
		return 0, fmt.Errorf("vectors must have same length: %d vs %d", len(v1), len(v2))
	}
	if len(v1) == 0 {
		return 0, fmt.Errorf("vectors cannot be empty")
	}

	var dotProduct, normA, normB float32
	for i := range v1 {
		dotProduct += v1[i] * v2[i]
		normA += v1[i] * v1[i]
		normB += v2[i] * v2[i]
	}

	if normA == 0 || normB == 0 {
		return 0, fmt.Errorf("cannot calculate similarity for zero-based vectors")
	}

	magA := math.Sqrt(float64(normA))
	magB := math.Sqrt(float64(normB))

	return dotProduct / (float32(magA) * float32(magB)), nil
}
