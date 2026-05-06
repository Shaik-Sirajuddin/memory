package log

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var Logger *slog.Logger
var LogFilePath string

func init() {
	Logger = NewDefault()
}

func New(out io.Writer) *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("DEV") != "" {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(out, &slog.HandlerOptions{
		Level: level,
	}))
}

func NewDefault() *slog.Logger {
	path, file, err := openDefaultLogFile()
	if err != nil {
		return New(os.Stderr)
	}
	LogFilePath = path
	return New(file)
}

func openDefaultLogFile() (string, *os.File, error) {
	logDir := filepath.Join(findMCPRoot(), ".temp", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return "", nil, err
	}
	path := filepath.Join(logDir, randomHex(16)+".txt")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return "", nil, err
	}
	return path, file, nil
}

func findMCPRoot() string {
	if cwd, err := os.Getwd(); err == nil {
		if root, ok := findUp(cwd); ok {
			return root
		}
		candidate := filepath.Join(cwd, "mcp")
		if hasGoMod(candidate) {
			return candidate
		}
	}
	if exe, err := os.Executable(); err == nil {
		if root, ok := findUp(filepath.Dir(exe)); ok {
			return root
		}
	}
	return "."
}

func findUp(start string) (string, bool) {
	dir := start
	for {
		if hasGoMod(dir) {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func hasGoMod(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "module github.com/Shaik-Sirajuddin/memory/mcp")
}

func randomHex(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "mcp"
	}
	return hex.EncodeToString(buf)
}
