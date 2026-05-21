package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Jasrags/AnotherMUD/internal/conn"
	"github.com/Jasrags/AnotherMUD/internal/logging"
)

// EchoHandler is the M0 handler: greet, echo each line back, recognize
// "quit" as a clean disconnect. Replaced in M1 by the command dispatcher.
func EchoHandler(ctx context.Context, c conn.Connection) error {
	if _, err := c.Write(ctx, []byte("welcome to anothermud (M0 echo)\r\n")); err != nil {
		return fmt.Errorf("greet: %w", err)
	}

	for {
		line, err := c.Read(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, conn.ErrClosed) {
				return nil
			}
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}

		trimmed := strings.TrimSpace(line)
		logging.From(ctx).Debug("input received", "line", trimmed)

		if strings.EqualFold(trimmed, "quit") {
			_, _ = c.Write(ctx, []byte("bye\r\n"))
			return nil
		}

		if _, err := c.Write(ctx, []byte(line+"\r\n")); err != nil {
			return fmt.Errorf("write echo: %w", err)
		}
	}
}
