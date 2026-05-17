// Package logger centralizes zap logging for all backend services.
//
// Logs are written both to stdout and to local daily files under
// logs/<service>/<YYYY-MM-DD>/INFO.log and ERR.log. The package also redirects
// the standard library log package into zap so older code paths share the same
// sink.
package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

const (
	defaultLogDir = "logs"
	timeLayout    = "2006-01-02 15:04:05.000"
	dateLayout    = "2006-01-02"
)

var (
	mu      sync.Mutex
	service = "app"
	sugar   *zap.SugaredLogger
	base    *zap.Logger
	sink    *dailySink
)

func init() {
	initLocked("app", resolveLogDir())
}

// InitService initializes logging for one service using CLARAN_LOG_DIR or logs/.
func InitService(name string) {
	InitServiceWithPath(name, resolveLogDir())
}

// InitServiceWithPath initializes logging for one service using an explicit base directory.
func InitServiceWithPath(name, logDir string) {
	mu.Lock()
	defer mu.Unlock()
	initLocked(name, logDir)
}

func initLocked(name, logDir string) {
	if name == "" {
		name = "app"
	}
	if logDir == "" {
		logDir = defaultLogDir
	}
	if base != nil {
		_ = base.Sync()
	}
	if sink != nil {
		_ = sink.Close()
	}

	service = name
	sink = newDailySink(filepath.Join(logDir, name))

	encoderCfg := zap.NewDevelopmentEncoderConfig()
	encoderCfg.TimeKey = "time"
	encoderCfg.LevelKey = "level"
	encoderCfg.NameKey = ""
	encoderCfg.CallerKey = "caller"
	encoderCfg.MessageKey = "msg"
	encoderCfg.EncodeTime = zapcore.TimeEncoderOfLayout(timeLayout)
	encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderCfg.EncodeCaller = zapcore.ShortCallerEncoder

	consoleCore := zapcore.NewCore(
		newServiceConsoleEncoder(name, encoderCfg),
		zapcore.AddSync(os.Stdout),
		zapcore.InfoLevel,
	)
	infoCore := zapcore.NewCore(
		newServiceConsoleEncoder(name, encoderCfg),
		zapcore.AddSync(&levelWriter{sink: sink, target: logTargetInfo}),
		zapcore.InfoLevel,
	)
	errorCore := zapcore.NewCore(
		newServiceConsoleEncoder(name, encoderCfg),
		zapcore.AddSync(&levelWriter{sink: sink, target: logTargetError}),
		zapcore.ErrorLevel,
	)

	base = zap.New(zapcore.NewTee(consoleCore, infoCore, errorCore), zap.AddCaller(), zap.AddCallerSkip(1))
	sugar = base.Sugar()
	zap.ReplaceGlobals(base)
	log.SetFlags(0)
	log.SetOutput(&stdLogWriter{})
}

func resolveLogDir() string {
	if dir := os.Getenv("CLARAN_LOG_DIR"); dir != "" {
		return dir
	}
	return defaultLogDir
}

// Info writes an info-level structured log.
func Info(msg string, fields ...interface{}) {
	current().Infow(msg, normalizeFields(fields...)...)
}

// Warn writes a warning-level structured log.
func Warn(msg string, fields ...interface{}) {
	current().Warnw(msg, normalizeFields(fields...)...)
}

// Error writes an error-level structured log and mirrors it to ERR.log.
func Error(msg string, fields ...interface{}) {
	current().Errorw(msg, normalizeFields(fields...)...)
}

// Fatal writes a fatal log and exits through zap.
func Fatal(msg string, fields ...interface{}) {
	current().Fatalw(msg, normalizeFields(fields...)...)
}

// Sync flushes zap and file sinks.
func Sync() {
	mu.Lock()
	defer mu.Unlock()
	if base != nil {
		_ = base.Sync()
	}
	if sink != nil {
		_ = sink.Sync()
	}
}

func current() *zap.SugaredLogger {
	mu.Lock()
	defer mu.Unlock()
	if sugar == nil {
		initLocked(service, resolveLogDir())
	}
	return sugar
}

