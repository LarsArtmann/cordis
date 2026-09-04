package cordis

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Level is a log severity. Lower is more severe, mirroring LoggerLevel
// upstream: a message is exported when the target level is greater than or
// equal to the message level.
type Level int

const (
	LevelError Level = 0
	LevelWarn  Level = 1
	LevelInfo  Level = 2
	LevelDebug Level = 3
	// LevelUnset marks an absent level override.
	LevelUnset Level = -1
)

func (l Level) String() string {
	switch l {
	case LevelError:
		return "error"
	case LevelWarn:
		return "warn"
	case LevelInfo:
		return "info"
	case LevelDebug:
		return "debug"
	}
	return "unknown"
}

// Message is one log record, mirroring the Message interface upstream.
type Message struct {
	SN    int64
	Time  time.Time
	Name  string
	Type  string
	Level Level
	Args  []any
}

// Exporter receives log messages. Implementations must be safe for
// concurrent use.
type Exporter interface {
	Export(Message)
}

// ExporterFunc adapts a function to Exporter.
type ExporterFunc func(Message)

func (f ExporterFunc) Export(m Message) { f(m) }

// LoggerIntercept overrides logger name and level for a scope, mirroring the
// logger intercept upstream. Install it with Context.Intercept:
//
//	ctx = ctx.Intercept("logger", cordis.LoggerIntercept{Name: "worker"})
type LoggerIntercept struct {
	Name  string
	Level Level
}

type exporterEntry struct {
	exporter Exporter
	levels   map[string]Level
}

// loggerService is the per-tree logging facility, mirroring LoggerService
// upstream. It fans messages out to registered exporters and keeps a bounded
// in-memory buffer of the most recent messages.
type loggerService struct {
	mu         sync.Mutex
	snMessage  int64
	snExporter int
	exporters  map[int]*exporterEntry

	bufferMu   sync.Mutex
	bufferSize int
	buffer     []Message
}

func newLoggerService() *loggerService {
	s := &loggerService{
		exporters:  make(map[int]*exporterEntry),
		bufferSize: 1000,
	}
	s.AddExporter(ExporterFunc(s.bufferExport))
	return s
}

// bufferExport is the default exporter keeping the last bufferSize messages.
func (s *loggerService) bufferExport(m Message) {
	s.bufferMu.Lock()
	defer s.bufferMu.Unlock()
	s.buffer = append(s.buffer, m)
	if overflow := len(s.buffer) - s.bufferSize; overflow > 0 {
		s.buffer = append([]Message(nil), s.buffer[overflow:]...)
	}
}

// Buffer returns a copy of the most recent messages, oldest first.
func (s *loggerService) Buffer() []Message {
	s.bufferMu.Lock()
	defer s.bufferMu.Unlock()
	return append([]Message(nil), s.buffer...)
}

// SetBufferSize resizes the message buffer, truncating the oldest entries.
func (s *loggerService) SetBufferSize(size int) {
	s.bufferMu.Lock()
	defer s.bufferMu.Unlock()
	s.bufferSize = size
	if overflow := len(s.buffer) - size; overflow > 0 {
		s.buffer = append([]Message(nil), s.buffer[overflow:]...)
	}
}

// AddExporter registers an exporter and returns a Disposer removing it.
// levels maps logger names to their minimum exported level; the "default"
// key applies to every other name. Nil levels export everything.
func (s *loggerService) AddExporter(e Exporter, levels ...map[string]Level) Disposer {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snExporter++
	id := s.snExporter
	entry := &exporterEntry{exporter: e}
	if len(levels) > 0 {
		entry.levels = levels[0]
	}
	s.exporters[id] = entry
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.exporters, id)
	}
}

// ClearExporters removes every exporter, including the default buffer.
func (s *loggerService) ClearExporters() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exporters = make(map[int]*exporterEntry)
}

func (s *loggerService) dispatch(name string, level Level, args []any) {
	s.mu.Lock()
	s.snMessage++
	sn := s.snMessage
	entries := make([]*exporterEntry, 0, len(s.exporters))
	for _, entry := range s.exporters {
		target := LevelInfo
		if entry.levels != nil {
			if l, ok := entry.levels[name]; ok {
				target = l
			} else if l, ok := entry.levels["default"]; ok {
				target = l
			}
		}
		if target < level {
			continue
		}
		entries = append(entries, entry)
	}
	s.mu.Unlock()

	if len(entries) == 0 {
		return
	}
	msg := Message{
		SN:    sn,
		Time:  time.Now(),
		Name:  name,
		Type:  level.String(),
		Level: level,
		Args:  args,
	}
	for _, entry := range entries {
		entry.exporter.Export(msg)
	}
}

// logError is the framework's internal error channel, mirroring
// ctx.logger.error upstream.
func (c *core) logError(name string, err error) {
	if name == "" {
		name = "root"
	}
	c.logger.dispatch(name, LevelError, []any{err})
}

