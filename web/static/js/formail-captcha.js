/**
 * formail-captcha.js — Formail 静态表单验证码组件（独立、无依赖）
 *
 * 用法：在任意包含 <form action="https://formail.example.com/f/{token}"> 的页面引入本脚本即可：
 *   <script src="https://formail.example.com/assets/js/formail-captcha.js" defer></script>
 *
 * 组件行为：
 *   1. 自动识别 action 以 /f/{token} 结尾的表单（也可用 data-formail-captcha 属性指定）
 *   2. 页面加载后自动向 GET /f/{token}/captcha 获取验证码并渲染
 *   3. 提交时自动附带 captcha_id / captcha_answer 隐藏字段
 *   4. 表单未开启验证码（captcha_required=false）时不产生任何干扰
 */
(function () {
  "use strict";

  function ready(fn) {
    if (document.readyState !== "loading") {
      fn();
    } else {
      document.addEventListener("DOMContentLoaded", fn);
    }
  }

  function findForms() {
    var list = Array.prototype.slice.call(
      document.querySelectorAll("form[data-formail-captcha], form[action*='/f/']")
    );
    return list.filter(function (f) {
      return /\/f\/[A-Za-z0-9_-]+/.test(f.getAttribute("action") || "");
    });
  }

  // ---------- 工具 ----------
  function getToken(action) {
    var m = action.match(/\/f\/([A-Za-z0-9_-]+)/);
    return m ? m[1] : "";
  }
  function getBase(action) {
    var m = action.match(/^(https?:)?\/\/[^/]+/);
    return m ? m[0] : "";
  }

  function el(tag, attrs, style, html) {
    var e = document.createElement(tag);
    if (attrs) {
      Object.keys(attrs).forEach(function (k) {
        e.setAttribute(k, attrs[k]);
      });
    }
    if (style) {
      Object.keys(style).forEach(function (k) {
        e.style[k] = style[k];
      });
    }
    if (html !== undefined) e.innerHTML = html;
    return e;
  }
  function clamp(v, lo, hi) {
    return Math.max(lo, Math.min(v, hi));
  }

  // ---------- 组件 ----------
  function FormCaptcha(form, token) {
    var self = this;
    self.form = form;
    self.token = token;
    self.base = getBase(form.getAttribute("action") || "");
    self.state = { ready: false, id: "", answer: "", captcha: null, loading: false, required: false };

    // 隐藏字段
    self.hidId = el("input", { type: "hidden", name: "captcha_id" });
    self.hidAns = el("input", { type: "hidden", name: "captcha_answer" });
    self.hidFc = el("input", { type: "hidden", name: "fc_token" });
    form.appendChild(self.hidId);
    form.appendChild(self.hidAns);
    form.appendChild(self.hidFc);

    // 提交按钮（用于"验证码未完成时禁用"，确定性拦截）
    self.submitBtn = form.querySelector('button[type="submit"], input[type="submit"], button:not([type])');

    // 容器：插到最后一个提交按钮之前，若没有按钮则放到末尾
    self.wrap = el(
      "div",
      { class: "formail-captcha", "data-captcha-state": "loading" },
      { margin: "12px 0", padding: "12px", "border": "1px solid #e2e8f0", "border-radius": "10px", background: "#f8fafc", "text-align": "center", "font-size": "13px", color: "#475569" }
    );
    self.wrap.innerHTML =
      '<div class="formail-captcha-hint" style="font-weight:600;color:#1e293b;margin-bottom:8px;"></div>' +
      '<div class="formail-captcha-stage" style="position:relative;display:inline-block;max-width:100%;"></div>' +
      '<div class="formail-captcha-sliderarea" style="display:none;max-width:320px;margin:8px auto 0;"></div>' +
      '<div class="formail-captcha-msg" style="margin-top:6px;min-height:16px;"></div>';

    var btn = form.querySelector('button[type="submit"], input[type="submit"], button:not([type])');
    if (btn && btn.parentNode === form) {
      form.insertBefore(self.wrap, btn);
    } else {
      form.appendChild(self.wrap);
    }

    self.hintEl = self.wrap.querySelector(".formail-captcha-hint");
    self.stageEl = self.wrap.querySelector(".formail-captcha-stage");
    self.sliderArea = self.wrap.querySelector(".formail-captcha-sliderarea");
    self.msgEl = self.wrap.querySelector(".formail-captcha-msg");

    self.hidId.value = "";
    self.hidAns.value = "";

    // 提交拦截：未就绪时阻止并加载；已就绪但未完成验证时提示
    self.form.addEventListener("submit", function (e) {
      if (!self.state.ready) {
        e.preventDefault();
        e.stopPropagation();
        self.setMsg("请先完成验证码，正在加载…", true);
        self.load();
        return;
      }
      if (self.state.required && !self.state.answer) {
        e.preventDefault();
        e.stopPropagation();
        self.setMsg("请先完成验证码验证，再提交", true);
        if (self.wrap.scrollIntoView) {
          self.wrap.scrollIntoView({ behavior: "smooth", block: "center" });
        }
      }
    });

    // 加载验证码
    self.load();
  }

  FormCaptcha.prototype.setMsg = function (text, isErr) {
    this.msgEl.textContent = text || "";
    this.msgEl.style.color = isErr ? "#dc2626" : "#16a34a";
  };

  FormCaptcha.prototype.fail = function (reason) {
    var self = this;
    self.wrap.setAttribute("data-captcha-state", "error");
    self.hintEl.textContent = "验证码加载失败";
    self.setMsg(reason || "加载失败，请刷新后重试", true);
    // 加载失败不阻塞表单提交
    self.state.ready = true;
    self.state.allowSubmit = true;
    self.state.required = false;
  };

  FormCaptcha.prototype.load = function () {
    var self = this;
    if (self.state.loading) return;
    self.state.loading = true;
    self.stageEl.style.display = "";
    self.stageEl.innerHTML = "加载验证码中…";
    self.hintEl.textContent = "";
    self.msgEl.textContent = "";
    self.wrap.setAttribute("data-captcha-state", "loading");

    fetch(self.base + "/f/" + encodeURIComponent(self.token) + "/captcha", {
      credentials: "same-origin",
    })
      .then(function (res) {
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (r) {
        if (r.code !== 0) throw new Error(r.message || "captcha error");
        var d = r.data;
        self.hidFc.value = (d && d.fc_token) || "";
        self.state.fcToken = (d && d.fc_token) || "";
        var needCaptcha = !!(d && d.captcha_required);
        var needChallenge = !!(d && d.require_challenge);
        if (!needCaptcha && !needChallenge) {
          // 未开启任何防护：隐藏组件，不阻塞提交
          self.state.required = false;
          self.wrap.style.display = "none";
          self.state.ready = true;
          self.state.allowSubmit = true;
          self.state.loading = false;
          return;
        }
        if (!needCaptcha) {
          // 仅开启签名挑战：fc_token 已保存到隐藏字段，无需图形验证码 UI
          self.state.required = false;
          self.wrap.style.display = "none";
          self.state.ready = true;
          self.state.allowSubmit = true;
          self.state.loading = false;
          return;
        }
        self.state.required = true;
        self.state.ready = false;
        self.state.loading = false;
        self.render(d);
      })
      .catch(function (err) {
        self.state.loading = false;
        self.fail(err && err.message ? err.message : "网络错误");
      });
  };

  FormCaptcha.prototype.verify = function (id, answer, cb) {
    var self = this;
    self.hidId.value = id || "";
    self.hidAns.value = answer || "";
    self.state.id = id || "";
    self.state.answer = answer || "";
    self.syncButton();
    if (cb) cb(true);
  };

  // 验证码未完成时禁用提交按钮（确定性拦截，不依赖其他脚本）
  FormCaptcha.prototype.syncButton = function () {
    var self = this;
    var btn = self.submitBtn;
    if (!btn) return;
    var shouldDisable = self.state.required && !self.state.answer;
    if (shouldDisable) {
      btn.setAttribute("disabled", "disabled");
      btn.setAttribute("title", "请先完成图形验证码");
      btn.style.opacity = "0.6";
      btn.style.cursor = "not-allowed";
    } else {
      btn.removeAttribute("disabled");
      btn.removeAttribute("title");
      btn.style.opacity = "";
      btn.style.cursor = "";
    }
  };

  FormCaptcha.prototype.render = function (cap) {
    var self = this;
    self.state.captcha = cap;
    self.state.loading = false;
    self.hintEl.textContent = cap.hint || "请完成验证码验证";
    self.stageEl.innerHTML = "";
    self.sliderArea.style.display = "none";

    var stage = self.stageEl;

    if (cap.captcha_type === "click_text" || cap.captcha_type === "click_shape") {
      self.renderClick(cap);
    } else if (cap.captcha_type === "slide") {
      self.renderSlide(cap, false);
    } else if (cap.captcha_type === "drag") {
      self.renderSlide(cap, true);
    } else if (cap.captcha_type === "rotate") {
      self.renderRotate(cap);
    } else {
      self.renderClick(cap);
    }
    self.wrap.setAttribute("data-captcha-state", "ready");
    self.state.ready = true;
    self.state.allowSubmit = true;
    // 验证码已渲染但未完成：禁用提交按钮
    self.syncButton();
  };

  // ---------- 点选 ----------
  FormCaptcha.prototype.renderClick = function (cap) {
    var self = this;
    var stage = self.stageEl;
    stage.style.display = "inline-block";
    // 清理可能残留的提示条
    var oldList = self.wrap.querySelector(".formail-captcha-charlist");
    if (oldList) oldList.parentNode.removeChild(oldList);
    var oldThumb = self.wrap.querySelector(".formail-captcha-orderthumb");
    if (oldThumb) oldThumb.parentNode.removeChild(oldThumb);

    var img = el("img", { alt: "验证码" }, { display: "block", "max-width": "320px", "max-height": "200px", width: "100%", cursor: "crosshair", "border-radius": "6px", "user-select": "none" });
    img.src = cap.captcha_base64;
    stage.appendChild(img);

    // go-captcha 自带顺序提示缩略图：按 1..N 排列的点选目标，让用户知道先点哪个
    if (cap.thumb_base64) {
      var thumb = el("img", { class: "formail-captcha-orderthumb", alt: "点选顺序", src: cap.thumb_base64 }, {
        display: "block", "max-width": "200px", margin: "8px auto 0",
        "border-radius": "6px", border: "1px solid #e2e8f0", background: "#fff"
      });
      stage.appendChild(thumb);
    }

    var chars = [];
    if (cap.chars && typeof cap.chars === "string") {
      chars = cap.chars.split(" ").map(function (s) { return s.trim(); }).filter(Boolean);
    }
    var needed = chars.length || 4;
    // click_shape 的 chars 是 shapes-N 这类标识符，不直接展示文本（用上面的顺序缩略图即可）
    var isShapeMode = chars.length > 0 && chars.every(function (c) { return /^shapes?-?\d+$/i.test(c); });
    while (chars.length < needed) chars.push("");

    // 有序字符提示条：仅文字点选模式显示（按 1..N 顺序，点击一个变绿一个）
    var cellEls = [];
    if (chars.length && !isShapeMode) {
      var list = el("div", { class: "formail-captcha-charlist" }, {
        display: "flex", "justify-content": "center", gap: "8px",
        margin: "10px auto 0", "flex-wrap": "wrap", position: "relative", zIndex: "10"
      });
      chars.forEach(function (ch, i) {
        var cell = el("span", {}, {
          display: "inline-flex", "align-items": "center", "justify-content": "center",
          "min-width": "30px", height: "30px", padding: "0 6px", "border-radius": "8px",
          background: "rgba(37,99,235,.12)", border: "2px solid #3b82f6",
          color: "#1e293b", "font-size": "16px", "font-weight": "700",
          "box-sizing": "border-box", transition: "all .2s ease"
        });
        cell.textContent = (i + 1) + (ch ? "." + ch : "");
        list.appendChild(cell);
        cellEls.push(cell);
      });
      self.wrap.insertBefore(list, self.msgEl);
    }

    var points = [];

    function place(x, y) {
      var c = el("span", {}, { position: "absolute", left: x + "px", top: y + "px", width: "18px", height: "18px", "line-height": "18px", "text-align": "center", background: "rgba(37,99,235,.9)", color: "#fff", "border-radius": "50%", "font-size": "11px", "font-weight": "700", "pointer-events": "none", "z-index": "5" });
      c.textContent = String(points.length);
      stage.appendChild(c);
      return c;
    }
    function markDone(idx) {
      if (cellEls[idx]) {
        cellEls[idx].style.background = "#10b981";
        cellEls[idx].style.borderColor = "#10b981";
        cellEls[idx].style.color = "#fff";
      }
    }
    img.addEventListener("click", function (e) {
      if (points.length >= needed) return;
      var rect = img.getBoundingClientRect();
      var scaleX = rect.width / (img.naturalWidth || 1);
      var scaleY = rect.height / (img.naturalHeight || rect.height || 1);
      var natX = Math.round((e.clientX - rect.left) / scaleX);
      var natY = Math.round((e.clientY - rect.top) / scaleY);
      points.push({ x: natX, y: natY });
      place(e.clientX - rect.left, e.clientY - rect.top);
      markDone(points.length - 1);
      if (points.length === needed) {
        var ans = points.map(function (p) { return p.x + "," + p.y; }).join(",");
        self.verify(cap.captcha_id, ans);
        self.setMsg("验证已就绪，可以提交了");
      } else {
        self.hintEl.textContent = cap.hint + "（还剩 " + (needed - points.length) + " 个，请按顺序提示点击）";
      }
    });
  };

  // ---------- 滑块 / 拖拽 ----------
  FormCaptcha.prototype.getScaleFromStage = function (img, stage) {
    var nat = img.naturalWidth || 300;
    var disp = stage.offsetWidth || Math.min(img.naturalWidth || 300, 320);
    return disp / nat;
  };

  FormCaptcha.prototype.bindDragMove = function (target, onDown, onMove, onUp) {
    target.addEventListener("mousedown", onDown);
    target.addEventListener("touchstart", onDown, { passive: false });
    document.addEventListener("mousemove", onMove);
    document.addEventListener("mouseup", onUp);
    document.addEventListener("touchmove", onMove, { passive: false });
    document.addEventListener("touchend", onUp);
    return function unbind() {
      document.removeEventListener("mousemove", onMove);
      document.removeEventListener("mouseup", onUp);
      document.removeEventListener("touchmove", onMove);
      document.removeEventListener("touchend", onUp);
    };
  };

  FormCaptcha.prototype.renderSlide = function (cap, isDrag) {
    var self = this;
    var stage = self.stageEl;
    stage.style.display = "inline-block";

    var img = el("img", { alt: "验证码" }, { display: "block", "max-width": "300px", "max-height": "200px", width: "100%", "border-radius": "6px", "user-select": "none", "-webkit-user-drag": "none" });
    img.src = cap.captcha_base64;
    stage.appendChild(img);

    var bw = cap.thumb_width || 50;
    var bh = cap.thumb_height || 50;
    var piece = el("img", { alt: "" }, { position: "absolute", "z-index": "4", "pointer-events": "none", "user-select": "none", "-webkit-user-drag": "none" });
    piece.src = cap.thumb_base64 || "";
    stage.appendChild(piece);

    // 滑动条
    var slider = el("div", { class: "formail-captcha-slider", title: "拖动滑块完成拼图" }, {
      position: "relative", "max-width": "300px", height: "40px", "line-height": "40px", background: "#e2e8f0", "border-radius": "20px", margin: "10px auto 0", cursor: "pointer", "user-select": "none", overflow: "hidden",
    });
    var progress = el("div", {}, { position: "absolute", left: "0", top: "0", height: "100%", width: "0", background: "rgba(37,99,235,.15)", "border-radius": "20px" });
    var textEl = el("div", {}, { position: "absolute", left: "0", top: "0", width: "100%", height: "100%", "text-align": "center", color: "#64748b", "font-size": "13px" });
    textEl.textContent = "向右拖动验证";
    var handle = el("div", { class: "formail-captcha-handle" }, {
      position: "absolute", left: "2px", top: "2px", width: "36px", height: "36px", "border-radius": "18px", background: "#2563eb", color: "#fff", cursor: "grab", "text-align": "center", "line-height": "36px", "font-size": "16px", "z-index": "2", "box-shadow": "0 2px 6px rgba(0,0,0,.25)",
    });
    handle.textContent = "→";
    slider.appendChild(progress);
    slider.appendChild(textEl);
    slider.appendChild(handle);
    self.sliderArea.style.display = "block";
    self.sliderArea.innerHTML = "";
    self.sliderArea.appendChild(slider);

    function layout() {
      var nat = img.naturalWidth || 300;
      var disp = img.offsetWidth || stage.offsetWidth || 300;
      var scale = disp / nat;
      var maxLeft = clamp(handle.parentElement.offsetWidth - handle.offsetWidth, 1, 1e6);
      return { nat: nat, disp: disp, scale: scale, maxLeft: maxLeft };
    }

    function placePiece(g) {
      var l = layout();
      var pw = Math.round(bw * l.scale);
      var ph = Math.round(bh * l.scale);
      piece.style.width = pw + "px";
      piece.style.height = ph + "px";
      if (isDrag) {
        piece.style.left = (Math.round((cap.init_x || 0) * l.scale)) + "px";
        piece.style.top = (Math.round((cap.init_y || 0) * l.scale)) + "px";
        piece.style.pointerEvents = "auto";
        piece.style.cursor = "grab";
      } else {
        piece.style.left = "0px";
        piece.style.top = (Math.round((cap.thumb_y || 0) * l.scale)) + "px";
      }
    }
    placePiece();

    var dragging = false;
    var startX = 0, startLeft = 0;

    function handleDown(e) {
      var x = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
      dragging = true;
      startX = x;
      startLeft = parseFloat(handle.style.left) || 0;
      e.preventDefault();
    }
    function handleMove(e) {
      if (!dragging) return;
      var x = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
      var l = layout();
      var left = clamp(startLeft + (x - startX), 0, l.maxLeft);
      handle.style.left = left + "px";
      progress.style.width = (left + handle.offsetWidth / 2) + "px";
      var natX = Math.round((left / l.maxLeft) * (l.nat - bw));
      if (isDrag) {
        piece.style.left = left + "px";
        piece.style.top = (Math.round((cap.thumb_y || 0) * l.scale)) + "px";
        self.verify(cap.captcha_id, natX + "," + Math.round((cap.thumb_y || 0)));
      } else {
        piece.style.left = left + "px";
        self.verify(cap.captcha_id, "" + natX);
      }
    }
    function handleUp() {
      if (!dragging) return;
      dragging = false;
    }

    if (isDrag) {
      // 拖拽模式：直接拖动拼图块
      // 将滑块条收起，改为主图内拖拽
      self.sliderArea.style.display = "none";
      var dragPiece = false, pStartX = 0, pStartY = 0, pLeft = 0, pTop = 0;
      function pDown(e) {
        var x = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
        var y = e.clientY || (e.touches && e.touches[0] ? e.touches[0].clientY : 0);
        dragPiece = true;
        pStartX = x; pStartY = y;
        pLeft = parseFloat(piece.style.left) || 0;
        pTop = parseFloat(piece.style.top) || 0;
        piece.style.cursor = "grabbing";
        e.preventDefault();
      }
      function pMove(e) {
        if (!dragPiece) return;
        var x = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
        var y = e.clientY || (e.touches && e.touches[0] ? e.touches[0].clientY : 0);
        var l = layout();
        var left = clamp(pLeft + (x - pStartX), 0, l.disp - bw * l.scale);
        var top = clamp(pTop + (y - pStartY), 0, l.disp * (img.naturalHeight / l.nat) - bh * l.scale);
        piece.style.left = left + "px";
        piece.style.top = top + "px";
        var natX = Math.round(left / l.scale);
        var natY = Math.round(top / l.scale);
        self.verify(cap.captcha_id, natX + "," + natY);
      }
      function pUp() {
        dragPiece = false;
        piece.style.cursor = "grab";
      }
      piece.addEventListener("mousedown", pDown);
      piece.addEventListener("touchstart", pDown, { passive: false });
      document.addEventListener("mousemove", pMove);
      document.addEventListener("mouseup", pUp);
      document.addEventListener("touchmove", pMove, { passive: false });
      document.addEventListener("touchend", pUp);
      return;
    }

    handle.addEventListener("mousedown", handleDown);
    handle.addEventListener("touchstart", handleDown, { passive: false });
    document.addEventListener("mousemove", handleMove);
    document.addEventListener("mouseup", handleUp);
    document.addEventListener("touchmove", handleMove, { passive: false });
    document.addEventListener("touchend", handleUp);
  };

  // ---------- 旋转 ----------
  FormCaptcha.prototype.renderRotate = function (cap) {
    var self = this;
    var stage = self.stageEl;
    stage.style.display = "inline-block";
    var size = 220;
    stage.style.width = Math.min(size, 320) + "px";

    var img = el("img", { alt: "验证码" }, { display: "block", width: "100%", "border-radius": "6px" });
    img.src = cap.captcha_base64;
    stage.appendChild(img);

    var thumb = el("img", { alt: "" }, {
      position: "absolute", left: "50%", top: "50%", "z-index": "2", "pointer-events": "none", "-webkit-user-drag": "none",
    });
    thumb.src = cap.thumb_base64 || "";
    stage.appendChild(thumb);

    function positionThumb() {
      var ratio = (cap.thumb_width || thumb.naturalWidth || 140) / size;
      thumb.style.width = Math.round(stage.offsetWidth * ratio) + "px";
      thumb.style.transform = "translate(-50%, -50%) rotate(0deg)";
    }
    thumb.onload = positionThumb;
    if (thumb.complete) positionThumb();

    var slider = el("div", {}, { position: "relative", "max-width": "300px", height: "40px", "line-height": "40px", background: "#e2e8f0", "border-radius": "20px", margin: "10px auto 0", cursor: "pointer", "user-select": "none", overflow: "hidden" });
    var progress = el("div", {}, { position: "absolute", left: "0", top: "0", height: "100%", width: "0", background: "rgba(37,99,235,.15)", "border-radius": "20px" });
    var textEl = el("div", {}, { position: "absolute", width: "100%", height: "100%", "text-align": "center", color: "#64748b", "font-size": "13px" });
    textEl.textContent = "拖动滑块旋转图片";
    var handle = el("div", {}, { position: "absolute", left: "2px", top: "2px", width: "36px", height: "36px", "border-radius": "18px", background: "#2563eb", color: "#fff", cursor: "grab", "text-align": "center", "line-height": "36px", "z-index": "2", "box-shadow": "0 2px 6px rgba(0,0,0,.25)" });
    handle.textContent = "↻";
    slider.appendChild(progress);
    slider.appendChild(textEl);
    slider.appendChild(handle);
    self.sliderArea.style.display = "block";
    self.sliderArea.innerHTML = "";
    self.sliderArea.appendChild(slider);

    var currentAngle = 0;
    function setAngle(a) {
      currentAngle = ((a % 360) + 360) % 360;
      var maxLeft = Math.max(slider.offsetWidth - handle.offsetWidth, 1);
      var left = (currentAngle / 360) * maxLeft;
      handle.style.left = left + "px";
      progress.style.width = (left + handle.offsetWidth / 2) + "px";
      thumb.style.transform = "translate(-50%, -50%) rotate(-" + currentAngle + "deg)";
      self.verify(cap.captcha_id, "" + Math.round((360 - currentAngle) % 360));
    }
    var dragging = false, startX = 0, startAngle = 0;
    function down(e) {
      var x = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
      dragging = true;
      startX = x;
      startAngle = currentAngle;
      e.preventDefault();
    }
    function move(e) {
      if (!dragging) return;
      var x = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
      var delta = (x - startX) / Math.max(slider.offsetWidth, 1) * 360;
      setAngle(startAngle + delta);
    }
    function up() {
      dragging = false;
    }
    handle.addEventListener("mousedown", down);
    handle.addEventListener("touchstart", down, { passive: false });
    document.addEventListener("mousemove", move);
    document.addEventListener("mouseup", up);
    document.addEventListener("touchmove", move, { passive: false });
    document.addEventListener("touchend", up);
    setAngle(0);
    handle.style.left = "2px";
    progress.style.width = (handle.offsetWidth / 2) + "px";
  };

  // ---------- 初始化 ----------
  ready(function () {
    findForms().forEach(function (f) {
      var token = getToken(f.getAttribute("action") || "");
      if (token) {
        new FormCaptcha(f, token);
      }
    });
  });
})();