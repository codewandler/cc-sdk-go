package oai_test

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/codewandler/cc-sdk-go/oai"
)

func requireCLI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not available")
	}
}

// testCase defines a chat completion request with validation.
type testCase struct {
	Name     string
	Request  oai.ChatCompletionRequest
	Validate func(t *testing.T, resp *oai.ChatCompletionResponse)
}

var cases = []testCase{
	{
		Name: "minimal",
		Request: oai.ChatCompletionRequest{
			Model: "haiku",
			Messages: []oai.ChatMessage{
				{Role: "user", Content: "Reply with exactly: PONG"},
			},
		},
		Validate: func(t *testing.T, resp *oai.ChatCompletionResponse) {
			requireContent(t, resp)
			if !strings.Contains(resp.Choices[0].Message.StringContent(), "PONG") {
				t.Errorf("expected PONG, got: %s", resp.Choices[0].Message.StringContent())
			}
			requireFinish(t, resp, "stop")
		},
	},
	{
		Name: "system_prompt",
		Request: oai.ChatCompletionRequest{
			Model: "haiku",
			Messages: []oai.ChatMessage{
				{Role: "system", Content: "You are a pirate. Every response must contain 'arrr'."},
				{Role: "user", Content: "How are you today?"},
			},
		},
		Validate: func(t *testing.T, resp *oai.ChatCompletionResponse) {
			requireContent(t, resp)
			text := strings.ToLower(resp.Choices[0].Message.StringContent())
			if !strings.Contains(text, "arrr") {
				t.Errorf("expected pirate speak with 'arrr', got: %s", text)
			}
			requireFinish(t, resp, "stop")
		},
	},
	{
		Name: "multi_turn",
		Request: oai.ChatCompletionRequest{
			Model: "haiku",
			Messages: []oai.ChatMessage{
				{Role: "user", Content: "My secret number is 42."},
				{Role: "assistant", Content: "Got it, I'll remember that your secret number is 42."},
				{Role: "user", Content: "What is my secret number? Reply with just the number."},
			},
		},
		Validate: func(t *testing.T, resp *oai.ChatCompletionResponse) {
			requireContent(t, resp)
			if !strings.Contains(resp.Choices[0].Message.StringContent(), "42") {
				t.Errorf("expected 42, got: %s", resp.Choices[0].Message.StringContent())
			}
		},
	},
	{
		Name: "low_effort",
		Request: oai.ChatCompletionRequest{
			Model: "haiku",
			Messages: []oai.ChatMessage{
				{Role: "user", Content: "What is 2+2? Reply with just the number."},
			},
		},
		Validate: func(t *testing.T, resp *oai.ChatCompletionResponse) {
			requireContent(t, resp)
			if !strings.Contains(resp.Choices[0].Message.StringContent(), "4") {
				t.Errorf("expected 4, got: %s", resp.Choices[0].Message.StringContent())
			}
		},
	},
	{
		Name: "simple_tool",
		Request: oai.ChatCompletionRequest{
			Model: "haiku",
			Messages: []oai.ChatMessage{
				{Role: "user", Content: "What's the weather in Berlin?"},
			},
			Tools: []oai.Tool{
				{
					Type: "function",
					Function: oai.FunctionDefinition{
						Name:        "get_weather",
						Description: "Get the current weather for a city",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"city": map[string]any{
									"type":        "string",
									"description": "City name",
								},
							},
							"required": []string{"city"},
						},
					},
				},
			},
		},
		Validate: func(t *testing.T, resp *oai.ChatCompletionResponse) {
			requireToolCalls(t, resp, "get_weather")
			args := parseArgs(t, resp.Choices[0].Message.ToolCalls[0])
			city, _ := args["city"].(string)
			if !strings.Contains(strings.ToLower(city), "berlin") {
				t.Errorf("expected city=berlin, got: %s", city)
			}
		},
	},
	{
		Name: "tool_with_enum",
		Request: oai.ChatCompletionRequest{
			Model: "haiku",
			Messages: []oai.ChatMessage{
				{Role: "user", Content: "Convert 100 USD to EUR."},
			},
			Tools: []oai.Tool{
				{
					Type: "function",
					Function: oai.FunctionDefinition{
						Name:        "convert_currency",
						Description: "Convert an amount between currencies",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"amount": map[string]any{"type": "number"},
								"from":   map[string]any{"type": "string", "enum": []string{"USD", "EUR", "GBP", "JPY"}},
								"to":     map[string]any{"type": "string", "enum": []string{"USD", "EUR", "GBP", "JPY"}},
							},
							"required": []string{"amount", "from", "to"},
						},
					},
				},
			},
		},
		Validate: func(t *testing.T, resp *oai.ChatCompletionResponse) {
			requireToolCalls(t, resp, "convert_currency")
			args := parseArgs(t, resp.Choices[0].Message.ToolCalls[0])
			if from, _ := args["from"].(string); from != "USD" {
				t.Errorf("expected from=USD, got: %s", from)
			}
			if to, _ := args["to"].(string); to != "EUR" {
				t.Errorf("expected to=EUR, got: %s", to)
			}
		},
	},
	{
		Name: "tool_followup",
		Request: oai.ChatCompletionRequest{
			Model: "haiku",
			Messages: []oai.ChatMessage{
				{Role: "user", Content: "What's the weather in Paris?"},
				{
					Role: "assistant",
					ToolCalls: []oai.ToolCall{
						{
							ID:   "call_0",
							Type: "function",
							Function: oai.FunctionCall{
								Name:      "get_weather",
								Arguments: `{"city":"Paris"}`,
							},
						},
					},
				},
				{Role: "tool", ToolCallID: "call_0", Content: `{"temp_c": 18, "condition": "sunny"}`},
				{Role: "user", Content: "Thanks! And in London?"},
			},
			Tools: []oai.Tool{
				{
					Type: "function",
					Function: oai.FunctionDefinition{
						Name:        "get_weather",
						Description: "Get the current weather for a city",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"city": map[string]any{"type": "string"},
							},
							"required": []string{"city"},
						},
					},
				},
			},
		},
		Validate: func(t *testing.T, resp *oai.ChatCompletionResponse) {
			requireToolCalls(t, resp, "get_weather")
			args := parseArgs(t, resp.Choices[0].Message.ToolCalls[0])
			city, _ := args["city"].(string)
			if !strings.Contains(strings.ToLower(city), "london") {
				t.Errorf("expected city=london, got: %s", city)
			}
		},
	},
	{
		Name: "nested_schema",
		Request: oai.ChatCompletionRequest{
			Model: "haiku",
			Messages: []oai.ChatMessage{
				{Role: "user", Content: "Create a user: John Doe, age 30, address is 123 Main St, Springfield, IL 62704, US."},
			},
			Tools: []oai.Tool{
				{
					Type: "function",
					Function: oai.FunctionDefinition{
						Name:        "create_user",
						Description: "Create a new user record",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"name": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"first": map[string]any{"type": "string"},
										"last":  map[string]any{"type": "string"},
									},
									"required": []string{"first", "last"},
								},
								"age": map[string]any{"type": "integer"},
								"address": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"street":  map[string]any{"type": "string"},
										"city":    map[string]any{"type": "string"},
										"state":   map[string]any{"type": "string"},
										"zip":     map[string]any{"type": "string"},
										"country": map[string]any{"type": "string"},
									},
									"required": []string{"street", "city", "state", "zip", "country"},
								},
							},
							"required": []string{"name", "age", "address"},
						},
					},
				},
			},
		},
		Validate: func(t *testing.T, resp *oai.ChatCompletionResponse) {
			requireToolCalls(t, resp, "create_user")
			args := parseArgs(t, resp.Choices[0].Message.ToolCalls[0])

			name, _ := args["name"].(map[string]any)
			if name == nil {
				t.Fatal("expected nested name object")
			}
			if first, _ := name["first"].(string); !strings.EqualFold(first, "John") {
				t.Errorf("expected first=John, got: %s", first)
			}

			addr, _ := args["address"].(map[string]any)
			if addr == nil {
				t.Fatal("expected nested address object")
			}
			if zip, _ := addr["zip"].(string); zip != "62704" {
				t.Errorf("expected zip=62704, got: %s", zip)
			}
		},
	},
	{
		Name: "multi_tool_choice",
		Request: oai.ChatCompletionRequest{
			Model: "haiku",
			Messages: []oai.ChatMessage{
				{Role: "user", Content: "Send an email to alice@example.com saying 'Meeting at 3pm'."},
			},
			Tools: []oai.Tool{
				{
					Type: "function",
					Function: oai.FunctionDefinition{
						Name:        "send_email",
						Description: "Send an email",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"to":      map[string]any{"type": "string", "format": "email"},
								"subject": map[string]any{"type": "string"},
								"body":    map[string]any{"type": "string"},
							},
							"required": []string{"to", "body"},
						},
					},
				},
				{
					Type: "function",
					Function: oai.FunctionDefinition{
						Name:        "send_sms",
						Description: "Send an SMS text message",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"phone": map[string]any{"type": "string"},
								"text":  map[string]any{"type": "string"},
							},
							"required": []string{"phone", "text"},
						},
					},
				},
				{
					Type: "function",
					Function: oai.FunctionDefinition{
						Name:        "create_calendar_event",
						Description: "Create a calendar event",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"title": map[string]any{"type": "string"},
								"time":  map[string]any{"type": "string"},
							},
							"required": []string{"title", "time"},
						},
					},
				},
			},
		},
		Validate: func(t *testing.T, resp *oai.ChatCompletionResponse) {
			requireToolCalls(t, resp, "send_email")
			args := parseArgs(t, resp.Choices[0].Message.ToolCalls[0])
			if to, _ := args["to"].(string); to != "alice@example.com" {
				t.Errorf("expected to=alice@example.com, got: %s", to)
			}
		},
	},
	{
		Name: "multiple_tool_calls",
		Request: oai.ChatCompletionRequest{
			Model: "haiku",
			Messages: []oai.ChatMessage{
				{Role: "system", Content: "When asked to look up multiple things, call the tool once for each item. Always use tools; never answer from memory."},
				{Role: "user", Content: "Look up the weather in Tokyo, London, and Sydney."},
			},
			Tools: []oai.Tool{
				{
					Type: "function",
					Function: oai.FunctionDefinition{
						Name:        "get_weather",
						Description: "Get the current weather for a city",
						Parameters: map[string]any{
							"type": "object",
							"properties": map[string]any{
								"city": map[string]any{"type": "string"},
							},
							"required": []string{"city"},
						},
					},
				},
			},
		},
		Validate: func(t *testing.T, resp *oai.ChatCompletionResponse) {
			requireFinish(t, resp, "tool_calls")
			calls := resp.Choices[0].Message.ToolCalls
			if len(calls) < 2 {
				t.Errorf("expected multiple tool calls, got %d", len(calls))
			}
			for _, tc := range calls {
				if tc.Function.Name != "get_weather" {
					t.Errorf("expected get_weather, got: %s", tc.Function.Name)
				}
			}
		},
	},
}