func normalizeFields(fields ...interface{}) []interface{} {
	if len(fields)%2 == 0 {
		return fields
	}
	return append(fields, "missing_value")
}

type serviceConsoleEncoder struct {
	appName string
	zapcore.Encoder
}

func newServiceConsoleEncoder(appName string, cfg zapcore.EncoderConfig) zapcore.Encoder {
	return &serviceConsoleEncoder{
		appName: appName,
		Encoder: zapcore.NewConsoleEncoder(cfg),
	}
}

// Clone duplicates the encoder for zap core fanout.
func (e *serviceConsoleEncoder) Clone() zapcore.Encoder {
	return &serviceConsoleEncoder{
		appName: e.appName,
		Encoder: e.Encoder.Clone(),
	}
}

// EncodeEntry prefixes every log line with the service name.
func (e *serviceConsoleEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	buf, err := e.Encoder.EncodeEntry(entry, fields)
	if err != nil {
		return nil, err
	}
	original := buf.String()
	buf.Reset()
	buf.AppendString("[")
	buf.AppendString(e.appName)
	buf.AppendString("] ")
	buf.AppendString(original)
	return buf, nil
}

type dailySink struct {
	mu      sync.Mutex
	baseDir string
	date    string
	info    *os.File
	err     *os.File
}

func newDailySink(baseDir string) *dailySink {
	return &dailySink{baseDir: baseDir}
}

// WriteInfo writes one encoded log line to the current INFO.log file.
func (s *dailySink) WriteInfo(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rotateLocked(time.Now()); err != nil {
		return 0, err
	}
	if s.info == nil {
		return len(p), nil
	}
	if _, err := s.info.Write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// WriteError writes one encoded log line to the current ERR.log file.
func (s *dailySink) WriteError(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.rotateLocked(time.Now()); err != nil {
		return 0, err
	}
	if s.err == nil {
		return len(p), nil
	}
	if _, err := s.err.Write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (s *dailySink) rotateLocked(now time.Time) error {
	date := now.Format(dateLayout)
	if s.date == date && s.info != nil && s.err != nil {
		return nil
	}
	_ = s.closeLocked()
	dir := filepath.Join(s.baseDir, date)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建日志目录失败: %w", err)
	}
	infoFile, err := os.OpenFile(filepath.Join(dir, "INFO.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("打开INFO日志失败: %w", err)
	}
	errFile, err := os.OpenFile(filepath.Join(dir, "ERR.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		_ = infoFile.Close()
		return fmt.Errorf("打开ERR日志失败: %w", err)
	}
	s.date = date
	s.info = infoFile
	s.err = errFile
	return nil
}

// Sync flushes currently open daily files.
func (s *dailySink) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	if s.info != nil {
		firstErr = s.info.Sync()
	}
	if s.err != nil {
		if err := s.err.Sync(); firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// Close closes currently open daily files.
func (s *dailySink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

func (s *dailySink) closeLocked() error {
	var firstErr error
	if s.info != nil {
		firstErr = s.info.Close()
		s.info = nil
	}
	if s.err != nil {
		if err := s.err.Close(); firstErr == nil {
			firstErr = err
		}
		s.err = nil
	}
	return firstErr
}

type levelWriter struct {
	sink   *dailySink
	target logTarget
}

// Write routes zap output to INFO.log or ERR.log.
func (w *levelWriter) Write(p []byte) (int, error) {
	if w.sink == nil {
		return len(p), nil
	}
	if w.target == logTargetError {
		return w.sink.WriteError(p)
	}
	return w.sink.WriteInfo(p)
}

// Sync flushes the underlying daily sink.
func (w *levelWriter) Sync() error {
	if w.sink == nil {
		return nil
	}
	return w.sink.Sync()
}

type stdLogWriter struct{}

type logTarget int

const (
	logTargetInfo logTarget = iota
	logTargetError
)

// Write redirects standard-library log output into the structured logger.
func (w *stdLogWriter) Write(p []byte) (int, error) {
	msg := string(p)
	for len(msg) > 0 && (msg[len(msg)-1] == '\n' || msg[len(msg)-1] == '\r') {
		msg = msg[:len(msg)-1]
	}
	Info(msg)
	return len(p), nil
}
