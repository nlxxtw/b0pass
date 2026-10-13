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

  function pwdKey(id) {
    return "np_access_" + id;
  }
  function getSavedPwd(id) {
    try {
      return sessionStorage.getItem(pwdKey(id)) || "";
    } catch (e) {
      return "";
    }
  }
  function savePwd(id, pwd) {
    try {
      if (pwd) sessionStorage.setItem(pwdKey(id), pwd);
      else sessionStorage.removeItem(pwdKey(id));
    } catch (e) {}
  }
  function identityQueryWithPwd(id, extra) {
    var q = Object.assign({}, extra || {});
    if (id) {
      var pwd = getSavedPwd(id);
      if (pwd) q.pwd = pwd;
    }
    return identityQuery(q);
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
      if (s.mode === "distribute" && s.needPassword) {
        meta += (meta ? " · " : "") + "需密码";
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
    if (s.mode === "distribute" && s.needPassword && !getSavedPwd(id)) {
      html =
        '<div class="block pwd-gate">' +
        "<h3>访问密码</h3>" +
        '<p class="meta">老师为该资料设置了密码，输入后才能查看下载</p>' +
        '<label class="field"><span>密码</span>' +
        '<input class="access-pwd" type="password" autocomplete="off" placeholder="请输入访问密码"></label>' +
        '<button type="button" class="btn block unlock">打开资料</button>' +
        '<p class="meta pwd-err" hidden></p></div>';
      box.innerHTML = html;
      var unlock = box.querySelector(".unlock");
      var inp = box.querySelector(".access-pwd");
      var err = box.querySelector(".pwd-err");
      function tryUnlock() {
        var pwd = (inp && inp.value.trim()) || "";
        if (!pwd) {
          if (err) {
            err.textContent = "请输入密码";
            err.hidden = false;
          }
          return;
        }
        savePwd(id, pwd);
        NP.api("/pass/public/files?" + identityQueryWithPwd(id, { id: id }))
          .then(function () {
            fillBody(id);
          })
          .catch(function (e) {
            savePwd(id, "");
            if (err) {
              err.textContent = e.message || "密码错误";
              err.hidden = false;
            }
          });
      }
      if (unlock) unlock.onclick = tryUnlock;
      if (inp) {
        inp.onkeydown = function (e) {
          if (e.key === "Enter") tryUnlock();
        };
      }
      return;
    }

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
          '<div class="up-progress" hidden><div class="up-bar"></div></div>' +
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

  function downloadMany(files) {
    if (!files || !files.length) return;
    NP.toast("开始下载 " + files.length + " 个文件…");
    var i = 0;
    function next() {
      if (i >= files.length) return;
      var f = files[i++];
      NP.downloadFile(f.url, f.name, f.size || 0);
      setTimeout(next, 450);
    }
    next();
  }

  async function loadFiles(id, box, s) {
    var list = box.querySelector(".flist");
    if (!list) return;
    try {
      var data = await NP.api("/pass/public/files?" + identityQueryWithPwd(id, { id: id }));
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
        var oldBar = box.querySelector(".batch-bar");
        if (oldBar) oldBar.remove();
        return;
      }
      var canBatch =
        s.mode === "distribute" &&
        items.filter(function (it) {
          return it.kind === "material" && it.canDownload;
        }).length > 1;

      list.innerHTML = items
        .map(function (it) {
          var meta = [];
          if (it.kind === "submission") {
            meta.push(it.mine ? "我的" : escapeHtml(it.studentName || "同学"));
            if (it.submittedAt) meta.push(it.submittedAt);
          }
          meta.push(it.sizeText || "");
          var action = "";
          var dlUrl =
            "/pass/public/download?" + identityQueryWithPwd(id, { id: id, f: it.path });
          var check = "";
          if (it.kind === "material" && it.canDownload) {
            if (canBatch) {
              check =
                '<label class="file-check"><input type="checkbox" class="fcheck" data-url="' +
                escapeAttr(dlUrl) +
                '" data-name="' +
                escapeAttr(it.name) +
                '" data-size="' +
                (it.size || 0) +
                '"></label>';
            }
            action =
              '<button type="button" class="btn ghost getfile" data-url="' +
              escapeAttr(dlUrl) +
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
              '<button type="button" class="btn ghost getfile" data-url="' +
              escapeAttr(dlUrl) +
              '" data-name="' +
              escapeAttr(it.name) +
              '" data-size="' +
              (it.size || 0) +
              '">下载</button>';
          } else {
            action = '<span class="meta">—</span>';
          }
          return (
            "<li>" +
            check +
            '<div class="file-main"><div class="title">' +
            escapeHtml(it.name) +
            '</div><div class="meta">' +
            meta.join(" · ") +
            "</div></div>" +
            action +
            "</li>"
          );
        })
        .join("");

      var bar = box.querySelector(".batch-bar");
      if (canBatch) {
        if (!bar) {
          bar = document.createElement("div");
          bar.className = "batch-bar";
          list.parentNode.insertBefore(bar, list);
        }
        bar.innerHTML =
          '<label class="batch-all"><input type="checkbox" class="check-all"> 全选</label>' +
          '<button type="button" class="btn send batch-dl" disabled>一键下载</button>';
        var checkAll = bar.querySelector(".check-all");
        var batchBtn = bar.querySelector(".batch-dl");
        function selectedFiles() {
          var out = [];
          list.querySelectorAll(".fcheck:checked").forEach(function (cb) {
            out.push({
              url: cb.dataset.url,
              name: cb.dataset.name,
              size: Number(cb.dataset.size) || 0,
            });
          });
          return out;
        }
        function syncBatch() {
          var n = selectedFiles().length;
          var all = list.querySelectorAll(".fcheck");
          batchBtn.disabled = n === 0;
          batchBtn.textContent = n ? "一键下载（" + n + "）" : "一键下载";
          if (checkAll) {
            checkAll.checked = all.length > 0 && n === all.length;
            checkAll.indeterminate = n > 0 && n < all.length;
          }
        }
        if (checkAll) {
          checkAll.onchange = function () {
            list.querySelectorAll(".fcheck").forEach(function (cb) {
              cb.checked = checkAll.checked;
            });
            syncBatch();
          };
        }
        list.querySelectorAll(".fcheck").forEach(function (cb) {
          cb.onchange = syncBatch;
        });
        batchBtn.onclick = function () {
          downloadMany(selectedFiles());
        };
        syncBatch();
      } else if (bar) {
        bar.remove();
      }

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
      if (s.mode === "distribute" && s.needPassword) {
        savePwd(id, "");
        fillBody(id);
        return;
      }
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
    var prog = box.querySelector(".up-progress");
    var bar = box.querySelector(".up-bar");
    if (pickBtn) pickBtn.disabled = true;
    if (prog) prog.hidden = false;
    var okCount = 0;
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
        if (hint) {
          hint.textContent =
            "正在上传 " + (i + 1) + "/" + fileList.length + "：" + f.name + "（请勿关闭）";
        }
        if (bar) bar.style.width = "0%";
        await NP.uploadFile(url, f, function (loaded, total) {
          var pct = total ? Math.floor((loaded / total) * 100) : 0;
          if (bar) bar.style.width = pct + "%";
          if (hint) {
            hint.textContent =
              "正在上传 " +
              (i + 1) +
              "/" +
              fileList.length +
              "：" +
              f.name +
              " " +
              pct +
              "%";
          }
        });
        okCount++;
        if (bar) bar.style.width = "100%";
      } catch (e) {
        if (hint) hint.textContent = "失败：" + (e.message || "上传失败") + "（未成功的不会计入）";
        if (pickBtn) pickBtn.disabled = false;
        NP.toast(e.message || "上传失败");
        await refresh({ toast: false, force: true });
        if (state.openId === id) {
          var nb = document.getElementById("body-" + id);
          var ns = findSession(id);
          if (nb && ns) loadFiles(id, nb, ns);
        }
        return;
      }
    }
    if (hint) hint.textContent = "全部上传成功（" + okCount + " 个）";
    if (pickBtn) pickBtn.disabled = false;
    NP.toast("上传成功");
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
    var btn = $("studentRefresh");
    if (btn) {
      btn.onclick = function () {
        refresh({ toast: true, force: true });
      };
    }
    // 秒开：先拉任务，更新检查放后台，不挡首屏
    NP.checkUpdate({ manual: false }).catch(function () {});
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
