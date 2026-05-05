function getDeviceFingerprint() {
  let fp = localStorage.getItem("formail_device_fp");
  if (!fp) {
    fp = "fp-" + Math.random().toString(36).slice(2) + Date.now().toString(36);
    localStorage.setItem("formail_device_fp", fp);
  }
  return fp;
}

function onLoginSuccess(data) {
  API.setToken(data.token);
  localStorage.setItem("formail_role", data.role || "user");
  location.href = "/dashboard/forms";
}

function markInputError(id) {
  const el = document.getElementById(id);
  if (!el) return;
  el.style.borderColor = "#ef4444";
  setTimeout(() => {
    el.style.borderColor = "";
  }, 1600);
}

function activateTab(tabId) {
  document
    .querySelectorAll(".tab-btn")
    .forEach((b) => b.classList.remove("active"));
  document
    .querySelectorAll(".tab-pane")
    .forEach((p) => p.classList.remove("active"));
  const btn = document.querySelector(`.tab-btn[data-tab="${tabId}"]`);
  const pane = document.getElementById(tabId);
  if (btn) btn.classList.add("active");
  if (pane) pane.classList.add("active");
  localStorage.setItem("formail_login_tab", tabId);
}

function bindTabs() {
  document.querySelectorAll(".tab-btn").forEach((btn) => {
    btn.onclick = () => activateTab(btn.dataset.tab);
  });
  const saved = localStorage.getItem("formail_login_tab");
  if (saved) activateTab(saved);
}

function setCooldown(btn, seconds) {
  btn.disabled = true;
  const original = btn.dataset.originalText || btn.textContent;
  btn.dataset.originalText = original;
  let left = seconds;
  btn.textContent = `${original} (${left}s)`;
  const timer = setInterval(() => {
    left -= 1;
    if (left <= 0) {
      clearInterval(timer);
      btn.disabled = false;
      btn.textContent = original;
      return;
    }
    btn.textContent = `${original} (${left}s)`;
  }, 1000);
}

function isValidEmail(email) {
  const v = String(email || "").trim();
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v);
}

async function fetchCaptcha(prefix) {
  const r = await API.request("/api/auth/captcha", {}, false);
  if (!r.captcha_enabled) {
    return null;
  }
  return {
    id: r.captcha_id,
    type: r.captcha_type,
    base64: r.captcha_base64,
    thumbBase64: r.thumb_base64,
    hint: r.hint,
    chars: r.chars || "",
    thumbX: r.thumb_x || 0,
    thumbY: r.thumb_y || 0,
    thumbWidth: r.thumb_width || 0,
    thumbHeight: r.thumb_height || 0,
    angle: r.angle || 0,
    initX: r.init_x || 0,
    initY: r.init_y || 0,
  };
}

