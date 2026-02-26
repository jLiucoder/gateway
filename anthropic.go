package main

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

type AnthropicProvider struct {
	client anthropic.Client
	name   string
	models []string
}

func NewAnthropicProvider(apiKey string, models []string) *AnthropicProvider {
	return &AnthropicProvider{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		name:   "anthropic",
		models: models,
	}
}

func (p *AnthropicProvider) Name() string     { return p.name }
func (p *AnthropicProvider) Models() []string { return p.models }

func (p *AnthropicProvider) Chat(ctx context.Context, req LLMRequest) (*LLMResponse, error) {
	res, err := p.client.Messages.New(ctx, toAnthropicParams(req))
	if err != nil {
		return nil, err
	}
	return fromAnthropicResponse(res), nil
}

func (p *AnthropicProvider) ChatStream(ctx context.Context, req LLMRequest) (<-chan StreamEvent, error) {
	stream := p.client.Messages.NewStreaming(ctx, toAnthropicParams(req))
	ch := make(chan StreamEvent, 16)
	go func() {
		defer close(ch)
		for stream.Next() {
			ch <- fromAnthropicStreamEvent(stream.Current())
		}
		if err := stream.Err(); err != nil {
			ch <- StreamEvent{Type: "error", Delta: &Delta{Text: err.Error()}}
		}
	}()
	return ch, nil
}

func toAnthropicParams(req LLMRequest) anthropic.MessageNewParams {
	// MaxTokens is required by Anthropic — default to 4096 if not set
	maxTokens := int64(4096)
	if req.MaxTokens != nil {
		maxTokens = int64(*req.MaxTokens)
	}
	if req.MaxCompletionTokens != nil {
		maxTokens = int64(*req.MaxCompletionTokens)
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: maxTokens,
	}

	// KEY DIFFERENCE: Anthropic system prompt is []TextBlockParam, not a string
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}

	for _, m := range req.Messages {
		if m.Role == "system" {
			// Fold inline system messages into the top-level system param
			params.System = append(params.System, anthropic.TextBlockParam{Text: m.GetContentString()})
			continue
		}
		params.Messages = append(params.Messages, toAnthropicMessage(m))
	}

	if req.Temperature != nil {
		params.Temperature = anthropic.Float(*req.Temperature)
	}
	if req.TopP != nil {
		params.TopP = anthropic.Float(*req.TopP)
	}
	if req.TopK != nil {
		params.TopK = anthropic.Int(int64(*req.TopK))
	}
	if len(req.Stop) > 0 {
		params.StopSequences = req.Stop // note: Anthropic calls it StopSequences, not Stop
	}
	if req.Thinking != nil && req.Thinking.Type == "enabled" && req.Thinking.BudgetTokens != nil {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(*req.Thinking.BudgetTokens))
	}
	if len(req.Tools) > 0 {
		tools := make([]anthropic.ToolUnionParam, len(req.Tools))
		for i, t := range req.Tools {
			var props any
			json.Unmarshal(t.InputSchema, &props) //nolint:errcheck
			tools[i] = anthropic.ToolUnionParam{
				OfTool: &anthropic.ToolParam{
					Name:        t.Name,
					Description: anthropic.String(t.Description),
					InputSchema: anthropic.ToolInputSchemaParam{Properties: props},
				},
			}
		}
		params.Tools = tools
	}
	return params
}

func toAnthropicMessage(m Message) anthropic.MessageParam {
	switch m.Role {
	case "tool":
		// KEY DIFFERENCE: tool results are user messages with tool_result content blocks
		return anthropic.NewUserMessage(anthropic.ContentBlockParamUnion{
			OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: m.ToolCallID,
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: m.GetContentString()}},
				},
			},
		})
	case "assistant":
		var blocks []anthropic.ContentBlockParamUnion
		if text := m.GetContentString(); text != "" {
			blocks = append(blocks, anthropic.ContentBlockParamUnion{
				OfText: &anthropic.TextBlockParam{Text: text},
			})
		}
		for _, tc := range m.ToolCalls {
			var input any
			json.Unmarshal([]byte(tc.Input), &input) //nolint:errcheck
			blocks = append(blocks, anthropic.ContentBlockParamUnion{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				},
			})
		}
		return anthropic.NewAssistantMessage(blocks...)
	default: // "user"
		return anthropic.NewUserMessage(anthropic.ContentBlockParamUnion{
			OfText: &anthropic.TextBlockParam{Text: m.GetContentString()},
		})
	}
}

func fromAnthropicResponse(res *anthropic.Message) *LLMResponse {
	r := &LLMResponse{
		ID:           res.ID,
		Model:        string(res.Model),
		Role:         "assistant",
		StopReason:   string(res.StopReason),
		StopSequence: res.StopSequence,
		Usage: Usage{
			InputTokens:              int(res.Usage.InputTokens),
			OutputTokens:             int(res.Usage.OutputTokens),
			CacheCreationInputTokens: int(res.Usage.CacheCreationInputTokens),
			CacheReadInputTokens:     int(res.Usage.CacheReadInputTokens),
		},
	}
	for _, block := range res.Content {
		switch block.Type {
		case "text":
			r.Content = append(r.Content, ContentBlock{Type: "text", Text: block.Text})
		case "thinking":
			r.Content = append(r.Content, ContentBlock{Type: "thinking", Text: block.Thinking})
		case "tool_use":
			r.Content = append(r.Content, ContentBlock{
				Type:  "tool_use",
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			})
		}
	}
	return r
}

func fromAnthropicStreamEvent(event anthropic.MessageStreamEventUnion) StreamEvent {
	switch event.Type {
	case "message_start":
		msg := event.Message
		return StreamEvent{
			Type: "message_start",
			Message: &LLMResponse{
				ID:    msg.ID,
				Model: string(msg.Model),
				Role:  "assistant",
				Usage: Usage{InputTokens: int(msg.Usage.InputTokens)},
			},
		}
	case "message_delta":
		se := StreamEvent{
			Type:  "message_delta",
			Delta: &Delta{StopReason: string(event.Delta.StopReason)},
		}
		if event.Usage.OutputTokens > 0 {
			se.Usage = &Usage{
				OutputTokens:             int(event.Usage.OutputTokens),
				CacheCreationInputTokens: int(event.Usage.CacheCreationInputTokens),
				CacheReadInputTokens:     int(event.Usage.CacheReadInputTokens),
			}
		}
		return se
	case "message_stop":
		return StreamEvent{Type: "message_stop"}
	case "content_block_start":
		return StreamEvent{Type: "content_block_start", Index: int(event.Index)}
	case "content_block_delta":
		se := StreamEvent{Type: "content_block_delta", Index: int(event.Index)}
		switch event.Delta.Type {
		case "text_delta":
			se.Delta = &Delta{Type: "text_delta", Text: event.Delta.Text}
		case "input_json_delta":
			se.Delta = &Delta{Type: "input_json_delta", PartialJSON: event.Delta.PartialJSON}
		case "thinking_delta":
			se.Delta = &Delta{Type: "thinking_delta", Text: event.Delta.Thinking}
		}
		return se
	case "content_block_stop":
		return StreamEvent{Type: "content_block_stop", Index: int(event.Index)}
	}
	return StreamEvent{Type: event.Type}
}
