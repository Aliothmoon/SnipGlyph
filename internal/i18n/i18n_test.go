package i18n

import (
	"reflect"
	"regexp"
	"testing"
)

var verbRe = regexp.MustCompile(`%[sdqwv]`)

func verbs(s string) []string { return verbRe.FindAllString(s, -1) }

func TestCatalogCompletenessAndFormatParity(t *testing.T) {
	zv := reflect.ValueOf(zhCN)
	ev := reflect.ValueOf(enUS)
	typ := zv.Type()

	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		switch f.Type.Kind() {
		case reflect.String:
			zs, es := zv.Field(i).String(), ev.Field(i).String()
			if zs == "" {
				t.Errorf("zhCN.%s is empty", f.Name)
			}
			if es == "" {
				t.Errorf("enUS.%s is empty", f.Name)
			}
			if zf, ef := verbs(zs), verbs(es); !reflect.DeepEqual(zf, ef) {
				t.Errorf("format verbs mismatch for %s: zh=%v en=%v", f.Name, zf, ef)
			}
		case reflect.Map:
			zm := zv.Field(i).Interface().(map[string]string)
			em := ev.Field(i).Interface().(map[string]string)
			for k, v := range zm {
				if v == "" {
					t.Errorf("zhCN.%s[%q] is empty", f.Name, k)
				}
				evVal, ok := em[k]
				if !ok || evVal == "" {
					t.Errorf("enUS.%s missing key %q", f.Name, k)
				}
			}
			for k := range em {
				if _, ok := zm[k]; !ok {
					t.Errorf("enUS.%s has extra key %q not in zhCN", f.Name, k)
				}
			}
		}
	}
}

func TestParseAndSet(t *testing.T) {
	orig := Active()
	t.Cleanup(func() { Set(orig) })

	if got := Parse("EN-US"); got != En {
		t.Fatalf("Parse(EN-US) = %q", got)
	}
	if got := Parse("zh_CN"); got != ZhCN {
		t.Fatalf("Parse(zh_CN) = %q", got)
	}
	if got := Parse(""); got != Auto {
		t.Fatalf("Parse('') = %q", got)
	}

	Set(En)
	if Active() != En || Current().SettingsTitle != "SnipGlyph Settings" {
		t.Fatalf("active=%q title=%q", Active(), Current().SettingsTitle)
	}
	if got := Current().CopiedLines(1); got != "Copied 1 line" {
		t.Errorf("CopiedLines(1) = %q", got)
	}
	if got := Current().CopiedLines(3); got != "Copied 3 lines" {
		t.Errorf("CopiedLines(3) = %q", got)
	}

	Set(ZhCN)
	if Active() != ZhCN || Current().SettingsTitle != "SnipGlyph 设置" {
		t.Fatalf("active=%q title=%q", Active(), Current().SettingsTitle)
	}
}
