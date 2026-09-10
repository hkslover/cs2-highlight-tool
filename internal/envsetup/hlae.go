package envsetup

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"cs2-highlight-tool-v2/internal/config"
	"cs2-highlight-tool-v2/internal/download"
	"cs2-highlight-tool-v2/internal/endpoints"
	"cs2-highlight-tool-v2/internal/release"
)

type componentVersionFile struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

func (s *Service) ensureHLAE(source DownloadSource) error {
	source = normalizeDownloadSource(string(source))
	cfg := s.currentConfig()

	checkStarted := s.logStepStart(componentHLAE, "check", "validate_local", string(source), 0, map[string]string{
		"local_exe": cfg.HLAEExe,
	})
	localVersion, localErr := resolveInstalledHLAEVersion(cfg.HLAEExe)
	localReady := localErr == nil
	s.logStepDone(componentHLAE, "check", "validate_local", string(source), 0, checkStarted, map[string]string{
		"local_ready": strconv.FormatBool(localReady),
		"local_ver":   localVersion,
	})

	releaseURL := endpoints.APIURLFor(componentHLAE, string(source))
	fetchStarted := s.logStepStart(componentHLAE, "fetch_release", "from_unified_snapshot", string(source), 0, map[string]string{
		"api_url": releaseURL,
	})
	candidates, err := s.collectReleaseAssetCandidates(componentHLAE, source, release.SelectHLAEAsset)
	if err != nil {
		s.logStepFail(componentHLAE, "fetch_release", "from_unified_snapshot", string(source), 0, fetchStarted, err, map[string]string{
			"api_url": releaseURL,
		})
		if localReady {
			return fmt.Errorf("获取 HLAE Release 失败: %v；本地版本可用，但更新获取失败，可能导致后续功能不可用", err)
		}
		return fmt.Errorf("获取 HLAE Release 失败: %w", err)
	}
	info := candidates[0].Info
	asset := candidates[0].Asset
	assetURL := candidates[0].AssetURL
	s.logStepDone(componentHLAE, "fetch_release", "from_unified_snapshot", string(source), 0, fetchStarted, map[string]string{
		"tag":             info.TagName,
		"selected_source": string(candidates[0].Source),
	})

	selectStarted := s.logStepStart(componentHLAE, "select_asset", "pick", string(source), 0, nil)
	s.logStepDone(componentHLAE, "select_asset", "pick", string(source), 0, selectStarted, map[string]string{
		"asset_name": asset.Name,
		"asset_url":  assetURL,
	})

	latest := firstNonEmpty(info.TagName, asset.Name)
	s.updateStep(componentHLAE, func(step *ComponentStatus) {
		step.RemoteVersion = latest
		step.ManualURL = infoManualURL(componentHLAE, source, info)
	})
	if localReady && release.CompareVersions(localVersion, latest) >= 0 {
		s.updateStep(componentHLAE, func(step *ComponentStatus) {
			step.Status = statusReady
			step.LocalVersion = localVersion
			step.Path = cfg.HLAEExe
		})
		s.emitLogWithFields("info", "本地 HLAE 已是最新版本", logFields{
			Component: componentHLAE,
			Stage:     "ready",
			Action:    "skip_install",
			Source:    string(source),
			Meta: map[string]string{
				"local_version":  localVersion,
				"remote_version": latest,
			},
		})
		ffmpegExe := filepath.Join(s.dataDir, "ffmpeg", "bin", "ffmpeg.exe")
		if _, statErr := os.Stat(ffmpegExe); statErr == nil {
			s.tryWriteHLAEFfmpegIni(ffmpegExe)
		}
		return nil
	}

	s.updateStep(componentHLAE, func(step *ComponentStatus) { step.Status = statusDownloading })
	if err := s.downloadAndInstallWithFallback(componentHLAE, latest, candidates, s.installHLAEFromArchive); err != nil {
		return fmt.Errorf("下载 HLAE 失败: %w", err)
	}
	return nil
}

