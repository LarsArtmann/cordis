package cordis

import (
	"context"
	"log/slog"
	"strings"
)

// SlogHandler is a slog.Handler that routes records into the context tree's
// logger service, so every cordis exporter (buffer, console, custom)
// receives slog output and per name level intercepts keep working:
//
//	logger := slog.New(cordis.NewSlogHandler(ctx, "worker"))
//	logger.Info("starting", "port", 8080)
//
// Levels map onto the cordis bands (lower slog levels clamp to Debug), the
// slog handler name becomes the cordis logger name and WithAttrs/WithGroup
// are carried into the message arguments.
type SlogHandler struct {
	service *loggerService
	name    string
	target  Level
	attrs   []string
	group   string
}

// NewSlogHandler returns a SlogHandler bound to the logger service of ctx's
// tree. The logger name resolves like Context.Logger: an explicit name wins,
// then the nearest logger intercept, then the fiber's name. Records below
// the effective level (intercept level, else Info) are dropped before they
// reach any exporter.
func NewSlogHandler(ctx *Context, names ...string) *SlogHandler {
	l := ctx.Logger(names...)
	target := l.level
	if target == LevelUnset {
		target = LevelInfo
	}
	return &SlogHandler{service: l.service, name: l.name, target: target}
}

// Slog returns a *slog.Logger routing into the cordis logger service through
// this logger's name and level override.
func (l *Logger) Slog() *slog.Logger {
	target := l.level
	if target == LevelUnset {
		target = LevelInfo
	}
	return slog.New(&SlogHandler{service: l.service, name: l.name, target: target})
}

// Enabled implements slog.Handler.
func (h *SlogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return cordisLevel(level) <= h.target
}

// Handle implements slog.Handler. The record renders as one cordis message:
// the record message followed by key=value arguments.
func (h *SlogHandler) Handle(_ context.Context, r slog.Record) error {
	level := cordisLevel(r.Level)
	attrs := make([]string, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = appendAttr(attrs, h.group, a)
		return true
	})
	args := make([]any, 1, 2+len(attrs))
	args[0] = "%s"
	args = append(args, r.Message)
	for _, a := range attrs {
		args = append(args, a)
	}
	h.service.dispatch(h.name, level, args)
	return nil
}

// WithAttrs implements slog.Handler.
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	for _, a := range attrs {
		clone.attrs = appendAttr(clone.attrs, h.group, a)
	}
	return &clone
}

// WithGroup implements slog.Handler.
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	clone := *h
	if name == "" {
		return &clone
	}
	if clone.group == "" {
		clone.group = name
	} else {
		clone.group += "." + name
	}
	return &clone
}

// cordisLevel maps a slog level onto the four cordis bands.
func cordisLevel(level slog.Level) Level {
	switch {
	case level >= slog.LevelError:
		return LevelError
	case level >= slog.LevelWarn:
		return LevelWarn
	case level >= slog.LevelInfo:
		return LevelInfo
	default:
		return LevelDebug
	}
}

// appendAttr renders a slog.Attr as "key=value", joining group prefixes and
// flattening group attrs. Empty group keys inline their members, mirroring
// slog.
func appendAttr(dst []string, prefix string, a slog.Attr) []string {
	if a.Value.Kind() == slog.KindGroup {
		group := a.Value.Group()
		next := prefix
		if a.Key != "" {
			if next == "" {
				next = a.Key
			} else {
				next += "." + a.Key
			}
		}
		for _, member := range group {
			dst = appendAttr(dst, next, member)
		}
		return dst
	}
	key := a.Key
	if prefix != "" {
		key = prefix + "." + key
	}
	var b strings.Builder
	b.WriteString(key)
	b.WriteByte('=')
	b.WriteString(a.Value.String())
	return append(dst, b.String())
}
