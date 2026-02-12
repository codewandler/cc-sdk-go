package ccwire

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Parser is a streaming NDJSON parser that reads Claude Code CLI output and
// returns typed [Message] values. It consumes lines from an [io.Reader] and
// deserializes each one into the appropriate concrete message type based on
// the "type" field.
//
// A Parser is not safe for concurrent use. Callers should synchronize access
// externally if multiple goroutines need to read from the same parser.
type Parser struct {
	scanner *bufio.Scanner
}

// NewParser creates a [Parser] that reads NDJSON lines from r. The parser
// allocates a 1 MB initial buffer and allows individual lines up to 10 MB,
// which accommodates large assistant responses and tool results.
func NewParser(r io.Reader) *Parser {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max line
	return &Parser{scanner: scanner}
}

// envelope is used for initial type discrimination.
type envelope struct {
	Type string `json:"type"`
}

// Next reads and returns the next typed [Message] from the NDJSON stream.
// It skips empty lines and lines with unrecognized "type" values.
//
// Next returns [io.EOF] when the underlying reader is exhausted. Parse errors
// on recognized message types are returned as wrapped errors. Malformed lines
// that cannot be unmarshaled into an envelope are silently skipped.
func (p *Parser) Next() (Message, error) {
	for p.scanner.Scan() {
		line := p.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var env envelope
		if err := json.Unmarshal(line, &env); err != nil {
			// Skip malformed lines
			continue
		}

		msg, err := parseTyped(env.Type, line)
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s message: %w", env.Type, err)
		}
		if msg == nil {
			continue
		}
		return msg, nil
	}

	if err := p.scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanner error: %w", err)
	}
	return nil, io.EOF
}

func parseTyped(typ string, data []byte) (Message, error) {
	switch MessageType(typ) {
	case TypeSystem:
		var msg SystemMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil

	case TypeAssistant:
		var msg AssistantMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil

	case TypeResult:
		var msg ResultMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return &msg, nil

	case TypeStreamEvent:
		// Use json.Number for numeric precision
		var raw struct {
			Type      string         `json:"type"`
			Event     map[string]any `json:"event"`
			SessionID string         `json:"session_id"`
		}
		dec := json.NewDecoder(jsonReader(data))
		dec.UseNumber()
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		return &StreamEventMessage{
			Event:     raw.Event,
			SessionID: raw.SessionID,
		}, nil

	default:
		// Unknown message types are silently skipped
		return nil, nil
	}
}

type byteReader struct {
	data []byte
	pos  int
}

func jsonReader(data []byte) io.Reader {
	return &byteReader{data: data}
}

func (r *byteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
