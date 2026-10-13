(function (global) {
  function toast(msg) {
    var el = document.getElementById("toast");
    if (!el) {
      el = document.createElement("div");
      el.id = "toast";
      el.className = "toast";
      document.body.appendChild(el);
    }
    el.textContent = msg;
    el.classList.add("show");
    clearTimeout(el._t);
    el._t = setTimeout(function () {
      el.classList.remove("show");
    }, 2200);
  }

  function getToken() {
    return localStorage.getItem("np_token") || "";
  }

  function getAuth() {
    return localStorage.getItem("np_auth") || "";
  }

  function setAuth(auth, token) {
    localStorage.setItem("np_auth", auth || "");
    localStorage.setItem("np_token", token || "");
  }

  function clearAuth() {
    localStorage.removeItem("np_auth");
    localStorage.removeItem("np_token");
  }

  function anonId() {
    var k = "np_anon";
    var v = localStorage.getItem(k);
    if (!v) {
      v = "a" + Math.random().toString(16).slice(2) + Date.now().toString(16);
      localStorage.setItem(k, v);
    }
    return v;
  }

  function studentProfile() {
    return {
      studentId: localStorage.getItem("np_sid") || "",
      studentName: localStorage.getItem("np_sname") || "",
      anonId: anonId(),
    };
  }

  function saveStudentProfile(id, name) {
    localStorage.setItem("np_sid", id || "");
    localStorage.setItem("np_sname", name || "");
  }

  async function api(url, opts) {
    opts = opts || {};
    var headers = Object.assign({}, opts.headers || {});
    var token = getToken();
    if (token) headers.token = token;
    if (opts.json) {
      headers["Content-Type"] = "application/json;charset=utf-8";
      opts.body = JSON.stringify(opts.json);
    }
    var res = await fetch(url, {
      method: opts.method || "GET",
      headers: headers,
      body: opts.body,
    });
    var data = await res.json().catch(function () {
      return { code: 500, msg: "响应解析失败" };
    });
    if (data.code === 401) {
      clearAuth();
      throw new Error(data.msg || "无权限");
    }
    if (data.code && data.code !== 0) {
      throw new Error(data.msg || "请求失败");
    }
    return data.data;
  }

  function qs(obj) {
    return Object.keys(obj)
      .filter(function (k) {
        return obj[k] !== undefined && obj[k] !== null && obj[k] !== "";
      })
      .map(function (k) {
        return encodeURIComponent(k) + "=" + encodeURIComponent(obj[k]);
      })
      .join("&");
  }

  function modeLabel(mode, who) {
    if (mode === "distribute") return "发资料";
    return who === "student" ? "交作业" : "收作业";
  }

  function modeClass(mode) {
    return mode === "distribute" ? "send" : "collect";
  }

  // Safari：必须在点击手势内同步跳转；fetch+blob 会丢手势导致点了没反应
  function downloadFile(url) {
    var a = document.createElement("a");
    a.href = url;
    a.rel = "noopener";
    document.body.appendChild(a);
    a.click();
    a.remove();
    return Promise.resolve();
  }

  // Safari 大文件：用 XHR + 显式文件名，避免 fetch FormData 偶发失败
  function uploadFile(url, file, onProgress) {
    return new Promise(function (resolve, reject) {
      var xhr = new XMLHttpRequest();
      xhr.open("POST", url, true);
      xhr.timeout = 0;
      var token = getToken();
      if (token) xhr.setRequestHeader("token", token);
      xhr.onload = function () {
        var data;
        try {
          data = JSON.parse(xhr.responseText || "{}");
        } catch (e) {
          reject(new Error("上传响应异常"));
          return;
        }
        if (data.code === 401) {
          clearAuth();
          reject(new Error(data.msg || "无权限"));
          return;
        }
        if (data.code && data.code !== 0) {
          reject(new Error(data.msg || "上传失败"));
          return;
        }
        resolve(data.data);
      };
      xhr.onerror = function () {
        reject(new Error("网络错误，请检查局域网与防火墙"));
      };
      xhr.ontimeout = function () {
        reject(new Error("上传超时"));
      };
      if (xhr.upload && typeof onProgress === "function") {
        xhr.upload.onprogress = function (ev) {
          if (ev.lengthComputable) onProgress(ev.loaded, ev.total);
        };
      }
      var fd = new FormData();
      fd.append("file", file, file.name || "upload.bin");
      xhr.send(fd);
    });
  }

  function ensureUpdateModal() {
    var el = document.getElementById("forceUpdate");
    if (el) return el;
    el = document.createElement("div");
    el.id = "forceUpdate";
    el.className = "force-update";
    el.hidden = true;
    el.innerHTML =
      '<div class="card force-card">' +
      "<h2>发现新版本，请更新后继续使用</h2>" +
      '<p class="meta" id="forceUpdateMeta"></p>' +
      '<p class="hint">将通过加速镜像下载 Releases 安装包，解压覆盖原程序后重新打开。</p>' +
      '<button type="button" class="btn block" id="forceUpdateBtn">立即更新</button>' +
      "</div>";
    document.body.appendChild(el);
    return el;
  }

  function openUpdateDownload(url) {
    if (!url) {
      toast("暂无下载地址");
      return;
    }
    // 加速镜像拉 Releases
    window.open(url, "_blank", "noopener");
    toast("已开始下载，请解压覆盖后重启");
  }

  async function checkUpdate(opts) {
    opts = opts || {};
    var data;
    try {
      data = await api("/pass/public/update-check", { noRedirect: true });
    } catch (e) {
      if (opts.manual) toast(e.message || "检查更新失败");
      return null;
    }
    var verEl = document.getElementById("appVersion");
    if (verEl) verEl.textContent = "v" + (data.current || "");
    if (data.needUpdate) {
      var box = ensureUpdateModal();
      var meta = document.getElementById("forceUpdateMeta");
      if (meta) {
        meta.textContent =
          "当前 v" +
          (data.current || "?") +
          " → 最新 v" +
          (data.latest || "?") +
          "（" +
          (data.asset || "安装包") +
          "）";
      }
      box.hidden = false;
      document.documentElement.classList.add("update-locked");
      var btn = document.getElementById("forceUpdateBtn");
      if (btn) {
        btn.onclick = function () {
          openUpdateDownload(data.downloadUrl || data.releaseUrl);
        };
      }
    } else if (opts.manual) {
      if (data.checkError) toast("检查失败：" + data.checkError);
      else toast("已是最新版本 v" + (data.current || ""));
    }
    return data;
  }

  global.NP = {
    toast: toast,
    api: api,
    qs: qs,
    getToken: getToken,
    getAuth: getAuth,
    setAuth: setAuth,
    clearAuth: clearAuth,
    anonId: anonId,
    studentProfile: studentProfile,
    saveStudentProfile: saveStudentProfile,
    modeLabel: modeLabel,
    modeClass: modeClass,
    downloadFile: downloadFile,
    uploadFile: uploadFile,
    checkUpdate: checkUpdate,
    openUpdateDownload: openUpdateDownload,
  };
})(window);