function showCaptchaModal(prefix, onSuccess, onCancel) {
  const modal = document.getElementById("captchaModal");
  const canvas = document.getElementById("captchaCanvas");
  const thumbEl = document.getElementById("captchaThumb");
  const hintTextEl = modal.querySelector(".captcha-hint-text");
  const answerEl = document.getElementById("captchaAnswer");
  const refreshBtn = document.getElementById("captchaRefresh");
  const confirmBtn = document.getElementById("captchaConfirm");
  const closeBtn = document.getElementById("captchaClose");
  const sliderArea = document.getElementById("captchaSliderArea");

  let currentCaptcha = null;
  let clickPoints = [];

  function cleanupDynamicWidgets() {
    var container = canvas.parentElement;
    container.querySelectorAll(".captcha-dot, .captcha-piece, .captcha-rotate-thumb").forEach(function(el) { el.remove(); });
    if (sliderArea) { sliderArea.style.display = "none"; sliderArea.innerHTML = ""; }
    canvas.onclick = null;
    canvas.onload = null;
    canvas.style.transform = "";
    canvas.style.cursor = "default";
    // 重置旋转模式下的容器和 canvas 样式
    container.style.backgroundImage = "";
    container.style.backgroundSize = "";
    container.style.backgroundRepeat = "";
    container.style.backgroundPosition = "";
    container.style.height = "";
    canvas.style.position = "";
    canvas.style.left = "";
    canvas.style.top = "";
    canvas.style.marginLeft = "";
    canvas.style.marginTop = "";
    canvas.style.width = "";
    canvas.style.zIndex = "";
  }

  // 计算 canvas 显示缩放比（无 object-fit，canvas 宽度/自然宽度）
  function getImageScale() {
    var nw = Math.max(canvas.naturalWidth || 0, 1);
    var dw = canvas.offsetWidth || nw;
    return dw / nw;
  }

  async function loadCaptcha() {
    cleanupDynamicWidgets();
    clickPoints = [];

    const captcha = await fetchCaptcha(prefix);
    if (!captcha) {
      onSuccess(null);
      closeModal();
      return;
    }
    currentCaptcha = captcha;
    answerEl.value = "";

    hintTextEl.textContent = captcha.hint || "请完成验证码验证";

    canvas.src = captcha.base64;
    thumbEl.src = captcha.thumbBase64 || "";

    if (captcha.type === "click_text" || captcha.type === "click_shape") {
      // 点选模式：显示提示缩略图
      thumbEl.classList.add("visible");
      answerEl.style.display = "none";
      canvas.onload = function () {
        canvas.style.cursor = "crosshair";
        canvas.onclick = handleCanvasClick;
      };
      if (canvas.complete) {
        canvas.style.cursor = "crosshair";
        canvas.onclick = handleCanvasClick;
      }
    } else if (captcha.type === "slide" || captcha.type === "drag") {
      // 滑动/拖拽模式：隐藏提示缩略图
      thumbEl.classList.remove("visible");
      answerEl.style.display = "none";
      answerEl.style.display = "none";
      canvas.onload = function () {
        initSlideCaptcha(captcha);
      };
      if (canvas.complete) {
        initSlideCaptcha(captcha);
      }
    } else if (captcha.type === "rotate") {
      // 旋转模式：隐藏提示缩略图
      thumbEl.classList.remove("visible");
      answerEl.style.display = "none";
      canvas.onload = function () {
        initRotateCaptcha(captcha);
      };
      if (canvas.complete) {
        initRotateCaptcha(captcha);
      }
    } else {
      answerEl.style.display = "block";
      answerEl.placeholder = captcha.hint || "请完成验证码验证";
    }

    modal.style.display = "flex";
  }

  async function verifyWithBackend() {
    if (!currentCaptcha) return false;
    try {
      var answer = answerEl.value.trim();
      if (!answer && clickPoints.length > 0) {
        answer = clickPoints.map(function (p) { return p.x + "," + p.y; }).join(",");
      }
      if (!answer) return false;
      var r = await API.request(
        "/api/auth/verify-captcha",
        { method: "POST", body: JSON.stringify({ captcha_id: currentCaptcha.id, answer: answer }) },
        false
      );
      return r.verified === true;
    } catch (e) {
      // 验证失败（无论是否达到失败次数上限）
      // 如果是失败次数过多，刷新验证码
      if (e.message.includes("刷新验证码") || e.message.includes("429")) {
        toast(e.message, true);
        loadCaptcha();
      }
      return false;
    }
  }

  async function handleCanvasClick(e) {
    var container = canvas.parentElement;
    var containerRect = container.getBoundingClientRect();
    var scale = getImageScale();

    // 点击在 canvas 内的自然坐标
    var clickInCanvasX = e.clientX - canvas.getBoundingClientRect().left;
    var clickInCanvasY = e.clientY - canvas.getBoundingClientRect().top;

    var originalX = Math.round(clickInCanvasX / scale);
    var originalY = Math.round(clickInCanvasY / scale);

    clickPoints.push({ x: originalX, y: originalY });

    var dot = document.createElement("div");
    dot.className = "captcha-dot";
    dot.style.left = (e.clientX - containerRect.left) + "px";
    dot.style.top = (e.clientY - containerRect.top) + "px";

    var number = document.createElement("span");
    number.className = "captcha-dot-number";
    number.textContent = clickPoints.length;
    dot.appendChild(number);

    container.appendChild(dot);

    answerEl.value = clickPoints.map(function (p) { return p.x + "," + p.y; }).join(",");

    // 点选4次后自动验证
    if (clickPoints.length >= 4) {
      (async function () {
        canvas.style.pointerEvents = "none";
        var success = await verifyWithBackend();
        if (success) {
          // 验证成功，关闭弹窗
          setTimeout(function () {
            closeModal();
            onSuccess({ id: currentCaptcha.id, answer: answerEl.value });
          }, 300);
        } else {
          // 验证失败，重置
          setTimeout(function () {
            container.querySelectorAll(".captcha-dot").forEach(function (el) { el.remove(); });
            clickPoints = [];
            answerEl.value = "";
            canvas.style.pointerEvents = "auto";
          }, 500);
        }
      })();
    }
  }

  function closeModal() {
    modal.style.display = "none";
    clickPoints = [];
    cleanupDynamicWidgets();
    if (onCancel) onCancel();
  }

  refreshBtn.onclick = function () {
    toast("正在刷新验证码...", false);
    loadCaptcha();
  };
  closeBtn.onclick = closeModal;

  // ==================== 滑动 / 拖拽验证码 ====================
  function initSlideCaptcha(captcha) {
    var container = canvas.parentElement;
    var scale = getImageScale();

    var bw = captcha.thumbWidth || 50;
    var bh = captcha.thumbHeight || 50;
    var pieceDispW = Math.round(bw * scale);
    var pieceDispH = Math.round(bh * scale);

    var targetX = captcha.thumbX || 0;
    var targetY = captcha.thumbY || 0;
    var targetDispX = targetX * scale;
    var targetDispY = targetY * scale;

    var maxDispX = (canvas.offsetWidth || 0) - pieceDispW;
    var maxDispY = (canvas.offsetHeight || 0) - pieceDispH;

    // 初始位置：拖拽模式从服务端指定位置开始，滑动模式从左侧边缘开始
    var initDispX = 0;
    var initDispY = Math.max(0, Math.min(targetDispY, maxDispY));

    if (captcha.type === "drag") {
      initDispX = Math.round((captcha.initX || 0) * scale);
      initDispY = Math.round((captcha.initY || 0) * scale);
      initDispX = Math.max(0, Math.min(initDispX, maxDispX));
      initDispY = Math.max(0, Math.min(initDispY, maxDispY));
    }

    var piece = document.createElement("img");
    piece.className = "captcha-piece";
    piece.src = captcha.thumbBase64 || "";
    piece.style.position = "absolute";
    piece.style.zIndex = "11";
    piece.style.userSelect = "none";
    piece.style.width = pieceDispW + "px";
    piece.style.height = pieceDispH + "px";
    container.appendChild(piece);

    function placePiece(dispX, dispY) {
      dispX = Math.max(0, Math.min(dispX, maxDispX));
      dispY = Math.max(0, Math.min(dispY, maxDispY));
      piece.style.left = (canvas.offsetLeft + dispX) + "px";
      piece.style.top = (canvas.offsetTop + dispY) + "px";
      var natX = Math.round(dispX / scale);
      var natY = Math.round(dispY / scale);
      if (captcha.type === "drag") {
        answerEl.value = natX + "," + natY;
      } else {
        answerEl.value = "" + natX;
      }
    }

    placePiece(initDispX, initDispY);

    // ===== 拖拽模式：直接在图片上拖拽拼图块，无滑动条 =====
    if (captcha.type === "drag") {
      piece.style.pointerEvents = "auto";
      piece.style.cursor = "grab";

      var verified = false;
      var dragStartX = 0, dragStartY = 0;
      var pieceStartLeft = 0, pieceStartTop = 0;
      var isDragging = false;

      function onDown(e) {
        if (verified) return;
        isDragging = true;
        piece.style.cursor = "grabbing";
        piece.style.transition = "none";
        dragStartX = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
        dragStartY = e.clientY || (e.touches && e.touches[0] ? e.touches[0].clientY : 0);
        pieceStartLeft = parseFloat(piece.style.left) || canvas.offsetLeft;
        pieceStartTop = parseFloat(piece.style.top) || canvas.offsetTop;
        e.preventDefault();
        document.addEventListener("mousemove", onMove);
        document.addEventListener("mouseup", onUp);
        document.addEventListener("touchmove", onMove, { passive: false });
        document.addEventListener("touchend", onUp);
      }

      function onMove(e) {
        if (!isDragging) return;
        var clientX = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
        var clientY = e.clientY || (e.touches && e.touches[0] ? e.touches[0].clientY : 0);
        var dx = clientX - dragStartX;
        var dy = clientY - dragStartY;
        var newLeft = Math.max(canvas.offsetLeft, Math.min(pieceStartLeft + dx, canvas.offsetLeft + maxDispX));
        var newTop = Math.max(canvas.offsetTop, Math.min(pieceStartTop + dy, canvas.offsetTop + maxDispY));
        piece.style.left = newLeft + "px";
        piece.style.top = newTop + "px";
        var natX = Math.round((newLeft - canvas.offsetLeft) / scale);
        var natY = Math.round((newTop - canvas.offsetTop) / scale);
        answerEl.value = natX + "," + natY;
      }

      async function onUp() {
        if (!isDragging) return;
        isDragging = false;
        piece.style.cursor = "grab";
        document.removeEventListener("mousemove", onMove);
        document.removeEventListener("mouseup", onUp);
        document.removeEventListener("touchmove", onMove);
        document.removeEventListener("touchend", onUp);

        var curLeft = parseFloat(piece.style.left) || 0;
        var curTop = parseFloat(piece.style.top) || 0;
        var curDispX = curLeft - canvas.offsetLeft;
        var curDispY = curTop - canvas.offsetTop;
        var dist = Math.sqrt(Math.pow(curDispX - targetDispX, 2) + Math.pow(curDispY - targetDispY, 2));

        // 调用后端验证
        var success = await verifyWithBackend();

        if (success) {
          verified = true;
          piece.style.transition = "left 0.2s ease, top 0.2s ease";
          placePiece(targetDispX, targetDispY);
          piece.style.cursor = "default";
          piece.style.pointerEvents = "none";
          piece.style.boxShadow = "0 0 0 2px rgba(16, 185, 129, 0.6)";
          answerEl.value = Math.round(targetDispX / scale) + "," + Math.round(targetDispY / scale);
          // 验证成功，关闭弹窗
          setTimeout(function () {
            closeModal();
            onSuccess({ id: currentCaptcha.id, answer: answerEl.value });
          }, 300);
        } else {
          // 验证失败，回弹到初始位置
          piece.style.transition = "left 0.35s cubic-bezier(0.25, 0.8, 0.25, 1.2), top 0.35s cubic-bezier(0.25, 0.8, 0.25, 1.2)";
          placePiece(initDispX, initDispY);
        }
      }

      piece.addEventListener("mousedown", onDown);
      piece.addEventListener("touchstart", onDown);
      return;
    }

    // ===== 滑动模式：滑动条控制拼图块 =====
    sliderArea.style.display = "block";
    sliderArea.innerHTML = "";

    var slider = document.createElement("div");
    slider.className = "captcha-slider";
    slider.innerHTML =
      '<div class="captcha-slider-progress"></div>' +
      '<div class="captcha-slider-text">向右滑动验证</div>' +
      '<div class="captcha-slider-handle"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M14 5l7 7m0 0l-7 7m7-7H3"/></svg></div>';
    sliderArea.appendChild(slider);

    var progress = slider.querySelector(".captcha-slider-progress");
    var handle = slider.querySelector(".captcha-slider-handle");
    var textEl = slider.querySelector(".captcha-slider-text");

    var isDraggingSlider = false;
    var startX = 0;
    var startLeft = 0;
    var slideVerified = false;

    function updateBySliderLeft(leftVal) {
      var maxLeft = Math.max(slider.offsetWidth - handle.offsetWidth, 1);
      var ratio = Math.max(0, Math.min(leftVal / maxLeft, 1));
      var dispX = ratio * maxDispX;
      placePiece(dispX, initDispY);
    }

    function onSliderDown(e) {
      if (slideVerified) return;
      isDraggingSlider = true;
      startX = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
      startLeft = parseFloat(handle.style.left) || 2;
      handle.style.transition = "none";
      progress.style.transition = "none";
      piece.style.transition = "none";
      e.preventDefault();
      document.addEventListener("mousemove", onSliderMove);
      document.addEventListener("mouseup", onSliderUp);
      document.addEventListener("touchmove", onSliderMove, { passive: false });
      document.addEventListener("touchend", onSliderUp);
    }

    function onSliderMove(e) {
      if (!isDraggingSlider) return;
      var clientX = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
      var deltaX = clientX - startX;
      var maxLeft = slider.offsetWidth - handle.offsetWidth;
      var newLeft = Math.max(0, Math.min(startLeft + deltaX, maxLeft));
      handle.style.left = newLeft + "px";
      progress.style.width = (newLeft + handle.offsetWidth / 2) + "px";
      updateBySliderLeft(newLeft);
    }

    async function onSliderUp() {
      if (!isDraggingSlider) return;
      isDraggingSlider = false;
      document.removeEventListener("mousemove", onSliderMove);
      document.removeEventListener("mouseup", onSliderUp);
      document.removeEventListener("touchmove", onSliderMove);
      document.removeEventListener("touchend", onSliderUp);

      var curLeft = parseFloat(handle.style.left) || 0;
      var maxLeft = slider.offsetWidth - handle.offsetWidth;
      var ratio = maxLeft > 0 ? curLeft / maxLeft : 0;
      var curDispX = ratio * maxDispX;
      var dist = Math.abs(curDispX - targetDispX);

      // 调用后端验证
      var success = await verifyWithBackend();

      if (success) {
        slideVerified = true;
        var snapRatio = maxDispX > 0 ? targetDispX / maxDispX : 0;
        var snapLeft = snapRatio * maxLeft;
        handle.style.transition = "left 0.2s ease";
        progress.style.transition = "width 0.2s ease";
        piece.style.transition = "left 0.2s ease, top 0.2s ease";
        handle.style.left = snapLeft + "px";
        progress.style.width = (snapLeft + handle.offsetWidth / 2) + "px";
        placePiece(targetDispX, targetDispY);
        handle.style.cursor = "default";
        handle.style.background = "#10B981";
        handle.style.border = "2px solid #10B981";
        handle.style.color = "#fff";
        handle.querySelector("svg").innerHTML = '<path d="M5 13l4 4L19 7"/>';
        progress.style.background = "linear-gradient(135deg, #10B981, #34D399)";
        textEl.textContent = "验证通过";
        textEl.style.color = "#fff";
        slider.style.background = "#10B981";
        // 验证成功，关闭弹窗
        setTimeout(function () {
          closeModal();
          onSuccess({ id: currentCaptcha.id, answer: answerEl.value });
        }, 300);
      } else {
        handle.style.transition = "left 0.35s cubic-bezier(0.25, 0.8, 0.25, 1.2)";
        progress.style.transition = "width 0.35s cubic-bezier(0.25, 0.8, 0.25, 1.2)";
        piece.style.transition = "left 0.35s cubic-bezier(0.25, 0.8, 0.25, 1.2), top 0.35s cubic-bezier(0.25, 0.8, 0.25, 1.2)";
        handle.style.left = "2px";
        progress.style.width = (handle.offsetWidth / 2) + "px";
        placePiece(initDispX, initDispY);
      }
    }

    handle.addEventListener("mousedown", onSliderDown);
    handle.addEventListener("touchstart", onSliderDown);

    handle.style.left = "2px";
    progress.style.width = (handle.offsetWidth / 2) + "px";
  }

  // ==================== 旋转验证码 ====================
  // master 图片是未旋转的正确方向（DrawWithNRGBA），作为静态背景参考
  // thumb 图片是旋转过的（DrawWithCropCircle），由用户拖动旋转以匹配 master
  function initRotateCaptcha(captcha) {
    var container = canvas.parentElement;
    var cw = container.offsetWidth;
    var masterNatSize = 220; // rotate.WithImageSquareSize(220)
    var thumbNatW = captcha.thumbWidth || canvas.naturalWidth || 140;
    var thumbRatio = thumbNatW / masterNatSize;

    // 容器设为正方形以匹配 master 图片
    container.style.height = cw + "px";
    // Master 图片作为静态背景（正确方向参考）
    container.style.backgroundImage = "url('" + captcha.base64 + "')";
    container.style.backgroundSize = "contain";
    container.style.backgroundRepeat = "no-repeat";
    container.style.backgroundPosition = "center center";

    // Canvas 显示 thumb（旋转过的图片），绝对定位居中
    canvas.src = captcha.thumbBase64 || "";
    canvas.style.position = "absolute";
    canvas.style.left = "50%";
    canvas.style.top = "50%";
    canvas.style.zIndex = "2";

    function positionThumb() {
      var displayW = Math.round(cw * thumbRatio);
      canvas.style.width = displayW + "px";
      canvas.style.height = "auto";
      // translate(-50%, -50%) 实现居中，rotate 实现旋转
      canvas.style.transform = "translate(-50%, -50%) rotate(0deg)";
    }

    canvas.onload = positionThumb;
    if (canvas.complete) positionThumb();

    // 滑动条
    sliderArea.style.display = "block";
    sliderArea.innerHTML = "";

    var slider = document.createElement("div");
    slider.className = "captcha-slider";
    slider.innerHTML =
      '<div class="captcha-slider-progress"></div>' +
      '<div class="captcha-slider-text">拖动滑块旋转图片</div>' +
      '<div class="captcha-slider-handle"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round"><path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/></svg></div>';
    sliderArea.appendChild(slider);

    var progress = slider.querySelector(".captcha-slider-progress");
    var handle = slider.querySelector(".captcha-slider-handle");
    var textEl = slider.querySelector(".captcha-slider-text");

    var isDragging = false;
    var startX = 0;
    var startAngle = 0;
    var currentAngle = 0;
    var verified = false;
    var targetAngle = captcha.angle || 0;

    function setAngle(angle) {
      var normalized = angle % 360;
      if (normalized < 0) normalized += 360;
      currentAngle = normalized;
      var maxLeft = Math.max(slider.offsetWidth - handle.offsetWidth, 1);
      var left = (currentAngle / 360) * maxLeft;
      handle.style.left = left + "px";
      progress.style.width = (left + handle.offsetWidth / 2) + "px";
      canvas.style.transform = "translate(-50%, -50%) rotate(-" + currentAngle + "deg)";
      answerEl.value = "" + Math.round((360 - currentAngle) % 360);
    }

    function onRotateDown(e) {
      if (verified) return;
      isDragging = true;
      startX = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
      startAngle = currentAngle;
      handle.style.transition = "none";
      progress.style.transition = "none";
      canvas.style.transition = "none";
      e.preventDefault();
      document.addEventListener("mousemove", onRotateMove);
      document.addEventListener("mouseup", onRotateUp);
      document.addEventListener("touchmove", onRotateMove, { passive: false });
      document.addEventListener("touchend", onRotateUp);
    }

    function onRotateMove(e) {
      if (!isDragging) return;
      var clientX = e.clientX || (e.touches && e.touches[0] ? e.touches[0].clientX : 0);
      var deltaX = clientX - startX;
      var deltaAngle = (deltaX / Math.max(slider.offsetWidth, 1)) * 360;
      setAngle(startAngle + deltaAngle);
    }

    async function onRotateUp() {
      if (!isDragging) return;
      isDragging = false;
      document.removeEventListener("mousemove", onRotateMove);
      document.removeEventListener("mouseup", onRotateUp);
      document.removeEventListener("touchmove", onRotateMove);
      document.removeEventListener("touchend", onRotateUp);

      var diff = Math.abs(currentAngle - targetAngle);
      diff = Math.min(diff, 360 - diff);

      // 调用后端验证
      var success = await verifyWithBackend();

      if (success) {
        verified = true;
        handle.style.transition = "left 0.2s ease";
        progress.style.transition = "width 0.2s ease";
        canvas.style.transition = "transform 0.2s ease";
        var maxLeft = Math.max(slider.offsetWidth - handle.offsetWidth, 1);
        var snapLeft = (targetAngle / 360) * maxLeft;
        handle.style.left = snapLeft + "px";
        progress.style.width = (snapLeft + handle.offsetWidth / 2) + "px";
        setAngle(targetAngle);
        handle.style.cursor = "default";
        handle.style.background = "#10B981";
        handle.style.border = "2px solid #10B981";
        handle.style.color = "#fff";
        handle.querySelector("svg").innerHTML = '<path d="M5 13l4 4L19 7"/>';
        progress.style.background = "linear-gradient(135deg, #10B981, #34D399)";
        textEl.textContent = "验证通过";
        textEl.style.color = "#fff";
        slider.style.background = "#10B981";
        // 验证成功，关闭弹窗
        setTimeout(function () {
          closeModal();
          onSuccess({ id: currentCaptcha.id, answer: answerEl.value });
        }, 300);
      }
    }

    setAngle(0);
    handle.style.left = "2px";
    progress.style.width = (handle.offsetWidth / 2) + "px";
    handle.addEventListener("mousedown", onRotateDown);
    handle.addEventListener("touchstart", onRotateDown);
  }

  loadCaptcha();
}

