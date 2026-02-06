package log

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Context keys for logger state
type ctxKey int

const (
	loggerCtxKey ctxKey = iota
	requestUuidCtxKey
	attrsCtxKey
)

// Public context key strings (kept for backward compatibility)
const RequestUuidKey = "request-uuid"
const ProcessPidKey = "pid"
const LoggerKey = "logger"
const LogLevel = "loglevel"

// ProcessName is set via SetProcessName and included in syslog output
var ProcessName = os.Args[0]

// EgLogger is the primary logging interface returned by LoggerWContext.
// It wraps zap.SugaredLogger for high-performance structured logging.
type EgLogger struct {
	sugar *zap.SugaredLogger
}

func (l EgLogger) Info(msg string, args ...any)  { l.sugar.Infow(msg, args...) }
func (l EgLogger) Debug(msg string, args ...any) { l.sugar.Debugw(msg, args...) }
func (l EgLogger) Warn(msg string, args ...any)  { l.sugar.Warnw(msg, args...) }
func (l EgLogger) Error(msg string, args ...any) { l.sugar.Errorw(msg, args...) }
func (l EgLogger) Crit(msg string, args ...any) {
	args = append(args, "severity", "critical")
	l.sugar.Errorw(msg, args...)
}

// New returns a child logger with additional key-value pairs.
func (l EgLogger) New(args ...any) EgLogger {
	return EgLogger{sugar: l.sugar.With(args...)}
}

// Sync flushes any buffered log entries.
func (l EgLogger) Sync() error {
	return l.sugar.Sync()
}

// loggerState holds the per-context logger configuration.
type loggerState struct {
	core    zapcore.Core
	level   zap.AtomicLevel
	pid     string
	inDebug bool
	name    string // level name for GetLevel
}

type logField struct {
	key string
	val any
}

var (
	defaultCore     zapcore.Core
	defaultCoreOnce sync.Once
)

func getDefaultCore() zapcore.Core {
	defaultCoreOnce.Do(func() {
		defaultCore = getLogBackend()
	})
	return defaultCore
}

// SetProcessName sets the process name used in log output.
func SetProcessName(name string) {
	ProcessName = name
	// Reset default core so it picks up new process name for syslog
	defaultCoreOnce = sync.Once{}
}

// LoggerNewContext initializes a new logger in the context with the current PID.
func LoggerNewContext(ctx context.Context) context.Context {
	lvl := zap.NewAtomicLevelAt(zap.InfoLevel)

	core := getDefaultCore()

	state := &loggerState{
		core: core,
		level: lvl,
		pid:  strconv.Itoa(os.Getpid()),
		name: "info",
	}

	// Check LOG_LEVEL env var
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		setLevel(state, envLevel)
	}

	ctx = context.WithValue(ctx, loggerCtxKey, state)
	ctx = context.WithValue(ctx, attrsCtxKey, []logField{})
	return ctx
}

// LoggerDummyContext returns a context with an initialized logger (empty context).
func LoggerDummyContext() context.Context {
	return LoggerNewContext(context.Background())
}

// Logger returns a logger without a specific context.
func Logger() EgLogger {
	return LoggerWContext(LoggerDummyContext())
}

// LoggerWContext returns a logger initialized with values from the context
// (pid, request UUID, and additional fields). Extra args are added as key-value pairs.
func LoggerWContext(ctx context.Context, args ...any) EgLogger {
	state := stateFromContext(ctx)
	if state == nil {
		ctx = LoggerNewContext(ctx)
		state = stateFromContext(ctx)
	}

	// Build the leveled core
	leveledCore := &levelFilterCore{
		inner: state.core,
		level: state.level,
	}

	logger := zap.New(leveledCore)
	sugar := logger.Sugar()

	// Always add PID
	baseArgs := []any{ProcessPidKey, state.pid}

	// Add request UUID if present
	if uid := ctx.Value(requestUuidCtxKey); uid != nil {
		baseArgs = append(baseArgs, RequestUuidKey, uid)
	}

	// Add additional context fields
	if fields, ok := ctx.Value(attrsCtxKey).([]logField); ok {
		for _, f := range fields {
			baseArgs = append(baseArgs, f.key, f.val)
		}
	}

	// Add caller-supplied args
	baseArgs = append(baseArgs, args...)

	return EgLogger{sugar: sugar.With(baseArgs...)}
}

// LoggerSetLevel sets the log level for the logger in the context.
// Valid levels: debug, info, warn, error, crit.
func LoggerSetLevel(ctx context.Context, levelStr string) context.Context {
	state := stateFromContext(ctx)
	if state == nil {
		ctx = LoggerNewContext(ctx)
		state = stateFromContext(ctx)
	}

	// Copy state to avoid mutating shared state
	newState := &loggerState{
		core:  state.core,
		level: zap.NewAtomicLevel(),
		pid:   state.pid,
	}

	setLevel(newState, levelStr)

	return context.WithValue(ctx, loggerCtxKey, newState)
}

// LoggerGetLevel returns the current log level name from the context.
func LoggerGetLevel(ctx context.Context) string {
	state := stateFromContext(ctx)
	if state == nil {
		return "info"
	}
	return state.name
}

// LoggerAddHandler adds an additional handler to the logger.
// The function receives message, level name, and attributes map for each log entry.
func LoggerAddHandler(ctx context.Context, f func(msg string, level string, attrs map[string]any) error) context.Context {
	state := stateFromContext(ctx)
	if state == nil {
		ctx = LoggerNewContext(ctx)
		state = stateFromContext(ctx)
	}

	newState := &loggerState{
		core: zapcore.NewTee(
			state.core,
			&funcCore{fn: f},
		),
		level:   state.level,
		pid:     state.pid,
		inDebug: state.inDebug,
		name:    state.name,
	}

	return context.WithValue(ctx, loggerCtxKey, newState)
}

