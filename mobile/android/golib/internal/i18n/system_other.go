//go:build !windows

package i18n

import (
	"os"
	"strings"
)

func SystemLanguage() string {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			if strings.HasPrefix(strings.ToLower(v), "ar") {
				return "ar"
			}
			return "en"
		}
	}
	return "en"
}
