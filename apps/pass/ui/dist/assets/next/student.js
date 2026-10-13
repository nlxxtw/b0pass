(function () {
  var state = {
    session: null,
    expired: false,
  };

  function $(id) {
    return document.getElementById(id);
  }

  function identityQuery() {
    var p = NP.studentProfile();
    return NP.qs({
      studentId: p.studentId,
      studentName: p.studentName,
      anonId: p.anonId,
    });
  }

  function renderEmpty(msg) {
    $("main").innerHTML =
      '<div class="card empty"><div class="big">📂</div><div>' +
      (msg || "老师尚未发布任务，请稍候") +
      "</div></div>";
  }

  function renderSession() {
    var s = state.session;
    if (!s) {
      renderEmpty();
      return;
    }
    var html = "";
    html +=
      '<div class="card"><div class="row" style="justify-content:space-between">' +
      "<div><div class=\"title\" style=\"font-size:20px;font-weight:700\">" +
      escapeHtml(s.name) +
      '</div><div class="muted" style="margin-top:6px">' +
      NP.modeLabel(s.mode) +
      (s.deadline ? " · 截止 " + escapeHtml(s.deadline.replace("T", " ")) : "") +
      "</div></div>" +
      '<span class="pill ' +
      (s.mode === "distribute" ? "" : "green") +
      '">' +
      NP.modeLabel(s.mode) +
      "</span></div>";

    if (state.expired) {
      html +=
        '<p class="muted" style="margin:12px 0 0;color:var(--orange)">已过截止时间，无法再上传</p>';
    }
    html += "</div>";

    if (s.mode === "collect" && s.requireIdentity) {
      var p = NP.studentProfile();
      html +=
        '<div class="card"><h2>我的身份</h2>' +
        '<label class="field"><span>学号</span><input id="sid" type="text" value="' +
        escapeAttr(p.studentId) +
        '" placeholder="必填"></label>' +
        '<label class="field"><span>姓名</span><input id="sname" type="text" value="' +
        escapeAttr(p.studentName) +
        '" placeholder="必填"></label>' +
        '<button class="btn secondary" id="saveId">保存身份</button></div>';
    }

    if (s.mode === "collect" && !state.expired) {
      html +=
        '<div class="card"><h2>上传作业</h2>' +
        '<input type="file" id="file" multiple style="display:none">' +
        '<button class="btn block" id="pick">选择文件并上传</button>' +
        '<p class="muted" id="uphint" style="margin:10px 0 0"></p></div>';
    }

    html +=
      '<div class="card"><h2 id="listTitle">文件</h2><ul class="list" id="flist"></ul></div>';
    $("main").innerHTML = html;

    if ($("saveId")) {
      $("saveId").onclick = function () {
        NP.saveStudentProfile($("sid").value.trim(), $("sname").value.trim());
        NP.toast("已保存");
        loadFiles();
      };
    }
    if ($("pick")) {
      $("pick").onclick = function () {
        $("file").click();
      };
      $("file").onchange = function () {
        uploadFiles($("file").files);
      };
    }
    loadFiles();
  }

  async function loadFiles() {
    var q = identityQuery();
    try {
      var data = await NP.api("/pass/public/files?" + q);
      var list = $("flist");
      if (!list) return;
      var items = [];
      (data.materials || []).forEach(function (it) {
        items.push(it);
      });
      (data.submissions || []).forEach(function (it) {
        items.push(it);
      });
      if (!items.length) {
        list.innerHTML =
          '<li><div><div class="title muted">暂无文件</div></div></li>';
        return;
      }
      list.innerHTML = items
        .map(function (it) {
          var meta = [];
          if (it.kind === "submission") {
            meta.push(it.mine ? "我的" : escapeHtml(it.studentName || "同学"));
            if (it.submittedAt) meta.push(it.submittedAt);
          } else {
            meta.push("老师资料");
          }
          meta.push(it.sizeText || "");
          var action = it.canDownload
            ? '<a href="/pass/public/download?' +
              NP.qs({
                f: it.path,
                studentId: NP.studentProfile().studentId,
                studentName: NP.studentProfile().studentName,
                anonId: NP.studentProfile().anonId,
              }) +
              '">下载</a>'
            : '<span class="muted">不可下载</span>';
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
    } catch (e) {
      NP.toast(e.message);
    }
  }

  async function uploadFiles(fileList) {
    if (!fileList || !fileList.length) return;
    var s = state.session;
    if (s.requireIdentity) {
      var sid = ($("sid") && $("sid").value.trim()) || NP.studentProfile().studentId;
      var sname =
        ($("sname") && $("sname").value.trim()) || NP.studentProfile().studentName;
      if (!sid || !sname) {
        NP.toast("请先填写学号和姓名");
        return;
      }
      NP.saveStudentProfile(sid, sname);
    }
    var hint = $("uphint");
    for (var i = 0; i < fileList.length; i++) {
      var f = fileList[i];
      if (hint) hint.textContent = "正在上传 " + f.name + " …";
      try {
        var fd = new FormData();
        fd.append("file", f);
        var p = NP.studentProfile();
        await NP.api(
          "/pass/public/upload?" +
            NP.qs({
              studentId: p.studentId,
              studentName: p.studentName,
              anonId: p.anonId,
            }),
          { method: "POST", body: fd, headers: {} }
        );
      } catch (e) {
        NP.toast(e.message);
        if (hint) hint.textContent = "";
        return;
      }
    }
    if (hint) hint.textContent = "上传完成";
    NP.toast("上传成功");
    loadFiles();
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

  async function boot() {
    try {
      var data = await NP.api("/pass/public/current");
      state.session = data.session;
      state.expired = !!data.expired;
      renderSession();
    } catch (e) {
      renderEmpty(e.message);
    }
  }

  boot();
})();
