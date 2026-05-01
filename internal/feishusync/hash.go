package feishusync

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var whitespacePattern = regexp.MustCompile(`\s+`)

func NormalizeFileName(name string) string {
	name = strings.TrimSpace(name)
	name = whitespacePattern.ReplaceAllString(name, " ")
	name = strings.ToLower(name)

	ext := filepath.Ext(name)
	if ext == "" {
		return name
	}
	base := strings.TrimSpace(strings.TrimSuffix(name, ext))
	ext = strings.TrimSpace(ext)
	if base == "" {
		return ext
	}
	return base + ext
}

func SlotKey(parentToken, fileName string) string {
	return strings.TrimSpace(parentToken) + ":" + NormalizeFileName(fileName)
}

func FileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
