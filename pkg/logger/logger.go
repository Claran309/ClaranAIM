// Package logger 集中初始化所有后端服务的 zap 日志。
// 日志会同时输出到控制台和本地按日期分片的文件：普通日志写入 logs/<service>/<YYYY-MM-DD>/INFO.log，
// 所有服务的错误日志统一写入 logs/ERR/<YYYY-MM-DD>/ERR.log，便于排查“哪个服务先报错”。
// 这里还会接管标准库 log 输出，使旧代码路径也进入同一套日志收集链路。
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

// 下面这组常量定义当前包使用的固定取值，集中声明可以避免业务代码中散落魔法字符串或魔法数字。
const (
	defaultLogDir = "logs"
	timeLayout    = "2006-01-02 15:04:05.000"
	dateLayout    = "2006-01-02"
)

// 下面这组变量保存当前包需要复用的运行时状态或配置入口，调用方应通过公开函数间接使用。
var (
	mu      sync.Mutex
	service = "app"
	sugar   *zap.SugaredLogger
	base    *zap.Logger
	sink    *dailySink
)

// init 是当前包内部使用的函数，用于拆分主流程中的局部业务步骤，避免调用方直接依赖实现细节。
func init() {
	initLocked("app", resolveLogDir())
}

// InitService 使用 CLARAN_LOG_DIR 或默认 logs/ 目录初始化某个服务的日志。
func InitService(name string) {
	InitServiceWithPath(name, resolveLogDir())
}

// InitServiceWithPath 使用显式日志根目录初始化某个服务的日志，测试或本地调试可指定临时目录。
func InitServiceWithPath(name, logDir string) {
	mu.Lock()
	defer mu.Unlock()
	initLocked(name, logDir)
}

// initLocked 在持有全局锁时重建 zap logger、文件 sink 和标准库 log 重定向。
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
	sink = newDailySink(filepath.Join(logDir, name), filepath.Join(logDir, "ERR"))

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

// resolveLogDir 读取日志根目录环境变量，未配置时回退到项目内 logs 目录。
func resolveLogDir() string {
	if dir := os.Getenv("CLARAN_LOG_DIR"); dir != "" {
		return dir
	}
	return defaultLogDir
}

// Info 写入 info 级别结构化日志。
func Info(msg string, fields ...interface{}) {
	current().Infow(msg, normalizeFields(fields...)...)
}

// Warn 写入 warn 级别结构化日志。
func Warn(msg string, fields ...interface{}) {
	current().Warnw(msg, normalizeFields(fields...)...)
}

// Error 写入 error 级别结构化日志，并同步进入统一 ERR.log。
func Error(msg string, fields ...interface{}) {
	current().Errorw(msg, normalizeFields(fields...)...)
}

// Fatal 写入 fatal 日志并交由 zap 结束进程。
func Fatal(msg string, fields ...interface{}) {
	current().Fatalw(msg, normalizeFields(fields...)...)
}

// Sync 刷新 zap 和当前打开的日志文件，服务退出前调用可减少日志丢失。
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

// current 返回当前全局 SugaredLogger；如果尚未初始化则用默认配置补初始化。
func current() *zap.SugaredLogger {
	mu.Lock()
	defer mu.Unlock()
	if sugar == nil {
		initLocked(service, resolveLogDir())
	}
	return sugar
}

// normalizeFields 兜底处理奇数个字段参数，避免 zap 因缺少 value 产生额外错误。
func normalizeFields(fields ...interface{}) []interface{} {
	if len(fields)%2 == 0 {
		return fields
	}
	return append(fields, "missing_value")
}

// serviceConsoleEncoder 在 zap 原始 encoder 外增加服务名前缀，使聚合日志更容易区分来源。
type serviceConsoleEncoder struct {
	appName string
	zapcore.Encoder
}

