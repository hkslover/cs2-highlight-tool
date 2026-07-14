package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type ProgressFunc func(active bool, percent float64, indeterminate bool)

var ErrCanceled = errors.New("下载已取消")

const (
	downloadDialTimeout           = 30 * time.Second
	downloadTLSHandshakeTimeout   = 30 * time.Second
	downloadResponseHeaderTimeout = 30 * time.Second
	downloadIdleTimeoutDefault    = 30 * time.Second
)

var downloadIdleTimeout = downloadIdleTimeoutDefault

type idleTimeoutError struct {
	err error
}

func (e *idleTimeoutError) Error() string {
	return fmt.Sprintf("下载读取超时（连续 %s 无数据）: %v", downloadIdleTimeout, e.err)
}

func (e *idleTimeoutError) Unwrap() error { return e.err }
func (e *idleTimeoutError) Timeout() bool { return true }
func (e *idleTimeoutError) Temporary() bool {
	return true
}

type idleTimeoutConn struct {
	net.Conn
	timeout time.Duration
}

func (c *idleTimeoutConn) Read(p []byte) (int, error) {
	if c.timeout > 0 {
		if err := c.Conn.SetReadDeadline(time.Now().Add(c.timeout)); err != nil {
			return 0, err
		}
	}
	n, err := c.Conn.Read(p)
	if err != nil {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return n, &idleTimeoutError{err: err}
		}
		return n, err
	}
	_ = c.Conn.SetReadDeadline(time.Time{})
	return n, nil
}

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: downloadDialTimeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   downloadTLSHandshakeTimeout,
		ResponseHeaderTimeout: downloadResponseHeaderTimeout,
		ExpectContinueTimeout: 1 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	}
	if downloadIdleTimeout > 0 {
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			return &idleTimeoutConn{Conn: conn, timeout: downloadIdleTimeout}, nil
		}
	}
	return &http.Client{Transport: transport}
}

func File(url, targetPath string, emitProgress ProgressFunc) error {
	return FileWithContext(context.Background(), url, targetPath, emitProgress)
}

func FileWithContext(ctx context.Context, url, targetPath string, emitProgress ProgressFunc) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return err
	}
	if emitProgress != nil {
		emitProgress(true, 0, true)
		defer emitProgress(false, 0, false)
	}

	client := newHTTPClient()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return ErrCanceled
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return fmt.Errorf("下载连接或响应头超时: %w", err)
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载失败，状态码: %d", resp.StatusCode)
	}

	out, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
		if err != nil {
			_ = os.Remove(targetPath)
		}
	}()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 64*1024)
	lastEmit := time.Now()
	for {
		select {
		case <-ctx.Done():
			return ErrCanceled
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				return err
			}
			downloaded += int64(n)
			if total > 0 && time.Since(lastEmit) > 200*time.Millisecond {
				if emitProgress != nil {
					emitProgress(true, float64(downloaded)*100/float64(total), false)
				}
				lastEmit = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if total > 0 && errors.Is(readErr, io.ErrUnexpectedEOF) {
				return fmt.Errorf("下载不完整: 已下载 %d/%d 字节", downloaded, total)
			}
			var idleErr *idleTimeoutError
			if errors.As(readErr, &idleErr) {
				return readErr
			}
			if errors.Is(readErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				return ErrCanceled
			}
			return readErr
		}
	}
	if total > 0 && downloaded != total {
		return fmt.Errorf("下载不完整: 已下载 %d/%d 字节", downloaded, total)
	}
	if emitProgress != nil {
		emitProgress(true, 100, false)
	}
	return nil
}
