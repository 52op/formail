function requireAuth() {
  const params = new URLSearchParams(location.search);
  const urlToken = params.get('token');
  if (urlToken) {
    API.setToken(urlToken);
    params.delete('token');
    const clean = params.toString();
    history.replaceState(null, '', location.pathname + (clean ? '?' + clean : ''));
  }

  // 启动后台任务，拿 role / app-config / SSO 会话检查，renderLayout 会 await 它
  window.__sessionReady = (async () => {
    // 先加载 app-config
    try {
      const cfgRes = await fetch('/api/app-config');
      if (cfgRes.ok) {
        const cfgData = await cfgRes.json();
        if (cfgData.data) window.__appConfig = cfgData.data;
      }
    } catch {}

    const appCfg = window.__appConfig;
    const isSSO = appCfg && appCfg.auth_mode === 'sso' && appCfg.sso_url;

    // SSO 模式：每次都检查 GoAuth 会话（保证退出/切换账号能同步）
    if (isSSO) {
      // HTTP 访问无法发送 Secure cookie，自动升级到 HTTPS
      if (location.protocol === 'http:') {
        location.replace(location.href.replace(/^http:/, 'https:'))
        return
      }
      try {
        const ssoRes = await fetch(appCfg.sso_url + '/api/auth/me', { credentials: 'include' });
        if (ssoRes.ok) {
          const ssoData = await ssoRes.json();
          const goauthToken = ssoData.data && ssoData.data.token;
          if (goauthToken) {
            if (goauthToken !== API.token()) {
              // GoAuth 用户变了（切换账号/首次）→ 更新 token
              API.setToken(goauthToken);
              if (ssoData.data.role) localStorage.setItem('formail_role', ssoData.data.role);
            }
            // 验证本地 token
            try {
              const me = await API.request('/api/auth/me');
              if (me && me.role) localStorage.setItem('formail_role', me.role);
            } catch {}
            return;
          }
        }
        // GoAuth 未登录 → 清除本地 token
        API.clearToken();
        localStorage.removeItem('formail_role');
        location.href = "/login";
        throw new Error('AUTH_REQUIRED');
      } catch (e) {
        if (e.message === 'AUTH_REQUIRED') throw e;
        // GoAuth 不可达，走本地验证
        if (!API.token()) {
          location.href = "/login";
          throw new Error('AUTH_REQUIRED');
        }
        try {
          const me = await API.request('/api/auth/me');
          if (me && me.role) localStorage.setItem('formail_role', me.role);
        } catch {}
        return;
      }
    }

    // standalone 模式：只验证本地 token
    if (!API.token()) {
      location.href = "/login";
      throw new Error('AUTH_REQUIRED');
    }
    try {
      const me = await API.request('/api/auth/me');
      if (me && me.role) localStorage.setItem('formail_role', me.role);
    } catch {}
  })();
}

function logout() {
  API.clearToken();
  localStorage.removeItem("formail_role");
  // SSO 模式：跳到 GoAuth 清 cookie 后再跳回本站 /login
  const appCfg = window.__appConfig;
  if (appCfg && appCfg.auth_mode === 'sso' && appCfg.sso_url) {
    location.href = appCfg.sso_url + '/logout?redirect=' + encodeURIComponent(location.origin + '/login');
    return;
  }
  location.href = "/login";
}

function isAdmin() {
  return localStorage.getItem("formail_role") === "admin";
}

function iconForms() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <rect x="3" y="3" width="18" height="18" rx="2" ry="2"/>
    <circle cx="8.5" cy="8.5" r="1.5"/>
    <polyline points="21 15 16 10 5 21"/>
  </svg>`;
}

function iconChannels() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"/>
  </svg>`;
}

function iconSubmissions() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
    <polyline points="14 2 14 8 20 8"/>
    <line x1="16" y1="13" x2="8" y2="13"/>
    <line x1="16" y1="17" x2="8" y2="17"/>
    <polyline points="10 9 9 9 8 9"/>
  </svg>`;
}

function iconStats() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <line x1="18" y1="20" x2="18" y2="10"/>
    <line x1="12" y1="20" x2="12" y2="4"/>
    <line x1="6" y1="20" x2="6" y2="14"/>
  </svg>`;
}

function iconQueue() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <polyline points="22 12 16 12 14 15 10 15 8 12 2 12"/>
    <path d="M5.45 5.11L2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/>
  </svg>`;
}

function iconUsers() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/>
    <circle cx="9" cy="7" r="4"/>
    <path d="M23 21v-2a4 4 0 0 0-3-3.87"/>
    <path d="M16 3.13a4 4 0 0 1 0 7.75"/>
  </svg>`;
}

function iconProfile() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/>
    <circle cx="12" cy="7" r="4"/>
  </svg>`;
}

function iconLogOut() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/>
    <polyline points="16 17 21 12 16 7"/>
    <line x1="21" y1="12" x2="9" y2="12"/>
  </svg>`;
}

function iconAPIKeys() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M21 2l-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0l3 3L22 7l-3-3m-3.5 3.5L19 4"/>
  </svg>`;
}

function iconDocs() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/>
    <polyline points="14 2 14 8 20 8"/>
    <line x1="16" y1="13" x2="8" y2="13"/>
    <line x1="16" y1="17" x2="8" y2="17"/>
    <polyline points="10 9 9 9 8 9"/>
  </svg>`;
}

