package client

import (
	"encoding/json"
	"io"
	"os"
	"runtime"
	"strconv"
	"unicode"
)

// homeDir returns the home directory for the current user
func homeDir() string {
	if runtime.GOOS == "windows" {
		// First prefer the HOME environmental variable
		if home := os.Getenv("HOME"); len(home) > 0 {
			if _, err := os.Stat(home); err == nil {
				return home
			}
		}
		if homeDrive, homePath := os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"); len(homeDrive) > 0 && len(homePath) > 0 {
			homeDir := homeDrive + homePath
			if _, err := os.Stat(homeDir); err == nil {
				return homeDir
			}
		}
		if userProfile := os.Getenv("USERPROFILE"); len(userProfile) > 0 {
			if _, err := os.Stat(userProfile); err == nil {
				return userProfile
			}
		}
	}
	return os.Getenv("HOME")
}

func environForKey(key string, defaultV string) string {
	v := os.Getenv(key)
	if v == "" {
		return defaultV
	}
	return v
}

func fileExists(file string) bool {
	_, err := os.Stat(file)
	return err == nil
}

func flushWriter(w io.Writer) {
	if s, ok := w.(interface {
		Sync() error
	}); ok {
		s.Sync() // nolint:errcheck
	} else if s, ok := w.(interface {
		Flush()
	}); ok {
		s.Flush()
	}
}

// checks if s printable, aka doesn't include tab, backspace, etc.
func isPrintable(s string) bool {
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

func unquote(bytes []byte) string {
	var str string
	err := json.Unmarshal(bytes, &str)
	if err == nil {
		return str
	}
	str = string(bytes)
	if us, err := strconv.Unquote(str); err == nil {
		return us
	}
	return str
}
