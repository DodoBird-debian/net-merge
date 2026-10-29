package i18n

import "golang.org/x/sys/windows"

func SystemLanguage() string {
	id, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage").Call()
	if id&0x3ff == 0x01 {
		return "ar"
	}
	return "en"
}
