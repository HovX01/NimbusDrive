package domain

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

const MaxNameLen = 255

// ValidateNodeName rejects empty, path-like, or oversized names.
func ValidateNodeName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrValidation)
	}
	if utf8.RuneCountInString(name) > MaxNameLen {
		return fmt.Errorf("%w: name too long", ErrValidation)
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("%w: invalid name", ErrValidation)
	}
	return nil
}

// SanitizeDownloadName keeps basename only.
func SanitizeDownloadName(name string) string {
	base := path.Base(strings.ReplaceAll(name, `\`, `/`))
	if base == "." || base == "/" || base == "" {
		return "download"
	}
	return base
}
