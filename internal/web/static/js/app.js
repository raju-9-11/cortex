/* ============================================================================
 * Forge Chat UI — Application Logic
 * ============================================================================
 * Vanilla JS, no frameworks. Communicates with Forge's Go backend via REST +
 * SSE streaming (POST-based, parsed with ReadableStream).
 * ========================================================================= */

'use strict';

(function () {
  // --------------------------------------------------------------------------
  // Application State
  // --------------------------------------------------------------------------
  const state = {
    sessions: [],          // Array of session list items
    currentSessionId: null, // Currently active session ID
    messages: [],          // Messages for current session
    models: [],            // Available models
    isStreaming: false,     // Whether currently streaming a response
    abortController: null, // For cancelling streams
  };

  // --------------------------------------------------------------------------
  // DOM Element Cache (populated on DOMContentLoaded)
  // --------------------------------------------------------------------------
  let els = {};

  function cacheElements() {
    els = {
      app:            document.getElementById('app'),
      sidebar:        document.getElementById('sidebar'),
      sidebarHeader:  document.getElementById('sidebar-header'),
      newChatBtn:     document.getElementById('new-chat-btn'),
      sessionList:    document.getElementById('session-list'),
      sidebarFooter:  document.getElementById('sidebar-footer'),
      main:           document.getElementById('main'),
      chatHeader:     document.getElementById('chat-header'),
      sidebarToggle:  document.getElementById('sidebar-toggle'),
      modelSelect:    document.getElementById('model-select'),
      chatTitle:      document.getElementById('chat-title'),
      messages:       document.getElementById('messages'),
      welcomeScreen:  document.getElementById('welcome-screen'),
      inputArea:      document.getElementById('input-area'),
      messageInput:   document.getElementById('message-input'),
      sendBtn:        document.getElementById('send-btn'),
    };
  }

  // --------------------------------------------------------------------------
  // Configuration
  // --------------------------------------------------------------------------
  const API_KEY_STORAGE = 'forge_api_key';

  function getApiKey() {
    return localStorage.getItem(API_KEY_STORAGE) || '';
  }

  function setApiKey(key) {
    if (key) {
      localStorage.setItem(API_KEY_STORAGE, key);
    } else {
      localStorage.removeItem(API_KEY_STORAGE);
    }
  }

  // --------------------------------------------------------------------------
  // Markdown Rendering Configuration
  // --------------------------------------------------------------------------
  function configureMarked() {
    if (typeof marked === 'undefined') return;

    marked.setOptions({
      highlight: function (code, lang) {
        if (typeof hljs === 'undefined') return code;
        if (lang && hljs.getLanguage(lang)) {
          try {
            return hljs.highlight(code, { language: lang }).value;
          } catch (_) { /* fall through */ }
        }
        try {
          return hljs.highlightAuto(code).value;
        } catch (_) {
          return code;
        }
      },
      breaks: true,
      gfm: true,
    });
  }

  function renderMarkdown(content) {
    if (!content) return '';
    if (typeof marked !== 'undefined') {
      try {
        return marked.parse(content);
      } catch (e) {
        console.error('Markdown parse error:', e);
      }
    }
    // Fallback: escape HTML and convert newlines
    return escapeHtml(content).replace(/\n/g, '<br>');
  }

  function escapeHtml(text) {
    const div = document.createElement('div');
    div.textContent = text;
    return div.innerHTML;
  }

  // --------------------------------------------------------------------------
  // Custom Error Class
  // --------------------------------------------------------------------------
  class ApiError extends Error {
    constructor(message, code, status) {
      super(message);
      this.name = 'ApiError';
      this.code = code;
      this.status = status;
    }
  }

  // --------------------------------------------------------------------------
  // API Client
  // --------------------------------------------------------------------------
  const api = {
    /** Build standard headers for JSON requests. */
    _headers(includeJson) {
      const h = {};
      if (includeJson) h['Content-Type'] = 'application/json';
      const key = getApiKey();
      if (key) h['Authorization'] = 'Bearer ' + key;
      return h;
    },

    /** Generic fetch wrapper with structured error handling. */
    async _fetch(url, opts = {}) {
      const needsBody = opts.method === 'POST' || opts.method === 'PATCH' || opts.method === 'PUT';
      opts.headers = { ...this._headers(needsBody), ...(opts.headers || {}) };

      const res = await fetch(url, opts);

      if (res.status === 401) {
        promptForApiKey();
        throw new ApiError('Authentication required', 'unauthorized', 401);
      }

      if (res.status === 204) return null;

      if (!res.ok) {
        let errBody;
        try { errBody = await res.json(); } catch (_) {
          throw new ApiError(res.statusText || 'Request failed', 'unknown', res.status);
        }
        const e = errBody?.error || {};
        throw new ApiError(e.message || res.statusText, e.code || 'unknown', res.status);
      }

      const ct = res.headers.get('content-type') || '';
      if (ct.includes('application/json')) return res.json();
      return null;
    },

    /** GET /api/sessions */
    async listSessions() {
      return this._fetch('/api/sessions');
    },

    /** POST /api/sessions */
    async createSession(title, model) {
      const body = { title: title || 'New Chat' };
      if (model) body.model = model;
      return this._fetch('/api/sessions', {
        method: 'POST',
        body: JSON.stringify(body),
      });
    },

    /** GET /api/sessions/{id} */
    async getSession(id) {
      return this._fetch('/api/sessions/' + encodeURIComponent(id));
    },

    /** PATCH /api/sessions/{id} */
    async updateSession(id, updates) {
      return this._fetch('/api/sessions/' + encodeURIComponent(id), {
        method: 'PATCH',
        body: JSON.stringify(updates),
      });
    },

    /** DELETE /api/sessions/{id} */
    async deleteSession(id) {
      return this._fetch('/api/sessions/' + encodeURIComponent(id), {
        method: 'DELETE',
      });
    },

    /**
     * POST /api/sessions/{id}/messages (streaming).
     * Returns the raw Response so the caller can read the SSE body.
     */
    async sendMessage(sessionId, content, signal) {
      const headers = this._headers(true);
      const res = await fetch(
        '/api/sessions/' + encodeURIComponent(sessionId) + '/messages',
        {
          method: 'POST',
          headers,
          body: JSON.stringify({ content, role: 'user', stream: true }),
          signal,
        },
      );

      if (res.status === 401) {
        promptForApiKey();
        throw new ApiError('Authentication required', 'unauthorized', 401);
      }

      if (!res.ok) {
        let errBody;
        try { errBody = await res.json(); } catch (_) {
          throw new ApiError(res.statusText || 'Stream request failed', 'unknown', res.status);
        }
        const e = errBody?.error || {};
        throw new ApiError(e.message || res.statusText, e.code || 'stream_failed', res.status);
      }

      return res;
    },

    /** GET /v1/models */
    async listModels() {
      return this._fetch('/v1/models');
    },
  };

  // --------------------------------------------------------------------------
  // Session Management
  // --------------------------------------------------------------------------

  async function loadSessions() {
    try {
      const data = await api.listSessions();
      state.sessions = data?.data || [];
      renderSessionList();
    } catch (err) {
      console.error('Failed to load sessions:', err);
      state.sessions = [];
      renderSessionList();
    }
  }

  function renderSessionList() {
    if (!els.sessionList) return;
    els.sessionList.innerHTML = '';

    if (state.sessions.length === 0) {
      const empty = document.createElement('div');
      empty.className = 'session-list-empty';
      empty.textContent = 'No conversations yet';
      els.sessionList.appendChild(empty);
      return;
    }

    // Most-recently-updated first
    const sorted = [...state.sessions].sort((a, b) => {
      return new Date(b.updated_at || b.created_at || 0) -
             new Date(a.updated_at || a.created_at || 0);
    });

    for (const session of sorted) {
      const item = document.createElement('div');
      item.className = 'session-item';
      if (session.id === state.currentSessionId) item.classList.add('active');
      item.dataset.id = session.id;

      const titleSpan = document.createElement('span');
      titleSpan.className = 'session-title';
      titleSpan.textContent = session.title || 'Untitled';

      const deleteBtn = document.createElement('button');
      deleteBtn.className = 'session-delete';
      deleteBtn.innerHTML = '&times;';
      deleteBtn.title = 'Delete conversation';

      item.appendChild(titleSpan);
      item.appendChild(deleteBtn);
      els.sessionList.appendChild(item);
    }
  }

  async function selectSession(id) {
    if (!id) return;

    // If we're streaming in a different session, abort first
    if (state.isStreaming && state.currentSessionId !== id) {
      abortStream();
    }

    state.currentSessionId = id;
    updateHash(id);

    // Highlight active session in sidebar
    document.querySelectorAll('.session-item').forEach(function (el) {
      el.classList.toggle('active', el.dataset.id === id);
    });

    hideWelcome();

    // Show a loading state while fetching messages
    if (els.messages) {
      els.messages.innerHTML = '<div class="loading-messages">Loading…</div>';
    }

    try {
      const data = await api.getSession(id);
      const session = data?.session || data;
      state.messages = data?.messages || [];

      if (els.chatTitle) els.chatTitle.textContent = session?.title || 'Chat';

      // Sync model selector
      if (els.modelSelect && session?.model) {
        els.modelSelect.value = session.model;
      }

      renderMessages(state.messages);
    } catch (err) {
      console.error('Failed to load session:', err);
      if (els.messages) els.messages.innerHTML = '';
      appendErrorMessage('Failed to load conversation: ' + err.message);
    }

    closeSidebarOnMobile();
  }

  async function createNewSession() {
    try {
      const model = els.modelSelect?.value || '';
      const session = await api.createSession('New Chat', model);
      state.sessions.unshift(session);
      renderSessionList();
      await selectSession(session.id);
    } catch (err) {
      console.error('Failed to create session:', err);
      appendErrorMessage('Failed to create new chat: ' + err.message);
    }
  }

  async function deleteSession(id) {
    if (!id) return;
    if (!confirm('Delete this conversation? This cannot be undone.')) return;

    try {
      await api.deleteSession(id);
      state.sessions = state.sessions.filter(function (s) { return s.id !== id; });
      renderSessionList();

      if (state.currentSessionId === id) {
        state.currentSessionId = null;
        state.messages = [];
        if (els.messages) els.messages.innerHTML = '';
        if (els.chatTitle) els.chatTitle.textContent = '';
        showWelcome();
        updateHash('');
      }
    } catch (err) {
      console.error('Failed to delete session:', err);
      appendErrorMessage('Failed to delete conversation: ' + err.message);
    }
  }

  // --------------------------------------------------------------------------
  // Message Rendering
  // --------------------------------------------------------------------------

  function renderMessages(messages) {
    if (!els.messages) return;
    els.messages.innerHTML = '';

    if (!messages || messages.length === 0) return;

    for (const msg of messages) {
      appendMessage(msg.role, msg.content, false);
    }
    scrollToBottom(false);
  }

  /**
   * Append a single message bubble to #messages.
   * Returns the created DOM element so streaming can update it in place.
   */
  function appendMessage(role, content, scroll) {
    if (scroll === undefined) scroll = true;
    if (!els.messages) return null;

    const msgEl = document.createElement('div');
    msgEl.className = 'message ' + role;

    const avatar = document.createElement('div');
    avatar.className = 'message-avatar';
    avatar.textContent = role === 'user' ? 'U' : 'F';

    const contentEl = document.createElement('div');
    contentEl.className = 'message-content';

    if (role === 'assistant') {
      contentEl.innerHTML = renderMarkdown(content);
    } else {
      contentEl.innerHTML = escapeHtml(content || '').replace(/\n/g, '<br>');
    }

    msgEl.appendChild(avatar);
    msgEl.appendChild(contentEl);
    els.messages.appendChild(msgEl);

    if (scroll) scrollToBottom();
    return msgEl;
  }

  function appendErrorMessage(text) {
    if (!els.messages) return;

    const msgEl = document.createElement('div');
    msgEl.className = 'message error';

    const contentEl = document.createElement('div');
    contentEl.className = 'message-content error-content';
    contentEl.textContent = text;

    msgEl.appendChild(contentEl);
    els.messages.appendChild(msgEl);
    scrollToBottom();
  }

  // --------------------------------------------------------------------------
  // Send Message + SSE Stream Parsing
  // --------------------------------------------------------------------------

  async function sendMessage() {
    if (!els.messageInput) return;

    const content = els.messageInput.value.trim();
    if (!content) return;
    if (state.isStreaming) return; // double-send prevention

    // Auto-create a session if none is active
    if (!state.currentSessionId) {
      await createNewSession();
      if (!state.currentSessionId) return; // creation failed
    }

    // Clear input and reset height
    els.messageInput.value = '';
    autoGrowTextarea();

    // Render user bubble immediately
    appendMessage('user', content);

    // Start streaming assistant response
    await streamResponse(state.currentSessionId, content);
  }

  /**
   * POST the user message and read the SSE response body token-by-token.
   * Uses ReadableStream because POST requests cannot use EventSource.
   */
  async function streamResponse(sessionId, content) {
    state.isStreaming = true;
    state.abortController = new AbortController();
    setInputEnabled(false);
    showTypingIndicator();

    let assistantContent = '';
    let msgEl = null;

    try {
      const response = await api.sendMessage(
        sessionId, content, state.abortController.signal,
      );

      hideTypingIndicator();

      // Prepare assistant message element for incremental updates
      msgEl = appendMessage('assistant', '');
      const contentEl = msgEl?.querySelector('.message-content');

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let isErrorEvent = false;

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop(); // keep the last (possibly incomplete) line

        for (const line of lines) {
          const trimmed = line.trim();
          if (!trimmed) continue;

          // Detect SSE event type lines (e.g. "event: error")
          if (trimmed.startsWith('event:')) {
            isErrorEvent = trimmed.slice(6).trim() === 'error';
            continue;
          }

          if (!trimmed.startsWith('data:')) continue;

          const data = trimmed.slice(5).trim();
          if (data === '[DONE]') continue;

          // Handle error events
          if (isErrorEvent) {
            try {
              const errObj = JSON.parse(data);
              appendErrorMessage(errObj?.error?.message || 'Stream error');
            } catch (_) {
              appendErrorMessage('Stream error: ' + data);
            }
            isErrorEvent = false;
            continue;
          }

          // Parse ChatCompletionChunk
          try {
            const chunk = JSON.parse(data);
            const choice = chunk.choices?.[0];
            if (choice?.finish_reason === 'stop') continue;

            const delta = choice?.delta?.content;
            if (delta) {
              assistantContent += delta;
              if (contentEl) {
                contentEl.innerHTML = renderMarkdown(assistantContent);
              }
              scrollToBottom();
            }
          } catch (e) {
            // Incomplete JSON — will be completed on next read
          }
        }
      }

      // Refresh sidebar: server may have auto-titled the session
      await loadSessions();

    } catch (err) {
      hideTypingIndicator();

      if (err.name === 'AbortError') {
        // User cancelled the stream — clean up empty bubble
        if (msgEl && !assistantContent) msgEl.remove();
      } else {
        console.error('Stream error:', err);
        appendErrorMessage('Error: ' + (err.message || 'Connection failed'));
      }
    } finally {
      state.isStreaming = false;
      state.abortController = null;
      setInputEnabled(true);
      hideTypingIndicator();
      els.messageInput?.focus();
    }
  }

  function abortStream() {
    if (state.abortController) {
      state.abortController.abort();
      state.abortController = null;
    }
    state.isStreaming = false;
    setInputEnabled(true);
    hideTypingIndicator();
  }

  // --------------------------------------------------------------------------
  // Model Management
  // --------------------------------------------------------------------------

  async function loadModels() {
    try {
      const data = await api.listModels();
      state.models = data?.data || [];
      renderModelSelect();
    } catch (err) {
      console.error('Failed to load models:', err);
      state.models = [];
      renderModelSelect();
    }
  }

  function renderModelSelect() {
    if (!els.modelSelect) return;
    els.modelSelect.innerHTML = '';

    if (state.models.length === 0) {
      const opt = document.createElement('option');
      opt.value = '';
      opt.textContent = 'No models available';
      opt.disabled = true;
      els.modelSelect.appendChild(opt);
      return;
    }

    for (const model of state.models) {
      const opt = document.createElement('option');
      opt.value = model.id;
      opt.textContent = model.id;
      els.modelSelect.appendChild(opt);
    }
  }

  async function onModelChange() {
    if (!state.currentSessionId || !els.modelSelect) return;
    const model = els.modelSelect.value;
    if (!model) return;
    try {
      await api.updateSession(state.currentSessionId, { model: model });
    } catch (err) {
      console.error('Failed to update session model:', err);
    }
  }

  // --------------------------------------------------------------------------
  // UI Helpers
  // --------------------------------------------------------------------------

  function scrollToBottom(smooth) {
    if (smooth === undefined) smooth = true;
    if (!els.messages) return;
    els.messages.scrollTo({
      top: els.messages.scrollHeight,
      behavior: smooth ? 'smooth' : 'auto',
    });
  }

  function autoGrowTextarea() {
    if (!els.messageInput) return;
    els.messageInput.style.height = 'auto';
    var maxHeight = 200;
    els.messageInput.style.height =
      Math.min(els.messageInput.scrollHeight, maxHeight) + 'px';
  }

  function toggleSidebar() {
    if (!els.sidebar) return;
    els.sidebar.classList.toggle('open');

    var overlay = document.getElementById('sidebar-overlay');
    if (overlay) {
      overlay.classList.toggle('visible', els.sidebar.classList.contains('open'));
    }
  }

  function closeSidebarOnMobile() {
    if (window.innerWidth <= 768 && els.sidebar?.classList.contains('open')) {
      toggleSidebar();
    }
  }

  function showWelcome() {
    if (els.welcomeScreen) els.welcomeScreen.classList.remove('hidden');
    if (els.messages) els.messages.classList.remove('active');
  }

  function hideWelcome() {
    if (els.welcomeScreen) els.welcomeScreen.classList.add('hidden');
    if (els.messages) els.messages.classList.add('active');
  }

  function showTypingIndicator() {
    if (!els.messages) return;
    hideTypingIndicator(); // prevent duplicates
    var indicator = document.createElement('div');
    indicator.className = 'typing-indicator';
    indicator.innerHTML = '<span></span><span></span><span></span>';
    els.messages.appendChild(indicator);
    scrollToBottom();
  }

  function hideTypingIndicator() {
    if (!els.messages) return;
    var existing = els.messages.querySelector('.typing-indicator');
    if (existing) existing.remove();
  }

  function setInputEnabled(enabled) {
    if (els.messageInput) els.messageInput.disabled = !enabled;
    if (els.sendBtn) els.sendBtn.disabled = !enabled;
  }

  // --------------------------------------------------------------------------
  // URL Hash Routing
  // --------------------------------------------------------------------------

  function updateHash(id) {
    if (id) {
      history.replaceState(null, '', '#' + id);
    } else {
      history.replaceState(null, '', window.location.pathname);
    }
  }

  function getSessionIdFromHash() {
    var hash = window.location.hash;
    return hash ? hash.slice(1) : null;
  }

  // --------------------------------------------------------------------------
  // API Key Prompt
  // --------------------------------------------------------------------------

  function promptForApiKey() {
    var key = prompt('Enter your Forge API key:');
    if (key !== null) {
      setApiKey(key.trim());
    }
  }

  // --------------------------------------------------------------------------
  // Event Listeners
  // --------------------------------------------------------------------------

  function bindEvents() {
    // New chat
    els.newChatBtn?.addEventListener('click', function () {
      createNewSession();
    });

    // Send
    els.sendBtn?.addEventListener('click', function () {
      sendMessage();
    });

    // Keyboard: Enter sends, Shift+Enter adds newline
    els.messageInput?.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    });

    // Auto-grow textarea
    els.messageInput?.addEventListener('input', autoGrowTextarea);

    // Model change
    els.modelSelect?.addEventListener('change', onModelChange);

    // Sidebar toggle
    els.sidebarToggle?.addEventListener('click', toggleSidebar);

    // Sidebar overlay — close sidebar when clicking outside on mobile
    var overlay = document.getElementById('sidebar-overlay');
    if (overlay) overlay.addEventListener('click', toggleSidebar);

    // Session list — delegated clicks
    els.sessionList?.addEventListener('click', function (e) {
      // Delete button
      var deleteBtn = e.target.closest('.session-delete');
      if (deleteBtn) {
        e.stopPropagation();
        var item = deleteBtn.closest('.session-item');
        if (item?.dataset.id) deleteSession(item.dataset.id);
        return;
      }
      // Session select
      var sessionItem = e.target.closest('.session-item');
      if (sessionItem?.dataset.id) selectSession(sessionItem.dataset.id);
    });

    // Browser back/forward
    window.addEventListener('hashchange', function () {
      var id = getSessionIdFromHash();
      if (id && id !== state.currentSessionId) {
        selectSession(id);
      } else if (!id) {
        state.currentSessionId = null;
        state.messages = [];
        if (els.messages) els.messages.innerHTML = '';
        showWelcome();
      }
    });

    // Viewport resize — keep chat scrolled to bottom
    window.addEventListener('resize', function () {
      if (state.currentSessionId) scrollToBottom(false);
    });
  }

  // --------------------------------------------------------------------------
  // Initialization
  // --------------------------------------------------------------------------

  async function init() {
    cacheElements();
    configureMarked();
    bindEvents();

    // Load models and sessions concurrently
    await Promise.all([loadModels(), loadSessions()]);

    // Restore session from URL hash if valid
    var hashId = getSessionIdFromHash();
    if (hashId && state.sessions.some(function (s) { return s.id === hashId; })) {
      await selectSession(hashId);
    } else {
      showWelcome();
    }

    els.messageInput?.focus();
  }

  // --------------------------------------------------------------------------
  // Boot
  // --------------------------------------------------------------------------
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
