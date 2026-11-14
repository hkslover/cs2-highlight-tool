(() => {
  "use strict";

  const $ = (id) => document.getElementById(id);

  let defaults = null;
  let pipelineAbort = null;
  let step7PollTimer = null;
  let nativePollTimer = null;

  function escapeHTML(str) {
    return String(str ?? "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;");
  }

  function formatBytes(n) {
    if (n === null || n === undefined || n < 0) return "未知";
    if (n < 1024) return `${n} B`;
    const units = ["KiB", "MiB", "GiB", "TiB"];
    let val = n;
    let idx = -1;
    while (val >= 1024 && idx < units.length - 1) {
      val /= 1024;
      idx++;
    }
    return `${val.toFixed(2)} ${units[idx]}`;
  }

  async function apiJSON(url, body) {
    const opts = body !== undefined
      ? {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(body),
        }
      : { method: "GET" };
    const resp = await fetch(url, opts);
    const data = await resp.json();
    if (!resp.ok && !data.error) {
      throw new Error(`HTTP ${resp.status}`);
    }
    return data;
  }

  function setStepState(stepKey, state, msText) {
    const badge = $(`badge-${stepKey}`);
    if (badge) {
      badge.className = `status-badge ${state === "idle" ? "" : state}`;
      badge.textContent = state.toUpperCase();
    }
    const pill = document.querySelector(`.step-pill[data-step="${stepKey}"]`);
    if (pill) {
      pill.className = `step-pill ${state === "idle" ? "" : state}`;
    }
    const msEl = $(`ms-${stepKey}`);
    if (msEl && msText !== undefined) {
      msEl.textContent = msText;
    }
  }

  function bannerHTML(state, summary, errorText) {
    const errBlock = errorText
      ? `<div class="banner-error">${escapeHTML(errorText)}</div>`
      : "";
    return `<div class="banner ${escapeHTML(state)}"><strong>${escapeHTML(summary)}</strong>${errBlock}</div>`;
  }

  function kvGridHTML(items) {
    const cards = items
      .filter((item) => item && item.value !== undefined && item.value !== "")
      .map((item) => {
        const val = String(item.value);
        const copyBtn = item.copy
          ? `<button type="button" class="copy-mini" data-copy="${escapeHTML(val)}">复制</button>`
          : "";
        return `
          <div class="kv-item">
            <div class="kv-label"><span>${escapeHTML(item.label)}</span>${copyBtn}</div>
            <div class="kv-val">${escapeHTML(val)}</div>
          </div>
        `;
      });
    return cards.length ? `<div class="kv-grid">${cards.join("")}</div>` : "";
  }

  function httpTraceDetailsHTML(title, req, resp, openByDefault) {
    if (!req && !resp) return "";
    const payload = {
      request: req
        ? {
            method: req.method,
            url: req.url,
            headers: req.headers,
            body: tryParseJSON(req.body),
          }
        : null,
      response: resp
        ? {
            status_code: resp.status_code,
            status: resp.status,
            elapsed_ms: resp.elapsed_ms,
            body_size: resp.body_size,
            truncated: resp.truncated,
            error: resp.error,
            headers: resp.headers,
            raw_body: tryParseJSON(resp.raw_body),
          }
        : null,
    };
    return `
      <details class="sub-details" ${openByDefault ? "open" : ""}>
        <summary>${escapeHTML(title)}</summary>
        <pre class="code-box">${escapeHTML(JSON.stringify(payload, null, 2))}</pre>
      </details>
    `;
  }

  function tryParseJSON(raw) {
    if (!raw || typeof raw !== "string") return raw;
    const trimmed = raw.trim();
    if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return raw;
    try {
      return JSON.parse(trimmed);
    } catch {
      return raw;
    }
  }

  function showResult(stepKey, html) {
    const el = $(`result-${stepKey}`);
    el.innerHTML = html;
    el.classList.remove("hidden");
  }

  function syncPresetChips() {
    const current = $("s3-appversion").value.trim();
    document.querySelectorAll("#s3-appversion-presets .chip-btn").forEach((btn) => {
      btn.classList.toggle("active", btn.dataset.appversion === current);
    });
  }

  function applyDefaults(d) {
    defaults = d;
    $("env-badge").textContent = `${d.goos} | 临时目录: ${d.work_dir}`;

    $("s1-url").value = d.local_login_url;
    $("s1-origin").value = d.allowed_origin;
    $("s1-referer").value = d.allowed_referer;
    $("s1-timeout").value = 5;
    $("s1-headers").value = "";

    $("s3-url").value = d.match_list_url;
    $("s3-appversion").value = d.app_version;
    $("s3-platform").value = d.platform;
    $("s3-page").value = d.page;
    $("s3-pagesize").value = d.page_size;
    $("s3-tosteamid").value = "";
    $("s3-gametype").value = d.game_type;
    $("s3-gametypestr").value = d.game_type_str;
    $("s3-apptheme").value = d.app_theme;
    $("s3-season").value = d.csgo_season_id;
    $("s3-pvptype").value = d.pvp_type;
    $("s3-datasource").value = d.data_source;
    $("s3-ts").value = "";
    $("s3-timeout").value = 15;
    $("s3-headers").value = "";
    syncPresetChips();

    $("s4-url").value = d.public_ip_api;
    $("s4-timeout").value = 8;

    $("s5-baseurl").value = d.demo_base_url;
    $("s5-cupid").value = d.cup_id;
    $("s5-appid").value = d.demo_app_id;
    $("s5-secret").value = d.demo_secret;
    $("s5-r").value = "";
    $("s5-t").value = "";
    $("s5-skip-numeric").checked = false;

    $("s6-referer").value = d.pwa_referer;
    $("s6-manual-sig").value = "";
    $("s6-timeout").value = 8;
    $("s6-follow").checked = false;
    $("s6-headers").value = "";

    $("native-page").value = 1;
  }

  async function runStep1() {
    setStepState(1, "running", "...");
    const data = await apiJSON("/api/step1/login", {
      url: $("s1-url").value,
      origin: $("s1-origin").value,
      referer: $("s1-referer").value,
      custom_headers: $("s1-headers").value,
      timeout_sec: Number($("s1-timeout").value) || 5,
    });
    const state = data.ok ? "pass" : "fail";
    const ms = data.response ? `${data.response.elapsed_ms}ms` : "";
    setStepState(1, state, ms);

    if (data.parsed && data.parsed.encoded_token) {
      $("s2-encoded").value = data.parsed.encoded_token;
    }
    if (data.step2_preview && data.step2_preview.ok) {
      fillFromStep2(data.step2_preview);
    }

    const kv = kvGridHTML([
      { label: "client_status", value: data.client_status },
      { label: "HTTP 状态", value: data.response ? data.response.status_code : "" },
      { label: "nickname", value: data.parsed ? data.parsed.nickname : "" },
      { label: "encoded_token", value: data.parsed ? data.parsed.encoded_token : "", copy: true },
    ]);
    showResult(
      1,
      bannerHTML(state, data.summary, data.error) +
        kv +
        httpTraceDetailsHTML("查看请求与响应原始报文 / Headers", data.request, data.response, !data.ok)
    );
    return data;
  }

  function fillFromStep2(data) {
    if (!data || !data.ok) return;
    $("s3-token").value = data.token;
    $("s3-steamid").value = data.steam_id;
    $("s5-token").value = data.token;
    $("s6-steamid").value = data.steam_id;
    $("s6-servertime").value = data.server_time;
  }

  async function runStep2() {
    setStepState(2, "running", "...");
    const started = performance.now();
    const data = await apiJSON("/api/step2/decode", {
      encoded_token: $("s2-encoded").value,
    });
    const ms = `${Math.round(performance.now() - started)}ms`;
    const state = data.ok ? "pass" : "fail";
    setStepState(2, state, ms);

    if (data.ok) {
      fillFromStep2(data);
    }

    const kv = kvGridHTML([
      { label: "^...$ 格式校验", value: `^:${data.has_caret} / $:${data.has_trailing_dollar} / hex:${data.hex_valid} (len=${data.hex_length})` },
      { label: "token", value: data.token, copy: true },
      { label: "steam_id", value: data.steam_id, copy: true },
      { label: "server_time", value: data.server_time_human ? `${data.server_time} (${data.server_time_human})` : data.server_time },
      { label: "decoded_raw", value: data.decoded_raw, copy: true },
    ]);
    showResult(2, bannerHTML(state, data.summary, data.error) + kv);
    return data;
  }

  function selectMatchForDownstream(matchId, downloadMatchId) {
    const val = downloadMatchId || matchId || "";
    $("s5-matchid").value = val;
    $("s7-matchid").value = val;
    $("native-matchid").value = val;
    document.querySelectorAll("#result-3 tbody tr").forEach((tr) => {
      tr.classList.toggle("selected-row", tr.dataset.downloadId === val);
    });
  }

  async function runStep3() {
    setStepState(3, "running", "...");
    const data = await apiJSON("/api/step3/matches", {
      url: $("s3-url").value,
      token: $("s3-token").value,
      steam_id: $("s3-steamid").value,
      to_steam_id: $("s3-tosteamid").value,
      page: Number($("s3-page").value) || 1,
      page_size: Number($("s3-pagesize").value) || 11,
      app_version: $("s3-appversion").value,
      platform: $("s3-platform").value,
      app_theme: $("s3-apptheme").value,
      game_type: $("s3-gametype").value,
      game_type_str: $("s3-gametypestr").value,
      pvp_type: Number($("s3-pvptype").value),
      data_source: Number($("s3-datasource").value),
      csgo_season_id: $("s3-season").value,
      timestamp: $("s3-ts").value,
      custom_headers: $("s3-headers").value,
      timeout_sec: Number($("s3-timeout").value) || 15,
    });
    const state = data.ok ? "pass" : "fail";
    const ms = data.response ? `${data.response.elapsed_ms}ms` : "";
    setStepState(3, state, ms);

    const kv = kvGridHTML([
      { label: "HTTP 状态", value: data.response ? data.response.status_code : "" },
      { label: "mySteamId → toSteamId", value: data.my_steam_id ? `${data.my_steam_id} → ${data.to_steam_id}` : "" },
      { label: "API statusCode / code", value: `${data.api_status_code} / ${data.api_code}` },
      { label: "API 消息", value: data.api_message || "-" },
      { label: "Data 结构", value: `${data.data_shape || "-"} (原始 ${data.raw_item_count} 条，有效 ${data.matches.length} 条)` },
    ]);

    let tableHTML = "";
    if (data.matches && data.matches.length > 0) {
      if (!$("s5-matchid").value.trim()) {
        selectMatchForDownstream(data.matches[0].match_id, data.matches[0].download_match_id);
      }
      const selectedID = $("s5-matchid").value.trim();
      const rows = data.matches
        .map((m) => {
          const isSel = selectedID === m.download_match_id;
          return `
            <tr class="${isSel ? "selected-row" : ""}" data-download-id="${escapeHTML(m.download_match_id)}">
              <td class="mono">${escapeHTML(m.download_match_id)}</td>
              <td>${escapeHTML(m.map_name)}</td>
              <td>${m.score1}:${m.score2}</td>
              <td>${m.kill}/${m.death}/${m.assist}</td>
              <td>${m.k4}/${m.k5}</td>
              <td>${Number(m.rating).toFixed(2)}</td>
              <td>${escapeHTML(m.end_time)}</td>
              <td><button type="button" class="btn btn-sm btn-ghost pick-match-btn" data-match="${escapeHTML(m.match_id)}" data-dl="${escapeHTML(m.download_match_id)}">选用</button></td>
            </tr>
          `;
        })
        .join("");
      tableHTML = `
        <div class="table-wrap">
          <table>
            <thead>
              <tr><th>Download ID</th><th>地图</th><th>比分</th><th>K/D/A</th><th>4K/5K</th><th>Rating</th><th>结束时间</th><th>操作</th></tr>
            </thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      `;
    }

    let skippedHTML = "";
    if (data.skipped_items && data.skipped_items.length > 0) {
      skippedHTML = `
        <details class="sub-details">
          <summary>被跳过的比赛记录 (${data.skipped_items.length} 条)</summary>
          <pre class="code-box">${escapeHTML(JSON.stringify(data.skipped_items, null, 2))}</pre>
        </details>
      `;
    }

    showResult(
      3,
      bannerHTML(state, data.summary, data.error) +
        kv +
        tableHTML +
        skippedHTML +
        httpTraceDetailsHTML("查看战绩请求与响应 JSON / Headers", data.request, data.response, !data.ok)
    );
    return data;
  }

  async function runStep4() {
    setStepState(4, "running", "...");
    const data = await apiJSON("/api/step4/ip", {
      url: $("s4-url").value,
      timeout_sec: Number($("s4-timeout").value) || 8,
    });
    const state = data.ok ? "pass" : "fail";
    const ms = data.response ? `${data.response.elapsed_ms}ms` : "";
    setStepState(4, state, ms);

    if (data.ok && data.ip) {
      $("s6-ip").value = data.ip;
    }

    const kv = kvGridHTML([
      { label: "公网 IPv4", value: data.ip, copy: true },
      { label: "IPv4 正则校验", value: String(data.valid_ipv4) },
      { label: "HTTP 状态", value: data.response ? data.response.status_code : "" },
    ]);
    showResult(
      4,
      bannerHTML(state, data.summary, data.error) +
        kv +
        httpTraceDetailsHTML("查看公网 IP 请求与响应", data.request, data.response, !data.ok)
    );
    return data;
  }

  async function runStep5() {
    setStepState(5, "running", "...");
    const started = performance.now();
    const data = await apiJSON("/api/step5/sign-url", {
      demo_base_url: $("s5-baseurl").value,
      match_id: $("s5-matchid").value,
      cup_id: $("s5-cupid").value,
      access_token: $("s5-token").value,
      app_id: $("s5-appid").value,
      secret: $("s5-secret").value,
      randnum: $("s5-r").value,
      timestamp: $("s5-t").value,
      skip_numeric_check: $("s5-skip-numeric").checked,
    });
    const ms = `${Math.round(performance.now() - started)}ms`;
    const state = data.ok ? "pass" : "fail";
    setStepState(5, state, ms);

    if (data.ok) {
      $("s6-url").value = data.signed_url;
      $("s7-matchid").value = data.download_match_id;
    }

    const kv = kvGridHTML([
      { label: "download_match_id", value: data.download_match_id },
      { label: "r / t", value: data.randnum ? `${data.randnum} / ${data.timestamp}` : "" },
      { label: "signed_url", value: data.signed_url, copy: true },
    ]);
    const cryptoDetails = data.ok
      ? `
        <details class="sub-details">
          <summary>查看签名计算过程 (MD5 → SHA1)</summary>
          <pre class="code-box">${escapeHTML(JSON.stringify({
            path: data.path,
            query_string: data.query_string,
            md5_input: data.md5_input,
            md5_hex: data.md5_hex,
            sha1_input: data.sha1_input,
            sha1_hex: data.sha1_hex,
            signed_url: data.signed_url,
          }, null, 2))}</pre>
        </details>
      `
      : "";
    showResult(5, bannerHTML(state, data.summary, data.error) + kv + cryptoDetails);
    return data;
  }

  async function runStep6() {
    setStepState(6, "running", "...");
    const data = await apiJSON("/api/step6/oss-url", {
      signed_url: $("s6-url").value,
      steam_id: $("s6-steamid").value,
      ip_addr: $("s6-ip").value,
      server_time: $("s6-servertime").value,
      referer: $("s6-referer").value,
      manual_signature: $("s6-manual-sig").value,
      custom_headers: $("s6-headers").value,
      follow_redirects: $("s6-follow").checked,
      timeout_sec: Number($("s6-timeout").value) || 8,
    });
    const state = data.ok ? "pass" : "fail";
    const ms = data.response ? `${data.response.elapsed_ms}ms` : "";
    setStepState(6, state, ms);

    if (data.ok && data.oss_url) {
      $("s7-url").value = data.oss_url;
    }

    const kv = kvGridHTML([
      { label: "HTTP 状态", value: data.response ? data.response.status_code : "" },
      { label: "Location 头", value: data.location, copy: true },
      { label: "解析后的 OSS URL", value: data.oss_url, copy: true },
      { label: "X-PWA-Signature", value: data.crypto ? data.crypto.signature : "", copy: true },
    ]);

    let candidateHTML = "";
    if (!data.ok && data.detected_urls && data.detected_urls.length > 0) {
      const items = data.detected_urls
        .map(
          (u) => `
            <div class="url-candidate-row">
              <span class="mono" style="word-break:break-all;">${escapeHTML(u)}</span>
              <button type="button" class="btn btn-sm btn-ghost use-oss-candidate" data-url="${escapeHTML(u)}">填入 Step 7</button>
            </div>
          `
        )
        .join("");
      candidateHTML = `<div class="url-candidates">${items}</div>`;
    }

    const cryptoDetails = data.crypto && data.crypto.aes_key
      ? `
        <details class="sub-details">
          <summary>查看 AES-CBC 签名详情</summary>
          <pre class="code-box">${escapeHTML(JSON.stringify(data.crypto, null, 2))}</pre>
        </details>
      `
      : "";

    showResult(
      6,
      bannerHTML(state, data.summary, data.error) +
        kv +
        candidateHTML +
        cryptoDetails +
        httpTraceDetailsHTML("查看换链请求与完整响应体（含 200 JSON / Headers）", data.request, data.response, !data.ok)
    );
    return data;
  }

  async function runStep7HeadProbe() {
    setStepState(7, "running", "...");
    const data = await apiJSON("/api/step7/probe-head", {
      oss_url: $("s7-url").value,
      timeout_sec: 15,
    });
    const state = data.ok ? (data.sniff && data.sniff.is_expected_zip ? "pass" : "warn") : "fail";
    setStepState(7, state, `${data.elapsed_ms || 0}ms`);

    const sniff = data.sniff || {};
    const kv = kvGridHTML([
      { label: "HTTP 状态", value: data.status_code },
      { label: "Content-Length", value: `${formatBytes(data.content_length)} (${data.content_length})` },
      { label: "Content-Type", value: data.content_type },
      { label: "魔数识别格式", value: `${sniff.format || "-"} (${sniff.description || ""})` },
      { label: "前 32 字节 Hex", value: sniff.hex_preview },
      { label: "前 32 字节 ASCII", value: sniff.ascii_preview },
    ]);

    const previewBox = sniff.text_preview
      ? `<details class="sub-details" open><summary>响应文本预览</summary><pre class="code-box">${escapeHTML(sniff.text_preview)}</pre></details>`
      : "";

    showResult(
      7,
      bannerHTML(state, data.summary, data.error) +
        kv +
        previewBox +
        httpTraceDetailsHTML("查看 OSS 响应头", data.request, { status_code: data.status_code, status: data.status, headers: data.headers, elapsed_ms: data.elapsed_ms }, !data.ok)
    );
    return data;
  }

  function renderStep7Status(st) {
    let state = "running";
    if (!st.running) {
      if (st.ok) state = "pass";
      else if (st.phase === "canceled") state = "warn";
      else if (st.phase === "idle") state = "idle";
      else state = "fail";
    }
    setStepState(7, state, st.elapsed_ms ? `${st.elapsed_ms}ms` : "");

    const wp = st.wanmei_pipeline || {};
    const fb = st.fallback_diag || {};
    const sniff = st.sniff || null;

    const progressHTML = st.running
      ? `
        <div>
          <div style="display:flex;justify-content:space-between;font-size:12px;color:var(--text-muted);margin-bottom:4px;">
            <span>阶段: ${escapeHTML(st.phase)}</span>
            <span>${st.indeterminate ? `已接收 ${formatBytes(st.downloaded_bytes)}` : `${Number(st.progress_percent || 0).toFixed(1)}% (${formatBytes(st.downloaded_bytes)})`}</span>
          </div>
          <div class="progress-track">
            <div class="progress-fill ${st.indeterminate ? "indeterminate" : ""}" style="width:${Math.min(100, Math.max(0, st.progress_percent || 0))}%"></div>
          </div>
        </div>
      `
      : "";

    const checksHTML = `
      <div class="pipeline-checks">
        <span class="check-chip ${wp.download_ok ? "pass" : wp.download_error ? "fail" : ""}">1. 下载文件 ${wp.download_ok ? `✓ (${wp.download_ms}ms)` : wp.download_error ? "✕" : "…"}</span>
        <span class="check-chip ${sniff ? (sniff.is_expected_zip ? "pass" : "fail") : ""}">2. 魔数识别 ${sniff ? sniff.format : "…"}</span>
        <span class="check-chip ${wp.unzip_ok ? "pass" : wp.unzip_error ? "fail" : ""}">3. UnzipWithContext ${wp.unzip_ok ? `✓ (${wp.unzip_ms}ms)` : wp.unzip_error ? "✕" : "…"}</span>
        <span class="check-chip ${wp.find_dem_ok ? "pass" : wp.find_dem_error ? "fail" : ""}">4. FindFirstByExt(.dem) ${wp.find_dem_ok ? "✓" : wp.find_dem_error ? "✕" : "…"}</span>
        <span class="check-chip ${wp.likely_demo_ok ? "pass" : wp.likely_demo_error ? "fail" : ""}">5. IsLikelyDemoFile ${wp.likely_demo_ok ? `✓ (${wp.likely_demo_stamp})` : "…"}</span>
        <span class="check-chip ${wp.commit_ok ? "pass" : wp.commit_error ? "fail" : ""}">6. 原子提交 ${wp.commit_ok ? "✓" : wp.commit_error ? "✕" : "…"}</span>
      </div>
    `;

    const kv = kvGridHTML([
      { label: "下载大小", value: st.archive_size ? `${formatBytes(st.archive_size)} (${st.archive_size} B)` : "" },
      { label: "魔数识别", value: sniff ? `${sniff.format} — ${sniff.description}` : "" },
      { label: "Hex 头", value: sniff ? sniff.hex_preview : "" },
      { label: ".dem 大小 / 文件头", value: wp.dem_size ? `${formatBytes(wp.dem_size)} / ${wp.likely_demo_stamp || "-"}` : "" },
      { label: "临时文件路径", value: wp.stable_commit_path || wp.dem_path || st.archive_path, copy: true },
    ]);

    let fallbackHTML = "";
    if (fb.attempted) {
      fallbackHTML = bannerHTML(
        fb.success ? "warn" : "fail",
        `格式兼容诊断 (${fb.format_detected})：${fb.message}`,
        fb.error
      );
    }

    const entries = (wp.archive_entries && wp.archive_entries.length ? wp.archive_entries : fb.entries) || [];
    let entriesHTML = "";
    if (entries.length > 0) {
      const rows = entries
        .map((e) => `<tr><td class="mono">${escapeHTML(e.name)}</td><td>${e.is_dir ? "目录" : formatBytes(e.uncompressed_size)}</td></tr>`)
        .join("");
      entriesHTML = `
        <details class="sub-details" open>
          <summary>压缩包内文件列表 (${entries.length})</summary>
          <div class="table-wrap"><table><thead><tr><th>文件名</th><th>未压缩大小</th></tr></thead><tbody>${rows}</tbody></table></div>
        </details>
      `;
    }

    const textPreviewHTML = sniff && sniff.text_preview
      ? `<details class="sub-details" open><summary>响应文本内容预览</summary><pre class="code-box">${escapeHTML(sniff.text_preview)}</pre></details>`
      : "";

    showResult(7, bannerHTML(state, st.summary, st.error) + progressHTML + checksHTML + kv + fallbackHTML + entriesHTML + textPreviewHTML);
  }

  async function runStep7Full() {
    if (step7PollTimer) {
      clearInterval(step7PollTimer);
      step7PollTimer = null;
    }
    setStepState(7, "running", "...");
    $("btn-s7-cancel").classList.remove("hidden");

    const started = await apiJSON("/api/step7/start", {
      oss_url: $("s7-url").value,
      match_id: $("s7-matchid").value,
      run_fallback_diag: $("s7-fallback").checked,
    });
    renderStep7Status(started);

    return new Promise((resolve) => {
      const poll = async () => {
        try {
          const st = await apiJSON(`/api/step7/status?task_id=${encodeURIComponent(started.task_id)}`);
          renderStep7Status(st);
          if (!st.running) {
            clearInterval(step7PollTimer);
            step7PollTimer = null;
            $("btn-s7-cancel").classList.add("hidden");
            resolve(st);
          }
        } catch (err) {
          clearInterval(step7PollTimer);
          step7PollTimer = null;
          $("btn-s7-cancel").classList.add("hidden");
          setStepState(7, "fail", "");
          showResult(7, bannerHTML("fail", "轮询 Step 7 状态失败", err.message));
          resolve({ ok: false, error: err.message });
        }
      };
      step7PollTimer = setInterval(poll, 350);
    });
  }

  async function runSingleStep(step) {
    try {
      switch (step) {
        case 1: return await runStep1();
        case 2: return await runStep2();
        case 3: return await runStep3();
        case 4: return await runStep4();
        case 5: return await runStep5();
        case 6: return await runStep6();
        case 7:
          return $("pipeline-head-only").checked ? await runStep7HeadProbe() : await runStep7Full();
      }
    } catch (err) {
      setStepState(step, "fail", "");
      showResult(step, bannerHTML("fail", `Step ${step} 请求异常`, err.message));
      return { ok: false, error: err.message };
    }
  }

  async function runPipelineFrom(startStep) {
    if (pipelineAbort) return;
    const ctrl = { aborted: false };
    pipelineAbort = ctrl;
    $("btn-run-all").disabled = true;
    $("btn-stop-all").disabled = false;

    try {
      for (let step = startStep; step <= 7; step++) {
        if (ctrl.aborted) break;
        const card = $(`card-${step}`);
        if (card) card.scrollIntoView({ behavior: "smooth", block: "nearest" });
        const res = await runSingleStep(step);
        if (ctrl.aborted) break;
        if (!res || !res.ok) {
          break;
        }
        if (step === 3 && (!res.matches || res.matches.length === 0) && !$("s5-matchid").value.trim()) {
          // Matches list returned empty; cannot pick a match_id for Step 5 automatically.
          break;
        }
      }
    } finally {
      pipelineAbort = null;
      $("btn-run-all").disabled = false;
      $("btn-stop-all").disabled = true;
    }
  }

  async function stopRunningWork() {
    if (pipelineAbort) {
      pipelineAbort.aborted = true;
    }
    try {
      await apiJSON("/api/step7/cancel", {});
    } catch {
      // ignore
    }
  }

  async function runNativeMatches() {
    setStepState("native", "running", "...");
    const data = await apiJSON("/api/native/matches", {
      page: Number($("native-page").value) || 1,
    });
    const state = data.ok ? "pass" : "fail";
    setStepState("native", state, `${data.elapsed_ms || 0}ms`);
    const summary = data.ok
      ? `wanmei.ListRecentMatches(${Number($("native-page").value) || 1}) 成功：${data.result.nickname || ""}，共 ${data.result.matches.length} 场`
      : `wanmei.ListRecentMatches 返回状态: ${data.result ? data.result.status : "error"}`;
    if (data.result && data.result.matches && data.result.matches.length > 0 && !$("native-matchid").value.trim()) {
      $("native-matchid").value = data.result.matches[0].download_match_id;
    }
    showResult(
      "native",
      bannerHTML(state, summary, data.error) +
        `<details class="sub-details" open><summary>原始返回结构</summary><pre class="code-box">${escapeHTML(JSON.stringify(data, null, 2))}</pre></details>`
    );
  }

  async function runNativeImport() {
    if (nativePollTimer) {
      clearInterval(nativePollTimer);
      nativePollTimer = null;
    }
    const matchID = $("native-matchid").value.trim();
    if (!matchID) {
      setStepState("native", "fail", "");
      showResult("native", bannerHTML("fail", "请先填写 match_id 或先运行 ListRecentMatches"));
      return;
    }
    setStepState("native", "running", "...");
    $("btn-native-cancel").classList.remove("hidden");
    const started = await apiJSON("/api/native/import/start", { match_id: matchID });
    if (started.error) {
      $("btn-native-cancel").classList.add("hidden");
      setStepState("native", "fail", "");
      showResult("native", bannerHTML("fail", "启动原生 ImportDemoContext 失败", started.error));
      return;
    }
    const renderNative = (st) => {
      const state = st.running ? "running" : st.ok ? "pass" : st.phase === "canceled" ? "warn" : "fail";
      setStepState("native", state, st.elapsed_ms ? `${st.elapsed_ms}ms` : "");
      showResult(
        "native",
        bannerHTML(state, st.summary, st.error) +
          kvGridHTML([
            { label: "阶段", value: st.phase },
            { label: "进度", value: st.indeterminate ? "进行中" : `${Number(st.progress_percent || 0).toFixed(1)}%` },
            { label: "输出 .dem 路径", value: st.native_import_path, copy: true },
          ])
      );
    };
    renderNative(started);
    nativePollTimer = setInterval(async () => {
      try {
        const st = await apiJSON(`/api/native/import/status?task_id=${encodeURIComponent(started.task_id)}`);
        renderNative(st);
        if (!st.running) {
          clearInterval(nativePollTimer);
          nativePollTimer = null;
          $("btn-native-cancel").classList.add("hidden");
        }
      } catch (err) {
        clearInterval(nativePollTimer);
        nativePollTimer = null;
        $("btn-native-cancel").classList.add("hidden");
        setStepState("native", "fail", "");
        showResult("native", bannerHTML("fail", "查询原生导入状态失败", err.message));
      }
    }, 350);
  }

  function wireEvents() {
    document.querySelectorAll(".step-pill").forEach((pill) => {
      pill.addEventListener("click", () => {
        const card = $(`card-${pill.dataset.step}`);
        if (card) card.scrollIntoView({ behavior: "smooth", block: "start" });
      });
    });

    document.querySelectorAll("[data-run-step]").forEach((btn) => {
      btn.addEventListener("click", () => runSingleStep(Number(btn.dataset.runStep)));
    });

    document.querySelectorAll("[data-run-from]").forEach((btn) => {
      btn.addEventListener("click", () => runPipelineFrom(Number(btn.dataset.runFrom)));
    });

    $("btn-run-all").addEventListener("click", () => runPipelineFrom(1));
    $("btn-stop-all").addEventListener("click", stopRunningWork);
    $("btn-reset-defaults").addEventListener("click", () => {
      if (defaults) applyDefaults(defaults);
    });

    $("s3-appversion").addEventListener("input", syncPresetChips);
    document.querySelectorAll("#s3-appversion-presets .chip-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        $("s3-appversion").value = btn.dataset.appversion;
        syncPresetChips();
      });
    });

    $("btn-s7-full").addEventListener("click", () => runStep7Full());
    $("btn-s7-head").addEventListener("click", () => runStep7HeadProbe());
    $("btn-s7-cancel").addEventListener("click", async () => {
      await apiJSON("/api/step7/cancel", {});
    });
    $("btn-s7-cleanup").addEventListener("click", async () => {
      const res = await apiJSON("/api/step7/cleanup", {});
      setStepState(7, "idle", "");
      showResult(7, bannerHTML("warn", `已清理临时目录 (${formatBytes(res.removed_bytes)})`, ""));
    });

    $("btn-native-matches").addEventListener("click", () => runNativeMatches().catch((e) => {
      setStepState("native", "fail", "");
      showResult("native", bannerHTML("fail", "调用失败", e.message));
    }));
    $("btn-native-import").addEventListener("click", () => runNativeImport().catch((e) => {
      setStepState("native", "fail", "");
      showResult("native", bannerHTML("fail", "调用失败", e.message));
    }));
    $("btn-native-cancel").addEventListener("click", async () => {
      await apiJSON("/api/native/import/cancel", {});
    });

    document.addEventListener("click", async (e) => {
      const copyBtn = e.target.closest("[data-copy]");
      if (copyBtn) {
        try {
          await navigator.clipboard.writeText(copyBtn.dataset.copy);
          const old = copyBtn.textContent;
          copyBtn.textContent = "已复制";
          setTimeout(() => { copyBtn.textContent = old; }, 1200);
        } catch {
          // ignore clipboard errors
        }
        return;
      }

      const pickBtn = e.target.closest(".pick-match-btn");
      if (pickBtn) {
        selectMatchForDownstream(pickBtn.dataset.match, pickBtn.dataset.dl);
        return;
      }

      const candidateBtn = e.target.closest(".use-oss-candidate");
      if (candidateBtn) {
        $("s7-url").value = candidateBtn.dataset.url;
        $("card-7").scrollIntoView({ behavior: "smooth", block: "start" });
      }
    });
  }

  async function init() {
    wireEvents();
    const d = await apiJSON("/api/defaults");
    applyDefaults(d);
  }

  init().catch((err) => {
    console.error("初始化失败:", err);
  });
})();