func TestCompletion(t *testing.T) {
	requireCLI(t)
	t.Parallel()
	client := oai.NewClientDefault()
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			resp, err := client.CreateChatCompletion(ctx, tc.Request)
			elapsed := time.Since(start)

			if err != nil {
				t.Fatalf("completion error (%s): %v", elapsed, err)
			}

			t.Logf("non-stream  %s  model=%s  tokens=%d/%d",
				elapsed.Round(time.Millisecond),
				resp.Model,
				resp.Usage.PromptTokens, resp.Usage.CompletionTokens)

			tc.Validate(t, resp)
		})
	}
}

func TestCompletionStream(t *testing.T) {
	requireCLI(t)
	t.Parallel()
	client := oai.NewClientDefault()
	ctx := context.Background()

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			start := time.Now()
			stream, err := client.CreateChatCompletionStream(ctx, tc.Request)
			if err != nil {
				t.Fatalf("stream error: %v", err)
			}
			defer stream.Close()

			resp := collectStream(t, stream)
			elapsed := time.Since(start)

			t.Logf("stream      %s  model=%s  chunks=%d",
				elapsed.Round(time.Millisecond),
				resp.Model,
				resp.ChunkCount)

			tc.Validate(t, resp.Assembled)
		})
	}
}

// --- stream assembly ---

