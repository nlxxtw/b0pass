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
      if (!opts.noRedirect) {
        location.href = "/login";
      }
      throw new Error(data.msg || "请先登录");
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

  function modeLabel(mode) {
    return mode === "distribute" ? "发资料" : "收作业";
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
  };
})(window);