func (s *Service) installHLAEFromArchive(archivePath string) error {
	s.updateStep(componentHLAE, func(step *ComponentStatus) { step.Status = statusInstalling })
	extractDir := filepath.Join(s.dataDir, "temp", "hlae_extract")
	_ = os.RemoveAll(extractDir)
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)
	extractStarted := s.logStepStart(componentHLAE, "extract", "unzip", string(s.currentSource()), 0, map[string]string{
		"archive_path": archivePath,
		"extract_dir":  extractDir,
	})
	if err := download.Unzip(archivePath, extractDir); err != nil {
		s.logStepFail(componentHLAE, "extract", "unzip", string(s.currentSource()), 0, extractStarted, err, nil)
		return fmt.Errorf("解压 HLAE 失败: %w", err)
	}
	s.logStepDone(componentHLAE, "extract", "unzip", string(s.currentSource()), 0, extractStarted, nil)

	validateStarted := s.logStepStart(componentHLAE, "validate", "verify_archive", string(s.currentSource()), 0, nil)
	hlaeExe, err := download.FindFile(extractDir, "hlae.exe")
	if err != nil {
		s.logStepFail(componentHLAE, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, err, nil)
		return err
	}
	// Validate the extracted component before moving the existing installation
	// out of the way.  ReplaceDirWithContentsWithReport deliberately protects
	// filesystem commit boundaries, while component-specific requirements stay
	// here in envsetup.
	if err := validateHLAE(hlaeExe); err != nil {
		s.logStepFail(componentHLAE, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, err, nil)
		return err
	}
	if _, err := resolveInstalledHLAEVersion(hlaeExe); err != nil {
		s.logStepFail(componentHLAE, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, err, nil)
		return err
	}
	root := filepath.Dir(hlaeExe)
	targetDir := filepath.Join(s.dataDir, "hlae")
	replaceReport, err := download.ReplaceDirWithContentsWithReport(root, targetDir)
	if err != nil {
		s.logStepFail(componentHLAE, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, err, nil)
		return err
	}
	if replaceReport.BackupCleanupError != nil {
		s.emitLogWithFields("warning", "HLAE 旧版本备份清理失败，已保留供恢复", logFields{
			Component: componentHLAE,
			Stage:     "commit",
			Action:    "cleanup_backup",
			Error:     replaceReport.BackupCleanupError.Error(),
			Meta: map[string]string{
				"backup_path": replaceReport.BackupPath,
			},
		})
	}
	targetExe := filepath.Join(targetDir, "HLAE.exe")
	if err := validateHLAE(targetExe); err != nil {
		s.logStepFail(componentHLAE, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, err, nil)
		return err
	}
	installedVersion, err := resolveInstalledHLAEVersion(targetExe)
	if err != nil {
		s.logStepFail(componentHLAE, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, err, nil)
		return err
	}
	if err := writeComponentVersion(targetDir, installedVersion, targetExe); err != nil {
		s.logStepFail(componentHLAE, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, err, nil)
		return err
	}
	s.logStepDone(componentHLAE, "validate", "verify_archive", string(s.currentSource()), 0, validateStarted, map[string]string{
		"target_exe": targetExe,
		"version":    installedVersion,
	})

	persistStarted := s.logStepStart(componentHLAE, "persist_config", "write", string(s.currentSource()), 0, nil)
	cfg, err := s.persistConfig(func(next *config.Config) error {
		next.HLAEExe = targetExe
		return nil
	})
	if err != nil {
		s.logStepFail(componentHLAE, "persist_config", "write", string(s.currentSource()), 0, persistStarted, err, nil)
		return err
	}
	s.logStepDone(componentHLAE, "persist_config", "write", string(s.currentSource()), 0, persistStarted, map[string]string{
		"config_hlae_exe": cfg.HLAEExe,
	})
	s.updateStep(componentHLAE, func(step *ComponentStatus) {
		step.Status = statusReady
		step.LocalVersion = installedVersion
		step.Path = cfg.HLAEExe
		step.Error = ""
	})
	s.emitLogWithFields("info", "HLAE 安装完成", logFields{
		Component: componentHLAE,
		Stage:     "ready",
		Action:    "component_ready",
		Source:    string(s.currentSource()),
		Meta: map[string]string{
			"version": installedVersion,
			"path":    cfg.HLAEExe,
		},
	})
	ffmpegExe := filepath.Join(s.dataDir, "ffmpeg", "bin", "ffmpeg.exe")
	if _, statErr := os.Stat(ffmpegExe); statErr == nil {
		s.tryWriteHLAEFfmpegIni(ffmpegExe)
	}
	return nil
}