// newServiceConsoleEncoder 创建带服务名前缀的控制台 encoder。
func newServiceConsoleEncoder(appName string, cfg zapcore.EncoderConfig) zapcore.Encoder {
	return &serviceConsoleEncoder{
		appName: appName,
		Encoder: zapcore.NewConsoleEncoder(cfg),
	}
}

// Clone 为 zap 多 core 输出复制 encoder，避免多个输出共享可变缓冲状态。
func (e *serviceConsoleEncoder) Clone() zapcore.Encoder {
	return &serviceConsoleEncoder{
		appName: e.appName,
		Encoder: e.Encoder.Clone(),
	}
}

// EncodeEntry 在每行日志前加上 [service] 前缀。
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

// dailySink 管理按日期切分的 INFO 与 ERR 日志文件句柄。
type dailySink struct {
	mu      sync.Mutex
	infoDir string
	errDir  string
	date    string
	info    *os.File
	err     *os.File
}

// newDailySink 创建按日期滚动的文件 sink。
func newDailySink(infoDir, errDir string) *dailySink {
	return &dailySink{infoDir: infoDir, errDir: errDir}
}

// WriteInfo 将一行编码后的日志写入当天 INFO.log。
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

// WriteError 将一行编码后的错误日志写入当天统一 ERR.log。
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

// rotateLocked 在日期变化时关闭旧文件并打开当天的新 INFO/ERR 文件。
func (s *dailySink) rotateLocked(now time.Time) error {
	date := now.Format(dateLayout)
	if s.date == date && s.info != nil && s.err != nil {
		return nil
	}
	_ = s.closeLocked()
	infoDir := filepath.Join(s.infoDir, date)
	errDir := filepath.Join(s.errDir, date)
	if err := os.MkdirAll(infoDir, 0o755); err != nil {
		return fmt.Errorf("创建INFO日志目录失败: %w", err)
	}
	if err := os.MkdirAll(errDir, 0o755); err != nil {
		return fmt.Errorf("创建ERR日志目录失败: %w", err)
	}
	infoFile, err := os.OpenFile(filepath.Join(infoDir, "INFO.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("打开INFO日志失败: %w", err)
	}
	errFile, err := os.OpenFile(filepath.Join(errDir, "ERR.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		_ = infoFile.Close()
		return fmt.Errorf("打开ERR日志失败: %w", err)
	}
	s.date = date
	s.info = infoFile
	s.err = errFile
	return nil
}

// Sync 刷新当前打开的每日日志文件。
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

// Close 关闭当前打开的每日日志文件。
func (s *dailySink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closeLocked()
}

// closeLocked 在持锁状态下关闭文件句柄，供滚动和退出流程复用。
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

// levelWriter 把 zap core 的输出分流到 INFO.log 或 ERR.log。
type levelWriter struct {
	sink   *dailySink
	target logTarget
}

// Write 根据目标级别把 zap 输出写入对应日志文件。
func (w *levelWriter) Write(p []byte) (int, error) {
	if w.sink == nil {
		return len(p), nil
	}
	if w.target == logTargetError {
		return w.sink.WriteError(p)
	}
	return w.sink.WriteInfo(p)
}

// Sync 刷新底层 dailySink。
func (w *levelWriter) Sync() error {
	if w.sink == nil {
		return nil
	}
	return w.sink.Sync()
}

// stdLogWriter 把标准库 log 输出桥接到项目 logger。
type stdLogWriter struct{}

// logTarget 标识 levelWriter 的目标文件类型。
type logTarget int

// 下面这组常量定义当前包使用的固定取值，集中声明可以避免业务代码中散落魔法字符串或魔法数字。
const (
	logTargetInfo logTarget = iota
	logTargetError
)

// Write 将标准库 log 的一行文本重定向为结构化 info 日志。
func (w *stdLogWriter) Write(p []byte) (int, error) {
	msg := string(p)
	for len(msg) > 0 && (msg[len(msg)-1] == '\n' || msg[len(msg)-1] == '\r') {
		msg = msg[:len(msg)-1]
	}
	Info(msg)
	return len(p), nil
}
