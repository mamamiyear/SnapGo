// Package logging centralises slog configuration.
//
// 设计动机:
//   - SnapGo 是 macOS GUI 应用, 由 Finder/open 启动时 stderr 会被重定向
//     到 /dev/null, slog 默认 handler 写到 stderr 因此对用户不可见.
//   - 为方便排查网络 / 权限 / SCP 协议层问题, 我们将日志同时写入磁盘文件
//     和 stderr, 让 `tail -f` 与开发期 `wails dev` 都能直接看到输出.
//   - 单独抽出包可避免 main / app 直接依赖文件 IO 细节, 也方便后续替换为
//     os_log 或集中式日志方案.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	// maxLogSize is the byte threshold that triggers a rotation. 5 MiB keeps
	// a useful amount of recent history while staying small enough to open
	// in any editor.
	maxLogSize = 5 * 1024 * 1024
	// maxBackups is how many rotated files to retain (snapgo.log.1,
	// snapgo.log.2). Older ones are deleted on each rotation.
	maxBackups = 2
)

// teeWriter is a thin io.Writer that fans out a single slog write to many
// underlying writers. We avoid log.MultiWriter from the std library because
// io.MultiWriter already covers the use case without an extra dependency.
type teeWriter struct {
	writers []io.Writer
}

// Write copies p to every underlying writer. We deliberately keep going on
// errors so a single broken writer (e.g. closed file handle) does not lose
// output to the surviving writers; the last error wins which is sufficient
// for a logger.
func (t *teeWriter) Write(p []byte) (int, error) {
	var lastErr error
	for _, w := range t.writers {
		if w == nil {
			continue
		}
		if _, err := w.Write(p); err != nil {
			lastErr = err
		}
	}
	return len(p), lastErr
}

// Init wires slog.Default to write to both ~/Library/Logs/SnapGo/snapgo.log
// and stderr. Returns a closer the caller (main) should defer so the file
// handle is released on shutdown.
//
// 日志级别可通过环境变量 SNAPGO_LOG_LEVEL 调整: debug|info|warn|error.
// 默认 debug 以便排查 SCP 阶段性问题, 待功能稳定后可调整为 info.
func Init() (closer func()) {
	level := parseLevel(os.Getenv("SNAPGO_LOG_LEVEL"))

	var fileW io.Writer
	logPath, err := defaultLogPath()
	if err == nil {
		// Size-based rotation keeps the log directory bounded; append mode is
		// preserved inside rotatingFile so prior-launch context survives.
		rf, ferr := newRotatingFile(logPath, maxLogSize, maxBackups)
		if ferr == nil {
			fileW = rf
			closer = func() { _ = rf.Close() }
		}
	}
	if closer == nil {
		closer = func() {}
	}

	writer := &teeWriter{writers: []io.Writer{os.Stderr}}
	if fileW != nil {
		writer.writers = append(writer.writers, fileW)
	}

	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))

	slog.Info("logging initialised",
		"level", level.String(),
		"file", logPath)
	return closer
}

// rotatingFile is a size-based rotating log writer.
//
// 设计理由:
//   - GUI 应用长期 append 写日志, 不加限制会无限增长, 占满磁盘.
//   - 标准库不提供滚动能力, 引入 lumberjack 等第三方库又会增加依赖,
//     而我们的需求很简单 (单文件 / 按大小 / 固定份数), 自实现成本更低.
//   - 实现策略: 每次 Write 前检查写入后是否超过 maxLogSize, 超过则先
//     rotate (snapgo.log -> snapgo.log.1 -> snapgo.log.2, 丢弃最老的),
//     再写入新建的 snapgo.log. 用互斥锁保证并发安全 (slog handler 自身
//     不保证对 io.Writer 的串行化).
type rotatingFile struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	backups int
	file    *os.File
	size    int64
}

// newRotatingFile opens (or creates, append mode) the target log file and
// initialises the current size from the existing file so a restart does not
// reset the rotation threshold.
func newRotatingFile(path string, maxSize int64, backups int) (*rotatingFile, error) {
	r := &rotatingFile{path: path, maxSize: maxSize, backups: backups}
	if err := r.openExisting(); err != nil {
		return nil, err
	}
	return r, nil
}

// openExisting opens the active log file in append mode and records its
// current size.
func (r *rotatingFile) openExisting() error {
	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	r.file = f
	r.size = info.Size()
	return nil
}

// Write appends p, rotating first when the write would exceed maxSize.
func (r *rotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.size+int64(len(p)) > r.maxSize {
		if err := r.rotate(); err != nil {
			// On rotation failure we keep writing to the current file rather
			// than dropping logs; oversized-by-a-bit beats data loss.
			r.size += int64(len(p))
			n, werr := r.file.Write(p)
			if werr != nil {
				return n, werr
			}
			return n, err
		}
	}
	n, err := r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate closes the current file, shifts backups (snapgo.log.1 ->
// snapgo.log.2, dropping the oldest), renames the active file to
// snapgo.log.1, then opens a fresh active file.
func (r *rotatingFile) rotate() error {
	if err := r.file.Close(); err != nil {
		return err
	}
	// Drop the oldest backup, then shift the rest up by one index.
	oldest := fmt.Sprintf("%s.%d", r.path, r.backups)
	_ = os.Remove(oldest)
	for i := r.backups - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", r.path, i)
		dst := fmt.Sprintf("%s.%d", r.path, i+1)
		_ = os.Rename(src, dst)
	}
	if r.backups >= 1 {
		_ = os.Rename(r.path, fmt.Sprintf("%s.1", r.path))
	}
	return r.openExisting()
}

// Close releases the underlying file handle.
func (r *rotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file == nil {
		return nil
	}
	return r.file.Close()
}

// defaultLogPath returns ~/Library/Logs/SnapGo/snapgo.log, creating the
// parent directory if needed. macOS users expect app logs to live there
// (Console.app surfaces this directory under "Reports" and `open ~/Library/Logs`
// is muscle-memory).
func defaultLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Library", "Logs", "SnapGo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "snapgo.log"), nil
}

// parseLevel translates a free-form string into an slog.Level. We default
// to debug because the app currently has plenty of diagnostic logs and the
// user-facing impact (slightly more disk IO on uploads) is negligible.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return slog.LevelError
	case "warn", "warning":
		return slog.LevelWarn
	case "info":
		return slog.LevelInfo
	case "debug", "":
		return slog.LevelDebug
	default:
		return slog.LevelDebug
	}
}
