const API = {
  base: "",
  token() {
    return localStorage.getItem("formail_token") || "";
  },
  setToken(token) {
    localStorage.setItem("formail_token", token || "");
  },
  clearToken() {
    localStorage.removeItem("formail_token");
  },
  async request(path, options = {}, auth = true) {
    const headers = options.headers || {};
    if (auth && this.token())
      headers["Authorization"] = `Bearer ${this.token()}`;
    if (!(options.body instanceof FormData))
      headers["Content-Type"] = "application/json";

    const timeoutMs = Number(options.timeoutMs || 15000);
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);

    let res;
    try {
      res = await fetch(this.base + path, {
        ...options,
        headers,
        signal: controller.signal,
      });
    } catch (err) {
      if (err && err.name === "AbortError") {
        throw new Error(
          `请求超时（>${Math.floor(timeoutMs / 1000)}s），请重试`,
        );
      }
      throw err;
    } finally {
      clearTimeout(timer);
    }
    const ct = res.headers.get("content-type") || "";
    if (ct.includes("application/json")) {
      const json = await res.json();
      if (!res.ok || json.code !== 0)
        throw new Error(json.message || `HTTP ${res.status}`);
      return json.data;
    }
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res;
  },
};

function toast(msg, isErr = false) {
  const el = document.createElement("div");
  el.className = "toast " + (isErr ? "toast-error" : "toast-success");
  el.innerText = msg;
  document.body.appendChild(el);
  setTimeout(() => el.classList.add("show"), 10);
  setTimeout(() => {
    el.classList.remove("show");
    el.classList.add("hide");
    setTimeout(() => el.remove(), 250);
  }, 2200);
}