type streamResult struct {
	Assembled  *oai.ChatCompletionResponse
	Model      string
	ChunkCount int
}

func collectStream(t *testing.T, stream *oai.ChatCompletionStream) streamResult {
	t.Helper()
	var (
		content   strings.Builder
		role      string
		model     string
		finish    string
		toolCalls []oai.ToolCall
		count     int
	)

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("recv error: %v", err)
		}
		count++

		if chunk.Model != "" {
			model = chunk.Model
		}

		for _, c := range chunk.Choices {
			if c.Delta.Role != "" {
				role = c.Delta.Role
			}
			if c.Delta.Content != nil {
				content.WriteString(*c.Delta.Content)
			}
			if len(c.Delta.ToolCalls) > 0 {
				toolCalls = append(toolCalls, c.Delta.ToolCalls...)
			}
			if c.FinishReason != nil {
				finish = *c.FinishReason
			}
		}
	}

	msg := oai.ChatMessage{Role: role}
	if text := content.String(); text != "" {
		msg.Content = text
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	return streamResult{
		Model:      model,
		ChunkCount: count,
		Assembled: &oai.ChatCompletionResponse{
			Model: model,
			Choices: []oai.Choice{
				{
					Index:        0,
					Message:      msg,
					FinishReason: finish,
				},
			},
		},
	}
}

