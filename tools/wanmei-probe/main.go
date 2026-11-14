package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:18999", "HTTP listen address")
	openBrowser := flag.Bool("open", true, "automatically open browser on startup")
	workDirFlag := flag.String("workdir", "", "temporary directory for Step 7 downloads (default: auto temp dir cleaned on exit)")
	flag.Parse()

	workDir := strings.TrimSpace(*workDirFlag)
	autoCleanupWorkDir := false
	if workDir == "" {
		created, err := os.MkdirTemp("", "wanmei-probe-*")
		if err != nil {
			fmt.Fprintf(os.Stderr, "创建临时工作目录失败: %v\n", err)
			os.Exit(1)
		}
		workDir = created
		autoCleanupWorkDir = true
	} else {
		if err := os.MkdirAll(workDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "准备指定工作目录失败: %v\n", err)
			os.Exit(1)
		}
	}

	defer func() {
		if autoCleanupWorkDir {
			_ = os.RemoveAll(workDir)
		}
	}()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "监听地址 %s 失败: %v\n", *addr, err)
		os.Exit(1)
	}

	url := "http://" + ln.Addr().String()
	defaults := defaultConfigResponse(workDir)
	manager := NewStep7Manager(workDir)

	handler, err := NewHandler(defaults, manager)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化 HTTP 路由失败: %v\n", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	fmt.Printf("Wanmei Demo 全链路诊断工具已启动: %s\n", url)
	fmt.Printf("临时下载目录: %s\n", workDir)
	fmt.Println("按 Ctrl+C 退出并清理临时文件。")

	if *openBrowser {
		go launchBrowser(url)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
		close(errCh)
	}()

	select {
	case serveErr := <-errCh:
		if serveErr != nil {
			fmt.Fprintf(os.Stderr, "服务异常退出: %v\n", serveErr)
			os.Exit(1)
		}
	case <-sigCh:
		fmt.Println("\n正在停止任务并退出...")
		_, _ = manager.Cleanup()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = srv.Shutdown(shutdownCtx)
		cancel()
	}
}

func launchBrowser(targetURL string) {
	time.Sleep(150 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	_ = cmd.Start()
}
