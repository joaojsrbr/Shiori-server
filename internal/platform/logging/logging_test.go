package logging_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/joaojsr/shiori-server/internal/platform/logging"
)

func TestSetupJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(&buf, "info", "json")

	logger.Info("test message", "key", "value")

	out := buf.String()
	if !strings.Contains(out, `"msg":"test message"`) {
		t.Errorf("JSON output missing msg field: %s", out)
	}
	if !strings.Contains(out, `"key":"value"`) {
		t.Errorf("JSON output missing key field: %s", out)
	}
}

func TestSetupText(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(&buf, "info", "text")

	logger.Info("hello world")

	out := buf.String()
	if !strings.Contains(out, "hello world") {
		t.Errorf("text output missing message: %s", out)
	}
}

func TestDebugFilteredAtInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(&buf, "info", "json")

	logger.Debug("should not appear")

	if buf.Len() != 0 {
		t.Errorf("debug message should be filtered at info level, got: %s", buf.String())
	}
}

func TestDebugVisibleAtDebug(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(&buf, "debug", "json")

	logger.Debug("should appear")

	if buf.Len() == 0 {
		t.Error("debug message should be visible at debug level")
	}
}
