package opkg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallFilePassesLocalPathAsOneArgument(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "opkg")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "package with spaces.ipk")
	if err := os.WriteFile(path, []byte("IPK"), 0600); err != nil {
		t.Fatal(err)
	}
	c := &Client{Bin: bin}
	out, err := c.InstallFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if out != "install\n"+path {
		t.Fatalf("arguments = %q", out)
	}
}

func TestInstallFileRejectsInvalidPackage(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.ipk")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "option", path: "--force-reinstall"},
		{name: "relative", path: "package.ipk"},
		{name: "URL", path: "https://example.test/package.ipk"},
		{name: "wrong extension", path: filepath.Join(dir, "package.txt")},
		{name: "missing", path: filepath.Join(dir, "missing.ipk")},
		{name: "empty", path: empty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewClient().InstallFile(tc.path); err == nil {
				t.Fatalf("invalid path accepted: %q", tc.path)
			} else if strings.Contains(err.Error(), "opkg not installed") {
				t.Fatalf("invalid path reached opkg: %v", err)
			}
		})
	}
}

// Поиск уходит в exec тем же путём, что install: аргумент, начинающийся с
// дефиса, opkg прочитает как флаг.
func TestSearch_RejectsFlagLikeQuery(t *testing.T) {
	c := NewClient()
	for _, q := range []string{"-f /tmp/my.conf", "--force-reinstall", "/tmp/evil.ipk"} {
		_, err := c.Search(q)
		if err == nil {
			t.Errorf("запрос %q принят", q)
			continue
		}
		// Отказ должен быть от валидации, а не от «opkg не установлен»:
		// иначе тест зеленеет на машине без Entware по чужой причине.
		if !strings.Contains(err.Error(), "invalid package name") {
			t.Errorf("запрос %q отклонён не валидацией: %v", q, err)
		}
	}
}

func TestValidatePkgNames(t *testing.T) {
	if err := validatePkgNames([]string{"curl", "libopenssl1.1", "zoneinfo-core"}); err != nil {
		t.Errorf("нормальные имена отвергнуты: %v", err)
	}
	for _, bad := range []string{"--force-depends", "-f", "/tmp/x.ipk", "pkg;rm -rf /", ""} {
		if err := validatePkgNames([]string{bad}); err == nil {
			t.Errorf("имя %q принято", bad)
		}
	}
}
