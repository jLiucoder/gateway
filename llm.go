package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/openai/openai-go/option"
)

// Request
type LLMRequest struct {
	Model               string            `json:"model"`
	Messages            []Message         `json:"messages"`
	System              string            `json:"system,omitempty"`
	MaxTokens           *int              `json:"max_tokens,omitempty"`
	Temperature         *float64          `json:"temperature,omitempty"`
	TopP                *float64          `json:"top_p,omitempty"`
	TopK                *int              `json:"top_k,omitempty"`
	Stream              bool              `json:"stream,omitempty"`
	StreamOptions       *StreamOptions    `json:"stream_options,omitempty"`
	Stop                []string          `json:"stop,omitempty"`
	Tools               []Tool            `json:"tools,omitempty"`
	ToolChoice          any               `json:"tool_choice,omitempty"`
	ResponseFormat      *ResponseFormat   `json:"response_format,omitempty"`
	Seed                *int              `json:"seed,omitempty"`
	PresencePenalty     *float64          `json:"presence_penalty,omitempty"`
	FrequencyPenalty    *float64          `json:"frequency_penalty,omitempty"`
	ReasoningEffort     string            `json:"reasoning_effort,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	MaxCompletionTokens *int              `json:"max_completion_tokens,omitempty"`
	Thinking            *ThinkingOptions  `json:"thinking,omitempty"`
	User                string            `json:"user,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// claude models only
type ThinkingOptions struct {
	Type         string `json:"type"` // "enabled" or "disabled"
	BudgetTokens *int   `json:"budget_tokens,omitempty"`
}

// Message and methods
type Message struct {
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	Name       string          `json:"name,omitempty"`
	ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
}

func (m *Message) GetContentString() string {
	var s string
	if err := json.Unmarshal(m.Content, &s); err != nil {
		return ""
	}
	return s
}

func (m *Message) SetContentString(s string) {
	m.Content, _ = json.Marshal(s)
}

func (m *Message) GetContentBlocks() []ContentBlock {
	var blocks []ContentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil
	}
	return blocks
}

func (m *Message) SetContentBlocks(blocks []ContentBlock) {
	m.Content, _ = json.Marshal(blocks)
}

type ContentBlock struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"`

	Source *ImageSource `json:"source,omitempty"`

	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	// tool_result block
	ToolUseID string `json:"tool_use_id,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type ToolCall struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Input string `json:"input"`
	Index *int   `json:"index,omitempty"`
}

type ToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// Response
type LLMResponse struct {
	ID           string         `json:"id"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	Role         string         `json:"role"`
	StopReason   string         `json:"stop_reason"`
	StopSequence string         `json:"stop_sequence,omitempty"`
	Usage        Usage          `json:"usage"`
}

type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	TotalTokens              int `json:"total_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	ReasoningTokens          int `json:"reasoning_tokens,omitempty"`
}

type ResponseFormat struct {
	Type       string          `json:"type"`
	JSONSchema json.RawMessage `json:"json_schema,omitempty"`
}

// StreamEvent for streaming response
type StreamEvent struct {
	Type         string        `json:"type"`
	Message      *LLMResponse  `json:"message,omitempty"`
	ContentBlock *ContentBlock `json:"content_block,omitempty"`
	Delta        *Delta        `json:"delta,omitempty"`
	Index        int           `json:"index,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
}

type Delta struct {
	Type        string `json:"type,omitempty"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
	StopReason  string `json:"stop_reason,omitempty"`
}

// interface for LLM providers that all the provider-specific implementation should follow
type LLMProvider interface {
	Chat(ctx context.Context, req LLMRequest) (*LLMResponse, error)
	ChatStream(ctx context.Context, req LLMRequest) (<-chan StreamEvent, error)
	Name() string
	Models() []string
}

func buildProviders(llmConfig LLMConfig) map[string]LLMProvider {
	res := map[string]LLMProvider{}

	providerConfigs := llmConfig.Providers
	for name, config := range providerConfigs {
		if config.Type == "anthropic" {
			res[name] = NewAnthropicProvider(os.Getenv(config.ApiKeyEnv), config.Models)
		} else {
			res[name] = NewOpenAICompatProvider(name, config.Models, option.WithAPIKey(os.Getenv(config.ApiKeyEnv)), option.WithBaseURL(config.BaseURL))
		}
	}
	return res
}

func classify(classifierModel string, classifierProvider LLMProvider, messages []Message, reqContext context.Context) (string, error) {

	res, err := classifierProvider.Chat(reqContext, LLMRequest{
		Model:    classifierModel,
		System:   "Classify the user query into exactly one complexity tier. Reply with one word only.\n\nsimple: factual lookups, basic math, definitions, yes/no questions.\nmedium: multi-step reasoning, writing code, explaining concepts, comparisons.\ncomplex: deep research, architecture decisions, highly specialized technical analysis, long-form generation.",
		Messages: messages,
	})

	if err != nil {
		return "medium", fmt.Errorf("classifier call failed: %w", err)
	}
	if len(res.Content) == 0 || res.Content[0].Text == "" {
		return "medium", fmt.Errorf("classifier returned empty response, medium is used")
	}
	return strings.TrimSpace(strings.ToLower(res.Content[0].Text)), nil
}
