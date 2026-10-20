package steam

import (
	"compress/bzip2"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cs2-highlight-tool-v2/internal/download"
)

// Import errors that the UI surfaces verbatim.
var (
	// ErrDemoLinkUnavailable means the match message carried no download URL,
	// which happens for old matches whose replay is gone.
	ErrDemoLinkUnavailable = errors.New("该场次没有可用的 demo 下载链接")
	// ErrDemoLinkExpired means Valve answered 404: replay links are only served
	// for a limited time after the match.
	ErrDemoLinkExpired = errors.New("demo 下载链接已过期，Valve 只在一段时间内提供官方比赛 demo")
)

const (
	// probeTimeout bounds the optional "is the link still alive" HEAD request.
	probeTimeout = 10 * time.Second
	// copyBufferSize is the bzip2 extraction chunk size. The decompressor has no
	// context support, so cancellation is checked between chunks.
	copyBufferSize = 64 * 1024
)

// ImportDemoContext downloads a Valve replay (a `.dem.bz2` stream) and commits
// the extracted demo into cacheRoot as `<matchID>.dem`. The stable path is only
// ever written by the final atomic commit; each task owns one staging directory
// under cacheRoot and removes only that directory.
//
// A cached, plausible demo short-circuits the download, and cancellation is
// checked before the download, between extraction phases and at the commit.
func ImportDemoContext(
	ctx context.Context,
	demoURL string,
	matchID string,
	cacheRoot string,
	onProgress func(active bool, percent float64, indeterminate bool),
) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	matchID = strings.TrimSpace(matchID)
	if matchID == "" {
		return "", fmt.Errorf("比赛 ID 为空")
	}
	// The match ID becomes part of a file name, so only digits are accepted.
	if !isDigitsOnly(matchID) {
		return "", fmt.Errorf("比赛 ID 格式不正确: %s", matchID)
	}
	demoURL = strings.TrimSpace(demoURL)
	if demoURL == "" {
		return "", ErrDemoLinkUnavailable
	}
	if err := os.MkdirAll(cacheRoot, 0755); err != nil {
		return "", fmt.Errorf("创建 Steam DEM 缓存目录失败: %w", err)
	}

	stablePath := filepath.Join(cacheRoot, matchID+".dem")
	if valid, err := download.IsLikelyDemoFile(stablePath); err != nil {
		return "", fmt.Errorf("检查 Steam DEM 缓存文件失败: %w", err)
	} else if valid {
		return stablePath, nil
	}

	stagingDir, err := os.MkdirTemp(cacheRoot, ".steam-import-*")
	if err != nil {
		return "", fmt.Errorf("创建 Steam DEM 临时目录失败: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	archivePath := filepath.Join(stagingDir, matchID+".dem.bz2")
	if err := fetchArchive(ctx, demoURL, archivePath, onProgress); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	extractedPath := filepath.Join(stagingDir, matchID+".dem")
	if err := extractBzip2(ctx, archivePath, extractedPath); err != nil {
		return "", fmt.Errorf("解压 Steam DEM 失败: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Guard against committing an error page or a truncated stream: the file
	// stamp check is deliberately shallow, the parser validates the rest.
	if valid, err := download.IsLikelyDemoFile(extractedPath); err != nil {
		return "", fmt.Errorf("检查解压后的 Steam DEM 失败: %w", err)
	} else if !valid {
		return "", fmt.Errorf("解压后的文件不是有效的 CS2 demo")
	}

	if err := download.CopyFileAtomicWithContext(ctx, extractedPath, stablePath); err != nil {
		return "", fmt.Errorf("写入 Steam DEM 缓存文件失败: %w", err)
	}
	return stablePath, nil
}

func fetchArchive(ctx context.Context, demoURL, targetPath string, onProgress func(active bool, percent float64, indeterminate bool)) error {
	err := download.FileWithContext(ctx, demoURL, targetPath, download.ProgressFunc(onProgress))
	if err == nil {
		return nil
	}
	var statusErr *download.HTTPStatusError
	if errors.As(err, &statusErr) {
		if statusErr.StatusCode == http.StatusNotFound || statusErr.StatusCode == http.StatusForbidden {
			return fmt.Errorf("%w: %v", ErrDemoLinkExpired, err)
		}
	}
	return fmt.Errorf("下载 Steam DEM 失败: %w", err)
}

// extractBzip2 decompresses a single-stream .dem.bz2 archive. The standard
// library decompressor cannot be cancelled, so the copy loop checks the context
// between chunks and a corrupted or truncated stream fails the extraction.
func extractBzip2(ctx context.Context, archivePath, targetPath string) error {
	archive, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer archive.Close()

	target, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		_ = target.Close()
		if !committed {
			_ = os.Remove(targetPath)
		}
	}()

	reader := bzip2.NewReader(archive)
	buffer := make([]byte, copyBufferSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		read, readErr := reader.Read(buffer)
		if read > 0 {
			if _, writeErr := target.Write(buffer[:read]); writeErr != nil {
				return writeErr
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if err := target.Close(); err != nil {
		return err
	}
	committed = true
	return nil
}

// ProbeDemoURL reports whether a replay link still resolves, so the list can
// mark matches the user can no longer download. Network failures are treated as
// "still available" because the download itself is the final arbiter; only a
// definitive HTTP error marks a link as gone.
func ProbeDemoURL(ctx context.Context, demoURL string) bool {
	demoURL = strings.TrimSpace(demoURL)
	if demoURL == "" {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodHead, demoURL, nil)
	if err != nil {
		return true
	}
	response, err := (&http.Client{Timeout: probeTimeout}).Do(request)
	if err != nil {
		return true
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)

	switch {
	case response.StatusCode >= 200 && response.StatusCode < 300:
		return true
	case response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusForbidden:
		return false
	default:
		return true
	}
}
