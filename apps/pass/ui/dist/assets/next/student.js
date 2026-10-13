(function () {
  var state = { sessions: [], openId: "" };

  function $(id) {
    return document.getElementById(id);
  }

  function identityQuery(extra) {
    var p = NP.studentProfile();
    return NP.qs(
      Object.assign(
        {
          studentId: p.studentId,
          studentName: p.studentName,
          anonId: p.anonId,
        },
        extra || {}
      )
    );
  }

  function findSession(id) {
    for (var i = 0; i < state.sessions.length; i++) {
      if (state.sessions[i].id === id) return state.sessions[i];
    }
    return null;
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

  function renderEmpty(msg) {
    $("main").innerHTML = '<div class="card empty">' + escapeHtml(msg || "暂无任务") + "</div>";
  }

  function badgeFor(s) {
    if (s.mode === "collect" && s.submitted) {
      return { text: "已提交", cls: "done" };
    }
    return { text: NP.modeLabel(s.mode, "student"), cls: NP.modeClass(s.mode) };
  }

  function renderList() {
    if (!state.sessions.length) {
      renderEmpty("暂无任务");
      return;
    }
    var html = "";
    state.sessions.forEach(function (s) {
      var open = state.openId === s.id;
      var badge = badgeFor(s);
      var meta = "";
      if (s.mode === "collect" && s.submitted && s.submittedAt) {
        meta = "最近提交 " + escapeHtml(s.submittedAt);
      } else if (s.deadline) {
        meta = "截止 " + escapeHtml(String(s.deadline).replace("T", " "));
      }
      if (s.expired) {
        meta += (meta ? " · " : "") + "已截止";
      }
      html +=
        '<div class="card acc' +
        (open ? " open" : "") +
        (s.mode === "collect" && s.submitted ? " done" : "") +
        '" data-id="' +
        escapeAttr(s.id) +
        '">' +
        '<button type="button" class="acc-head">' +
        "<div><div class=\"title\">" +
        escapeHtml(s.name) +
        '<span class="badge ' +
        badge.cls +
        '">' +
        escapeHtml(badge.text) +
        "</span></div>" +
        (meta ? '<div class="meta">' + meta + "</div>" : "") +
        "</div>" +
        '<span class="acc-che">' +
        (open ? "▾" : "▸") +
        "</span></button>" +
        '<div class="acc-body" id="body-' +
        escapeAttr(s.id) +
        '"' +
        (open ? "" : " hidden") +
        "></div></div>";
    });
    $("main").innerHTML = html;

    document.querySelectorAll(".acc").forEach(function (card) {
      card.querySelector(".acc-head").onclick = function () {
        var id = card.getAttribute("data-id");
        state.openId = state.openId === id ? "" : id;
        renderList();
        if (state.openId) fillBody(state.openId);
      };
    });
    if (state.openId) fillBody(state.openId);
  }

  function fillBody(id) {
    var s = findSession(id);
    var box = document.getElementById("body-" + id);
    if (!s || !box) return;

    var html = "";
    if (s.mode === "distribute") {
      html += '<div class="block"><ul class="list flist"><li><div class="meta">加载中</div></li></ul></div>';
    } else {
      if (s.requireIdentity) {
        var p = NP.studentProfile();
        html +=
          '<div class="block"><h3>身份</h3>' +
          '<label class="field"><span>学号</span><input class="sid" type="text" value="' +
          escapeAttr(p.studentId) +
          '"></label>' +
          '<label class="field"><span>姓名</span><input class="sname" type="text" value="' +
          escapeAttr(p.studentName) +
          '"></label></div>';
      }
      if (!s.expired) {
        html +=
          '<div class="block"><h3>上传</h3>' +
          '<input type="file" class="file" multiple style="display:none">' +
          '<button type="button" class="btn block collect pick">选择文件</button>' +
          '<p class="meta uphint"></p></div>';
      } else {
        html += '<div class="block"><p class="meta" style="color:var(--danger)">已截止</p></div>';
      }
      html += '<div class="block"><ul class="list flist"><li><div class="meta">加载中</div></li></ul></div>';
    }
    box.innerHTML = html;

    var pick = box.querySelector(".pick");
    var fileInput = box.querySelector(".file");
    if (pick && fileInput) {
      pick.onclick = function () {
        fileInput.click();
      };
      fileInput.onchange = function () {
        uploadFiles(id, s, box, fileInput.files);
      };
    }
    loadFiles(id, box, s);
  }

  async function revealFile(id, f, dirOnly) {
    try {
      await NP.api("/pass/public/reveal?id=" + encodeURIComponent(id), {
        method: "POST",
        json: { id: id, f: f || "", dir: !!dirOnly },
        noRedirect: true,
      });
      NP.toast(dirOnly ? "已在主电脑打开作业文件夹" : "已在主电脑打开");
    } catch (e) {
      NP.toast(e.message || "打开失败");
    }
  }

  async function deleteMine(id, f, box, s) {
    if (!confirm("删除后可重新提交，确定删除？")) return;
    try {
      await NP.api("/pass/public/delete-submission?" + identityQuery({ id: id }), {
        method: "POST",
        json: { f: f },
        noRedirect: true,
      });
      NP.toast("已删除，可重新提交");
      await refresh({ toast: false });
      if (state.openId === id) {
        var nb = document.getElementById("body-" + id);
        var ns = findSession(id);
        if (nb && ns) loadFiles(id, nb, ns);
      }
    } catch (e) {
      NP.toast(e.message || "删除失败");
    }
  }

  async function loadFiles(id, box, s) {
    var list = box.querySelector(".flist");
    if (!list) return;
    try {
      var data = await NP.api("/pass/public/files?" + identityQuery({ id: id }));
      var items = [];
      (data.materials || []).forEach(function (it) {
        items.push(it);
      });
      if (s.mode === "collect") {
        (data.submissions || []).forEach(function (it) {
          items.push(it);
        });
      }
      if (!items.length) {
        list.innerHTML = '<li><div class="meta">暂无文件</div></li>';
        return;
      }
      list.innerHTML = items
        .map(function (it) {
          var meta = [];
          if (it.kind === "submission") {
            meta.push(it.mine ? "我的" : escapeHtml(it.studentName || "同学"));
            if (it.submittedAt) meta.push(it.submittedAt);
          }
          meta.push(it.sizeText || "");
          var action = "";
          if (it.kind === "material" && it.canDownload) {
            action =
              '<button type="button" class="btn ghost getfile" data-url="/pass/public/download?' +
              identityQuery({ id: id, f: it.path }) +
              '" data-name="' +
              escapeAttr(it.name) +
              '" data-size="' +
              (it.size || 0) +
              '">下载</button>';
          } else if (it.kind === "submission" && it.mine) {
            action =
              '<div class="sess-actions">' +
              '<button type="button" class="btn ghost openfile" data-f="' +
              escapeAttr(it.path) +
              '">打开</button>' +
              '<button type="button" class="btn danger delfile" data-f="' +
              escapeAttr(it.path) +
              '">删除</button></div>';
          } else if (it.canDownload) {
            action =
              '<button type="button" class="btn ghost getfile" data-url="/pass/public/download?' +
              identityQuery({ id: id, f: it.path }) +
              '" data-name="' +
              escapeAttr(it.name) +
              '" data-size="' +
              (it.size || 0) +
              '">下载</button>';
          } else {
            action = '<span class="meta">—</span>';
          }
          return (
            "<li><div><div class=\"title\">" +
            escapeHtml(it.name) +
            '</div><div class="meta">' +
            meta.join(" · ") +
            "</div></div>" +
            action +
            "</li>"
          );
        })
        .join("");
      list.querySelectorAll(".getfile").forEach(function (btn) {
        btn.onclick = function () {
          NP.toast("开始下载…");
          NP.downloadFile(btn.dataset.url, btn.dataset.name, Number(btn.dataset.size) || 0);
        };
      });
      list.querySelectorAll(".openfile").forEach(function (btn) {
        btn.onclick = function () {
          revealFile(id, btn.dataset.f, true);
        };
      });
      list.querySelectorAll(".delfile").forEach(function (btn) {
        btn.onclick = function () {
          deleteMine(id, btn.dataset.f, box, s);
        };
      });
    } catch (e) {
      list.innerHTML = '<li><div class="meta">' + escapeHtml(e.message) + "</div></li>";
    }
  }

  async function uploadFiles(id, s, box, fileList) {
    if (!fileList || !fileList.length) return;
    if (s.requireIdentity) {
      var sid = (box.querySelector(".sid") && box.querySelector(".sid").value.trim()) || "";
      var sname = (box.querySelector(".sname") && box.querySelector(".sname").value.trim()) || "";
      if (!sid || !sname) {
        NP.toast("填写学号姓名");
        return;
      }
      NP.saveStudentProfile(sid, sname);
    }
    var hint = box.querySelector(".uphint");
    var pickBtn = box.querySelector(".pick");
    if (pickBtn) pickBtn.disabled = true;
    for (var i = 0; i < fileList.length; i++) {
      var f = fileList[i];
      try {
        var p = NP.studentProfile();
        var url =
          "/pass/public/upload?" +
          NP.qs({
            id: id,
            studentId: p.studentId,
            studentName: p.studentName,
            anonId: p.anonId,
          });
        await NP.uploadFile(url, f, function (loaded, total) {
          if (!hint) return;
          var pct = total ? Math.floor((loaded / total) * 100) : 0;
          hint.textContent = f.name + " " + pct + "%";
        });
      } catch (e) {
        if (hint) hint.textContent = "";
        if (pickBtn) pickBtn.disabled = false;
        NP.toast(e.message);
        return;
      }
    }
    if (hint) hint.textContent = "完成";
    if (pickBtn) pickBtn.disabled = false;
    NP.toast("已提交");
    // 已提交：自动折叠，刷新状态（灰 + 日期）
    state.openId = "";
    await refresh({ toast: false, force: true });
  }

  async function refresh(opts) {
    opts = opts || {};
    var keepOpen = state.openId;
    try {
      var data = await NP.api("/pass/public/sessions?" + identityQuery());
      var next = data.sessions || [];
      var same =
        !opts.force &&
        next.length === state.sessions.length &&
        next.every(function (s, i) {
          return (
            state.sessions[i] &&
            state.sessions[i].id === s.id &&
            state.sessions[i].name === s.name &&
            state.sessions[i].mode === s.mode &&
            !!state.sessions[i].expired === !!s.expired &&
            !!state.sessions[i].submitted === !!s.submitted &&
            state.sessions[i].submittedAt === s.submittedAt
          );
        });
      state.sessions = next;
      if (keepOpen && findSession(keepOpen)) {
        state.openId = keepOpen;
      } else if (!state.openId && state.sessions.length === 1 && !state.sessions[0].submitted) {
        state.openId = state.sessions[0].id;
      } else if (keepOpen && !findSession(keepOpen)) {
        state.openId = "";
      }
      if (!same) {
        renderList();
      } else if (state.openId) {
        var box = document.getElementById("body-" + state.openId);
        var s = findSession(state.openId);
        if (box && s && !box.hidden) {
          await loadFiles(state.openId, box, s);
        }
      } else {
        renderList();
      }
      if (opts.toast) NP.toast("已刷新");
    } catch (e) {
      if (opts.toast) NP.toast(e.message || "刷新失败");
    }
  }

  async function boot() {
    var updBtn = $("checkUpdateBtn");
    if (updBtn) {
      updBtn.onclick = function () {
        NP.checkUpdate({ manual: true });
      };
    }
    await NP.checkUpdate({ manual: false });
    if (document.documentElement.classList.contains("update-locked")) return;

    var btn = $("studentRefresh");
    if (btn) {
      btn.onclick = function () {
        refresh({ toast: true, force: true });
      };
    }
    try {
      await refresh({ force: true });
      if (!state.sessions.length) {
        renderEmpty("暂无任务");
      }
    } catch (e) {
      renderEmpty(e.message);
    }
    setInterval(function () {
      refresh({ toast: false });
    }, 4000);
  }

  boot();
})();
