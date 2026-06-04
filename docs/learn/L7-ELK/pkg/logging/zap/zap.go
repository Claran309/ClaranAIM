package log

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/buffer"
	"go.uber.org/zap/zapcore"
)

// 颜色配置
const (
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorReset  = "\033[0m"
)

// MyEncoder 自定义解码器模型
type MyEncoder struct {
	AppName string
	zapcore.Encoder
	errFile *os.File
	writer  MyLogWriter
}

// MyLogWriter 自定义日志文件写入器模型
type MyLogWriter struct {
	mu      sync.Mutex
	logDate string
	file    *os.File
	logPath string
}

// 自定义解码器
func (m *MyEncoder) EncodeEntry(entry zapcore.Entry, fields []zapcore.Field) (*buffer.Buffer, error) {
	// 获取原始解码数据
	buf, err := m.Encoder.EncodeEntry(entry, fields)
	if err != nil {
		return nil, err
	}

	// 自定义前缀数据
	DataStr := buf.String()
	buf.Reset()
	buf.AppendString("[" + m.AppName + "] " + DataStr)

	// 时间分片
	m.writer.mu.Lock()
	defer m.writer.mu.Unlock()
	currentDate := time.Now().Format("2006-01-02")

	// 检查是否需要切换到新的日志文件
	if m.writer.logDate != currentDate {
		// 关闭当前日志文件
		if m.writer.file != nil {
			m.writer.file.Close()
		}

		// 创建新的日志文件
		newPath := filepath.Join(m.writer.logPath, "/", currentDate)
		if err := os.MkdirAll(newPath, 0755); err != nil {
			return nil, errors.New("创建文件夹错误")
		}
		filePath := filepath.Join(newPath, " INFO.log")
		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644) // 不存在则创建，只写入
		if err != nil {
			return nil, errors.New("打开文件错误" + filePath)
		}
		// 更新writer
		m.writer.file = file
		m.writer.logDate = currentDate
	}

	// 如果是err及以下的log_level
	if entry.Level >= zapcore.ErrorLevel {
		if m.errFile == nil {
			// 创建新的日志文件
			newPath := filepath.Join(m.writer.logPath, "/", currentDate)
			filePath := filepath.Join(newPath, " ERR.log")
			errFile, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644) // 不存在则创建，只写入
			if err != nil {
				return nil, err
			}
			m.errFile = errFile
		}
		m.errFile.WriteString(buf.String())
	}

	if m.writer.logDate == currentDate {
		m.writer.file.WriteString(buf.String())
	}

	// 返回数据
	return buf, nil
}

func InitLogManager(appName string, logPath string) {

	cfg := zap.NewProductionConfig() // 使用 Production 配置

	// JSON 编码器配置
	cfg.Encoding = "json"
	cfg.EncoderConfig = zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     "\n",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder, // ELK 友好的时间格式
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 时间格式化配置
	cfg.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05")

	// 配置自定义解码器
	myEncoder := &MyEncoder{
		AppName: appName,
		Encoder: zapcore.NewJSONEncoder(cfg.EncoderConfig), // 改为 JSONEncoder
		writer: MyLogWriter{
			logPath: logPath,
		},
	}

	// 创建core
	core := zapcore.NewCore(
		myEncoder,                  // 解码器
		zapcore.AddSync(os.Stdout), // 输出到控制台
		zapcore.InfoLevel,          // log_level
	)

	// 创建logger
	Logger := zap.New(core, zap.AddCaller())

	// 全局日志
	zap.ReplaceGlobals(Logger)
}
