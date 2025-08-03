package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var QuitChan = make(chan os.Signal, 1)
var ProcessId = make(chan int, 1)

func Shutdown(reason string) {
	fmt.Printf("🚨 %s\n", reason)
	os.Exit(-1)
}

func GracefulExit(reason string) {
	fmt.Printf("🚨 %s", reason)
	process, err := os.FindProcess(os.Getpid())
	if err == nil {
		process.Signal(syscall.SIGTERM)
	}
}

// BaseName returns the base name of a path without the extension
func BaseName(path string) string {
	ext := filepath.Ext(path)
	if ext == "" {
		return path
	}
	return strings.TrimSuffix(path, ext)
}
