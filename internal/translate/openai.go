package translate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxAnswerBytes bounds what is read of the service's answer.
const maxAnswerBytes = 64 << 10

// maxTokens bounds how long an answer may run: a chat line is at most 500
// characters, and a translation of one is well inside this.
const maxTokens = 1024

// answer is what the model is asked to give back.
type answer struct {
	Source string `json:"source"`
	Text   string `json:"text"`
}

// answerSchema is the JSON schema of answer, sent as response_format so a
// server that supports it holds the model to it.
var answerSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"source": map[string]any{"type": "string"},
		"text":   map[string]any{"type": "string"},
	},
	"required":             []string{"source", "text"},
	"additionalProperties": false,
}

// openAI speaks the /chat/completions endpoint that Ollama, llama.cpp's
// server and vLLM serve.
type openAI struct {
	base   string // ends in /v1, or wherever /chat/completions lives
	model  string
	client *http.Client
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model          string         `json:"model"`
	Messages       []chatMessage  `json:"messages"`
	Temperature    float64        `json:"temperature"`
	MaxTokens      int            `json:"max_tokens"`
	Stream         bool           `json:"stream"`
	ResponseFormat map[string]any `json:"response_format"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// errAnswer is an answer that was not the JSON asked for.
var errAnswer = errors.New("translate: the answer is not a translation")

// ask sends the fixed instructions and the line, as data, and reads the
// answer.
func (o *openAI) ask(ctx context.Context, system, line string) (answer, error) {
	body, err := json.Marshal(chatRequest{
		Model: o.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: line},
		},
		MaxTokens: maxTokens,
		ResponseFormat: map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "translation", "strict": true, "schema": answerSchema},
		},
	})
	if err != nil {
		return answer{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.base+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return answer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(req)
	if err != nil {
		return answer{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes))
	if err != nil {
		return answer{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return answer{}, fmt.Errorf("translate: the service answered %s", resp.Status)
	}
	var cr chatResponse
	if err := json.Unmarshal(raw, &cr); err != nil || len(cr.Choices) == 0 {
		return answer{}, fmt.Errorf("translate: the service's answer is not a chat completion")
	}
	var a answer
	if err := json.Unmarshal([]byte(unwrap(cr.Choices[0].Message.Content)), &a); err != nil {
		return answer{}, errAnswer
	}
	return a, nil
}

// unwrap takes off what a model may put around the JSON despite being asked
// not to: its thinking, and a Markdown code fence.
func unwrap(s string) string {
	s = strings.TrimSpace(s)
	if rest, ok := strings.CutPrefix(s, "<think>"); ok {
		if _, after, found := strings.Cut(rest, "</think>"); found {
			s = strings.TrimSpace(after)
		}
	}
	if rest, ok := strings.CutPrefix(s, "```"); ok {
		rest = strings.TrimPrefix(rest, "json")
		s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "```"))
	}
	return s
}