func validateHLAE(hlaeExe string) error {
	if hlaeExe == "" {
		return fmt.Errorf("HLAE 路径为空")
	}
	if _, err := os.Stat(hlaeExe); err != nil {
		return fmt.Errorf("HLAE.exe 不存在: %s", hlaeExe)
	}
	hookDLL := filepath.Join(filepath.Dir(hlaeExe), "x64", "AfxHookSource2.dll")
	if _, err := os.Stat(hookDLL); err != nil {
		return fmt.Errorf("AfxHookSource2.dll 不存在: %s", hookDLL)
	}
	return nil
}

func resolveInstalledHLAEVersion(hlaeExe string) (string, error) {
	if err := validateHLAE(hlaeExe); err != nil {
		return "", err
	}
	changelogPath := filepath.Join(filepath.Dir(hlaeExe), "changelog.xml")
	version, err := readHLAEVersionFromChangelog(changelogPath)
	if err != nil {
		return "", err
	}
	return version, nil
}

func readHLAEVersionFromChangelog(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("读取 changelog.xml 失败: %w", err)
	}
	defer file.Close()

	decoder := xml.NewDecoder(file)
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("解析 changelog.xml 失败: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || !strings.EqualFold(start.Name.Local, "version") {
			continue
		}
		var value string
		if err := decoder.DecodeElement(&value, &start); err != nil {
			return "", fmt.Errorf("解析 changelog.xml version 失败: %w", err)
		}
		normalized := normalizeHLAEVersion(value)
		if normalized == "" {
			return "", fmt.Errorf("changelog.xml 中 version 为空")
		}
		return normalized, nil
	}
	return "", fmt.Errorf("changelog.xml 中未找到 version")
}

func normalizeHLAEVersion(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.TrimPrefix(value, "v"), "V")
	return strings.TrimSpace(value)
}

func writeComponentVersion(dir, version, path string) error {
	data, err := json.MarshalIndent(componentVersionFile{Version: version, Path: path}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "version.json"), data, 0644)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func sanitizeFileName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	return replacer.Replace(value)
}

func writeHLAEFfmpegIni(hlaeDir, ffmpegExePath string) error {
	iniDir := filepath.Join(hlaeDir, "ffmpeg")
	if err := os.MkdirAll(iniDir, 0755); err != nil {
		return fmt.Errorf("创建 HLAE ffmpeg 目录失败: %w", err)
	}
	content := "[Ffmpeg]\r\nPath=" + ffmpegExePath + "\r\n"
	iniPath := filepath.Join(iniDir, "ffmpeg.ini")
	if err := os.WriteFile(iniPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("写入 ffmpeg.ini 失败: %w", err)
	}
	return nil
}

func validateHLAEFfmpegIni(hlaeDir, expectedFfmpegExe string) error {
	iniPath := filepath.Join(hlaeDir, "ffmpeg", "ffmpeg.ini")
	f, err := os.Open(iniPath)
	if err != nil {
		return fmt.Errorf("ffmpeg.ini 不存在: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(strings.ToLower(line), "path=") {
			value := strings.TrimSpace(line[len("path="):])
			if !strings.EqualFold(filepath.Clean(value), filepath.Clean(expectedFfmpegExe)) {
				return fmt.Errorf("ffmpeg.ini Path 不匹配: 期望 %s, 实际 %s", expectedFfmpegExe, value)
			}
			return nil
		}
	}
	return fmt.Errorf("ffmpeg.ini 中未找到 Path 配置")
}

func (s *Service) tryWriteHLAEFfmpegIni(ffmpegExePath string) {
	hlaeDir := filepath.Join(s.dataDir, "hlae")
	if _, err := os.Stat(hlaeDir); err != nil {
		return
	}
	if err := validateHLAEFfmpegIni(hlaeDir, ffmpegExePath); err == nil {
		return
	}
	if err := writeHLAEFfmpegIni(hlaeDir, ffmpegExePath); err != nil {
		s.emitLogWithFields("warning", "写入 HLAE ffmpeg.ini 失败", logFields{
			Component: componentHLAE,
			Stage:     "ffmpeg_ini",
			Action:    "write_failed",
			Error:     err.Error(),
		})
		return
	}
	s.emitLogWithFields("info", "已写入 HLAE ffmpeg.ini", logFields{
		Component: componentHLAE,
		Stage:     "ffmpeg_ini",
		Action:    "write_ok",
		Meta: map[string]string{
			"ffmpeg_exe": ffmpegExePath,
			"ini_path":   filepath.Join(hlaeDir, "ffmpeg", "ffmpeg.ini"),
		},
	})
}
