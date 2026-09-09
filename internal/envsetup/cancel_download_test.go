package envsetup

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cs2-highlight-tool-v2/internal/download"
	"cs2-highlight-tool-v2/internal/release"
)

func TestCancelStartupDownloadIgnoresInactiveOrUnsupportedComponent(t *testing.T) {
	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)

	state := svc.CancelStartupDownload(componentHLAE)
	if got := findStepStatusForTest(state, componentHLAE); got != statusPending {
		t.Fatalf("inactive HLAE status = %s, want %s", got, statusPending)
	}

	state = svc.CancelStartupDownload(componentCS2)
	if got := findStepStatusForTest(state, componentCS2); got != statusPending {
		t.Fatalf("unsupported CS2 status = %s, want %s", got, statusPending)
	}
}

func findStepStatusForTest(state StartupState, componentID string) string {
	for _, step := range state.Steps {
		if step.ID == componentID {
			return step.Status
		}
	}
	return ""
}

func TestDownloadAndInstallWithFallbackCancelStopsRace(t *testing.T) {
	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	firstCanceled := make(chan struct{})
	secondCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/first.zip":
			_, _ = w.Write([]byte("partial"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(firstStarted)
			<-r.Context().Done()
			close(firstCanceled)
		case "/second.zip":
			_, _ = w.Write([]byte("partial"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(secondStarted)
			<-r.Context().Done()
			close(secondCanceled)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)
	candidates := []releaseAssetCandidate{
		{
			Source:   DownloadSourceGitHub,
			Asset:    release.Asset{Name: "first.zip"},
			AssetURL: server.URL + "/first.zip",
			URLKind:  urlKindDirect,
		},
		{
			Source:   DownloadSourceGitHub,
			Asset:    release.Asset{Name: "second.zip"},
			AssetURL: server.URL + "/second.zip",
			URLKind:  urlKindMirror,
		},
	}

	installCalled := make(chan struct{}, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.downloadAndInstallWithFallback(componentHLAE, "v2.0.0", candidates, func(path string) error {
			installCalled <- struct{}{}
			return nil
		})
	}()

	// 竞速会同时启动两条链路；确认都已进入下载后再取消。
	<-firstStarted
	<-secondStarted
	svc.CancelStartupDownload(componentHLAE)

	err := <-errCh
	if !errors.Is(err, download.ErrCanceled) {
		t.Fatalf("downloadAndInstallWithFallback error = %v, want ErrCanceled", err)
	}
	for name, canceled := range map[string]chan struct{}{
		"first":  firstCanceled,
		"second": secondCanceled,
	} {
		select {
		case <-canceled:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s download was not canceled", name)
		}
	}
	select {
	case <-installCalled:
		t.Fatal("install was called after cancel")
	default:
	}
}

func TestCancelStartupDownloadIgnoresFinishedGroup(t *testing.T) {
	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)
	active, _, cancelCause := svc.beginDownloadGroup(componentHLAE)
	defer svc.endDownloadGroup(componentHLAE, active)
	// 模拟竞速已经提交胜者：此时再取消不应把步骤改成 failed。
	cancelCause(errDownloadFinished)

	state := svc.CancelStartupDownload(componentHLAE)
	if got := findStepStatusForTest(state, componentHLAE); got == statusFailed {
		t.Fatalf("status = %s, want not failed after race already finished", got)
	}
}

func TestDownloadAndInstallWithFallback_UserCancelWinsOverCompletedDownload(t *testing.T) {
	svc := New(t.TempDir(), "1.0.0")
	svc.Startup(nil)

	fastServed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/fast.zip":
			// 响应体完整写出后、连接关闭前触发用户取消。
			// 客户端要等到 EOF 才会产生成功结果，因此成功结果必然在取消之后才入队。
			_, _ = w.Write([]byte("fast-payload"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			svc.CancelStartupDownload(componentHLAE)
			close(fastServed)
		case "/slow.zip":
			_, _ = w.Write([]byte("partial"))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	candidates := []releaseAssetCandidate{
		{
			Source:   DownloadSourceGitHub,
			Asset:    release.Asset{Name: "hlae_2_0_0.zip"},
			AssetURL: server.URL + "/fast.zip",
			URLKind:  urlKindDirect,
		},
		{
			Source:   DownloadSourceGitHub,
			Asset:    release.Asset{Name: "hlae_2_0_0.zip"},
			AssetURL: server.URL + "/slow.zip",
			URLKind:  urlKindMirror,
		},
	}

	installCalled := make(chan struct{}, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.downloadAndInstallWithFallback(componentHLAE, "v2.0.0", candidates, func(path string) error {
			installCalled <- struct{}{}
			return nil
		})
	}()

	select {
	case <-fastServed:
	case <-time.After(3 * time.Second):
		t.Fatal("fast download did not serve")
	}
	select {
	case err := <-errCh:
		if !errors.Is(err, download.ErrCanceled) {
			t.Fatalf("downloadAndInstallWithFallback error = %v, want ErrCanceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("downloadAndInstallWithFallback did not return")
	}
	select {
	case <-installCalled:
		t.Fatal("install was called after user cancel")
	default:
	}
}
