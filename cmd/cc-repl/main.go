// cc-repl is an interactive REPL that demonstrates the oai.Client agent loop.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/codewandler/cc-sdk-go/oai"
)

const defaultSystemPrompt = "You are a helpful assistant."

func main() {
	var (
		model  = flag.String("model", "", "Model name (e.g. sonnet, opus, haiku)")
		system = flag.String("system", defaultSystemPrompt, "System prompt")
	)
	flag.Parse()

	client := oai.NewClientDefault()

	var history []oai.ChatMessage
	history = append(history, oai.ChatMessage{Role: "system", Content: *system})

	// Read stdin lines in a background goroutine so we can select on signals.
	lines := make(chan string)
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		close(lines)
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		fmt.Print("you> ")

		var line string
		select {
		case l, ok := <-lines:
			if !ok {
				return // EOF
			}
			line = l
		case <-sig:
			fmt.Println()
			return
		}

		switch strings.TrimSpace(line) {
		case "exit", "quit":
			return
		case "":
			continue
		}

		history = append(history, oai.ChatMessage{Role: "user", Content: line})

		// Per-turn context: Ctrl+C cancels the stream, returns to prompt.
		turnCtx, turnCancel := context.WithCancel(context.Background())
		go func() {
			select {
			case <-sig:
				turnCancel()
			case <-turnCtx.Done():
			}
		}()

		err := turn(turnCtx, client, *model, lines, &history)
		turnCancel()

		if err != nil {
			if turnCtx.Err() != nil {
				fmt.Println("\n(interrupted)")
				continue
			}
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", err)
		}
	}
}

// turn sends history to the model, streams the response, and loops on tool calls.
func turn(ctx context.Context, client *oai.Client, model string, lines <-chan string, history *[]oai.ChatMessage) error {
	for {
		req := oai.ChatCompletionRequest{
			Model:    model,
			Messages: *history,
		}

		stream, err := client.CreateChatCompletionStream(ctx, req)
		if err != nil {
			return err
		}

		var (
			text       strings.Builder
			toolCalls  []oai.ToolCall
			finishStop bool
		)

		fmt.Print("assistant> ")
		for {
			chunk, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				stream.Close()
				return err
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			c := chunk.Choices[0]

			if c.Delta.Content != nil {
				fmt.Print(*c.Delta.Content)
				text.WriteString(*c.Delta.Content)
			}
			if len(c.Delta.ToolCalls) > 0 {
				toolCalls = mergeToolCallDeltas(toolCalls, c.Delta.ToolCalls)
			}
			if c.FinishReason != nil {
				finishStop = *c.FinishReason == "stop"
			}
		}
		stream.Close()
		fmt.Println()

		if len(toolCalls) > 0 {
			*history = append(*history, oai.ChatMessage{
				Role:      "assistant",
				Content:   text.String(),
				ToolCalls: toolCalls,
			})
			for _, tc := range toolCalls {
				fmt.Printf("\n[tool_call] %s(%s)\n", tc.Function.Name, tc.Function.Arguments)
				fmt.Printf("result for %s> ", tc.ID)
				select {
				case line, ok := <-lines:
					if !ok {
						return nil
					}
					*history = append(*history, oai.ChatMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    line,
					})
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			continue // auto-send next turn
		}

		// Normal stop — record assistant message and return to user prompt.
		if finishStop || text.Len() > 0 {
			*history = append(*history, oai.ChatMessage{
				Role:    "assistant",
				Content: text.String(),
			})
		}
		return nil
	}
}

// mergeToolCallDeltas accumulates streamed tool call deltas into complete tool calls.
// Each delta may carry a new tool call (with Index and ID set) or append to an
// existing one (same Index, only Function.Arguments populated).
func mergeToolCallDeltas(existing []oai.ToolCall, deltas []oai.ToolCall) []oai.ToolCall {
	for _, d := range deltas {
		if d.ID != "" {
			existing = append(existing, d)
		} else if len(existing) > 0 {
			last := &existing[len(existing)-1]
			last.Function.Arguments += d.Function.Arguments
		}
	}
	return existing
}
