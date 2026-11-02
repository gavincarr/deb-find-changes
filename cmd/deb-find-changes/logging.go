package main

import (
	"io"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
	"golang.org/x/term"
)

// newLogger returns a tint-backed logger writing to w. Level is Debug for
// verbose>=2, Info for verbose==1, otherwise Warn (Error when !warnings).
func newLogger(w io.Writer, verbose int, warnings bool) *slog.Logger {
	level := slog.LevelWarn
	switch {
	case verbose >= 2:
		level = slog.LevelDebug
	case verbose == 1:
		level = slog.LevelInfo
	case !warnings:
		level = slog.LevelError
	}
	noColor := true
	if f, ok := w.(*os.File); ok {
		noColor = !term.IsTerminal(int(f.Fd()))
	}
	return slog.New(tint.NewHandler(w, &tint.Options{Level: level, NoColor: noColor}))
}