function iconSettings() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/>
    <circle cx="12" cy="12" r="3"/>
  </svg>`;
}

function navHTML(active) {
  const items = [
    ["/dashboard/forms", "表单管理", "forms", iconForms()],
    ["/dashboard/channels", "邮件渠道", "channels", iconChannels()],
    ["/dashboard/submissions", "提交记录", "submissions", iconSubmissions()],
    ["/dashboard/stats", "数据统计", "stats", iconStats()],
    ["/docs", "文档中心", "docs", iconDocs()],
  ];
  
  if (isAdmin()) {
    items.splice(3, 0, ["/dashboard/users", "用户管理", "users", iconUsers()]);
    items.splice(5, 0, ["/dashboard/settings", "站点设置", "settings", iconSettings()]);
    items.splice(6, 0, ["/dashboard/queue", "邮件队列", "queue", iconQueue()]);
  }
  
  items.push(["/dashboard/profile", "账号设置", "profile", iconProfile()]);
  
  return `
    <aside class="sidebar">
      <div style="display: flex; align-items: center; gap: var(--space-sm); margin-bottom: var(--space-lg);">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 28px; height: 28px; color: var(--color-primary-light);">
          <path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72 12.84 12.84 0 0 0 .7 2.81 2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45 12.84 12.84 0 0 0 2.81.7A2 2 0 0 1 22 16.92z"/>
        </svg>
        <h2>Formail</h2>
      </div>
      ${items.map((it) => `
        <a class="nav ${active === it[2] ? "active" : ""}" href="${it[0]}">
          ${it[3]}
          ${it[1]}
        </a>
      `).join("")}
      <div style="margin-top: auto;">
        <button class="btn btn-danger" onclick="logout()" style="width: 100%; justify-content: center; gap: var(--space-sm);">
          ${iconLogOut()}
          退出登录
        </button>
      </div>
    </aside>
  `;
}

async function loadSiteSettingsForLayout() {
  try {
    const response = await fetch('/api/site-settings');
    if (response.ok) {
      const data = await response.json();
      if (data.data) {
        return data.data;
      }
    }
  } catch (e) {
    console.log('加载站点设置失败:', e);
  }
  return null;
}

async function renderLayout(active, title, contentHTML) {
  if (window.__sessionReady) await window.__sessionReady;
  const settings = await loadSiteSettingsForLayout();

  let logoHTML = `
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="width: 28px; height: 28px; color: var(--color-primary-light);">
      <path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72 12.84 12.84 0 0 0 .7 2.81 2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45 12.84 12.84 0 0 0 2.81.7A2 2 0 0 1 22 16.92z"/>
    </svg>
    <h2>${settings?.site_title || 'Formail'}</h2>
  `;

  if (settings?.logo_url) {
    logoHTML = `<img src="${settings.logo_url}" alt="Logo" style="height: 64px;" />`;
  }

  if (settings?.favicon_url) {
    document.getElementById('favicon').href = settings.favicon_url;
  }

  document.body.innerHTML = `
    <div class="app-shell">
      <div class="mobile-topbar">
        <button class="hamburger-btn" onclick="toggleSidebar()">${iconHamburger()}</button>
        <h2>${settings?.site_title || 'Formail'}</h2>
      </div>
      <div class="sidebar-overlay" id="sidebarOverlay" onclick="closeSidebar()"></div>
      <aside class="sidebar" id="sidebar">
        <div style="display: flex; align-items: center; gap: var(--space-sm); margin-bottom: var(--space-lg);">
          ${logoHTML}
        </div>
        ${navItemsHTML(active)}
        <div style="margin-top: auto;">
          <button class="btn btn-danger" onclick="logout()" style="width: 100%; justify-content: center; gap: var(--space-sm);">
            ${iconLogOut()}
            退出登录
          </button>
        </div>
      </aside>
      <main class="main">
        <div class="topbar">
          <h1>${title}</h1>
        </div>
        ${contentHTML}
      </main>
    </div>
  `;
}

function iconHamburger() {
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
    <line x1="3" y1="12" x2="21" y2="12"/>
    <line x1="3" y1="6" x2="21" y2="6"/>
    <line x1="3" y1="18" x2="21" y2="18"/>
  </svg>`;
}

function navItemsHTML(active) {
  const items = [
    ["/dashboard/forms", "表单管理", "forms", iconForms()],
    ["/dashboard/channels", "邮件渠道", "channels", iconChannels()],
    ["/dashboard/submissions", "提交记录", "submissions", iconSubmissions()],
    ["/dashboard/apikeys", "API Keys", "apikeys", iconAPIKeys()],
    ["/dashboard/stats", "数据统计", "stats", iconStats()],
    ["/docs", "文档中心", "docs", iconDocs()],
  ];

  if (isAdmin()) {
    items.splice(3, 0, ["/dashboard/users", "用户管理", "users", iconUsers()]);
    items.splice(5, 0, ["/dashboard/settings", "站点设置", "settings", iconSettings()]);
    items.splice(6, 0, ["/dashboard/queue", "邮件队列", "queue", iconQueue()]);
  }

  items.push(["/dashboard/profile", "账号设置", "profile", iconProfile()]);

  return items.map((it) => `
    <a class="nav ${active === it[2] ? "active" : ""}" href="${it[0]}">
      ${it[3]}
      ${it[1]}
    </a>
  `).join("");
}

function toggleSidebar() {
  document.getElementById('sidebar').classList.toggle('open');
  document.getElementById('sidebarOverlay').classList.toggle('open');
}

function closeSidebar() {
  document.getElementById('sidebar').classList.remove('open');
  document.getElementById('sidebarOverlay').classList.remove('open');
}