package config

import (
	"os"
	"path/filepath"
	"strings"
)

func DataDir(flag string) (string, error) {
	dir := flag
	if dir == "" {
		dir = os.Getenv("MOVEMENT_DATA_DIR")
	}
	if dir == "" {
		home, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		dir = filepath.Join(home, ".movement")
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") {
		home, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		if dir == "~" {
			dir = home
		} else {
			dir = filepath.Join(home, strings.TrimPrefix(dir, "~/"))
		}
	}
	return filepath.Abs(dir)
}