function bindEnterShortcuts() {
  ["password", "code_login_code", "reg_code"].forEach((id) => {
    const el = document.getElementById(id);
    if (!el) return;
    el.addEventListener("keydown", (e) => {
      if (e.key !== "Enter") return;
      if (id === "password") document.getElementById("btnLogin").click();
      if (id === "code_login_code")
        document.getElementById("btnLoginCode").click();
      if (id === "reg_code") document.getElementById("btnRegister").click();
    });
  });
}

function initLoginPage() {
  bindTabs();
  bindEnterShortcuts();

  document.getElementById("btnLogin").onclick = async () => {
    try {
      const username = document.getElementById("username").value.trim();
      const password = document.getElementById("password").value;
      if (!username) {
        markInputError("username");
        throw new Error("请输入用户名");
      }
      if (!password) {
        markInputError("password");
        throw new Error("请输入密码");
      }
      const data = await API.request(
        "/api/auth/login",
        { method: "POST", body: JSON.stringify({ username, password }) },
        false,
      );
      onLoginSuccess(data);
    } catch (e) {
      toast(e.message, true);
    }
  };

  document.getElementById("btnSendLoginCode").onclick = async () => {
    try {
      const email = document.getElementById("code_login_email").value.trim();
      if (!email) {
        markInputError("code_login_email");
        throw new Error("请输入邮箱");
      }
      if (!isValidEmail(email)) {
        markInputError("code_login_email");
        throw new Error("邮箱格式不正确");
      }

      const openLoginCaptcha = () => {
        showCaptchaModal("login", async (captcha) => {
          if (!captcha) {
            await sendCode(
              "login",
              email,
              document.getElementById("btnSendLoginCode"),
            );
            return;
          }

          try {
            await sendCodeWithCaptcha(
              "login",
              email,
              captcha.id,
              captcha.answer,
              document.getElementById("btnSendLoginCode"),
            );
          } catch (e) {
            toast(e.message, true);
            if (/刷新验证码/.test(e.message)) {
              openLoginCaptcha();
            }
          }
        });
      };
      openLoginCaptcha();
    } catch (e) {
      toast(e.message, true);
    }
  };

  document.getElementById("btnLoginCode").onclick = async () => {
    try {
      const email = document.getElementById("code_login_email").value.trim();
      const code = document.getElementById("code_login_code").value.trim();
      if (!email) {
        markInputError("code_login_email");
        throw new Error("请输入邮箱");
      }
      if (!code) {
        markInputError("code_login_code");
        throw new Error("请输入验证码");
      }
      const data = await API.request(
        "/api/auth/login-code",
        {
          method: "POST",
          body: JSON.stringify({ email, code }),
        },
        false,
      );
      onLoginSuccess(data);
    } catch (e) {
      toast(e.message, true);
    }
  };

  document.getElementById("btnSendRegCode").onclick = async () => {
    try {
      const email = document.getElementById("reg_email").value.trim();
      if (!email) {
        markInputError("reg_email");
        throw new Error("请输入邮箱");
      }
      if (!isValidEmail(email)) {
        markInputError("reg_email");
        throw new Error("邮箱格式不正确");
      }

      const openRegisterCaptcha = () => {
        showCaptchaModal("register", async (captcha) => {
          if (!captcha) {
            await sendCode(
              "register",
              email,
              document.getElementById("btnSendRegCode"),
            );
            return;
          }

          try {
            await sendCodeWithCaptcha(
              "register",
              email,
              captcha.id,
              captcha.answer,
              document.getElementById("btnSendRegCode"),
            );
          } catch (e) {
            toast(e.message, true);
            if (/刷新验证码/.test(e.message)) {
              openRegisterCaptcha();
            }
          }
        });
      };
      openRegisterCaptcha();
    } catch (e) {
      toast(e.message, true);
    }
  };

  document.getElementById("btnRegister").onclick = async () => {
    try {
      const email = document.getElementById("reg_email").value.trim();
      const code = document.getElementById("reg_code").value.trim();
      const password = document.getElementById("reg_password").value;
      const displayName = document
        .getElementById("reg_display_name")
        .value.trim();
      if (!email) {
        markInputError("reg_email");
        throw new Error("请输入邮箱");
      }
      if (!code) {
        markInputError("reg_code");
        throw new Error("请输入验证码");
      }
      if (!password) {
        markInputError("reg_password");
        throw new Error("请输入密码");
      }
      await API.request(
        "/api/auth/register",
        {
          method: "POST",
          body: JSON.stringify({
            email,
            code,
            password,
            display_name: displayName,
          }),
        },
        false,
      );
      toast("注册成功，请等待状态启用后登录");
    } catch (e) {
      toast(e.message, true);
    }
  };
}

async function sendCode(purpose, email, btn) {
  const r = await API.request(
    "/api/auth/send-code",
    {
      method: "POST",
      headers: { "X-Device-Fingerprint": getDeviceFingerprint() },
      body: JSON.stringify({ email, purpose }),
    },
    false,
  );
  setCooldown(btn, Number(r.cooldown_seconds || 60));
  toast("验证码已发送（如账号存在）");
}

async function sendCodeWithCaptcha(
  purpose,
  email,
  captchaId,
  captchaAnswer,
  btn,
) {
  const r = await API.request(
    "/api/auth/send-code",
    {
      method: "POST",
      headers: { "X-Device-Fingerprint": getDeviceFingerprint() },
      body: JSON.stringify({
        email,
        purpose,
        captcha_id: captchaId,
        captcha_answer: captchaAnswer,
      }),
    },
    false,
  );
  setCooldown(btn, Number(r.cooldown_seconds || 60));
  toast("验证码已发送");
}

document.addEventListener("DOMContentLoaded", initLoginPage);
