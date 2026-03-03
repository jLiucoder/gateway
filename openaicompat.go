package main

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAICompatProvider implements LLMProvider for OpenAI and OpenAI-compatible APIs.
// Pass option.WithBaseURL to target Kimi, MiniMax, DeepSeek, Groq, etc.
type OpenAICompatProvider struct {
	client openai.Client
	name   string
	models []string
}

func NewOpenAICompatProvider(name string, models []string, opts ...option.RequestOption) *OpenAICompatProvider {
	return &OpenAICompatProvider{
		client: openai.NewClient(opts...),
		name:   name,
		models: models,
	}
}

func (p *OpenAICompatProvider) Name() string     { return p.name }
func (p *OpenAICompatProvider) Models() []string { return p.models }

func (p *OpenAICompatProvider) Chat(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	res, err := p.client.Chat.Completions.New(ctx, toOpenAIParams(req))
	if err != nil {
		return nil, err
	}
	return fromOpenAIResponse(res), nil
}

func (p *OpenAICompatProvider) ChatStream(ctx context.Context, req LLMRequest) (<-chan StreamEvent, error) {
	stream := p.client.Chat.Completions.NewStreaming(ctx, toOpenAIParams(req))
	ch := make(chan StreamEvent, 16)
	go func() {
		defer close(ch)
		for stream.Next() {
			ch <- fromOpenAIChunk(stream.Current())
		}
		if err := stream.Err(); err != nil {
			ch <- StreamEvent{Type: "error", Delta: &Delta{Text: err.Error()}}
		}
	}()
	return ch, nil
}

func toOpenAIParams(req LLMRequest) openai.ChatCompletionNewParams {
	params := openai.ChatCompletionNewParams{
		Model: req.Model,
	}

	// Build messages, prepending system prompt if top-level System field is set
	var msgs []openai.ChatCompletionMessageParamUnion
	if req.System != "" {
		msgs = append(msgs, openai.SystemMessage(req.System))
	}
	for _, m := range req.Messages {
		msgs = append(msgs, toOpenAIMessage(m))
	}
	params.Messages = msgs

	if req.Temperature != nil {
		params.Temperature = openai.Float(*req.Temperature)
	}
	// Always use max_completion_tokens; map max_tokens for client compatibility
	if req.MaxCompletionTokens != nil {
		params.MaxCompletionTokens = openai.Int(int64(*req.MaxCompletionTokens))
	} else if req.MaxTokens != nil {
		params.MaxCompletionTokens = openai.Int(int64(*req.MaxTokens))
	}
	if req.TopP != nil {
		params.TopP = openai.Float(*req.TopP)
	}
	if req.Seed != nil {
		params.Seed = openai.Int(int64(*req.Seed))
	}
	if req.PresencePenalty != nil {
		params.PresencePenalty = openai.Float(*req.PresencePenalty)
	}
	if req.FrequencyPenalty != nil {
		params.FrequencyPenalty = openai.Float(*req.FrequencyPenalty)
	}
	if req.User != "" {
		params.User = openai.String(req.User)
	}
	if req.ReasoningEffort != "" {
		params.ReasoningEffort = openai.ReasoningEffort(req.ReasoningEffort)
	}
	if req.StreamOptions != nil {
		params.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(req.StreamOptions.IncludeUsage),
		}
	}
	if len(req.Tools) > 0 {
		tools := make([]openai.ChatCompletionToolParam, len(req.Tools))
		for i, t := range req.Tools {
			var schema openai.FunctionParameters
			json.Unmarshal(t.InputSchema, &schema) //nolint:errcheck
			tools[i] = openai.ChatCompletionToolParam{
				Function: openai.FunctionDefinitionParam{
					Name:        t.Name,
					Description: openai.String(t.Description),
					Parameters:  schema,
				},
			}
		}
		params.Tools = tools
	}
	return params
}

func toOpenAIMessage(m Message) openai.ChatCompletionMessageParamUnion {
	switch m.Role {
	case "system":
		return openai.SystemMessage(m.GetContentString())
	case "user":
		return openai.UserMessage(m.GetContentString())
	case "assistant":
		if len(m.ToolCalls) > 0 {
			toolCalls := make([]openai.ChatCompletionMessageToolCallParam, len(m.ToolCalls))
			for i, tc := range m.ToolCalls {
				toolCalls[i] = openai.ChatCompletionMessageToolCallParam{
					ID: tc.ID,
					Function: openai.ChatCompletionMessageToolCallFunctionParam{
						Name:      tc.Name,
						Arguments: tc.Input,
					},
				}
			}
			return openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					ToolCalls: toolCalls,
				},
			}
		}
		return openai.AssistantMessage(m.GetContentString())
	case "tool":
		return openai.ToolMessage(m.GetContentString(), m.ToolCallID)
	default:
		return openai.UserMessage(m.GetContentString())
	}
}

func fromOpenAIResponse(res *openai.ChatCompletion) *LLMResponse {
	r := &LLMResponse{
		ID:    res.ID,
		Model: res.Model,
		Role:  "assistant",
		Usage: Usage{
			InputTokens:  int(res.Usage.PromptTokens),
			OutputTokens: int(res.Usage.CompletionTokens),
			TotalTokens:  int(res.Usage.TotalTokens),
		},
	}
	if len(res.Choices) == 0 {
		return r
	}
	choice := res.Choices[0]
	r.StopReason = fromOpenAIFinishReason(choice.FinishReason)
	if choice.Message.Content != "" {
		r.Content = append(r.Content, ContentBlock{Type: "text", Text: choice.Message.Content})
	}
	for _, tc := range choice.Message.ToolCalls {
		r.Content = append(r.Content, ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: json.RawMessage(tc.Function.Arguments),
		})
	}
	return r
}

func fromOpenAIChunk(chunk openai.ChatCompletionChunk) StreamEvent {
	// Usage-only final chunk (when stream_options.include_usage=true)
	if len(chunk.Choices) == 0 {
		return StreamEvent{
			Type: "message_delta",
			Usage: &Usage{
				InputTokens:  int(chunk.Usage.PromptTokens),
				OutputTokens: int(chunk.Usage.CompletionTokens),
				TotalTokens:  int(chunk.Usage.TotalTokens),
			},
		}
	}
	choice := chunk.Choices[0]
	if choice.FinishReason != "" {
		return StreamEvent{
			Type:  "message_delta",
			Delta: &Delta{StopReason: fromOpenAIFinishReason(choice.FinishReason)},
		}
	}
	if len(choice.Delta.ToolCalls) > 0 {
		tc := choice.Delta.ToolCalls[0]
		return StreamEvent{
			Type:  "content_block_delta",
			Index: int(choice.Index),
			Delta: &Delta{Type: "input_json_delta", PartialJSON: tc.Function.Arguments},
		}
	}
	return StreamEvent{
		Type:  "content_block_delta",
		Index: int(choice.Index),
		Delta: &Delta{Type: "text_delta", Text: choice.Delta.Content},
	}
}

func fromOpenAIFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		return reason
	}
}
