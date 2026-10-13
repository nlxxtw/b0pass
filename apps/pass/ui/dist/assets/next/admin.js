(function () {
  var state = { index: null, stats: null, statsId: "", _wired: false, _poll: null };

  function $(id) {
    return document.getElementById(id);
  }

  function todayFolder() {
    var d = new Date();
    var m = d.getMonth() + 1;
    var day = d.getDate();
    return (
      "" +
      d.getFullYear() +
      (m < 10 ? "0" : "") +
      m +
      (day < 10 ? "0" : "") +
      day
    );
  }

  var cachedRoot = location.origin + "/";
  function rootLink() {
    return cachedRoot;
  }

  async function renderShare() {
    try {
      var ip = await NP.api("/pass/read-ip", { noRedirect: true });
      var port = location.port || (location.protocol === "https:" ? "443" : "80");
      if (ip) cachedRoot = location.protocol + "//" + ip + (port ? ":" + port : "") + "/";
    } catch (e) {
      cachedRoot = location.origin + "/";
    }
    $("rootLink").textContent = rootLink();
    var box = $("qr");
    box.innerHTML = "";
    if (window.QRCode) {
      new QRCode(box, { text: rootLink(), width: 128, height: 128, correctLevel: QRCode.CorrectLevel.M });
    }
  }

  function currentMode() {
    var b = document.querySelector("#modeSeg button.active");
    return b ? b.dataset.mode : "collect";
  }

  function closeStats() {
    var card = $("statsCard");
    if (card) card.hidden = true;
    state.statsId = "";
    state.stats = null;
  }

  function syncModeUI() {
    var mode = currentMode();
    var collect = mode === "collect";
    $("collectOpts").style.display = collect ? "" : "none";
    $("deadlineField").style.display = collect ? "" : "none";
    $("collectFolderField").hidden = !collect;
    $("collectFolderField").style.display = collect ? "" : "none";
    $("sharePathField").hidden = collect;
    $("sharePathField").style.display = collect ? "none" : "";
    $("accessPwdField").hidden = collect;
    $("accessPwdField").style.display = collect ? "none" : "";
    var createBtn = $("createBtn");
    createBtn.className = "btn block " + (collect ? "collect" : "");
    createBtn.textContent = collect ? "创建收作业" : "创建发资料";
    $("listTitle").textContent = collect ? "收作业列表" : "发资料列表";
    if (collect && !$("folder").value.trim()) {
      $("folder").value = todayFolder();
    }
    // 切换收作业/发资料时关掉详情，避免串台
    closeStats();
    renderList();
  }

  function listSignature(idx, mode) {
    var sessions = ((idx && idx.sessions) || []).filter(function (s) {
      return s.mode === mode;
    });
    return sessions
      .map(function (s) {
        return [s.id, s.name, s.folder || "", s.sharePath || "", s.createdAt || ""].join("|");
      })
      .join(";");
  }

  async function loadList(opts) {
    opts = opts || {};
    var modeBefore = currentMode();
    try {
      var next = await NP.api("/pass/session/list", { noRedirect: true });
      // 刷新绝不动 modeSeg；内容没变就不重绘，避免页面跳动
      if (currentMode() !== modeBefore) return;
      var changed = listSignature(state.index, modeBefore) !== listSignature(next, modeBefore);
      state.index = next;
      if (changed || opts.force) renderList();
      if (opts.toast) NP.toast("列表已刷新");
    } catch (e) {
      NP.toast(e.message);
    }
  }

  async function refreshAll(opts) {
    opts = opts || {};
    var modeBefore = currentMode();
    await loadList({ toast: false });
    if (currentMode() !== modeBefore) return;
    if (state.statsId && $("statsCard") && !$("statsCard").hidden) {
      await loadStats(state.statsId, { quiet: true });
    }
    if (opts.toast) NP.toast("已刷新");
  }

  function escapeHtml(s) {
    return String(s || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }
  function escapeAttr(s) {
    return escapeHtml(s).replace(/'/g, "&#39;");
  }

  function shortPath(p) {
    p = String(p || "");
    if (p.length <= 42) return p;
    return "…" + p.slice(-40);
  }

  function isAbsPath(p) {
    p = String(p || "").trim();
    return /^[a-zA-Z]:[\\/]/.test(p) || p.indexOf("\\\\") === 0 || (p.charAt(0) === "/" && p.length > 1);
  }

  function renderList() {
    var idx = state.index || { sessions: [] };
    var mode = currentMode();
    var ul = $("sessList");
    var sessions = (idx.sessions || []).filter(function (s) {
      return s.mode === mode;
    });
    if (!sessions.length) {
      ul.innerHTML =
        '<li class="empty-row"><div class="meta">暂无' +
        (mode === "collect" ? "收作业" : "发资料") +
        "，先在上方创建</div></li>";
      return;
    }
    ul.innerHTML = sessions
      .map(function (s) {
        var mc = NP.modeClass(s.mode);
        var isCollect = s.mode === "collect";
        var actions =
          '<button class="btn ghost" data-stats="' +
          s.id +
          '">详情</button>' +
          '<button class="btn danger" data-del="' +
          s.id +
          '" data-name="' +
          escapeAttr(s.name) +
          '">删除</button>';
        var whereFull = isCollect
          ? s.sharePath || s.folder || (s.createdAt ? s.createdAt.replace(/[-:T]/g, "").slice(0, 8) : "")
          : s.sharePath || "";
        var where = shortPath(whereFull);
        var extra = "";
        if (!isCollect && s.needPassword) {
          extra += " · 需密码";
        }
        if (!isCollect && (s.downloadCount || 0) > 0) {
          extra += " · 下载 " + s.downloadCount + " 人";
        }
        return (
          '<li class="sess-item">' +
          '<div class="sess-main"><div class="title">' +
          escapeHtml(s.name) +
          '<span class="badge ' +
          mc +
          '">' +
          NP.modeLabel(s.mode) +
          "</span></div><div class=\"meta\" title=\"" +
          escapeAttr(whereFull) +
          '">' +
          escapeHtml(where) +
          (s.createdAt ? " · " + escapeHtml(s.createdAt.replace("T", " ").slice(0, 16)) : "") +
          extra +
          '</div></div><div class="sess-actions">' +
          actions +
          "</div></li>"
        );
      })
      .join("");

    ul.querySelectorAll("[data-stats]").forEach(function (btn) {
      btn.onclick = function () {
        loadStats(btn.dataset.stats);
      };
    });
    ul.querySelectorAll("[data-del]").forEach(function (btn) {
      btn.onclick = async function () {
        if (!confirm("删除「" + btn.dataset.name + "」？")) return;
        try {
          await NP.api("/pass/session/delete?id=" + encodeURIComponent(btn.dataset.del), {
            method: "POST",
            json: {},
          });
          $("statsCard").hidden = true;
          loadList();
        } catch (e) {
          NP.toast(e.message);
        }
      };
    });
  }

  async function pickFolder(anyLocal, kind) {
    kind = kind || "folder";
    NP.toast(kind === "any" ? "请选择文件或文件夹…" : kind === "file" ? "请选择文件…" : "请选择文件夹…");
    try {
      var q = (anyLocal ? "?any=1" : "?any=0") + "&kind=" + encodeURIComponent(kind);
      return await NP.api("/pass/session/pick-folder" + q, { method: "POST", json: {} });
    } catch (e) {
      NP.toast(e.message);
      return null;
    }
  }

  function looksLikePath(s) {
    s = String(s || "").trim().replace(/^["']|["']$/g, "");
    if (!s) return false;
    if (/^[a-zA-Z]:[\\/]/.test(s)) return true;
    if (/^\\\\[^\\]+\\/.test(s)) return true;
    if (/^file:/i.test(s)) return true;
    return false;
  }

  function pathFromDataTransfer(dt) {
    if (!dt) return "";
    var plain = (dt.getData("text/plain") || dt.getData("Text") || "").trim();
    if (looksLikePath(plain)) {
      return plain.split(/\r?\n/)[0].trim().replace(/^["']|["']$/g, "");
    }
    var uri = (dt.getData("text/uri-list") || "").trim();
    if (uri) {
      var line = uri.split(/\r?\n/).filter(function (l) {
        return l && l.charAt(0) !== "#";
      })[0];
      if (line && /^file:/i.test(line)) return line.trim();
    }
    if (dt.files && dt.files.length) {
      var f = dt.files[0];
      if (f && (f.path || f.mozFullPath)) return f.path || f.mozFullPath;
    }
    return "";
  }

  async function applySharePath(raw) {
    if (!raw) return false;
    try {
      var data = await NP.api("/pass/session/resolve-path", {
        method: "POST",
        json: { path: raw },
      });
      var p = (data && (data.sharePath || data.abs)) || "";
      if (!p) return false;
      $("sharePath").value = p;
      NP.toast("已识别路径");
      return true;
    } catch (e) {
      NP.toast(e.message || "路径无效");
      return false;
    }
  }

  function wireSharePathDrop() {
    var zone = $("sharePathDrop");
    var input = $("sharePath");
    if (!zone || zone._dropWired) return;
    zone._dropWired = true;

    function setOver(on) {
      if (on) zone.classList.add("drag-over");
      else zone.classList.remove("drag-over");
    }

    // 浏览器出于安全不给网页读本机绝对路径；能读到则填入，否则弹出本机选择框
    ["dragenter", "dragover"].forEach(function (ev) {
      zone.addEventListener(ev, function (e) {
        e.preventDefault();
        e.stopPropagation();
        setOver(true);
        if (e.dataTransfer) e.dataTransfer.dropEffect = "copy";
      });
    });
    zone.addEventListener("dragleave", function (e) {
      if (zone.contains(e.relatedTarget)) return;
      setOver(false);
    });
    zone.addEventListener("drop", async function (e) {
      e.preventDefault();
      e.stopPropagation();
      setOver(false);
      var p = pathFromDataTransfer(e.dataTransfer);
      if (p) {
        await applySharePath(p);
        return;
      }
      var data = await pickFolder(true);
      if (data && (data.sharePath || data.abs)) {
        $("sharePath").value = data.sharePath || data.abs;
      }
    });
    if (input) {
      input.addEventListener("paste", function (e) {
        var text =
          (e.clipboardData && e.clipboardData.getData("text")) ||
          (window.clipboardData && window.clipboardData.getData("Text")) ||
          "";
        text = String(text || "").trim();
        if (!looksLikePath(text)) return;
        e.preventDefault();
        applySharePath(text);
      });
      input.addEventListener("change", function () {
        var v = (input.value || "").trim();
        if (looksLikePath(v)) applySharePath(v);
      });
    }
  }

  async function loadStats(id, opts) {
    opts = opts || {};
    try {
      state.statsId = id;
      state.stats = await NP.api("/pass/session/stats?id=" + encodeURIComponent(id));
      var card = $("statsCard");
      card.hidden = false;
      if (!opts.quiet) {
        card.scrollIntoView({ behavior: "smooth", block: "start" });
      }
      var s = state.stats.session || {};
      var isCollect = s.mode !== "distribute";

      $("statsTitle").textContent = s.name || "详情";
      if (isCollect) {
        $("statsSummary").textContent =
          (state.stats.studentCount || 0) + " 人已交 · " + (state.stats.fileCount || 0) + " 个文件";
        $("statsPathLabel").textContent = "作业存放位置";
        $("statsPath").value = state.stats.inboxPath || "";
        $("statsHead").innerHTML =
          "<tr><th>学号</th><th>姓名</th><th>文件</th><th>大小</th><th>时间</th><th></th></tr>";
        var rows = state.stats.submissions || [];
        $("statsBody").innerHTML = rows.length
          ? rows
              .map(function (it) {
                return (
                  "<tr><td>" +
                  escapeHtml(it.studentId || "-") +
                  "</td><td>" +
                  escapeHtml(it.studentName || "-") +
                  "</td><td>" +
                  escapeHtml(it.name) +
                  "</td><td>" +
                  escapeHtml(it.sizeText) +
                  "</td><td>" +
                  escapeHtml(it.submittedAt || "") +
                  '</td><td><button type="button" class="btn danger del-sub" data-f="' +
                  escapeAttr(it.path) +
                  '">删除</button></td></tr>'
                );
              })
              .join("")
          : '<tr><td colspan="6" class="meta">还没有学生提交</td></tr>';
      } else {
        $("statsSummary").textContent =
          (state.stats.materialCount || 0) +
          " 个文件 · 学生下载 " +
          (state.stats.downloadCount || 0) +
          " 人（按 IPv4 去重）";
        $("statsPathLabel").textContent = "本地资料文件夹";
        $("statsPath").value = s.sharePath || state.stats.materialsPath || "";
        $("statsHead").innerHTML =
          "<tr><th>文件</th><th>大小</th><th>学生下载</th></tr>";
        var mats = state.stats.materials || [];
        $("statsBody").innerHTML = mats.length
          ? mats
              .map(function (it) {
                return (
                  "<tr><td>" +
                  escapeHtml(it.name) +
                  "</td><td>" +
                  escapeHtml(it.sizeText) +
                  "</td><td>" +
                  (it.downloadCount || 0) +
                  "</td></tr>"
                );
              })
              .join("")
          : '<tr><td colspan="3" class="meta">该文件夹里还没有文件</td></tr>';
      }
      card.querySelectorAll(".del-sub").forEach(function (btn) {
        btn.onclick = async function () {
          if (!confirm("删除后学生可重新提交，确定删除？")) return;
          try {
            await NP.api(
              "/pass/session/delete-submission?id=" + encodeURIComponent(id),
              { method: "POST", json: { id: id, f: btn.dataset.f } }
            );
            NP.toast("已删除");
            await loadStats(id, { quiet: true });
            await loadList({ force: true });
          } catch (e) {
            NP.toast(e.message);
          }
        };
      });
    } catch (e) {
      NP.toast(e.message);
    }
  }

  async function revealLocal(id, f, dirOnly) {
    try {
      // 与原 b0pass「主电脑打开」同一链路（服务端 cmd.Open）
      await NP.api("/pass/session/reveal?id=" + encodeURIComponent(id), {
        method: "POST",
        json: { id: id, f: f || "", dir: !!dirOnly },
      });
      NP.toast("已在主电脑打开");
    } catch (e) {
      NP.toast(e.message || "打开失败");
    }
  }

  async function changeStatsFolder() {
    if (!state.statsId || !state.stats || !state.stats.session) return;
    var data = await pickFolder(true);
    if (!data) return;
    try {
      var abs = data.sharePath || data.abs || "";
      if (!abs) {
        NP.toast("未选择文件夹");
        return;
      }
      await NP.api("/pass/session/set-folder?id=" + encodeURIComponent(state.statsId), {
        method: "POST",
        json: { sharePath: abs },
      });
      NP.toast("已修改");
      await loadList();
      await loadStats(state.statsId);
    } catch (e) {
      NP.toast(e.message);
    }
  }

  async function createSession() {
    var name = $("name").value.trim();
    if (!name) {
      NP.toast("填写名称");
      return;
    }
    var mode = currentMode();
    var body = {
      name: name,
      mode: mode,
      requireIdentity: mode === "collect" && $("requireIdentity").checked,
      canSeeOthers: mode === "collect" && $("canSeeOthers").checked,
      canDownloadOthers: mode === "collect" && $("canDownloadOthers").checked,
      canDownloadMaterials: true,
      deadline: mode === "collect" ? $("deadline").value || "" : "",
    };
    if (mode === "collect") {
      var loc = ($("folder").value || "").trim() || todayFolder();
      if (isAbsPath(loc)) {
        body.sharePath = loc;
        body.folder = "";
      } else {
        body.folder = loc;
      }
    } else {
      body.sharePath = ($("sharePath").value || "").trim();
      if (!body.sharePath) {
        NP.toast("请选择要发给学生的文件或文件夹");
        return;
      }
      try {
        var resolved = await NP.api("/pass/session/resolve-path", {
          method: "POST",
          json: { path: body.sharePath },
        });
        if (resolved && (resolved.sharePath || resolved.abs)) {
          body.sharePath = resolved.sharePath || resolved.abs;
          $("sharePath").value = body.sharePath;
        }
      } catch (re) {
        NP.toast(re.message || "路径无效");
        return;
      }
      body.accessPassword = ($("accessPwd").value || "").trim();
    }
    try {
      var sess = await NP.api("/pass/session/create", { method: "POST", json: body });
      $("name").value = "";
      if (mode === "collect") {
        $("folder").value = todayFolder();
      } else {
        $("sharePath").value = "";
        $("accessPwd").value = "";
      }
      await loadList({ force: true });
      NP.toast(
        mode === "collect"
          ? "已创建收作业 · " + (sess.folder || body.folder)
          : "已创建发资料"
      );
    } catch (e) {
      NP.toast(e.message);
    }
  }

  function wire() {
    document.querySelectorAll("#modeSeg button").forEach(function (btn) {
      btn.onclick = function () {
        document.querySelectorAll("#modeSeg button").forEach(function (b) {
          b.classList.remove("active", "send", "collect");
        });
        btn.classList.add("active", NP.modeClass(btn.dataset.mode));
        syncModeUI();
      };
    });
    $("canSeeOthers").onchange = function () {
      $("canDownloadOthers").disabled = !$("canSeeOthers").checked;
      if (!$("canSeeOthers").checked) $("canDownloadOthers").checked = false;
    };
    $("canSeeOthers").onchange();
    $("folder").value = todayFolder();
    $("pickFolderBtn").onclick = async function () {
      var data = await pickFolder(true);
      if (!data) return;
      $("folder").value = data.abs || data.sharePath || data.folder || "";
    };
    async function applyPickedShare(data) {
      if (!data || !(data.sharePath || data.abs)) return;
      $("sharePath").value = data.sharePath || data.abs;
      if (data.wasFile) NP.toast("已填入该文件所在文件夹");
      else NP.toast("已填入文件夹路径");
    }
    $("pickShareFileBtn").onclick = async function () {
      await applyPickedShare(await pickFolder(true, "file"));
    };
    $("pickShareBtn").onclick = async function () {
      await applyPickedShare(await pickFolder(true, "folder"));
    };
    wireSharePathDrop();
    $("statsPickBtn").onclick = changeStatsFolder;
    $("statsOpenBtn").onclick = function () {
      if (!state.statsId) return;
      revealLocal(state.statsId, "", true);
    };
    $("createBtn").onclick = createSession;
    $("copyBtn").onclick = async function () {
      try {
        await navigator.clipboard.writeText(rootLink());
        NP.toast("已复制");
      } catch (e) {
        NP.toast("复制失败");
      }
    };
    $("openStudent").onclick = function () {
      window.open("/", "_blank");
    };
    $("globalRefresh").onclick = function () {
      refreshAll({ toast: true });
    };
    $("listRefresh").onclick = function () {
      loadList({ toast: true, force: true });
    };
    $("statsRefresh").onclick = function () {
      if (!state.statsId) return;
      loadStats(state.statsId, { quiet: true }).then(function () {
        NP.toast("已刷新");
      });
    };
    $("statsClose").onclick = function () {
      closeStats();
    };
    $("logoutBtn").onclick = function () {
      NP.clearAuth();
      showLogin();
    };
    syncModeUI();
  }

  function showLogin() {
    $("loginGate").hidden = false;
    $("adminApp").hidden = true;
    if (state._poll) {
      clearInterval(state._poll);
      state._poll = null;
    }
  }

  function showApp() {
    $("loginGate").hidden = true;
    $("adminApp").hidden = false;
  }

  async function tryLogin() {
    var pwd = ($("loginPwd").value || "").trim();
    var err = $("loginErr");
    err.hidden = true;
    if (!pwd) {
      err.textContent = "请输入教师密码";
      err.hidden = false;
      return;
    }
    try {
      var res = await fetch("/pass/login?code=" + encodeURIComponent(pwd));
      var data = await res.json();
      if (!data || data.code !== 0) {
        throw new Error((data && data.msg) || "密码错误");
      }
      var raw = String(data.data || "");
      var i = raw.indexOf(":");
      if (i < 0) throw new Error("登录响应异常");
      NP.setAuth(raw.slice(0, i), raw.slice(i + 1));
      $("loginPwd").value = "";
      await enterApp();
    } catch (e) {
      err.textContent = e.message || "登录失败";
      err.hidden = false;
    }
  }

  async function enterApp() {
    showApp();
    if (!state._wired) {
      wire();
      state._wired = true;
    }
    await renderShare();
    await loadList({ force: true });
    if (!state._poll) {
      state._poll = setInterval(function () {
        refreshAll({ toast: false });
      }, 4000);
    }
  }

  async function boot() {
    var updBtn = $("checkUpdateBtn");
    if (updBtn) {
      updBtn.onclick = function () {
        NP.checkUpdate({ manual: true });
      };
    }
    // 有新版本则强制弹窗，未更新不能继续用
    await NP.checkUpdate({ manual: false });
    if (document.documentElement.classList.contains("update-locked")) return;

    $("loginBtn").onclick = tryLogin;
    $("loginPwd").onkeydown = function (e) {
      if (e.key === "Enter") tryLogin();
    };
    if (!NP.getToken()) {
      showLogin();
      return;
    }
    try {
      await NP.api("/pass/session/list", { noRedirect: true });
      await enterApp();
    } catch (e) {
      NP.clearAuth();
      showLogin();
    }
  }

  boot();
})();