// Logger is a named, leveled logging handle, mirroring Logger upstream.
type Logger struct {
	service *loggerService
	name    string
	level   Level
}

// Logger returns a logger for this context. Name resolution order mirrors
// upstream: an explicit argument wins, then the nearest logger intercept,
// then the name of the fiber owning the context.
func (c *Context) Logger(names ...string) *Logger {
	l := &Logger{service: c.core.logger, level: LevelUnset}
	if len(names) > 0 && names[0] != "" {
		l.name = names[0]
	}
	if intercept, ok := c.Intercepted("logger"); ok {
		if li, ok := intercept.(LoggerIntercept); ok {
			if l.name == "" {
				l.name = li.Name
			}
			l.level = li.Level
		}
	}
	if l.name == "" {
		l.name = c.fiber.Name()
	}
	return l
}

// Name returns the logger name.
func (l *Logger) Name() string { return l.name }

// Error logs at LevelError. A single error argument is formatted with its
// message; wrapped and joined errors are expanded like upstream.
func (l *Logger) Error(args ...any) { l.log(LevelError, args) }

// Warn logs at LevelWarn.
func (l *Logger) Warn(args ...any) { l.log(LevelWarn, args) }

// Info logs at LevelInfo.
func (l *Logger) Info(args ...any) { l.log(LevelInfo, args) }

// Debug logs at LevelDebug.
func (l *Logger) Debug(args ...any) { l.log(LevelDebug, args) }

func (l *Logger) log(level Level, args []any) {
	// Expand a lone error argument the way upstream does: joined errors fan
	// out, wrapped errors unwrap.
	if len(args) == 1 {
		if err, ok := args[0].(error); ok {
			if joined, ok := err.(interface{ Unwrap() []error }); ok {
				for _, inner := range joined.Unwrap() {
					l.log(level, []any{inner})
				}
				return
			}
			if inner := errors.Unwrap(err); inner != nil {
				l.log(level, []any{inner})
				return
			}
		}
	}
	if l.level != LevelUnset && l.level < level {
		return
	}
	l.service.dispatch(l.name, level, args)
}

// FormatMessage renders a message to a single string, mirroring the format
// pipeline upstream: the first argument is a printf style format string
// supporting %s, %d, %i, %f, %o, %O and %% verbs, and remaining arguments
// are appended.
func FormatMessage(m Message) string {
	args := append([]any(nil), m.Args...)
	if len(args) > 0 {
		if err, ok := args[0].(error); ok {
			args[0] = err.Error()
			args = append([]any{"%s"}, args...)
		} else if _, ok := args[0].(string); !ok {
			args = append([]any{"%o"}, args...)
		}
	}
	if len(args) == 0 {
		return ""
	}

	format, _ := args[0].(string)
	args = args[1:]

	var b strings.Builder
	argIndex := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i+1 >= len(format) {
			b.WriteByte(format[i])
			continue
		}
		verb := format[i+1]
		i++
		if verb == '%' {
			b.WriteByte('%')
			continue
		}
		if argIndex >= len(args) {
			b.WriteByte('%')
			b.WriteByte(verb)
			continue
		}
		value := args[argIndex]
		argIndex++
		switch verb {
		case 's':
			fmt.Fprint(&b, value)
		case 'd', 'i':
			fmt.Fprint(&b, truncateNumber(value))
		case 'f':
			fmt.Fprint(&b, toFloat(value))
		case 'o', 'O':
			b.WriteString(toJSON(value))
		case 'c', 'C':
			// Color directives render as plain text in Go.
		default:
			b.WriteByte('%')
			b.WriteByte(verb)
		}
	}
	for ; argIndex < len(args); argIndex++ {
		b.WriteByte(' ')
		if err, ok := args[argIndex].(error); ok {
			b.WriteString(err.Error())
		} else {
			fmt.Fprint(&b, args[argIndex])
		}
	}
	return b.String()
}

func truncateNumber(value any) string {
	switch v := value.(type) {
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case float32:
		return strconv.FormatInt(int64(v), 10)
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return strconv.FormatInt(int64(f), 10)
		}
		return "0"
	}
	return fmt.Sprint(value)
}

func toFloat(value any) string {
	switch v := value.(type) {
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'g', -1, 32)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	}
	return fmt.Sprint(value)
}

func toJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

// ConsoleExporter writes formatted messages to an io.Writer, mirroring the
// logger-console package upstream.
type ConsoleExporter struct {
	W io.Writer
}

// NewConsoleExporter returns an exporter writing to w, or os.Stderr when w
// is nil.
func NewConsoleExporter(w io.Writer) *ConsoleExporter {
	if w == nil {
		w = os.Stderr
	}
	return &ConsoleExporter{W: w}
}

// Export implements Exporter.
func (e *ConsoleExporter) Export(m Message) {
	fmt.Fprintf(e.W, "[%s] %s: %s\n", m.Type, m.Name, FormatMessage(m))
}
