package i18n

import "golang.org/x/sys/windows"

var procGetUserDefaultUILanguage = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// SystemLang maps the Windows user UI language to a supported SnipGlyph
// language: Chinese locales use ZhCN, everything else uses En.
func SystemLang() Lang {
	if err := procGetUserDefaultUILanguage.Find(); err != nil {
		return En
	}
	r, _, _ := procGetUserDefaultUILanguage.Call()
	const langChinese = 0x04
	if r&0x3FF == langChinese {
		return ZhCN
	}
	return En
}
