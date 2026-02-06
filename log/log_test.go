package log

import (
	"context"
	"testing"
)

func init() {
	ProcessName = "log-testing"
}

type TestLoggerContainer struct {
	elements []struct {
		msg   string
		level string
		attrs map[string]any
	}
}

func (tlc *TestLoggerContainer) add(msg string, level string, attrs map[string]any) {
	tlc.elements = append(tlc.elements, struct {
		msg   string
		level string
		attrs map[string]any
	}{msg, level, attrs})
}

func testLogger(ctx context.Context) (context.Context, *TestLoggerContainer) {
	tlc := &TestLoggerContainer{}
	ctx = LoggerNewContext(ctx)
	ctx = LoggerAddHandler(ctx, func(msg string, level string, attrs map[string]any) error {
		tlc.add(msg, level, attrs)
		return nil
	})
	return ctx, tlc
}

func TestTestLogger(t *testing.T) {
	msg := "testing1234"
	ctx, tlc := testLogger(context.Background())
	LoggerWContext(ctx).Info(msg)
	if len(tlc.elements) == 0 {
		t.Fatal("test logger didn't capture any messages")
	}
	if tlc.elements[0].msg != msg {
		t.Error("test logger isn't logging to the testing backend")
	}
}

func TestLeveledLogger(t *testing.T) {
	msg := "testing1234"
	ctx, tlc := testLogger(context.Background())

	ctx = LoggerSetLevel(ctx, "info")
	LoggerWContext(ctx).Debug(msg)
	if len(tlc.elements) > 0 {
		t.Error("Debug message was logged although the level is info")
	}

	ctx = LoggerSetLevel(ctx, "debug")
	LoggerWContext(ctx).Debug(msg)
	if len(tlc.elements) == 0 {
		t.Fatal("Debug message wasn't logged although the level is debug")
	}
	if tlc.elements[0].msg != msg {
		t.Error("Debug message wasn't logged although the level is debug")
	}
}

func TestLoggerDebugFunc(t *testing.T) {
	msg := "testing1234"
	ctx, tlc := testLogger(context.Background())

	ctx = LoggerSetLevel(ctx, "info")
	LoggerDebugFunc(ctx, func() string {
		return msg
	})
	if len(tlc.elements) > 0 {
		t.Error("Debug message was logged although the level is info")
	}

	ctx = LoggerSetLevel(ctx, "debug")
	LoggerDebugFunc(ctx, func() string {
		return msg
	})
	if len(tlc.elements) == 0 {
		t.Fatal("Debug message wasn't logged although the level is debug")
	}
	if tlc.elements[0].msg != msg {
		t.Error("Debug message wasn't logged although the level is debug")
	}
}

func TestLoggerNewRequest(t *testing.T) {
	ctx, tlc := testLogger(context.Background())
	ctx = LoggerNewRequest(ctx)

	LoggerWContext(ctx).Info("test request")
	if len(tlc.elements) == 0 {
		t.Fatal("logger didn't capture any messages")
	}

	attrs := tlc.elements[0].attrs
	if _, ok := attrs[RequestUuidKey]; !ok {
		t.Error("request UUID not found in log attributes after calling LoggerNewRequest")
	}
}

func TestAddToLogContext(t *testing.T) {
	testKey := "testkey"
	testVal := "testVal"

	ctx, tlc := testLogger(context.Background())
	changedCtx := AddToLogContext(ctx, testKey, testVal)

	LoggerWContext(changedCtx).Info("test context")
	if len(tlc.elements) == 0 {
		t.Fatal("logger didn't capture any messages")
	}

	attrs := tlc.elements[0].attrs
	if v, ok := attrs[testKey]; ok {
		if v != testVal {
			t.Errorf("additional element value is %v, expected %v", v, testVal)
		}
	} else {
		t.Error("additional element that was added isn't part of the log attributes")
	}

	// Original ctx shouldn't include the extra field
	tlc.elements = nil
	LoggerWContext(ctx).Info("test original")
	if len(tlc.elements) == 0 {
		t.Fatal("logger didn't capture any messages")
	}
	attrs = tlc.elements[0].attrs
	if _, ok := attrs[testKey]; ok {
		t.Error("key was written to context that shouldn't have been changed (original one)")
	}
}

func TestLoggerGetLevel(t *testing.T) {
	ctx := LoggerNewContext(context.Background())

	if level := LoggerGetLevel(ctx); level != "info" {
		t.Errorf("expected default level 'info', got '%s'", level)
	}

	ctx = LoggerSetLevel(ctx, "debug")
	if level := LoggerGetLevel(ctx); level != "debug" {
		t.Errorf("expected level 'debug', got '%s'", level)
	}

	ctx = LoggerSetLevel(ctx, "warn")
	if level := LoggerGetLevel(ctx); level != "warn" {
		t.Errorf("expected level 'warn', got '%s'", level)
	}
}

func TestLogLevels(t *testing.T) {
	ctx, tlc := testLogger(context.Background())
	ctx = LoggerSetLevel(ctx, "warn")

	LoggerWContext(ctx).Info("should not appear")
	if len(tlc.elements) > 0 {
		t.Error("Info message was logged although the level is warn")
	}

	LoggerWContext(ctx).Warn("should appear")
	if len(tlc.elements) == 0 {
		t.Error("Warn message wasn't logged although the level is warn")
	}

	LoggerWContext(ctx).Error("should also appear")
	if len(tlc.elements) != 2 {
		t.Errorf("expected 2 messages, got %d", len(tlc.elements))
	}
}

func TestLoggerCrit(t *testing.T) {
	ctx, tlc := testLogger(context.Background())
	LoggerWContext(ctx).Crit("critical message")
	if len(tlc.elements) == 0 {
		t.Fatal("Crit message wasn't logged")
	}
	if tlc.elements[0].attrs["severity"] != "critical" {
		t.Error("Crit should add severity=critical attribute")
	}
}

func TestTransferLogContext(t *testing.T) {
	srcCtx, _ := testLogger(context.Background())
	srcCtx = AddToLogContext(srcCtx, "key1", "val1")
	srcCtx = LoggerSetLevel(srcCtx, "debug")

	destCtx := context.Background()
	destCtx = TranferLogContext(srcCtx, destCtx)

	if level := LoggerGetLevel(destCtx); level != "debug" {
		t.Errorf("expected transferred level 'debug', got '%s'", level)
	}
}

func TestConvenienceLogFunctions(t *testing.T) {
	ctx, tlc := testLogger(context.Background())

	LogInfo(ctx, "info msg")
	if len(tlc.elements) == 0 || tlc.elements[0].msg != "info msg" {
		t.Error("LogInfo failed")
	}

	LogWarn(ctx, "warn msg")
	if len(tlc.elements) < 2 || tlc.elements[1].msg != "warn msg" {
		t.Error("LogWarn failed")
	}

	LogError(ctx, "error msg")
	if len(tlc.elements) < 3 || tlc.elements[2].msg != "error msg" {
		t.Error("LogError failed")
	}

	LogInfof(ctx, "formatted %s", "info")
	if len(tlc.elements) < 4 || tlc.elements[3].msg != "formatted info" {
		t.Error("LogInfof failed")
	}
}

func TestDie(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			if r != "die" {
				t.Error("Die didn't die with the right message")
			}
		} else {
			t.Error("Die didn't die properly")
		}
	}()
	Die("die")
}
