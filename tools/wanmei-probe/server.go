package main

import (
	"embed"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"time"

	"cs2-highlight-tool-v2/internal/wanmei"
)

//go:embed web/*
var embeddedWebFS embed.FS

// NewHandler wires the diagnostic HTTP API and embedded web UI.
func NewHandler(defaults DefaultsResponse, manager *Step7Manager) (http.Handler, error) {
	webRoot, err := fs.Sub(embeddedWebFS, "web")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(webRoot))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/defaults", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, defaults)
	})
	mux.HandleFunc("POST /api/step1/login", func(w http.ResponseWriter, r *http.Request) {
		var req Step1Request
		if !decodeJSONBody(w, r, &req) {
			return
		}
		writeJSON(w, http.StatusOK, RunStep1(r.Context(), req))
	})
	mux.HandleFunc("POST /api/step2/decode", func(w http.ResponseWriter, r *http.Request) {
		var req Step2Request
		if !decodeJSONBody(w, r, &req) {
			return
		}
		writeJSON(w, http.StatusOK, RunStep2(req))
	})
	mux.HandleFunc("POST /api/step3/matches", func(w http.ResponseWriter, r *http.Request) {
		var req Step3Request
		if !decodeJSONBody(w, r, &req) {
			return
		}
		writeJSON(w, http.StatusOK, RunStep3(r.Context(), req))
	})
	mux.HandleFunc("POST /api/step4/ip", func(w http.ResponseWriter, r *http.Request) {
		var req Step4Request
		if !decodeJSONBody(w, r, &req) {
			return
		}
		writeJSON(w, http.StatusOK, RunStep4(r.Context(), req))
	})
	mux.HandleFunc("POST /api/step5/sign-url", func(w http.ResponseWriter, r *http.Request) {
		var req Step5Request
		if !decodeJSONBody(w, r, &req) {
			return
		}
		writeJSON(w, http.StatusOK, RunStep5(req))
	})
	mux.HandleFunc("POST /api/step6/oss-url", func(w http.ResponseWriter, r *http.Request) {
		var req Step6Request
		if !decodeJSONBody(w, r, &req) {
			return
		}
		writeJSON(w, http.StatusOK, RunStep6(r.Context(), req))
	})
	mux.HandleFunc("POST /api/step7/probe-head", func(w http.ResponseWriter, r *http.Request) {
		var req Step7HeadRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		writeJSON(w, http.StatusOK, RunStep7ProbeHead(r.Context(), req))
	})
	mux.HandleFunc("POST /api/step7/start", func(w http.ResponseWriter, r *http.Request) {
		var req Step7StartRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		status, startErr := manager.StartStep7(req)
		if startErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": startErr.Error()})
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("GET /api/step7/status", func(w http.ResponseWriter, r *http.Request) {
		taskID := r.URL.Query().Get("task_id")
		writeJSON(w, http.StatusOK, manager.Status(taskID))
	})
	mux.HandleFunc("POST /api/step7/cancel", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, manager.Cancel())
	})
	mux.HandleFunc("POST /api/step7/cleanup", func(w http.ResponseWriter, r *http.Request) {
		removed, cleanErr := manager.Cleanup()
		if cleanErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": cleanErr.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"removed_bytes": removed,
			"work_dir":      defaults.WorkDir,
		})
	})

	// Native internal/wanmei verification endpoints.
	mux.HandleFunc("POST /api/native/matches", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Page int `json:"page"`
		}
		if !decodeJSONBody(w, r, &req) {
			return
		}
		started := time.Now()
		res, listErr := wanmei.ListRecentMatches(req.Page)
		elapsed := time.Since(started).Milliseconds()
		if listErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":         false,
				"elapsed_ms": elapsed,
				"error":      listErr.Error(),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":         res.Status == wanmei.ClientReady,
			"elapsed_ms": elapsed,
			"result":     res,
		})
	})
	mux.HandleFunc("POST /api/native/import/start", func(w http.ResponseWriter, r *http.Request) {
		var req NativeImportRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		status, startErr := manager.StartNativeImport(req)
		if startErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": startErr.Error()})
			return
		}
		writeJSON(w, http.StatusOK, status)
	})
	mux.HandleFunc("GET /api/native/import/status", func(w http.ResponseWriter, r *http.Request) {
		taskID := r.URL.Query().Get("task_id")
		writeJSON(w, http.StatusOK, manager.Status(taskID))
	})
	mux.HandleFunc("POST /api/native/import/cancel", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, manager.Cancel())
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		fileServer.ServeHTTP(w, r)
	})

	return mux, nil
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(dst)
	if err != nil && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":    false,
			"error": "无效的 JSON 请求体: " + err.Error(),
		})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}
