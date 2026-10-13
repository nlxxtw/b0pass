(function () {
  var state = { index: null, stats: null };

  function $(id) {
    return document.getElementById(id);
  }

  function requireAuth() {
    if (!NP.getToken() && !NP.getAuth()) {
      // still try; empty codes allow through
    }
  }

  var cachedRoot = location.origin + "/";

  function rootLink() {
    return cachedRoot;
  }

  async function renderShare() {
    try {
      var ip = await NP.api("/pass/read-ip", { noRedirect: true });
      var port = location.port || (location.protocol === "https:" ? "443" : "80");
      if (ip) {
        cachedRoot = location.protocol + "//" + ip + (port ? ":" + port : "") + "/";
      }
    } catch (e) {
      cachedRoot = location.origin + "/";
    }
    var link = rootLink();
    $("rootLink").textContent = link;
    var box = $("qr");
    box.innerHTML = "";
    if (window.QRCode) {
      new QRCode(box, {
        text: link,
        width: 152,
        height: 152,
        correctLevel: QRCode.CorrectLevel.M,
      });
    }
  }

  async function loadList() {
    requireAuth();
    try {
      state.index = await NP.api("/pass/session/list");
      renderList();
      var pub = state.index.publishedId;
      if (pub) loadStats(pub);
      else {
        $("statsCard").style.display = "none";
      }
    } catch (e) {
      if (String(e.message || "").indexOf("登录") >= 0 || !NP.getToken()) {
        location.href = "/login";
        return;
      }
      NP.toast(e.message);
    }
  }

  function renderList() {
    var idx = state.index || { sessions: [], publishedId: "" };
    var ul = $("sessList");
    if (!idx.sessions || !idx.sessions.length) {
      ul.innerHTML =
        '<li><div><div class="title muted">还没有活动，先创建一个</div></div></li>';
      return;
    }
    ul.innerHTML = idx.sessions
      .map(function (s) {
        var published = s.id === idx.publishedId;
        return (
          "<li><div><div class=\"title\">" +
          escapeHtml(s.name) +
          (published ? ' <span class="pill green">学生首页</span>' : "") +
          '</div><div class="meta">' +
          NP.modeLabel(s.mode) +
          " · " +
          escapeHtml(s.createdAt.replace("T", " ").slice(0, 16)) +
          (s.requireIdentity ? " · 强制身份" : "") +
          "</div></div><div class=\"row\">" +
          (published
            ? ""
            : '<button class="btn secondary" data-pub="' +
              s.id +
              '">发布</button>') +
          '<button class="btn secondary" data-stats="' +
          s.id +
          '">统计</button></div></li>'
        );
      })
      .join("");

    ul.querySelectorAll("[data-pub]").forEach(function (btn) {
      btn.onclick = async function () {
        try {
          await NP.api("/pass/session/publish?id=" + encodeURIComponent(btn.dataset.pub), {
            method: "POST",
            json: {},
          });
          NP.toast("已发布到学生首页");
          loadList();
        } catch (e) {
          NP.toast(e.message);
        }
      };
    });
    ul.querySelectorAll("[data-stats]").forEach(function (btn) {
      btn.onclick = function () {
        loadStats(btn.dataset.stats);
      };
    });
  }

  async function loadStats(id) {
    try {
      state.stats = await NP.api("/pass/session/stats?id=" + encodeURIComponent(id));
      var card = $("statsCard");
      card.style.display = "";
      var s = state.stats.session || {};
      $("statsTitle").textContent = s.name || "统计";
      $("statsSummary").textContent =
        "已交 " +
        (state.stats.studentCount || 0) +
        " 人 · 文件 " +
        (state.stats.fileCount || 0) +
        " 个";
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
                "</td></tr>"
              );
            })
            .join("")
        : '<tr><td colspan="5" class="muted">暂无提交</td></tr>';
    } catch (e) {
      NP.toast(e.message);
    }
  }

  async function createSession() {
    var name = $("name").value.trim();
    if (!name) {
      NP.toast("请填写活动名称");
      return;
    }
    var mode = document.querySelector(".seg button.active").dataset.mode;
    var body = {
      name: name,
      mode: mode,
      requireIdentity: $("requireIdentity").checked,
      canSeeOthers: $("canSeeOthers").checked,
      canDownloadOthers: $("canDownloadOthers").checked,
      canDownloadMaterials: $("canDownloadMaterials").checked,
      deadline: $("deadline").value || "",
    };
    try {
      await NP.api("/pass/session/create", { method: "POST", json: body });
      NP.toast("已创建并发布到学生首页");
      $("name").value = "";
      loadList();
    } catch (e) {
      NP.toast(e.message);
    }
  }

  async function uploadMaterial() {
    var id =
      (state.index && state.index.publishedId) ||
      (state.stats && state.stats.session && state.stats.session.id);
    if (!id) {
      NP.toast("请先创建活动");
      return;
    }
    var input = $("matFile");
    if (!input.files || !input.files.length) {
      input.click();
      return;
    }
    for (var i = 0; i < input.files.length; i++) {
      var fd = new FormData();
      fd.append("file", input.files[i]);
      try {
        await NP.api("/pass/session/material-upload?id=" + encodeURIComponent(id), {
          method: "POST",
          body: fd,
        });
      } catch (e) {
        NP.toast(e.message);
        return;
      }
    }
    input.value = "";
    NP.toast("资料已上传");
    if (id) loadStats(id);
  }

  function escapeHtml(s) {
    return String(s || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function wire() {
    document.querySelectorAll(".seg button").forEach(function (btn) {
      btn.onclick = function () {
        document.querySelectorAll(".seg button").forEach(function (b) {
          b.classList.remove("active");
        });
        btn.classList.add("active");
        var collect = btn.dataset.mode === "collect";
        $("collectOpts").style.display = collect ? "" : "none";
      };
    });
    $("canSeeOthers").onchange = function () {
      if (!$("canSeeOthers").checked) {
        $("canDownloadOthers").checked = false;
        $("canDownloadOthers").disabled = true;
      } else {
        $("canDownloadOthers").disabled = false;
      }
    };
    $("canSeeOthers").onchange();
    $("createBtn").onclick = createSession;
    $("copyBtn").onclick = async function () {
      try {
        await navigator.clipboard.writeText(rootLink());
        NP.toast("链接已复制");
      } catch (e) {
        NP.toast("请手动复制链接");
      }
    };
    $("matBtn").onclick = function () {
      $("matFile").click();
    };
    $("matFile").onchange = uploadMaterial;
    $("logoutBtn").onclick = function () {
      NP.clearAuth();
      location.href = "/login";
    };
    $("openStudent").onclick = function () {
      window.open("/", "_blank");
    };
  }

  wire();
  if (!NP.getToken()) {
    location.href = "/login";
  } else {
    renderShare().then(loadList);
  }
})();
