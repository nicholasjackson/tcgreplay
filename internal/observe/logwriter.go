package observe

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
)

// maxLine caps a buffered partial line; anything longer is cut.
const maxLine = 64 << 10

// lineWriter is a second destination of the standard log package: it turns
// each plain playmat log line (start-up, card sets, images, warnings) into
// a log record, at warning severity for lines that start "warning:". It
// never blocks: records go to the batch queue, which drops when full.
type lineWriter struct {
	logger otellog.Logger
	now    func() time.Time

	mu  sync.Mutex
	buf []byte
}

// LogWriter returns the writer to add beside standard error as the std log
// package's output (io.Discard when export is off).
func (t *Telemetry) LogWriter() io.Writer {
	if !t.Enabled() {
		return io.Discard
	}
	return &lineWriter{logger: t.loggerProvider.Logger(scope), now: time.Now}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			room := maxLine - len(w.buf)
			if room > 0 {
				w.buf = append(w.buf, p[:min(room, len(p))]...)
			}
			break
		}
		chunk := p[:i]
		if room := maxLine - len(w.buf); len(chunk) > room {
			chunk = chunk[:max(room, 0)]
		}
		w.emit(string(append(w.buf, chunk...)))
		w.buf = w.buf[:0]
		p = p[i+1:]
	}
	return n, nil
}

// stdPrefix is the length of the std log package's default prefix,
// "2006/01/02 15:04:05 ".
const stdPrefix = len("2006/01/02 15:04:05 ")

// stripStdPrefix drops the std log date and time prefix, if line has one.
func stripStdPrefix(line string) string {
	if len(line) >= stdPrefix {
		if _, err := time.Parse("2006/01/02 15:04:05", line[:stdPrefix-1]); err == nil && line[stdPrefix-1] == ' ' {
			return line[stdPrefix:]
		}
	}
	return line
}

func (w *lineWriter) emit(line string) {
	line = stripStdPrefix(line)
	if line == "" {
		return
	}
	var rec otellog.Record
	now := w.now()
	rec.SetTimestamp(now)
	rec.SetObservedTimestamp(now)
	if strings.HasPrefix(line, "warning:") {
		rec.SetSeverity(otellog.SeverityWarn)
		rec.SetSeverityText("WARN")
	} else {
		rec.SetSeverity(otellog.SeverityInfo)
		rec.SetSeverityText("INFO")
	}
	rec.SetBody(attribute.StringValue(line))
	rec.AddAttributes(attribute.String("log.source", "log"))
	w.logger.Emit(context.Background(), rec)
}
