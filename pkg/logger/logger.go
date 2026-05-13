package logger

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	mu      sync.Mutex
	service string
)

func InitService(name string) {
	mu.Lock()
	defer mu.Unlock()
	service = name
	log.SetFlags(0)
	log.SetOutput(os.Stdout)
}

func formatLog(level, msg string, fields ...interface{}) string {
	ts := time.Now().Format("2006-01-02 15:04:05.000")
	prefix := fmt.Sprintf("[%s] [%s] [%s]", ts, level, service)
	if len(fields) > 0 && len(fields)%2 == 0 {
		var sb strings.Builder
		sb.WriteString(prefix)
		sb.WriteString(" ")
		sb.WriteString(msg)
		for i := 0; i < len(fields); i += 2 {
			sb.WriteString(fmt.Sprintf(" | %v=%v", fields[i], fields[i+1]))
		}
		return sb.String()
	}
	return fmt.Sprintf("%s %s", prefix, msg)
}

func Info(msg string, fields ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	log.Println(formatLog("INFO", msg, fields...))
}

func Warn(msg string, fields ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	log.Println(formatLog("WARN", msg, fields...))
}

func Error(msg string, fields ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	log.Println(formatLog("ERROR", msg, fields...))
}

func Fatal(msg string, fields ...interface{}) {
	mu.Lock()
	defer mu.Unlock()
	log.Fatalln(formatLog("FATAL", msg, fields...))
}
