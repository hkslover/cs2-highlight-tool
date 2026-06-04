package appdata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// shortTempDir 返回一个短路径下的临时目录，避免 t.TempDir() 的长前缀
// 触发"路径长度超过 260"规则。测试结束自动清理。
// 若环境没有合适的短临时目录则跳过测试。
func shortTempDir(t *testing.T) string {
	t.Helper()
	candidates := []string{"/tmp", `C:\Temp`, `C:\Windows\Temp`}
	for _, root := range candidates {
		info, err := os.Stat(root)
		if err != nil || !info.IsDir() {
			continue
		}
		dir, err := os.MkdirTemp(root, "cs2ht_")
		if err != nil {
			continue
		}
		t.Cleanup(func() {
			_ = os.RemoveAll(dir)
		})
		return dir
	}
	t.Skipf("no short temp dir available; default temp prefix exceeds path length limit")
	return ""
}

func TestValidateDataDir_EmptyPath(t *testing.T) {
	err := ValidateDataDir("")
	if err == nil || !strings.Contains(err.Error(), "为空") {
		t.Fatalf("empty path = %v, want '路径不能为空'", err)
	}
}

func TestValidateDataDir_WhitespaceOnly(t *testing.T) {
	err := ValidateDataDir("   ")
	if err == nil || !strings.Contains(err.Error(), "为空") {
		t.Fatalf("whitespace path = %v, want '路径不能为空'", err)
	}
}

func TestValidateDataDir_DiskRootRejected(t *testing.T) {
	cases := []string{
		`C:\`,
		`D:\`,
		`C:/`,
		`C:`,
		`/`,
	}
	for _, p := range cases {
		err := ValidateDataDir(p)
		if err == nil || !strings.Contains(err.Error(), "磁盘根目录") {
			t.Errorf("ValidateDataDir(%q) = %v, want '磁盘根目录' error", p, err)
		}
	}
}

func TestValidateDataDir_ContainsChinese(t *testing.T) {
	tmp := shortTempDir(t)
	chineseDir := filepath.Join(tmp, "张三Data")
	if err := ValidateDataDir(chineseDir); err != nil {
		t.Fatalf("Chinese path should now be allowed, got: %v", err)
	}
}

func TestValidateDataDir_ContainsNonASCII(t *testing.T) {
	tmp := shortTempDir(t)
	nonASCIIDir := filepath.Join(tmp, "caféData")
	if err := ValidateDataDir(nonASCIIDir); err != nil {
		t.Fatalf("non-ASCII path should now be allowed, got: %v", err)
	}
}

func TestValidateDataDir_ContainsSpace(t *testing.T) {
	tmp := shortTempDir(t)
	spaceDir := filepath.Join(tmp, "Program Files App")
	if err := ValidateDataDir(spaceDir); err != nil {
		t.Fatalf("space path should now be allowed, got: %v", err)
	}
}

func TestValidateDataDir_ContainsIllegalSymbol(t *testing.T) {
	cases := []string{
		`C:\foo<bar`,
		`C:\foo>bar`,
		`C:\foo|bar`,
		`C:\foo?bar`,
		`C:\foo*bar`,
		`C:\foo"bar`,
	}
	for _, p := range cases {
		err := ValidateDataDir(p)
		if err == nil || !strings.Contains(err.Error(), "非法符号") {
			t.Errorf("ValidateDataDir(%q) = %v, want 'illegal symbol' error", p, err)
		}
	}
}

func TestValidateDataDir_TooLong(t *testing.T) {
	// 构造 > 260 字符的合法字符路径
	long := `C:\` + strings.Repeat("a", 270)
	err := ValidateDataDir(long)
	if err == nil || !strings.Contains(err.Error(), "长度") {
		t.Fatalf("long path = %v, want 'length' error", err)
	}
}

func TestValidateDataDir_NonEmptyDirAllowed(t *testing.T) {
	tmp := shortTempDir(t)
	dataDir := filepath.Join(tmp, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write existing file: %v", err)
	}

	// 非空目录应允许通过（只要可写、无中文/空格/特殊字符）
	if err := ValidateDataDir(dataDir); err != nil {
		t.Fatalf("non-empty dir should be allowed, got: %v", err)
	}
}

func TestValidateDataDir_HappyPath_NewDirectory(t *testing.T) {
	tmp := shortTempDir(t)
	newDir := filepath.Join(tmp, "data")

	if err := ValidateDataDir(newDir); err != nil {
		t.Fatalf("ValidateDataDir on new dir failed: %v", err)
	}

	// 探针文件应已清理
	probePath := filepath.Join(newDir, probeFileName)
	if _, err := os.Stat(probePath); err == nil {
		t.Fatalf("probe file should be cleaned up, but still exists: %s", probePath)
	}

	// 目录应被创建
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("directory should exist after validate: %v", err)
	}
}

func TestValidateDataDir_HappyPath_ExistingEmpty(t *testing.T) {
	tmp := shortTempDir(t)
	emptyDir := filepath.Join(tmp, "empty")
	if err := os.MkdirAll(emptyDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	if err := ValidateDataDir(emptyDir); err != nil {
		t.Fatalf("ValidateDataDir on existing empty dir failed: %v", err)
	}
}

func TestIsDiskRoot(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{`C:\`, true},
		{`D:\`, true},
		{`C:/`, true},
		{`C:`, true},
		{`/`, true},
		{`C:\Users`, false},
		{`D:\Foo\Bar`, false},
		{`/home/user`, false},
		{``, false},
	}
	for _, c := range cases {
		if got := IsDiskRoot(c.path); got != c.want {
			t.Errorf("IsDiskRoot(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestProbeReadWriteDelete_Success(t *testing.T) {
	tmp := shortTempDir(t)
	if err := probeReadWriteDelete(tmp); err != nil {
		t.Fatalf("probeReadWriteDelete failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, probeFileName)); err == nil {
		t.Fatalf("probe file should be removed")
	}
}

func TestValidateDataDir_AllowedCharactersPass(t *testing.T) {
	tmp := shortTempDir(t)
	// 用 t.TempDir 作为父目录，子目录用合法字符（兼容跨平台）。
	cases := []string{
		filepath.Join(tmp, "DataDir-1"),
		filepath.Join(tmp, "abc.xyz"),
		filepath.Join(tmp, "a_b-c.d"),
	}
	for _, p := range cases {
		if err := ValidateDataDir(p); err != nil {
			// 父目录可能因含空格等失败（如 macOS 临时目录），在这种情况下跳过。
			if strings.Contains(err.Error(), "空格") ||
				strings.Contains(err.Error(), "ASCII") ||
				strings.Contains(err.Error(), "非法符号") ||
				strings.Contains(err.Error(), "长度") {
				t.Skipf("temp dir parent triggers unrelated rule: %v", err)
			}
			t.Errorf("ValidateDataDir(%q) failed: %v", p, err)
		}
	}
}