// LoggerNewRequest generates a new UUID for request tracking and adds it to the context.
func LoggerNewRequest(ctx context.Context) context.Context {
	u, _ := uuid.NewUUID()
	return context.WithValue(ctx, requestUuidCtxKey, u.String())
}

// GetRequestUuid returns the request UUID from the context, or empty string if not set.
func GetRequestUuid(ctx context.Context) string {
	if uid, ok := ctx.Value(requestUuidCtxKey).(string); ok {
		return uid
	}
	return ""
}

// AddToLogContext adds key-value pairs to the context's additional log fields.
// Args should be tuples: "key1", "val1", "key2", "val2".
func AddToLogContext(ctx context.Context, args ...any) context.Context {
	if len(args)%2 != 0 {
		return ctx
	}

	existing, _ := ctx.Value(attrsCtxKey).([]logField)

	// Copy existing fields (immutable context pattern)
	newFields := make([]logField, len(existing), len(existing)+len(args)/2)
	copy(newFields, existing)

	for i := 0; i < len(args)-1; i += 2 {
		key, ok := args[i].(string)
		if !ok {
			key = strings.Replace(strings.TrimSpace(args[i].(string)), " ", "_", -1)
		}
		newFields = append(newFields, logField{key: key, val: args[i+1]})
	}

	return context.WithValue(ctx, attrsCtxKey, newFields)
}

// TranferLogContext copies logger state from one context to another.
func TranferLogContext(sourceCtx context.Context, destCtx context.Context) context.Context {
	if state := stateFromContext(sourceCtx); state != nil {
		destCtx = context.WithValue(destCtx, loggerCtxKey, state)
	}
	if fields, ok := sourceCtx.Value(attrsCtxKey).([]logField); ok {
		copied := make([]logField, len(fields))
		copy(copied, fields)
		destCtx = context.WithValue(destCtx, attrsCtxKey, copied)
	}
	return destCtx
}

// LoggerDebugFunc executes f only if the logger is in debug mode.
// If f returns a non-empty string, it is logged at debug level.
func LoggerDebugFunc(ctx context.Context, f func() string) {
	state := stateFromContext(ctx)
	if state != nil && state.inDebug {
		if msg := f(); msg != "" {
			LoggerWContext(ctx).Debug(msg)
		}
	}
}

// Die logs a critical message and panics.
func Die(msg string, args ...any) {
	Logger().Crit(msg, args...)
	panic(msg)
}

// --- internal helpers ---

func stateFromContext(ctx context.Context) *loggerState {
	if v := ctx.Value(loggerCtxKey); v != nil {
		return v.(*loggerState)
	}
	return nil
}

func setLevel(state *loggerState, levelStr string) {
	levelStr = strings.ToLower(levelStr)
	state.name = levelStr
	state.inDebug = levelStr == "debug"

	switch levelStr {
	case "debug":
		state.level.SetLevel(zap.DebugLevel)
	case "info":
		state.level.SetLevel(zap.InfoLevel)
	case "warn":
		state.level.SetLevel(zap.WarnLevel)
	case "error", "crit":
		state.level.SetLevel(zap.ErrorLevel)
	default:
		state.level.SetLevel(zap.InfoLevel)
	}
}

// levelFilterCore wraps a zapcore.Core with level filtering.
type levelFilterCore struct {
	inner zapcore.Core
	level zap.AtomicLevel
}

func (c *levelFilterCore) Enabled(lvl zapcore.Level) bool {
	return c.level.Enabled(lvl)
}

func (c *levelFilterCore) With(fields []zapcore.Field) zapcore.Core {
	return &levelFilterCore{inner: c.inner.With(fields), level: c.level}
}

func (c *levelFilterCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.level.Enabled(ent.Level) {
		return c.inner.Check(ent, ce)
	}
	return ce
}

func (c *levelFilterCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	return c.inner.Write(ent, fields)
}

func (c *levelFilterCore) Sync() error {
	return c.inner.Sync()
}

// funcCore adapts a function to zapcore.Core for LoggerAddHandler compatibility.
type funcCore struct {
	fn     func(msg string, level string, attrs map[string]any) error
	fields []zapcore.Field
}

func (c *funcCore) Enabled(_ zapcore.Level) bool { return true }

func (c *funcCore) With(fields []zapcore.Field) zapcore.Core {
	newFields := make([]zapcore.Field, len(c.fields), len(c.fields)+len(fields))
	copy(newFields, c.fields)
	newFields = append(newFields, fields...)
	return &funcCore{fn: c.fn, fields: newFields}
}

func (c *funcCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(ent, c)
}

func (c *funcCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	attrs := make(map[string]any)
	// Add accumulated fields first
	for _, f := range c.fields {
		attrs[f.Key] = fieldValue(f)
	}
	// Add entry-level fields
	for _, f := range fields {
		attrs[f.Key] = fieldValue(f)
	}
	return c.fn(ent.Message, ent.Level.String(), attrs)
}

func (c *funcCore) Sync() error { return nil }

// fieldValue extracts the Go value from a zapcore.Field.
func fieldValue(f zapcore.Field) any {
	switch f.Type {
	case zapcore.StringType:
		return f.String
	case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
		return f.Integer
	case zapcore.BoolType:
		return f.Integer == 1
	case zapcore.Float64Type, zapcore.Float32Type:
		return f.Interface
	default:
		if f.Interface != nil {
			return f.Interface
		}
		return f.String
	}
}
