package sink

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/fadesany/Stellar-Trade-Processor/event"
)

// JSONLineSink writes each event as a single JSON object followed by a
// newline.
type JSONLineSink struct {
	buf *bufio.Writer
	enc *json.Encoder
	dst io.Writer
}

// NewJSONLineSink returns a sink writing JSON lines to w. If w is nil, events
// are written to os.Stdout. Output is flushed after every Write.
func NewJSONLineSink(w io.Writer) *JSONLineSink {
	if w == nil {
		w = os.Stdout
	}
	buf := bufio.NewWriter(w)
	return &JSONLineSink{buf: buf, enc: json.NewEncoder(buf), dst: w}
}

// Write encodes each event as one JSON line and flushes the batch.
func (s *JSONLineSink) Write(events []event.TradeEvent) error {
	for i := range events {
		if err := s.enc.Encode(&events[i]); err != nil {
			return fmt.Errorf("encoding trade event %s op %d: %w", events[i].TxHash, events[i].OpIndex, err)
		}
	}
	if err := s.buf.Flush(); err != nil {
		return fmt.Errorf("flushing json lines: %w", err)
	}
	return nil
}

// Close flushes buffered output. It closes the destination only if it
// implements io.Closer and is not os.Stdout or os.Stderr.
func (s *JSONLineSink) Close() error {
	if err := s.buf.Flush(); err != nil {
		return fmt.Errorf("flushing json lines: %w", err)
	}
	if s.dst == os.Stdout || s.dst == os.Stderr {
		return nil
	}
	if c, ok := s.dst.(io.Closer); ok {
		if err := c.Close(); err != nil {
			return fmt.Errorf("closing json line destination: %w", err)
		}
	}
	return nil
}
