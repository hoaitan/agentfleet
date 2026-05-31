package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/tan/agentfleet/internal/fleet"
)

const defaultAPIURL = "https://api.anthropic.com/v1/messages"

const generateSystemPrompt = `You are a task planner for an AI agent fleet runner.
Given a goal, output a JSON array of tasks. Each task must have:
- id: string (unique, kebab-case)
- name: string (human-readable title)
- command: string (CLI binary to run, e.g. "claude")
- steps: array of objects with "delay" (float, seconds to wait) and "command" (string to inject; empty string stops the agent)

Return ONLY a valid JSON array. No markdown, no code fences, no explanation.`

// GenerateSource calls the Claude API to generate tasks from a natural-language goal.
// Set ANTHROPIC_API_KEY in the environment before calling Load().
type GenerateSource struct {
	goal   string
	apiURL string
	apiKey string
}

// NewGenerateSource creates a GenerateSource. apiURL defaults to the Anthropic API;
// override it in tests. apiKey is read from ANTHROPIC_API_KEY if empty.
func NewGenerateSource(goal, apiURL, apiKey string) *GenerateSource {
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return &GenerateSource{goal: goal, apiURL: apiURL, apiKey: apiKey}
}

func (s *GenerateSource) Load() ([]fleet.Task, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}

	body, err := json.Marshal(map[string]any{
		"model":      "claude-sonnet-4-6",
		"max_tokens": 2048,
		"system":     generateSystemPrompt,
		"messages":   []map[string]any{{"role": "user", "content": s.goal}},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call API: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", result.Error.Message)
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from model")
	}

	var raw []*fleet.BasicTask
	if err := json.Unmarshal([]byte(result.Content[0].Text), &raw); err != nil {
		return nil, fmt.Errorf("parse generated tasks: %w", err)
	}

	tasks := make([]fleet.Task, len(raw))
	for i, t := range raw {
		tasks[i] = t
	}
	return tasks, nil
}