// --- helpers ---

func requireContent(t *testing.T, resp *oai.ChatCompletionResponse) {
	t.Helper()
	if len(resp.Choices) == 0 {
		t.Fatal("no choices in response")
	}
	if resp.Choices[0].Message.StringContent() == "" {
		t.Fatal("empty content in response")
	}
}

func requireFinish(t *testing.T, resp *oai.ChatCompletionResponse, reason string) {
	t.Helper()
	if len(resp.Choices) == 0 {
		t.Fatal("no choices")
	}
	if resp.Choices[0].FinishReason != reason {
		t.Errorf("expected finish_reason=%s, got: %s", reason, resp.Choices[0].FinishReason)
	}
}

func requireToolCalls(t *testing.T, resp *oai.ChatCompletionResponse, name string) {
	t.Helper()
	requireFinish(t, resp, "tool_calls")
	calls := resp.Choices[0].Message.ToolCalls
	if len(calls) == 0 {
		t.Fatalf("expected tool calls, got none. content: %s", resp.Choices[0].Message.StringContent())
	}
	if calls[0].Function.Name != name {
		t.Errorf("expected tool %s, got: %s", name, calls[0].Function.Name)
	}
}

func parseArgs(t *testing.T, tc oai.ToolCall) map[string]any {
	t.Helper()
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Fatalf("invalid tool call arguments JSON: %v\nraw: %s", err, tc.Function.Arguments)
	}
	return args
}

// TestTimingSummary runs all cases in both modes and prints a comparison table.
func TestTimingSummary(t *testing.T) {
	requireCLI(t)
	t.Parallel()
	client := oai.NewClientDefault()
	ctx := context.Background()

	type result struct {
		name              string
		nonStream, stream time.Duration
		pass              bool
	}
	var results []result

	for _, tc := range cases {
		r := result{name: tc.Name, pass: true}

		start := time.Now()
		resp, err := client.CreateChatCompletion(ctx, tc.Request)
		r.nonStream = time.Since(start)
		if err != nil {
			t.Logf("FAIL %s (non-stream): %v", tc.Name, err)
			r.pass = false
		} else {
			func() {
				defer func() {
					if v := recover(); v != nil {
						t.Logf("FAIL %s (non-stream validate): %v", tc.Name, v)
						r.pass = false
					}
				}()
				tc.Validate(t, resp)
			}()
		}

		start = time.Now()
		stream, err := client.CreateChatCompletionStream(ctx, tc.Request)
		if err != nil {
			t.Logf("FAIL %s (stream): %v", tc.Name, err)
			r.pass = false
		} else {
			sr := collectStream(t, stream)
			r.stream = time.Since(start)
			stream.Close()
			func() {
				defer func() {
					if v := recover(); v != nil {
						t.Logf("FAIL %s (stream validate): %v", tc.Name, v)
						r.pass = false
					}
				}()
				tc.Validate(t, sr.Assembled)
			}()
		}

		results = append(results, r)
	}

	t.Logf("\n%-25s %12s %12s %s", "TEST", "NON-STREAM", "STREAM", "STATUS")
	t.Logf("%-25s %12s %12s %s", strings.Repeat("-", 25), "----------", "----------", "------")
	for _, r := range results {
		status := "PASS"
		if !r.pass {
			status = "FAIL"
		}
		t.Logf("%-25s %12s %12s %s",
			r.name,
			r.nonStream.Round(time.Millisecond),
			r.stream.Round(time.Millisecond),
			status)
	}
}
