package logger

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"

	"gopkg.in/natefinch/lumberjack.v2"

	"novel/internal/config"
)

// fileEnabled 控制文件日志开关，原子操作保证并发安全。
var fileEnabled atomic.Bool

func init() { fileEnabled.Store(true) }

// SetFileEnabled 设置文件日志开关。
func SetFileEnabled(enabled bool) { fileEnabled.Store(enabled) }

// IsFileEnabled 返回文件日志是否启用。
func IsFileEnabled() bool { return fileEnabled.Load() }

// New 创建结构化日志器。
func New(level slog.Level, format string, out io.Writer) *slog.Logger {
	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level, AddSource: true}
	switch format {
	case "json":
		handler = slog.NewJSONHandler(out, opts)
	default:
		handler = slog.NewTextHandler(out, opts)
	}
	return slog.New(handler)
}

// Default 返回日志器：同时写 stderr 和文件。
func Default() *slog.Logger {
	logPath := filepath.Join(config.DataDirPath(), "goink.log")
	os.MkdirAll(filepath.Dir(logPath), 0700)
	return New(slog.LevelDebug, "text", &fanWriter{
		writers: []io.Writer{
			os.Stderr,
			&lumberjack.Logger{
				Filename:   logPath,
				MaxSize:    10,
				MaxBackups: 3,
				MaxAge:     30,
				Compress:   true,
			},
		},
	})
}

type fanWriter struct{ writers []io.Writer }

func (fw *fanWriter) Write(p []byte) (int, error) {
	// 第一个 writer 是 stderr，始终写入
	fw.writers[0].Write(p)
	// 第二个 writer 是文件，受开关控制
	if fileEnabled.Load() && len(fw.writers) > 1 {
		fw.writers[1].Write(p)
	}
	return len(p), nil
}
