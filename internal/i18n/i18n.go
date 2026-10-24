// Package i18n owns human-readable text. Protocol/configuration identifiers stay
// English; OS diagnostics are appended unchanged to translated explanations.
package i18n

import (
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
)

//go:embed ar.json en.json
var messages embed.FS
var arabic map[string]string
var lang atomic.Value

func init() {
	b, _ := messages.ReadFile("ar.json")
	if err := json.Unmarshal(b, &arabic); err != nil {
		panic(err)
	}
	lang.Store("en")
}
func Language() string { return lang.Load().(string) }
func RTL() bool        { return Language() == "ar" }
func Set(language string) bool {
	if language != "en" && language != "ar" {
		return false
	}
	lang.Store(language)
	return true
}
func T(message string) string {
	if RTL() {
		if v, ok := arabic[message]; ok {
			return v
		}
	}
	return message
}

// LTR isolates names, versions, paths and addresses inside an Arabic sentence.
func LTR(value string) string {
	if RTL() {
		return "\u2066" + value + "\u2069"
	}
	return value
}
func Init(dir string) error {
	b, err := os.ReadFile(filepath.Join(dir, "language"))
	if err == nil {
		if !Set(strings.TrimSpace(string(b))) {
			return &os.PathError{Op: "read", Path: "language", Err: os.ErrInvalid}
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	Set(SystemLanguage())
	return nil
}
func Save(dir, language string) error {
	if language != "ar" && language != "en" {
		return os.ErrInvalid
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".language-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.WriteString(language + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Chmod(0644); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp.Name(), filepath.Join(dir, "language")); err != nil {
		return err
	}
	Set(language)
	return nil
}
