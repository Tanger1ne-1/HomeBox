package version

import "testing"

// TestParse 验证能从 INI 文本里取出 version 值。
func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"; 注释\n[homebox]\nversion=1.2.3\n", "1.2.3"},
		{"[homebox]\nversion = 1.2.3\n", "1.2.3"},
		{"[homebox]\n", "0.0.0"},
		{"", "0.0.0"},
	}
	for _, c := range cases {
		if got := parse(c.in); got != c.want {
			t.Errorf("parse(%q) = %q，期望 %q", c.in, got, c.want)
		}
	}
}

// TestVersionNotEmpty 确认嵌入的版本号不是兜底值。
func TestVersionNotEmpty(t *testing.T) {
	if Version == "" || Version == "0.0.0" {
		t.Fatalf("Version = %q，version.ini 可能没被正确嵌入", Version)
	}
}
