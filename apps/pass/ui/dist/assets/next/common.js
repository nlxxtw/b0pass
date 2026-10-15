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
    // skipToken：学生端发文本时不带教师 token，避免误标「教师」
    if (token && !opts.skipToken) headers.token = token;
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
      if (!opts.skipToken) clearAuth();
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
    if (mode === "text") return "发文本";
    return who === "student" ? "交作业" : "收作业";
  }

  function modeClass(mode) {
    if (mode === "distribute") return "send";
    if (mode === "text") return "text";
    return "collect";
  }

  function formatMsgTime(iso) {
    if (!iso) return "";
    var s = String(iso).replace("T", " ");
    if (s.length >= 16) return s.slice(5, 16);
    return s;
  }

  function escapeChat(s) {
    return String(s || "")
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/\n/g, "<br>");
  }

  function escapeAttr(s) {
    return String(s || "")
      .replace(/&/g, "&amp;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#39;")
      .replace(/</g, "&lt;");
  }

  function copyToClipboard(str) {
    if (navigator.clipboard && navigator.clipboard.writeText) {
      return navigator.clipboard.writeText(str);
    }
    return new Promise(function (resolve, reject) {
      var el = document.createElement("textarea");
      el.value = str;
      document.body.appendChild(el);
      el.select();
      try {
        document.execCommand("copy");
        resolve();
      } catch (e) {
        reject(e);
      }
      document.body.removeChild(el);
    });
  }

  function htmlToPlain(html) {
    var d = document.createElement("div");
    d.innerHTML = html || "";
    return (d.innerText || d.textContent || "").replace(/\u00a0/g, " ").trim();
  }

  function richHasMedia(html) {
    return /<img\b/i.test(html || "") || /<video\b/i.test(html || "");
  }

  function richLooksEmpty(html) {
    // 纯截图/图片也可发送，不必再加文字
    if (richHasMedia(html)) return false;
    return !htmlToPlain(html);
  }

  /** 去掉粘贴整页 / 尾部空行，避免气泡被撑得又高又宽 */
  function cleanRichHtml(html) {
    var d = document.createElement("div");
    d.innerHTML = html || "";
    var bodyEl = d.querySelector("body");
    if (bodyEl) {
      d.innerHTML = bodyEl.innerHTML;
    }
    ["html", "head", "meta", "title", "link", "style", "script"].forEach(function (tag) {
      d.querySelectorAll(tag).forEach(function (n) {
        n.remove();
      });
    });
    var guard = 0;
    while (d.lastChild && guard++ < 40) {
      var n = d.lastChild;
      if (n.nodeType === 3) {
        if (!String(n.textContent || "").replace(/\u00a0/g, " ").trim()) {
          d.removeChild(n);
          continue;
        }
        break;
      }
      if (n.nodeType === 1) {
        var tag = (n.tagName || "").toUpperCase();
        var text = String(n.textContent || "").replace(/\u00a0/g, " ").trim();
        var hasMedia = !!(n.querySelector && n.querySelector("img,video"));
        if (tag === "BR" || (!text && !hasMedia)) {
          d.removeChild(n);
          continue;
        }
      }
      break;
    }
    return d.innerHTML;
  }

  /** 渲染发文本消息；教师标签只认绑定的 teacherIp */
  function textMessagesHtml(messages, myNick, teacherIp) {
    messages = messages || [];
    teacherIp = String(teacherIp || "").trim();
    if (!messages.length) {
      return '<div class="text-empty">还没有消息，发一条试试</div>';
    }
    return messages
      .map(function (m) {
        var mine = myNick && m.nick === myNick;
        // 教师标签只认绑定的管理端 IP；删除：教师可删全部，学生只能删自己的（无权限则不显示 ✕）
        var isTeacherMsg = !!(teacherIp && m.nick === teacherIp);
        var iAmTeacher = !!(teacherIp && myNick && myNick === teacherIp);
        var canDelete = iAmTeacher || !!(mine && myNick);
        var roleCls = isTeacherMsg ? "teacher" : "student";
        var mid = escapeAttr(m.id);
        var body = m.body || "";
        if (body.indexOf("<") < 0) {
          body = escapeChat(body);
        } else {
          body = cleanRichHtml(body);
        }
        var hasImg = richHasMedia(body);
        var hasText = !!htmlToPlain(body);
        // 有图：不显示复制文字/HTML；纯文字才显示
        var copyBtns =
          !hasImg && hasText
            ? '<button type="button" class="text-act" data-copyhtml="' +
              mid +
              '">【复制HTML】</button>' +
              '<button type="button" class="text-act" data-copytext="' +
              mid +
              '">【复制文字】</button>'
            : "";
        return (
          '<div class="text-msg ' +
          roleCls +
          (mine ? " mine" : "") +
          (hasImg ? " has-img" : "") +
          '" data-msgid="' +
          mid +
          '">' +
          '<div class="text-bar">' +
          '<span class="text-bar-left"><span class="text-nick">' +
          escapeChat(m.nick) +
          "</span>" +
          (isTeacherMsg ? '<span class="text-role">教师</span>' : "") +
          '<span class="text-time">' +
          escapeChat(formatMsgTime(m.createdAt)) +
          "</span></span>" +
          '<span class="text-bar-actions">' +
          copyBtns +
          (canDelete
            ? '<button type="button" class="text-act text-x" data-delmsg="' +
              mid +
              '" title="删除">✕</button>'
            : "") +
          "</span></div>" +
          '<div class="text-bubble btext" id="tmsg_' +
          mid +
          '">' +
          body +
          "</div></div>"
        );
      })
      .join("");
  }

  function richToolbarHtml() {
    return (
      '<div class="rt-toolbar" role="toolbar">' +
      '<button type="button" class="rt-btn" data-cmd="fontSize" data-val="4" title="字号">T<small>T</small></button>' +
      '<div class="rt-drop" data-drop="fontSize">' +
      '<button type="button" data-cmd="fontSize" data-val="2">小</button>' +
      '<button type="button" data-cmd="fontSize" data-val="3">标准</button>' +
      '<button type="button" data-cmd="fontSize" data-val="4">大</button>' +
      '<button type="button" data-cmd="fontSize" data-val="5">更大</button>' +
      '<button type="button" data-cmd="fontSize" data-val="6">特大</button></div>' +
      '<button type="button" class="rt-btn" data-drop-toggle="color" title="颜色"><span class="rt-a">A</span></button>' +
      '<div class="rt-drop rt-colors" data-drop="color">' +
      ["#12141a", "#e03131", "#b45309", "#0b7a75", "#3b5bdb", "#7c3aed", "#ffffff"].map(function (c) {
        return (
          '<button type="button" class="rt-swatch" data-cmd="foreColor" data-val="' +
          c +
          '" style="background:' +
          c +
          '" title="' +
          c +
          '"></button>'
        );
      }).join("") +
      "</div>" +
      '<button type="button" class="rt-btn" data-cmd="bold" title="加粗"><b>B</b></button>' +
      '<span class="rt-sep"></span>' +
      '<button type="button" class="rt-btn" data-cmd="insertOrderedList" title="有序列表">1.</button>' +
      '<button type="button" class="rt-btn" data-cmd="insertUnorderedList" title="无序列表">•</button>' +
      '<span class="rt-sep"></span>' +
      '<button type="button" class="rt-btn" data-cmd="removeFormat" title="清除格式">T̲</button>' +
      '<button type="button" class="rt-btn" data-cmd="code" title="代码">&lt;/&gt;</button>' +
      '<span class="rt-sep"></span>' +
      '<button type="button" class="rt-btn" data-cmd="pickImage" title="插入图片">图</button>' +
      "</div>"
    );
  }

  /**
   * 挂载发文本聊天区（富文本，对齐原版能力）
   * opts: { sessionId, asTeacher, topic, pollMs }
   */
  function mountTextChat(root, opts) {
    opts = opts || {};
    var sessionId = opts.sessionId;
    var asTeacher = !!opts.asTeacher;
    var pollMs = opts.pollMs || 2500;
    var destroyed = false;
    var timer = null;
    var myNick = "";
    var teacherIp = "";
    var lastSig = "";
    var apiOpts = { noRedirect: true, skipToken: !asTeacher };

    root.innerHTML =
      '<div class="text-chat">' +
      '<div class="text-stream" aria-live="polite"></div>' +
      '<div class="text-composer">' +
      richToolbarHtml() +
      '<div class="rt-editor" contenteditable="true" data-placeholder="输入内容…"></div>' +
      '<div class="text-compose-actions">' +
      '<button type="button" class="btn text text-send">发送</button></div></div></div>';

    var stream = root.querySelector(".text-stream");
    var editor = root.querySelector(".rt-editor");
    var sendBtn = root.querySelector(".text-send");
    var toolbar = root.querySelector(".rt-toolbar");
    var imgInput = document.createElement("input");
    imgInput.type = "file";
    imgInput.accept = "image/*";
    imgInput.style.display = "none";
    root.appendChild(imgInput);

    function sig(list) {
      return (list || [])
        .map(function (m) {
          return m.id;
        })
        .join(",");
    }

    function closeDrops() {
      toolbar.querySelectorAll(".rt-drop.open").forEach(function (d) {
        d.classList.remove("open");
      });
    }

    function uploadChatImage(file) {
      if (!file) return Promise.reject(new Error("无图片"));
      return uploadFile(
        "/pass/public/text/upload?id=" + encodeURIComponent(sessionId),
        file
      ).then(function (data) {
        var url = data && data.url;
        if (!url) throw new Error("上传失败");
        return url;
      });
    }

    function insertImageUrl(url) {
      editor.focus();
      document.execCommand(
        "insertHTML",
        false,
        '<img src="' +
          escapeAttr(url) +
          '" alt="图片" style="max-width:100%;height:auto;border-radius:8px;display:block;margin:4px 0">'
      );
    }

    async function insertImageFile(file) {
      if (!file || !String(file.type || "").startsWith("image/")) {
        toast("请选择图片");
        return;
      }
      toast("图片上传中…");
      try {
        var url = await uploadChatImage(file);
        insertImageUrl(url);
      } catch (err) {
        toast((err && err.message) || "图片上传失败");
      }
    }

    /** 发送前：把编辑器里残留的 data: 图也上传成短链接 */
    async function materializeEditorImages() {
      var imgs = editor.querySelectorAll("img");
      for (var i = 0; i < imgs.length; i++) {
        var src = imgs[i].getAttribute("src") || "";
        if (src.indexOf("data:image") !== 0) continue;
        try {
          var blob = await (await fetch(src)).blob();
          var ext = (blob.type || "image/png").split("/")[1] || "png";
          var file = new File([blob], "paste." + ext, { type: blob.type || "image/png" });
          var url = await uploadChatImage(file);
          imgs[i].setAttribute("src", url);
        } catch (e) {
          throw new Error("图片处理失败，请重试");
        }
      }
    }

    function runCmd(cmd, val) {
      editor.focus();
      try {
        document.execCommand("styleWithCSS", false, true);
      } catch (e) {}
      if (cmd === "pickImage") {
        imgInput.value = "";
        imgInput.click();
        return;
      }
      if (cmd === "code") {
        var sel = window.getSelection();
        var t = sel && !sel.isCollapsed ? sel.toString() : "code";
        document.execCommand(
          "insertHTML",
          false,
          "<code style=\"font-family:ui-monospace,Consolas,monospace;background:#f3f5f9;padding:1px 6px;border-radius:4px\">" +
            escapeChat(t) +
            "</code>&nbsp;"
        );
        return;
      }
      if (cmd === "fontSize") {
        document.execCommand("fontSize", false, val || "3");
        return;
      }
      if (val != null && val !== "") document.execCommand(cmd, false, val);
      else document.execCommand(cmd, false, null);
    }

    imgInput.addEventListener("change", function () {
      var f = imgInput.files && imgInput.files[0];
      if (f) insertImageFile(f);
    });

    toolbar.addEventListener("mousedown", function (e) {
      // 避免点工具栏时编辑器失焦丢选区
      if (e.target.closest("button")) e.preventDefault();
    });
    toolbar.addEventListener("click", function (e) {
      var t = e.target.closest("button");
      if (!t || !toolbar.contains(t)) return;
      var dropToggle = t.getAttribute("data-drop-toggle");
      if (dropToggle) {
        var panel = toolbar.querySelector('[data-drop="' + dropToggle + '"]');
        var open = panel && panel.classList.contains("open");
        closeDrops();
        if (panel && !open) panel.classList.add("open");
        return;
      }
      if (t.getAttribute("data-cmd") === "fontSize" && t.classList.contains("rt-btn")) {
        var fp = toolbar.querySelector('[data-drop="fontSize"]');
        var fo = fp && fp.classList.contains("open");
        closeDrops();
        if (fp && !fo) fp.classList.add("open");
        return;
      }
      var cmd = t.getAttribute("data-cmd");
      if (!cmd) return;
      closeDrops();
      runCmd(cmd, t.getAttribute("data-val"));
    });
    document.addEventListener(
      "click",
      function onDoc(e) {
        if (destroyed) {
          document.removeEventListener("click", onDoc, true);
          return;
        }
        if (!toolbar.contains(e.target)) closeDrops();
      },
      true
    );

    function wireMsgActions() {
      root.querySelectorAll("[data-copyhtml]").forEach(function (btn) {
        btn.onclick = function (e) {
          e.preventDefault();
          var id = btn.getAttribute("data-copyhtml");
          var el = document.getElementById("tmsg_" + id);
          if (!el) return;
          copyToClipboard(el.innerHTML)
            .then(function () {
              toast("已复制 HTML");
            })
            .catch(function () {
              toast("复制失败");
            });
        };
      });
      root.querySelectorAll("[data-copytext]").forEach(function (btn) {
        btn.onclick = function (e) {
          e.preventDefault();
          var id = btn.getAttribute("data-copytext");
          var el = document.getElementById("tmsg_" + id);
          if (!el) return;
          copyToClipboard(htmlToPlain(el.innerHTML) || el.textContent || "")
            .then(function () {
              toast("已复制文字");
            })
            .catch(function () {
              toast("复制失败");
            });
        };
      });
      root.querySelectorAll("[data-delmsg]").forEach(function (btn) {
        btn.onclick = async function (e) {
          e.preventDefault();
          e.stopPropagation();
          if (!confirm("删除这条消息？")) return;
          try {
            await api("/pass/public/text/delete", {
              method: "POST",
              json: { id: sessionId, msgId: btn.getAttribute("data-delmsg") },
              skipToken: apiOpts.skipToken,
              noRedirect: true,
            });
            toast("已删除");
            await refresh();
          } catch (err) {
            toast(err.message || "删除失败");
          }
        };
      });
    }

    async function refresh() {
      if (destroyed || !sessionId) return;
      try {
        var data = await api(
          "/pass/public/text/messages?id=" + encodeURIComponent(sessionId),
          apiOpts
        );
        myNick = (data && data.myNick) || "";
        teacherIp =
          (data && data.teacherIp) ||
          (data && data.session && data.session.teacherIp) ||
          teacherIp ||
          "";
        var list = (data && data.messages) || [];
        var next = sig(list) + "|" + teacherIp;
        if (next === lastSig && stream.innerHTML) return;
        var nearBottom =
          stream.scrollHeight - stream.scrollTop - stream.clientHeight < 80;
        stream.innerHTML = textMessagesHtml(list, myNick, teacherIp);
        lastSig = next;
        wireMsgActions();
        if (nearBottom || !stream.dataset.ready) {
          stream.scrollTop = stream.scrollHeight;
          stream.dataset.ready = "1";
        }
      } catch (err) {
        if (!stream.dataset.ready) {
          stream.innerHTML =
            '<div class="text-empty">' + escapeChat(err.message || "加载失败") + "</div>";
        }
      }
    }

    async function send() {
      sendBtn.disabled = true;
      try {
        await materializeEditorImages();
        var body = cleanRichHtml((editor.innerHTML || "").trim());
        if (richLooksEmpty(body)) {
          toast("写点内容或贴一张图再发");
          return;
        }
        await api("/pass/public/text/send", {
          method: "POST",
          json: { id: sessionId, body: body },
          skipToken: apiOpts.skipToken,
          noRedirect: true,
        });
        editor.innerHTML = "";
        lastSig = "";
        await refresh();
      } catch (err) {
        toast(err.message || "发送失败");
      } finally {
        sendBtn.disabled = false;
        editor.focus();
      }
    }

    sendBtn.onclick = send;
    editor.addEventListener("keydown", function (e) {
      // 回车发送；Shift+回车换行
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        send();
      }
    });
    // 粘贴截图：直接上传插入，不必再加文字
    editor.addEventListener("paste", function (e) {
      var cd = e.clipboardData || window.clipboardData;
      if (!cd) return;
      var items = cd.items || [];
      var file = null;
      for (var i = 0; i < items.length; i++) {
        if (items[i].type && items[i].type.indexOf("image/") === 0) {
          file = items[i].getAsFile();
          break;
        }
      }
      if (!file && cd.files && cd.files.length) {
        for (var j = 0; j < cd.files.length; j++) {
          if (String(cd.files[j].type || "").indexOf("image/") === 0) {
            file = cd.files[j];
            break;
          }
        }
      }
      if (!file) return;
      e.preventDefault();
      var text = (cd.getData("text/plain") || "").replace(/\u00a0/g, " ").trim();
      insertImageFile(file).then(function () {
        if (!text) return;
        editor.focus();
        document.execCommand("insertText", false, text);
      });
    });

    function schedulePoll() {
      if (timer) clearInterval(timer);
      timer = null;
      if (destroyed) return;
      // 标签页隐藏时降低轮询，减轻后台刷请求
      var ms = document.hidden ? Math.max(pollMs * 4, 10000) : pollMs;
      timer = setInterval(function () {
        if (!document.hidden) refresh();
      }, ms);
      if (!document.hidden) refresh();
    }
    document.addEventListener("visibilitychange", schedulePoll);
    schedulePoll();

    return {
      refresh: refresh,
      destroy: function () {
        destroyed = true;
        if (timer) clearInterval(timer);
        timer = null;
      },
    };
  }

  // Safari：必须在点击手势内同步跳转；fetch+blob 会丢手势导致点了没反应。
  // 用 <a download> 立刻触发浏览器下载栏，不要等整文件收完。
  function downloadFile(url, name) {
    var a = document.createElement("a");
    a.href = url;
    a.rel = "noopener";
    if (name) {
      try {
        a.setAttribute("download", String(name));
      } catch (e) {}
    }
    // 同步点击，尽量让浏览器马上出现下载进度
    document.body.appendChild(a);
    a.click();
    setTimeout(function () {
      if (a.parentNode) a.parentNode.removeChild(a);
    }, 0);
    return Promise.resolve();
  }

  // 与原版 layui 上传一致：multipart 字段名 file + 流式大文件接口；附带 X-File-Size 供服务端校验
  function uploadFile(url, file, onProgress) {
    return new Promise(function (resolve, reject) {
      var xhr = new XMLHttpRequest();
      xhr.open("POST", url, true);
      xhr.timeout = 0;
      var token = getToken();
      if (token) xhr.setRequestHeader("token", token);
      if (file && typeof file.size === "number") {
        xhr.setRequestHeader("X-File-Size", String(file.size));
      }
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
      xhr.onabort = function () {
        reject(new Error("上传已取消"));
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

  // 浏览器端不再弹强制更新/显示版本条；更新请在教师机桌面面板操作
  async function checkUpdate(opts) {
    opts = opts || {};
    if (opts.manual) {
      toast("请到教师机「NextPass 面板」里检查更新");
    }
    return null;
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
    formatMsgTime: formatMsgTime,
    textMessagesHtml: textMessagesHtml,
    mountTextChat: mountTextChat,
    downloadFile: downloadFile,
    uploadFile: uploadFile,
    checkUpdate: checkUpdate,
    openUpdateDownload: openUpdateDownload,
  };
})(window);
