package appdata

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	// MaxDataDirLength 数据目录路径最大长度（字符）。
	MaxDataDirLength = 200

	// probeFileName 可写探针文件名。
	probeFileName = ".cs2ht_init_probe"
)

// 字符白名单：A-Z a-z 0-9 _ - . : \ /
var dataDirAllowedCharsRe = regexp.MustCompile(`^[A-Za-z0-9_\-.:\\/]+$`)

// ValidateDataDir 按 7 条规则顺序校验用户选择的数据目录。
// 校验通过返回 nil；失败返回分类的中文错误信息，便于前端精准提示。
func ValidateDataDir(path string) error {
	// 1. 非空、去引号、Clean
	path = strings.TrimSpace(path)
	path = strings.Trim(path, `"'`)
	if path == "" {
		return errors.New("路径不能为空")
	}
	path = filepath.Clean(path)

	// 2. 不能是磁盘根目录
	if IsDiskRoot(path) {
		return errors.New("路径不能是磁盘根目录")
	}

	// 3. 字符白名单 + 分类报错
	if err := validateDataDirChars(path); err != nil {
		return err
	}

	// 4. 长度 ≤ 100
	if len(path) > MaxDataDirLength {
		return fmt.Errorf("路径长度超过 %d 字符", MaxDataDirLength)
	}

	// 5. 父目录可写（MkdirAll）
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("无法创建目录: %w", err)
	}

	// 6. 可读写删测试
	if err := probeReadWriteDelete(path); err != nil {
		return err
	}

	return nil
}

// validateDataDirChars 按"中文/非 ASCII"、"空格"、"非法符号"分类报错。
func validateDataDirChars(path string) error {
	// 先检测非 ASCII（含中文）
	for _, r := range path {
		if r > 127 {
			return errors.New("路径不能包含中文或非 ASCII 字符")
		}
	}
	// 空格
	if strings.ContainsRune(path, ' ') {
		return errors.New("路径不能包含空格")
	}
	// Windows 非法字符 < > " | ? *
	if strings.ContainsAny(path, `<>"|?*`) {
		return errors.New(`路径不能包含非法符号 < > " | ? *`)
	}
	// 其他符号（白名单之外）
	if !dataDirAllowedCharsRe.MatchString(path) {
		return errors.New("路径包含不允许的字符（仅支持字母、数字、下划线、连字符、点、冒号和路径分隔符）")
	}
	return nil
}

// IsDiskRoot 判断给定路径是否为磁盘根目录或 UNC 根。
// 例如：C:\、D:\、/、\\server\share。
//
// 注意 filepath.Clean 的平台差异：Windows 上 Clean("C:") == "C:."（盘符相对
// 路径）、Clean("/") == `\`（当前盘根），所以裸分隔符与裸盘符都要单独识别，
// 否则这两个输入会绕过根目录校验。
func IsDiskRoot(path string) bool {
	clean := filepath.Clean(path)

	// 仅由当前平台分隔符组成的路径：Unix 根 /，以及 Windows 上 Clean("/") == `\`。
	// 只裁剪本平台分隔符，避免把 POSIX 上名为 `\` 的普通目录误判为根。
	if clean != "" && strings.Trim(clean, string(filepath.Separator)) == "" {
		return true
	}

	// Windows 盘符根：C:、C:\、C:/，以及 Windows Clean("C:") 得到的 "C:."。
	if len(clean) >= 2 && clean[1] == ':' {
		rest := clean[2:]
		if rest == "" || rest == "." || strings.Trim(rest, `\/`) == "" {
			return true
		}
	}

	// UNC 根：VolumeName 已吃掉 \\server\share，只剩空余量即根。
	volume := filepath.VolumeName(clean)
	if volume != "" {
		remainder := strings.TrimPrefix(clean, volume)
		remainder = strings.Trim(remainder, `\/`)
		if remainder == "" {
			return true
		}
	}

	return false
}

// probeReadWriteDelete 在目录下写入 .cs2ht_init_probe，
// 写入随机 8 字节后读回校验、再删除。失败均返回中文错误。
func probeReadWriteDelete(dir string) error {
	probePath := filepath.Join(dir, probeFileName)

	// 写入前清理（防止旧探针残留）
	_ = os.Remove(probePath)

	payload := make([]byte, 8)
	if _, err := rand.Read(payload); err != nil {
		return fmt.Errorf("生成探针数据失败: %w", err)
	}

	if err := os.WriteFile(probePath, payload, 0o644); err != nil {
		return fmt.Errorf("无法在该位置写入文件: %w", err)
	}
	// 任何后续失败都要清理探针
	cleanup := func() {
		_ = os.Remove(probePath)
	}

	got, err := os.ReadFile(probePath)
	if err != nil {
		cleanup()
		return fmt.Errorf("无法读取探针文件: %w", err)
	}
	if len(got) != len(payload) {
		cleanup()
		return errors.New("探针数据校验失败")
	}
	for i := range got {
		if got[i] != payload[i] {
			cleanup()
			return errors.New("探针数据校验失败")
		}
	}

	if err := os.Remove(probePath); err != nil {
		return fmt.Errorf("无法删除探针文件: %w", err)
	}
	return nil
}
