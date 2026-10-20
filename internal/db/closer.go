package db

import (
	"io"
	"log/slog"
)

// tryClose closes v and logs a failure, so it never masks the caller's error.
func tryClose(v io.Closer) {
	if err := v.Close(); err != nil {
		slog.Error("db: failed to close", "err", err)
	}
}
