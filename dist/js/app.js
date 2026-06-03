let groupConversationMap = {};
let conversationGroupMap = {};
let conversationOpenSeq = 0;
let botChatSeq = 0;
let localPinnedConversations = JSON.parse(localStorage.getItem('claran_pinned_conversations') || '{}');
let localHiddenConversations = JSON.parse(localStorage.getItem('claran_hidden_conversations') || '{}');
let voiceRecorder = null;
let voiceRecordChunks = [];
let voiceRecordStream = null;
let voiceRecordStartedAt = 0;
let voiceRecordTimer = null;
let mediaObjectURLs = {};
let mediaPayloadCache = {};
let currentMessages = [];
let pendingReplyMessage = null;
let currentConversationRecipientCount = 0;
let botCache = [];
let agentUserIDToBot = {};
let groupMembersCache = {};
let avatarLongPressTimer = null;
let pendingMentionTargets = {};
let botCreateSubmitting = false;
let agentRunHistories = {};
let agentMenuCloseTimer = null;
let pendingAgentThinkingByConversation = {};
let conversationParticipantCache = {};
let conversationGroupCollapsed = JSON.parse(localStorage.getItem('claran_conversation_group_collapsed') || '{}');
let lastOnlineSyncAt = 0;
let friendGroupCollapsed = JSON.parse(localStorage.getItem('claran_friend_group_collapsed') || '{}');
let agentContextSidebarVisible = false;
let agentNativeStateByConversation = {};
let llmProfilesCache = [];
let messageTranslations = {};
let ragDocumentsCache = [];
let ragSearchDocumentID = '';
let ragGraphCache = { nodes: [], edges: [], communities: [] };
let ragUploadJobs = {};
let ragSearchTimer = null;
let knowledgeGraphCache = { nodes: [], edges: [], communities: [], stats: {} };
let knowledgeGraphInstance = null;
let knowledgeGraphSelected = null;
let knowledgePathSelection = { sourceID: '', targetID: '', path: null };
let adminMCPTraceCache = [];
let systemNoticeCache = [];
let systemNoticeUnread = 0;
let activeWorkspace = 'chat';
const LOCAL_LOG_KEY = 'claran_frontend_logs';
const LOCAL_LOG_LIMIT = 500;

function writeLocalLog(level, message, details = null) {
    const entry = {
        time: new Date().toISOString(),
        level,
        message: String(message || ''),
        details
    };
    try {
        const logs = JSON.parse(localStorage.getItem(LOCAL_LOG_KEY) || '[]');
        logs.push(entry);
        localStorage.setItem(LOCAL_LOG_KEY, JSON.stringify(logs.slice(-LOCAL_LOG_LIMIT)));
    } catch (err) {
        console.warn('写入本地日志失败:', err);
    }
    const consoleFn = level === 'error' ? console.error : (level === 'warn' ? console.warn : console.log);
    consoleFn('[ClaranAIM]', entry.message, entry.details || '');
}

function readLocalLogs() {
    try {
        return JSON.parse(localStorage.getItem(LOCAL_LOG_KEY) || '[]');
    } catch (err) {
        return [];
    }
}

function clearClaranLogs() {
    localStorage.removeItem(LOCAL_LOG_KEY);
    showToast('本地日志已清空', 'info');
}

function downloadClaranLogs() {
    const logs = readLocalLogs();
    const lines = logs.map(item => {
        const details = item.details ? ` ${JSON.stringify(item.details)}` : '';
        return `[${item.time}] [${item.level}] ${item.message}${details}`;
    });
    const blob = new Blob([lines.join('\n') || '暂无日志'], { type: 'text/plain;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `claran-frontend-${new Date().toISOString().replace(/[:.]/g, '-')}.log`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
}

window.downloadClaranLogs = downloadClaranLogs;
window.clearClaranLogs = clearClaranLogs;

window.addEventListener('error', event => {
    writeLocalLog('error', '未捕获前端错误', {
        message: event.message,
        source: event.filename,
        line: event.lineno,
        column: event.colno,
        stack: event.error && event.error.stack
    });
});

window.addEventListener('unhandledrejection', event => {
    const reason = event.reason || {};
    writeLocalLog('error', '未处理Promise异常', {
        message: reason.message || String(reason),
        stack: reason.stack || ''
    });
});

document.addEventListener('click', event => {
    if (!event.target.closest('.agent-menu-wrapper') && !event.target.closest('.agent-item-menu')) {
        closeAgentItemMenus();
    }
    if (event.target.closest('.agent-item-menu button')) {
        setTimeout(() => closeAgentItemMenus(), 0);
    }
    const target = event.target.closest('button,[onclick],a');
    if (!target) return;
    writeLocalLog('info', '点击元素', {
        tag: target.tagName,
        text: (target.textContent || '').trim().slice(0, 80),
        id: target.id || '',
        className: target.className || '',
        onclick: target.getAttribute('onclick') || '',
    });
}, true);

document.addEventListener('keydown', event => {
    if (!event.ctrlKey || !event.shiftKey) return;
    const key = event.key.toLowerCase();
    if (key === 'l') {
        event.preventDefault();
        downloadClaranLogs();
    } else if (key === 'k') {
        event.preventDefault();
        clearClaranLogs();
    }
});

function sameID(a, b) {
    return String(a) === String(b);
}

function idString(value) {
    const text = String(value ?? '').trim();
    return text && text !== '0' ? text : '';
}

function workspaceFromHash() {
    const raw = (location.hash || '#/chat').replace(/^#\/?/, '').split('?')[0].trim();
    return raw || 'chat';
}

function workspaceTitle(name) {
    return {
        chat: '消息中心',
        agents: 'Agent 工作台',
        knowledge: '知识工作台',
        memory: '记忆中心',
        settings: '系统设置',
        admin: '系统治理台',
    }[name] || '消息中心';
}

function navigateWorkspace(name = 'chat') {
    const target = name || 'chat';
    if (location.hash !== `#/${target}`) {
        location.hash = `#/${target}`;
        return;
    }
    renderWorkspace(target);
}

function updateWorkspaceNavigation(name) {
    document.querySelectorAll('.workspace-nav-item').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.workspace === name);
    });
}

function setWorkspaceMode(name, { hideSidebar = true } = {}) {
    activeWorkspace = name;
    updateWorkspaceNavigation(name);
    const mainLayout = document.querySelector('.main-layout');
    const sidebar = document.querySelector('.sidebar');
    const content = document.querySelector('.content');
    if (mainLayout) mainLayout.dataset.workspace = name;
    if (sidebar) sidebar.style.display = hideSidebar ? 'none' : 'flex';
    if (content) content.classList.toggle('workspace-expanded', hideSidebar);
}

function activateChatWorkspaceForContent() {
    activeWorkspace = 'chat';
    updateWorkspaceNavigation('chat');
    const mainLayout = document.querySelector('.main-layout');
    const sidebar = document.querySelector('.sidebar');
    const content = document.querySelector('.content');
    if (mainLayout) mainLayout.dataset.workspace = 'chat';
    if (sidebar) sidebar.style.display = 'flex';
    if (content) content.classList.remove('workspace-expanded');
    if (location.hash !== '#/chat') {
        history.replaceState(null, '', '#/chat');
    }
}

function activateStandaloneWorkspace(name) {
    setWorkspaceMode(name, { hideSidebar: true });
    if (location.hash !== `#/${name}`) {
        history.replaceState(null, '', `#/${name}`);
    }
}

function renderWorkspaceShell(name, eyebrow, title, subtitle, actionsHTML = '') {
    const chat = document.getElementById('chat-area');
    const welcome = document.getElementById('welcome-area');
    if (chat) chat.style.display = 'none';
    if (!welcome) return null;
    welcome.style.display = 'flex';
    welcome.innerHTML = `
        <div class="workspace-page workspace-${escapeHTML(name)}">
            <header class="workspace-page-header">
                <div>
                    <span class="eyebrow">${escapeHTML(eyebrow)}</span>
                    <h2>${escapeHTML(title)}</h2>
                    <p>${escapeHTML(subtitle)}</p>
                </div>
                ${actionsHTML ? `<div class="workspace-page-actions">${actionsHTML}</div>` : ''}
            </header>
            <div id="workspace-page-body" class="workspace-page-body workspace-fade-in"></div>
        </div>
    `;
    return document.getElementById('workspace-page-body');
}

function ensureSystemNoticeBell() {
    if (!token || !currentUser) return;
    let bell = document.getElementById('system-notice-bell');
    if (!bell) {
        bell = document.createElement('button');
        bell.id = 'system-notice-bell';
        bell.className = 'system-notice-bell';
        bell.type = 'button';
        bell.title = '系统公告';
        bell.onclick = showSystemNoticePanel;
        document.body.appendChild(bell);
    }
    bell.innerHTML = `
        <span class="notice-bell-icon">!</span>
        ${systemNoticeUnread > 0 ? `<em>${systemNoticeUnread > 99 ? '99+' : systemNoticeUnread}</em>` : ''}
    `;
}

async function loadSystemNotices({ silent = true } = {}) {
    if (!token || !currentUser || typeof systemAPI === 'undefined') return;
    const resp = await systemAPI.notices({ limit: 30 });
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        if (!silent) showToast(resp?.message || resp?.data?.msg || '读取系统公告失败', 'error');
        ensureSystemNoticeBell();
        return;
    }
    systemNoticeCache = resp.data.notices || [];
    const seenKey = 'claran_seen_notice_ids';
    let seen = [];
    try { seen = JSON.parse(localStorage.getItem(seenKey) || '[]').map(String); } catch (err) { seen = []; }
    systemNoticeUnread = systemNoticeCache.filter(item => !seen.includes(String(item.id || ''))).length;
    ensureSystemNoticeBell();
}

function showSystemNoticePanel() {
    if (!systemNoticeCache.length) {
        loadSystemNotices({ silent: false }).then(() => showSystemNoticePanel());
        return;
    }
    const seenIDs = systemNoticeCache.map(item => String(item.id || '')).filter(Boolean);
    localStorage.setItem('claran_seen_notice_ids', JSON.stringify(seenIDs));
    systemNoticeUnread = 0;
    ensureSystemNoticeBell();
    showModal('系统公告', `
        <div class="system-notice-panel">
            <div class="system-notice-head">
                <strong>最近公告</strong>
                <button class="btn-small ghost" onclick="loadSystemNotices({ silent: false }).then(showSystemNoticePanel)">刷新</button>
            </div>
            ${systemNoticeCache.length ? systemNoticeCache.map(renderSystemNoticeItem).join('') : '<div class="empty-tip">暂无系统公告</div>'}
        </div>
    `);
}

function renderSystemNoticeItem(item) {
    const level = item.level || 'info';
    return `
        <article class="system-notice-item ${escapeHTML(level)}">
            <header>
                <strong>${escapeHTML(item.title || '系统公告')}</strong>
                <span>${escapeHTML(level)} · ${escapeHTML(item.created_at || '')}</span>
            </header>
            <p>${escapeHTML(item.content || '')}</p>
        </article>
    `;
}

async function renderWorkspace(name = workspaceFromHash()) {
    if (!currentUser) return;
    if (name === 'admin' && (!currentUser || currentUser.role !== 'admin')) {
        showToast('只有管理员可以打开治理台', 'warning');
        name = 'chat';
        if (location.hash !== '#/chat') {
            location.hash = '#/chat';
            return;
        }
    }
    switch (name) {
        case 'agents':
            setWorkspaceMode('agents');
            await showAgentWorkspace();
            break;
        case 'knowledge':
            setWorkspaceMode('knowledge');
            await showKnowledgeHomeWorkspace();
            break;
        case 'memory':
            setWorkspaceMode('memory');
            await showMemoryWorkspace();
            break;
        case 'settings':
            setWorkspaceMode('settings');
            await showSettingsWorkspace();
            break;
        case 'admin':
            setWorkspaceMode('admin');
            await showAdminWorkspace();
            break;
        case 'chat':
        default:
            setWorkspaceMode('chat', { hideSidebar: false });
            showChatHome();
            break;
    }
}

function entityID(entity, ...fallbackKeys) {
    if (entity === undefined || entity === null) return '';
    if (typeof entity !== 'object') return String(entity);
    const keys = ['id', 'Id', 'ID', 'job_id', 'JobId', 'trace_id', ...fallbackKeys];
    for (const key of keys) {
        const value = entity[key];
        if (value !== undefined && value !== null && value !== '') {
            return String(value);
        }
    }
    return '';
}

function escapeRegExp(value) {
    return String(value || '').replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function cssEscapeValue(value) {
    if (window.CSS && typeof window.CSS.escape === 'function') {
        return window.CSS.escape(String(value || ''));
    }
    return String(value || '').replace(/["\\]/g, '\\$&');
}

function agentRoleLabel(role) {
    const labels = {
        owner: '创建者',
        admin: '协管员',
        operator: '使用者',
        viewer: '查看者',
    };
    return labels[role] || role || '未知权限';
}

function toolPolicyLabel(policy) {
    const labels = {
        safe: '常规模式',
        approval_required: '操作前确认',
        readonly: '只读模式',
        disabled: '禁用工具',
    };
    return labels[policy] || policy || '常规模式';
}

function agentSourceLabel(type) {
    return type === 'custom' ? '自定义模型' : '系统模型';
}

function getBotDisplayName(bot) {
    if (!bot) return '智能助手';
    return bot.nickname || bot.name || bot.username || bot.display_name || `Agent ${bot.id || ''}`.trim();
}

function safeImageHTML(src, className = 'avatar-img') {
    const url = String(src || '').trim();
    if (!url) return '';
    return `<img class="${escapeHTML(className)}" src="${escapeHTML(url)}" alt="" loading="lazy" referrerpolicy="no-referrer">`;
}

function renderAvatarHTML(src, fallback = 'A', extraClass = '') {
    const content = src ? safeImageHTML(src) : escapeHTML(String(fallback || 'A').slice(0, 2).toUpperCase());
    return `<div class="avatar ${escapeHTML(extraClass)}">${content}</div>`;
}

function llmUsageLabel(type) {
    const labels = {
        translation: '翻译',
        rag_router: 'RAG 路由小模型',
        rag_answer: 'RAG 回答模型',
        agent: 'Agent',
        embedding: '向量模型',
        ocr: 'OCR 模型',
        rerank: 'Rerank 模型',
        general: '通用',
    };
    return labels[type] || type || '通用';
}

function routeTypeLabel(type) {
    const labels = {
        exact: '精确匹配',
        prefix: '前缀匹配',
        regex: '规则匹配',
        keyword: '关键词匹配',
        agent_keyword: 'Agent 关键词触发',
        agent_command: 'Agent 命令触发',
        agent_record: 'Agent 静默记录',
    };
    return labels[type] || type || '未知规则';
}

function agentActionLabel(action) {
    const labels = {
        run: '执行任务',
        summarize: '会话总结',
        ask: '上下文问答',
        insights: '提取结论',
        replyCandidates: '生成回复',
    };
    return labels[action] || action || '执行任务';
}

function agentActionHint(action) {
    const hints = {
        run: '让智能助手按你的指令完成一项具体工作。',
        summarize: '把当前会话整理成可阅读的摘要、结论和待办。',
        ask: '围绕当前会话内容提问，适合追问背景和细节。',
        insights: '提取结论、分歧、风险、待办和负责人。',
        replyCandidates: '根据上下文生成几条可直接发送的回复。',
    };
    return hints[action] || '选择一种智能助手能力后执行。';
}

function selectAgentAction(selectID, action, hintID = '') {
    const select = document.getElementById(selectID);
    if (select) select.value = action;
    document.querySelectorAll(`[data-agent-action-for="${selectID}"]`).forEach(btn => {
        btn.classList.toggle('active', btn.dataset.agentAction === action);
    });
    if (hintID) {
        const hint = document.getElementById(hintID);
        if (hint) hint.textContent = agentActionHint(action);
    }
}

function renderAgentActionGrid(selectID, hintID, selected = 'summarize') {
    const actions = [
        ['summarize', '总结', '整理上下文'],
        ['ask', '问答', '追问细节'],
        ['insights', '洞察', '结论风险'],
        ['replyCandidates', '回复', '候选话术'],
        ['run', '执行', '具体任务'],
    ];
    return `
        <div class="agent-action-grid">
            ${actions.map(([action, title, desc]) => `
                <button type="button" class="agent-action-card ${action === selected ? 'active' : ''}" data-agent-action-for="${selectID}" data-agent-action="${action}" onclick="selectAgentAction(${jsStringArg(selectID)}, ${jsStringArg(action)}, ${jsStringArg(hintID)})">
                    <span>${title}</span>
                    <small>${desc}</small>
                </button>
            `).join('')}
        </div>
        <div id="${escapeHTML(hintID)}" class="agent-action-hint">${escapeHTML(agentActionHint(selected))}</div>
    `;
}

function saveConversationGroupState() {
    localStorage.setItem('claran_conversation_group_collapsed', JSON.stringify(conversationGroupCollapsed));
}

function toggleConversationGroup(groupKey) {
    conversationGroupCollapsed[groupKey] = !conversationGroupCollapsed[groupKey];
    saveConversationGroupState();
    loadConversations();
}

function saveFriendGroupState() {
    localStorage.setItem('claran_friend_group_collapsed', JSON.stringify(friendGroupCollapsed));
}

function toggleFriendGroup(groupKey) {
    friendGroupCollapsed[groupKey] = !friendGroupCollapsed[groupKey];
    saveFriendGroupState();
    renderFriendListFromCache();
}

function conversationGroupLabel(groupKey) {
    const labels = {
        pinned: '置顶会话',
        agent: '智能助手',
        group: '群聊',
        private: '私聊',
        other: '其他会话',
    };
    return labels[groupKey] || '其他会话';
}

function conversationGroupIcon(groupKey) {
    const icons = {
        pinned: 'PIN',
        agent: 'AI',
        group: 'G',
        private: 'P',
        other: 'O',
    };
    return icons[groupKey] || 'O';
}

function friendGroupKey(friend) {
    return friend && friend.group_id ? String(friend.group_id) : 'default';
}

function classifyConversation(c) {
    if (c._is_pinned) return 'pinned';
    if (c.type === 'private' && c.participant_ids) {
        const otherID = c.participant_ids.find(id => !sameID(id, currentUser.id));
        if (otherID && isAgentUser(otherID)) return 'agent';
    }
    if (c.type === 'group') return 'group';
    if (c.type === 'private') return 'private';
    return 'other';
}

function renderConversationItem(c) {
    const unread = unreadMap[c.conversation_id] || 0;
    const isActive = sameID(currentConversationID, c.conversation_id);
    const typeLabel = c.is_deleted_group ? '已解散' : (c.type === 'private' ? '私聊' : '群聊');
    const pinnedPrefix = c._is_pinned ? '置顶 · ' : '';
    let displayName = conversationNameCache[c.conversation_id] || c.target_name;
    let otherID = '';
    if (!displayName || displayName.startsWith('用户') || displayName.startsWith('群聊#')) {
        if (c.type === 'private' && c.participant_ids) {
            otherID = c.participant_ids.find(id => !sameID(id, currentUser.id));
            if (otherID) displayName = getUserName(otherID);
        }
    } else if (c.type === 'private' && c.participant_ids) {
        otherID = c.participant_ids.find(id => !sameID(id, currentUser.id));
    }
    if (!displayName) displayName = '会话 #' + c.conversation_id;
    conversationNameCache[c.conversation_id] = displayName;

    let avatarHTML;
    let avatarClass = 'conv-avatar';
    if (c.type === 'private' && c.participant_ids) {
        if (!otherID) otherID = c.participant_ids.find(id => !sameID(id, currentUser.id));
        const agent = getAgentBotByUserID(otherID);
        if (agent) {
            avatarClass += ' agent-avatar';
            avatarHTML = agent.avatar ? safeImageHTML(agent.avatar) : 'A';
        } else if (otherID) {
            avatarHTML = getUserAvatarHTML(otherID, 'conv-avatar');
        } else {
            avatarHTML = 'P';
        }
    } else {
        avatarClass += ' group-avatar';
        avatarHTML = 'G';
    }

    return `
        <div class="list-item conversation-item ${isActive ? 'active' : ''} ${c._is_pinned ? 'pinned' : ''} ${c.is_deleted_group ? 'deleted-group' : ''}" data-conversation-id="${escapeHTML(String(c.conversation_id))}" onclick="openConversation(${jsArg(c.conversation_id)}, ${jsStringArg(c.type)}, ${c.is_deleted_group ? 'true' : 'false'})">
            <div class="avatar ${avatarClass}">${avatarHTML}</div>
            <div class="list-item-info">
                <div class="list-item-top">
                    <span class="list-item-name">${escapeHTML(displayName)}</span>
                    <span class="list-item-type">${pinnedPrefix}${typeLabel}</span>
                </div>
                <div class="list-item-msg">${escapeHTML(c.last_message || '暂无消息')}</div>
            </div>
            <button class="btn-icon-sm danger-soft" onclick="event.stopPropagation(); hideConversation(${jsArg(c.conversation_id)})" title="从列表移除">×</button>
            ${unread > 0 ? `<span class="item-unread">${unread > 99 ? '99+' : unread}</span>` : ''}
        </div>
    `;
}

function conversationOptionLabel(c) {
    if (!c) return '未知会话';
    const name = conversationNameCache[c.conversation_id] || c.target_name || `会话 #${c.conversation_id}`;
    const type = c.type === 'group' ? '群聊' : '私聊';
    return `${name} · ${type}`;
}

async function fetchAgentContextConversations(selectedID = 0) {
    const options = [{ id: '0', label: '不使用会话上下文' }];
    const resp = await messageAPI.getConversations();
    if (!(resp && resp.code === 0 && resp.data && resp.data.conversations)) {
        if (selectedID) {
            options.push({
                id: String(selectedID),
                label: conversationNameCache[selectedID] || `当前会话 #${selectedID}`,
            });
        }
        return options;
    }
    const convs = resp.data.conversations.filter(c =>
        c && c.conversation_id &&
        !isConversationHidden(c.conversation_id) &&
        (c.type !== 'group' || (c.group_id && String(c.group_id) !== '0'))
    );
    const ids = [];
    convs.forEach(c => {
        if (c.last_sender_id) ids.push(c.last_sender_id);
        if (c.participant_ids) {
            conversationParticipantCache[String(c.conversation_id)] = c.participant_ids;
            c.participant_ids.forEach(pid => ids.push(pid));
        }
    });
    if (ids.length > 0) await resolveUserNames([...new Set(ids)]);
    convs.forEach(c => {
        if (c.type === 'private' && c.participant_ids) {
            const otherID = c.participant_ids.find(id => !sameID(id, currentUser.id));
            if (otherID && !conversationNameCache[c.conversation_id]) {
                conversationNameCache[c.conversation_id] = getUserName(otherID);
            }
        }
        if (c.target_name && !conversationNameCache[c.conversation_id]) {
            conversationNameCache[c.conversation_id] = c.target_name;
        }
        if (c.group_id && c.group_id > 0) {
            groupConversationMap[c.group_id] = c.conversation_id;
            conversationGroupMap[c.conversation_id] = c.group_id;
            const group = groupsCache.find(g => sameID(g.id, c.group_id));
            if (group && group.name) conversationNameCache[c.conversation_id] = group.name;
        }
        options.push({ id: String(c.conversation_id), label: conversationOptionLabel(c) });
    });
    if (selectedID && !options.some(o => sameID(o.id, selectedID))) {
        options.push({
            id: String(selectedID),
            label: conversationNameCache[selectedID] || `当前会话 #${selectedID}`,
        });
    }
    return options;
}

function jsArg(value) {
    return JSON.stringify(String(value ?? '')).replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

function jsStringArg(value) {
    return jsArg(value);
}

function parseIDList(value) {
    return (value || '')
        .split(',')
        .map(s => s.trim())
        .filter(s => /^\d+$/.test(s));
}

function getPinnedKey(conversationID) {
    return currentUser && currentUser.id ? `${currentUser.id}:${conversationID}` : String(conversationID);
}

function isConversationPinned(conversationID) {
    return !!localPinnedConversations[getPinnedKey(conversationID)];
}

function setConversationPinned(conversationID, pinned) {
    const key = getPinnedKey(conversationID);
    if (pinned) {
        localPinnedConversations[key] = true;
    } else {
        delete localPinnedConversations[key];
    }
    localStorage.setItem('claran_pinned_conversations', JSON.stringify(localPinnedConversations));
}

function isConversationHidden(conversationID) {
    return !!localHiddenConversations[getPinnedKey(conversationID)];
}

function setConversationHidden(conversationID, hidden) {
    const key = getPinnedKey(conversationID);
    if (hidden) {
        localHiddenConversations[key] = true;
    } else {
        delete localHiddenConversations[key];
    }
    localStorage.setItem('claran_hidden_conversations', JSON.stringify(localHiddenConversations));
}

function makeMediaPayload(url, id, name) {
    const payload = {
        url: url || '',
        id: id || '',
        file_id: id || '',
        name: name || '',
    };
    return JSON.stringify(payload);
}

function makeMediaPayloadWithMeta(url, id, name, file = null) {
    const payload = {
        url: url || '',
        id: id || '',
        file_id: id || '',
        name: name || '',
        content_type: file?.type || '',
        size: file?.size || 0,
    };
    return JSON.stringify(payload);
}

function parseMediaPayload(content, tag) {
    const match = (content || '').match(new RegExp(`\\[${tag}\\]([\\s\\S]*?)\\[\\/${tag}\\]`));
    const raw = match ? match[1] : (content || '');
    if (raw.trim().startsWith('{')) {
        try {
            const parsed = JSON.parse(raw);
            return {
                url: parsed.url || '',
                id: parsed.id || parsed.file_id || '',
                name: parsed.name || parsed.file_name || parsed.url?.split('/').pop() || '文件',
                content_type: parsed.content_type || '',
                size: parsed.size || 0,
            };
        } catch (err) {
            console.warn('媒体消息JSON解析失败:', err);
        }
    }
    const parts = raw.split('|');
    if (parts.length >= 3) {
        return {
            url: decodeURIComponent(parts[0] || ''),
            id: decodeURIComponent(parts[1] || ''),
            name: decodeURIComponent(parts.slice(2).join('|') || '')
        };
    }
    return {
        url: raw,
        id: '',
        name: raw.split('/').pop() || '文件'
    };
}

function stripMediaTags(content) {
    return String(content || '')
        .replace(/\[(img|voice|file)\]([\s\S]*?)\[\/\1\]/g, (_, type, raw) => {
            const media = parseMediaPayload(`[${type}]${raw}[/${type}]`, type);
            return `[${type === 'img' ? '图片' : type === 'voice' ? '语音' : '文件'}] ${media.name || media.url || ''}`;
        })
        .replace(/<[^>]+>/g, ' ');
}

function resolveMediaURL(media) {
    if (!media) return '';
    if (media.id) return fileAPI.previewURL(media.id);
    if (media.url && media.url.startsWith('/')) return `${API_ORIGIN}${media.url}`;
    if (media.url) return media.url;
    return media.url || '';
}


function resolveDownloadURL(media) {
    if (!media) return '';
    if (media.id) return fileAPI.downloadURL(media.id);
    return resolveMediaURL(media);
}

function mediaKey(media) {
    return media && media.id ? media.id : (media && media.url ? media.url : '');
}

function rememberMedia(media) {
    const key = mediaKey(media);
    if (key) {
        mediaPayloadCache[key] = media;
    }
    return key;
}

function publicMediaURL(media) {
    if (!media || !media.url) return '';
    if (media.url.startsWith('/files/')) return `${API_ORIGIN}${media.url}`;
    return '';
}

async function loadAuthedMedia(el, media) {
    const key = mediaKey(media);
    if (!key || !media || !media.id) return;
    const fallbackURL = publicMediaURL(media);
    if (mediaObjectURLs[key]) {
        el.src = mediaObjectURLs[key];
        return;
    }
    try {
        const blob = await fileAPI.fetchBlob(media.id);
        const objectURL = URL.createObjectURL(blob);
        mediaObjectURLs[key] = objectURL;
        el.src = objectURL;
    } catch (err) {
        if (fallbackURL) {
            el.src = fallbackURL;
            return;
        }
        if (el.tagName === 'IMG') {
            el.outerHTML = '<span class="media-error">图片加载失败</span>';
        } else {
            showToast(err.message || '媒体加载失败', 'error');
        }
    }
}

async function downloadMedia(fileID, filename, mediaKeyValue = '') {
    const media = mediaPayloadCache[mediaKeyValue] || mediaPayloadCache[fileID] || null;
    try {
        const blob = await fileAPI.fetchBlob(fileID);
        const objectURL = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = objectURL;
        a.download = filename || 'download';
        document.body.appendChild(a);
        a.click();
        a.remove();
        setTimeout(() => URL.revokeObjectURL(objectURL), 3000);
    } catch (err) {
        const fallbackURL = publicMediaURL(media);
        if (fallbackURL) {
            const a = document.createElement('a');
            a.href = fallbackURL;
            a.download = filename || 'download';
            a.target = '_blank';
            document.body.appendChild(a);
            a.click();
            a.remove();
            return;
        }
        showToast(err.message || '文件下载失败', 'error');
    }
}

function hydrateMedia(container) {
    container.querySelectorAll('[data-media-id]').forEach(el => {
        const media = {
            id: el.dataset.mediaId,
            url: el.dataset.mediaUrl || '',
            name: el.dataset.mediaName || ''
        };
        const fallbackURL = publicMediaURL(media);
        if (fallbackURL && el.src) {
            return;
        }
        loadAuthedMedia(el, media);
    });
}

function formatRecordDuration(ms) {
    const totalSeconds = Math.max(0, Math.floor(ms / 1000));
    const minutes = String(Math.floor(totalSeconds / 60)).padStart(2, '0');
    const seconds = String(totalSeconds % 60).padStart(2, '0');
    return `${minutes}:${seconds}`;
}

function getMessageByID(messageID) {
    return currentMessages.find(m => sameID(m.id || m.msg_id, messageID));
}

function renderCurrentMessages(scrollToBottom = true) {
    const msgList = document.getElementById('message-list');
    if (currentConversationID) {
        currentMessages = currentMessages.filter(m => !m.conversation_id || sameID(m.conversation_id, currentConversationID));
    }
    msgList.innerHTML = currentMessages.map(m => createMessageHTML(m)).join('');
    hydrateMedia(msgList);
    if (scrollToBottom) {
        msgList.scrollTop = msgList.scrollHeight;
    }
    renderAgentNativeStatus();
    renderAgentContextSidebar();
}

function setAgentNativeState(conversationID, status, detail = '') {
    if (!conversationID) return;
    agentNativeStateByConversation[String(conversationID)] = {
        status,
        detail,
        updatedAt: Date.now(),
    };
    if (sameID(currentConversationID, conversationID)) {
        renderAgentNativeStatus();
        renderAgentContextSidebar();
    }
}

function agentNativeStatusLabel(status) {
    const labels = {
        thinking: '思考中',
        completed: '已完成',
        waiting: '等待确认',
        silent: '静默记录',
        failed: '执行失败',
        blocked: '策略拦截',
        idle: '可用',
    };
    return labels[status] || '可用';
}

function renderAgentNativeStatus() {
    const bar = document.getElementById('agent-native-status');
    if (!bar) return;
    if (currentBotID !== null || !currentConversationID || document.getElementById('chat-area')?.style.display === 'none') {
        bar.style.display = 'none';
        bar.innerHTML = '';
        return;
    }
    const state = agentNativeStateByConversation[String(currentConversationID || '')];
    const agents = getCurrentConversationAgents();
    if (!state && agents.length === 0) {
        bar.style.display = 'none';
        bar.innerHTML = '';
        return;
    }
    const status = state?.status || 'idle';
    const agentNames = agents.length ? agents.map(getBotDisplayName).join('、') : 'Agent';
    const detail = state?.detail || (agents.length ? '当前会话中有 Agent 成员，可私聊、@ 或通过规则触发。' : '当前会话可使用 Agent 上下文工具。');
    bar.className = `agent-native-status ${status}`;
    bar.style.display = 'flex';
    bar.innerHTML = `
        <span class="agent-native-dot"></span>
        <strong>${escapeHTML(agentNativeStatusLabel(status))}</strong>
        <span>${escapeHTML(agentNames)}</span>
        <em>${escapeHTML(detail)}</em>
        <button type="button" onclick="toggleAgentContextSidebar(true)">查看上下文</button>
    `;
}

function getCurrentConversationAgents() {
    const participants = conversationParticipantCache[String(currentConversationID || '')] || [];
    return participants.map(id => getAgentBotByUserID(id)).filter(Boolean);
}

function toggleAgentContextSidebar(forceOpen = null) {
    agentContextSidebarVisible = forceOpen === null ? !agentContextSidebarVisible : !!forceOpen;
    renderAgentContextSidebar();
}

function summarizeVisibleMessagesForSidebar() {
    const recent = currentMessages.slice(-10).filter(m => m && (m.content || m.msg_type));
    if (!recent.length) return '<div class="agent-context-empty">暂无可分析消息</div>';
    return recent.map(m => {
        const agent = getAgentBotByUserID(m.sender_id);
        const name = agent ? getBotDisplayName(agent) : getUserName(m.sender_id);
        const content = stripMediaTags(m.content || `[${m.msg_type || '消息'}]`).replace(/\s+/g, ' ').slice(0, 110);
        return `
            <li>
                <span class="agent-context-avatar">${escapeHTML((name || '?').slice(0, 1).toUpperCase())}</span>
                <div><strong>${escapeHTML(name)}</strong><p>${escapeHTML(content)}</p></div>
            </li>
        `;
    }).join('');
}

function renderAgentContextSidebar() {
    const side = document.getElementById('agent-context-sidebar');
    if (!side) return;
    if (!agentContextSidebarVisible || !currentConversationID) {
        side.style.display = 'none';
        side.innerHTML = '';
        return;
    }
    const agents = getCurrentConversationAgents();
    const state = agentNativeStateByConversation[String(currentConversationID || '')] || { status: 'idle', detail: '等待事件触发或人工运行。' };
    const statusLabel = agentNativeStatusLabel(state.status);
    side.style.display = 'flex';
    side.innerHTML = `
        <div class="agent-context-head">
            <div>
                <span class="eyebrow">Context</span>
                <strong>会话感知</strong>
            </div>
            <button type="button" aria-label="关闭上下文侧栏" onclick="toggleAgentContextSidebar(false)">×</button>
        </div>
        <section class="agent-context-status ${escapeHTML(state.status || 'idle')}">
            <div class="agent-context-status-dot"></div>
            <div>
                <label>原生状态</label>
                <p><strong>${escapeHTML(statusLabel)}</strong><span>${escapeHTML(state.detail || '当前没有正在运行的 Agent 任务')}</span></p>
            </div>
        </section>
        <section>
            <label>会话 Agent</label>
            <div class="agent-context-chips">
                ${agents.length ? agents.map(b => `<span>${escapeHTML(getBotDisplayName(b))}</span>`).join('') : '<em>当前会话暂无 Agent 成员</em>'}
            </div>
        </section>
        <section>
            <label>最近上下文</label>
            <ul class="agent-context-message-list">${summarizeVisibleMessagesForSidebar()}</ul>
        </section>
        <section>
            <label>可执行动作</label>
            <div class="agent-context-actions">
                <button type="button" onclick="showAgentConversationTools()">总结/问答</button>
                <button type="button" onclick="showMentionPicker()">@ Agent</button>
                <button type="button" onclick="showMemoryManager('candidates')">候选记忆</button>
            </div>
        </section>
    `;
}

function setActiveConversationHighlight(conversationID) {
    document.querySelectorAll('#conversation-list .list-item').forEach(item => {
        item.classList.toggle('active', sameID(item.dataset.conversationId, conversationID));
    });
}

function clearPendingReply() {
    pendingReplyMessage = null;
    const bar = document.getElementById('reply-preview-bar');
    if (bar) bar.remove();
}

function setPendingReply(messageID) {
    const msg = getMessageByID(messageID);
    if (!msg) return;
    pendingReplyMessage = msg;
    let bar = document.getElementById('reply-preview-bar');
    if (!bar) {
        bar = document.createElement('div');
        bar.id = 'reply-preview-bar';
        bar.className = 'reply-preview-bar';
        const input = document.querySelector('.chat-input');
        input.parentNode.insertBefore(bar, input);
    }
    bar.innerHTML = `
        <div>
            <strong>回复 ${escapeHTML(getUserName(msg.sender_id))}</strong>
            <span>${escapeHTML((msg.content || '[媒体消息]').slice(0, 80))}</span>
        </div>
        <button type="button" onclick="clearPendingReply()">取消</button>
    `;
    document.getElementById('msg-input')?.focus();
}

function extractMentions(content) {
    const ids = [];
    const re = /@\[(?:[^\]]+)\]\((\d+)\)|@(\d+)/g;
    let match;
    while ((match = re.exec(content || '')) !== null) {
        const id = match[1] || match[2];
        if (!sameID(id, currentUser.id)) ids.push(id);
    }
    const pendingKey = String(currentConversationID || '');
    (pendingMentionTargets[pendingKey] || []).forEach(item => {
        const namePattern = new RegExp(`(^|\\s)@${escapeRegExp(item.name)}(?=\\s|$)`);
        if (namePattern.test(content || '') && !sameID(item.id, currentUser.id)) {
            ids.push(item.id);
        }
    });
    return [...new Set(ids)];
}

function insertMention(userID, displayName) {
    const input = document.getElementById('msg-input');
    if (!input) return;
    if (sameID(userID, currentUser.id)) {
        showToast('不能 @ 自己', 'warning');
        return;
    }
    const name = displayName || ('用户' + userID);
    const token = `@${name} `;
    const start = input.selectionStart || input.value.length;
    const end = input.selectionEnd || input.value.length;
    input.value = input.value.slice(0, start) + token + input.value.slice(end);
    const pendingKey = String(currentConversationID || '');
    pendingMentionTargets[pendingKey] = (pendingMentionTargets[pendingKey] || []).filter(item => !sameID(item.id, userID));
    pendingMentionTargets[pendingKey].push({ id: String(userID), name });
    input.focus();
    input.selectionStart = input.selectionEnd = start + token.length;
    closeModal();
}

function startAvatarMentionPress(event, userID, displayName) {
    if (event.button !== undefined && event.button !== 0) return;
    clearTimeout(avatarLongPressTimer);
    avatarLongPressTimer = setTimeout(() => {
        avatarLongPressTimer = null;
        mentionFromAvatar(userID, displayName);
    }, 520);
}

function cancelAvatarMentionPress() {
    if (avatarLongPressTimer) {
        clearTimeout(avatarLongPressTimer);
        avatarLongPressTimer = null;
    }
}

function mentionFromAvatar(userID, displayName) {
    cancelAvatarMentionPress();
    if (!currentConversationID || currentConversationType !== 'group') {
        showToast('只有群聊里可以 @ 成员', 'warning');
        return;
    }
    if (!userID || sameID(userID, currentUser.id)) return;
    insertMention(userID, displayName || getUserName(userID));
    showToast(`已插入 @${displayName || getUserName(userID)}`, 'success');
}

async function showMentionPicker() {
    if (!currentConversationID || currentConversationType !== 'group') {
        showToast('@ 仅支持群聊', 'warning');
        return;
    }
    const groupID = conversationGroupMap[currentConversationID];
    if (!groupID) {
        showToast('无法获取当前群组', 'warning');
        return;
    }
    const members = await getGroupMembersCached(groupID, true);
    if (!members.length) {
        showToast('当前没有可 @ 的成员', 'warning');
        return;
    }
    const mentionableMembers = members.filter(m => !sameID(m.user_id, currentUser.id));
    showModal('@ 成员', `
        <div class="mention-grid">
            <button class="mention-item mention-all" onclick="insertRawMentionAll()">所有人</button>
            ${mentionableMembers.map(m => {
                const userID = m.user_id;
                const agent = getAgentBotByUserID(userID);
                const name = agent ? getBotDisplayName(agent) : getUserName(userID);
                const avatar = agent && agent.avatar ? renderAvatarHTML(agent.avatar, 'A', 'small agent-avatar') : `<div class="avatar small">${getUserAvatarHTML(userID, 'small')}</div>`;
                return `
                    <button class="mention-item" onclick="insertMention(${jsArg(userID)}, ${jsStringArg(name)})">
                        ${avatar}
                        <span>${escapeHTML(name)}</span>
                        ${agent ? '<em>智能助手</em>' : ''}
                    </button>
                `;
            }).join('')}
        </div>
    `);
}

function insertRawMentionAll() {
    const input = document.getElementById('msg-input');
    if (!input) return;
    const start = input.selectionStart || input.value.length;
    input.value = input.value.slice(0, start) + '@所有人 ' + input.value.slice(input.selectionEnd || start);
    input.focus();
    closeModal();
}

function applyMessageStateUpdate(data) {
    const messageID = data.id || data.msg_id;
    const idx = currentMessages.findIndex(m => sameID(m.id || m.msg_id, messageID));
    if (idx >= 0) {
        currentMessages[idx] = {
            ...currentMessages[idx],
            ...data,
            id: currentMessages[idx].id || data.msg_id,
            content: data.content !== undefined ? data.content : currentMessages[idx].content,
            status: data.status !== undefined ? data.status : currentMessages[idx].status,
            is_edited: data.is_edited !== undefined ? data.is_edited : currentMessages[idx].is_edited,
            edited_at: data.edited_at !== undefined ? data.edited_at : currentMessages[idx].edited_at,
        };
        renderCurrentMessages();
    }
}

async function updateMessageReadReceipt(data) {
    if (!data) return;
    if (sameID(data.conversation_id, currentConversationID)) {
        const resp = await messageAPI.getHistory(currentConversationID);
        if (resp && resp.code === 0 && resp.data && resp.data.messages) {
            currentMessages = resp.data.messages;
            renderCurrentMessages();
        }
    }
    loadConversations();
}

function readReceiptHTML(m) {
    if (!currentUser || !sameID(m.sender_id, currentUser.id) || !(m.id || m.msg_id)) return '';
    const readCount = Number(m.read_count || 0);
    const recipientCount = Number(m.recipient_count || currentConversationRecipientCount || 0);
    if (recipientCount <= 0) return '';
    if (currentConversationType === 'group') {
        return `<span class="message-read-state ${readCount > 0 ? 'read' : 'unread'}">已读 ${readCount}/${recipientCount}</span>`;
    }
    const read = readCount >= recipientCount;
    return `<span class="message-read-state ${read ? 'read' : 'unread'}">${read ? '已读' : '未读'}</span>`;
}

async function updateCurrentRecipientCount(conversationID) {
    currentConversationRecipientCount = 0;
    const resp = await messageAPI.getConversations();
    if (!resp || resp.code !== 0 || !resp.data || !resp.data.conversations) return;
    const conv = resp.data.conversations.find(c => sameID(c.conversation_id, conversationID));
    if (conv && conv.participant_ids) {
        conversationParticipantCache[String(conversationID)] = conv.participant_ids;
        currentConversationRecipientCount = Math.max(0, conv.participant_ids.filter(id => !sameID(id, currentUser.id)).length);
    }
}

function getVoiceMimeType() {
    if (!window.MediaRecorder) return '';
    const candidates = [
        'audio/webm;codecs=opus',
        'audio/webm',
        'audio/mp4',
        'audio/ogg;codecs=opus'
    ];
    return candidates.find(type => MediaRecorder.isTypeSupported(type)) || '';
}

function setVoiceRecordingUI(isRecording) {
    const btn = document.getElementById('voice-record-btn');
    const panel = document.getElementById('voice-record-panel');
    if (btn) btn.classList.toggle('recording', isRecording);
    if (panel) panel.style.display = isRecording ? 'flex' : 'none';
}

function updateVoiceRecordTime() {
    const timeEl = document.getElementById('voice-record-time');
    if (timeEl) {
        timeEl.textContent = formatRecordDuration(Date.now() - voiceRecordStartedAt);
    }
}

function switchAuthTab(tab) {
    document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
    document.querySelectorAll('.auth-form').forEach(f => f.classList.remove('active'));
    if (tab === 'login') {
        document.querySelectorAll('.tab-btn')[0].classList.add('active');
        document.getElementById('login-form').classList.add('active');
    } else {
        document.querySelectorAll('.tab-btn')[1].classList.add('active');
        document.getElementById('register-form').classList.add('active');
    }
}

function switchSidebar(panel, btn) {
    document.querySelectorAll('.sidebar-tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.sidebar-panel').forEach(p => p.classList.remove('active'));
    if (btn) btn.classList.add('active');
    document.getElementById(`${panel}-panel`).classList.add('active');
    if (panel === 'conversations') loadConversations();
    if (panel === 'friends') loadFriends();
    if (panel === 'groups') loadGroups();
    if (panel === 'bots') loadBotSidebar();
    if (panel === 'knowledge') loadKnowledgeSidebar();
}

function showToast(msg, type = 'info') {
    const container = document.getElementById('toast-container');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    const icons = { success: '✓', error: '✗', warning: '⚠', info: 'ℹ' };
    toast.innerHTML = `<span class="toast-icon">${icons[type] || 'ℹ'}</span><span>${msg}</span>`;
    container.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('toast-exit');
        setTimeout(() => toast.remove(), 300);
    }, 2500);
}

function updateAdminConsoleEntry() {
    const entry = document.getElementById('admin-console-entry');
    const navEntry = document.getElementById('admin-workspace-nav');
    const isAdmin = currentUser && currentUser.role === 'admin';
    if (entry) {
        entry.style.display = '';
        entry.classList.toggle('disabled-soft', !isAdmin);
        entry.title = isAdmin ? '打开系统治理台' : '当前账号不是管理员，只能由 admin 角色进入治理台';
    }
    if (navEntry) {
        navEntry.style.display = '';
        navEntry.classList.toggle('disabled-soft', !isAdmin);
        navEntry.title = isAdmin ? '治理台' : '当前账号不是管理员，只能由 admin 角色进入治理台';
    }
}

function showChatHome() {
    const chat = document.getElementById('chat-area');
    const welcome = document.getElementById('welcome-area');
    if (currentConversationID || currentBotID) {
        if (welcome) welcome.style.display = 'none';
        if (chat) chat.style.display = 'flex';
        return;
    }
    if (chat) chat.style.display = 'none';
    if (welcome) {
        welcome.style.display = 'flex';
        welcome.innerHTML = `
            <div class="welcome-card">
                <div class="welcome-orbit" aria-hidden="true">
                    <span></span><span></span><span></span>
                    <strong>AIM</strong>
                </div>
                <span class="eyebrow">Agent Native IM</span>
                <h2>消息、Agent 和知识在同一个工作流里</h2>
                <p>从左侧打开会话，或切换到上方工作台管理 Agent、知识库、记忆和系统配置。</p>
                <div class="workspace-quick-grid">
                    <button onclick="navigateWorkspace('agents')"><strong>Agent 工作台</strong><span>配置、运行、授权与 Skill</span></button>
                    <button onclick="navigateWorkspace('knowledge')"><strong>知识工作台</strong><span>RAG、GraphRAG 和联网检索</span></button>
                    <button onclick="navigateWorkspace('memory')"><strong>记忆中心</strong><span>长期记忆与候选确认</span></button>
                </div>
            </div>
        `;
    }
}

async function showAgentWorkspace() {
    const body = renderWorkspaceShell(
        'agents',
        'Agent Operations',
        'Agent 工作台',
        '集中管理智能助手、运行任务、Skill、权限、触发规则和运行成本。',
        `
            <button class="btn-secondary" onclick="showSkillManager()">Skill 中心</button>
            <button class="btn-secondary" onclick="showMemoryManager('candidates')">候选记忆</button>
            <button class="btn-primary" onclick="showCreateBotForm()">创建 Agent</button>
        `
    );
    if (!body) return;
    body.innerHTML = `
        <section class="workspace-hero-grid">
            <button class="workspace-command-card" onclick="showCreateBotForm()">
                <span>新建</span><strong>创建一个可入群、可私聊的 Agent</strong><small>绑定模型、工作目录和工具策略</small>
            </button>
            <button class="workspace-command-card" onclick="showSkillManager()">
                <span>Skill</span><strong>维护 Agent 工作方法</strong><small>上传、编辑、摘要和注入 Skill</small>
            </button>
            <button class="workspace-command-card" onclick="showConversationIntelligencePanel()">
                <span>归档</span><strong>从聊天中提炼摘要和候选记忆</strong><small>生成 summary / decision / task / topic</small>
            </button>
        </section>
        <section class="workspace-section">
            <div class="workspace-section-head">
                <div><h3>我的 Agent</h3><p>点击卡片可进入独立对话，使用“管理”打开配置和权限。</p></div>
                <button class="btn-small ghost" onclick="refreshAgentWorkspaceList()">刷新</button>
            </div>
            <div id="agent-workspace-list" class="agent-workspace-grid"><div class="empty-tip">加载中...</div></div>
        </section>
        <section class="workspace-section">
            <div class="workspace-section-head">
                <div><h3>待确认动作</h3><p>Agent 高风险动作会先进入确认流。</p></div>
                <button class="btn-small ghost" onclick="loadAgentWorkspaceApprovals()">刷新</button>
            </div>
            <div id="agent-workspace-approvals" class="data-list"><div class="empty-tip">加载中...</div></div>
        </section>
    `;
    await refreshAgentWorkspaceList();
    await loadAgentWorkspaceApprovals();
}

async function showSkillWorkspace() {
    const body = renderWorkspaceShell(
        'skills',
        'Agent Skills',
        'Skill 中心',
        '上传、编辑、摘要和注入 Agent 工作方法。Skill 会以文件夹形式保存，可用于全局或单个 Agent。',
        `
            <button class="btn-secondary" onclick="navigateWorkspace('agents')">返回 Agent</button>
            <button class="btn-primary" onclick="uploadGlobalSkill()">上传 Skill</button>
        `
    );
    if (!body) return;
    body.innerHTML = `
        <section class="workspace-section skill-workspace-panel">
            <div class="workspace-section-head">
                <div><h3>上传 Skill 包</h3><p>支持单个 SKILL.md、zip 或浏览器文件夹上传。上传后会提取摘要，并可在创建或编辑 Agent 时注入。</p></div>
            </div>
            <div class="skill-upload-board">
                <div class="profile-form-grid">
                    <div class="form-group">
                        <label>Skill 名称</label>
                        <input id="setting-skill-name" type="text" placeholder="例如：代码审查 / 资料总结">
                    </div>
                    <div class="form-group">
                        <label>说明</label>
                        <input id="setting-skill-desc" type="text" placeholder="这个 Skill 会给 Agent 增加什么能力">
                    </div>
                </div>
                <div class="profile-form-grid">
                    <label class="skill-drop-zone">
                        <span>上传 SKILL.md 或 zip</span>
                        <small>适合单文件或打包后的 Skill</small>
                        <input id="setting-skill-file" type="file" accept=".md,.zip">
                    </label>
                    <label class="skill-drop-zone">
                        <span>上传 Skill 文件夹</span>
                        <small>保留子文件结构，入口为 SKILL.md</small>
                        <input id="setting-skill-folder" type="file" webkitdirectory directory multiple>
                    </label>
                </div>
                <label class="checkbox-row"><input id="setting-skill-default" type="checkbox"><span>设为默认全局 Skill</span></label>
            </div>
        </section>
        <section class="workspace-section">
            <div class="workspace-section-head">
                <div><h3>全局 Skill</h3><p>这些 Skill 可被多个 Agent 复用，也可以复制目录给运行时检查。</p></div>
                <button class="btn-small ghost" onclick="loadSkillManagerList()">刷新</button>
            </div>
            <div id="global-skill-list" class="data-list skill-list-expanded">加载中...</div>
        </section>
    `;
    await loadSkillManagerList();
}

async function refreshAgentWorkspaceList() {
    const list = document.getElementById('agent-workspace-list');
    if (!list) return;
    list.innerHTML = '<div class="empty-tip">加载 Agent...</div>';
    const resp = await agentAPI.list();
    if (!(resp && resp.code === 0 && resp.data?.bots)) {
        list.innerHTML = `<div class="empty-tip">Agent 列表不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    botCache = resp.data.bots || [];
    agentUserIDToBot = {};
    botCache.forEach(bot => {
        if (bot.agent_user_id) {
            agentUserIDToBot[String(bot.agent_user_id)] = bot;
            userNickCache[bot.agent_user_id] = getBotDisplayName(bot);
            if (bot.avatar) userAvatarCache[bot.agent_user_id] = bot.avatar;
        }
    });
    if (!botCache.length) {
        list.innerHTML = '<div class="empty-tip">暂无 Agent<br><small>先创建一个项目助手或会话助手。</small></div>';
        return;
    }
    list.innerHTML = botCache.map(renderAgentWorkspaceCard).join('');
    const sidebarList = document.getElementById('bot-list');
    if (sidebarList) loadBotSidebar();
}

function renderAgentWorkspaceCard(bot) {
    const name = getBotDisplayName(bot);
    return `
        <article class="agent-workspace-card">
            <div class="agent-card-top">
                ${renderAvatarHTML(bot.avatar, 'A', 'agent-avatar')}
                <div>
                    <strong>${escapeHTML(name)}</strong>
                    <span>${escapeHTML(agentSourceLabel(bot.type))} · UID ${escapeHTML(String(bot.agent_user_id || '未绑定'))}</span>
                </div>
                <em class="${bot.is_active ? 'is-on' : 'is-off'}">${bot.is_active ? '可用' : '停用'}</em>
            </div>
            <p>${escapeHTML(bot.description || bot.signature || '这个 Agent 还没有说明。')}</p>
            <div class="agent-card-meta">
                <span>${escapeHTML(bot.model_name || '默认模型')}</span>
                <span>${escapeHTML(toolPolicyLabel(bot.tool_policy || 'safe'))}</span>
                <span>${escapeHTML(bot.workspace_root || '默认工作目录')}</span>
            </div>
            <div class="agent-card-actions">
                <button class="btn-small" onclick="showAgentRunModal(${jsArg(bot.id)}, ${jsStringArg(name)})">运行</button>
                <button class="btn-small ghost" onclick="chatWithBot(${jsArg(bot.id)})">对话</button>
                <button class="btn-small ghost" onclick="showEditAgentForm(${jsArg(bot.id)})">配置</button>
                <button class="btn-small ghost" onclick="showAgentPermissions(${jsArg(bot.id)}, ${jsStringArg(name)})">权限</button>
            </div>
        </article>
    `;
}

async function loadAgentWorkspaceApprovals() {
    const area = document.getElementById('agent-workspace-approvals');
    if (!area) return;
    const resp = await agentAPI.listApprovals();
    if (!(resp && resp.code === 0)) {
        area.innerHTML = `<div class="empty-tip">审批列表不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const approvals = resp.data?.approvals || [];
    area.innerHTML = approvals.length ? approvals.slice(0, 6).map(item => `
        <div class="data-row">
            <div class="data-row-main">
                <strong>${escapeHTML(item.title || item.action || 'Agent 审批')}</strong>
                <span>${escapeHTML(item.status || 'pending')} · ${escapeHTML(item.created_at || '')}</span>
            </div>
            <div class="data-row-actions">
                <button class="btn-small" onclick="agentAPI.confirmApproval(${jsArg(item.id)}, '前端确认').then(loadAgentWorkspaceApprovals)">通过</button>
                <button class="btn-small danger-soft" onclick="agentAPI.rejectApproval(${jsArg(item.id)}).then(loadAgentWorkspaceApprovals)">拒绝</button>
            </div>
        </div>
    `).join('') : '<div class="empty-tip">暂无待确认动作</div>';
}

async function showKnowledgeHomeWorkspace() {
    const body = renderWorkspaceShell(
        'knowledge',
        'Knowledge Systems',
        '知识工作台',
        '管理 RAG 文档、GraphRAG 图谱、联网搜索和知识候选审核。',
        `
            <button class="btn-secondary" onclick="showWebSearchPanel()">联网搜索</button>
            <button class="btn-secondary" onclick="showKnowledgeGraphWorkspace()">知识图谱</button>
            <button class="btn-primary" onclick="showRAGWorkspace('ingest')">录入知识</button>
        `
    );
    if (!body) return;
    body.innerHTML = `
        <section class="workspace-hero-grid">
            <button class="workspace-command-card" onclick="showRAGWorkspace('search')">
                <span>RAG</span><strong>检索项目知识</strong><small>Hybrid Search、Rerank、CRAG 与 Self-RAG</small>
            </button>
            <button class="workspace-command-card" onclick="showKnowledgeGraphWorkspace()">
                <span>Graph</span><strong>查看知识图谱</strong><small>实体、关系、社区和证据来源</small>
            </button>
            <button class="workspace-command-card" onclick="showWebSearchPanel()">
                <span>Web</span><strong>一次性联网增强</strong><small>搜索、抓正文、清洗相关段落</small>
            </button>
        </section>
        <section class="workspace-section">
            <div class="workspace-section-head">
                <div><h3>最近知识文档</h3><p>上传 txt、Markdown、PDF、docx、图片和代码文件后会出现在这里。</p></div>
                <button class="btn-small ghost" onclick="loadKnowledgeWorkspaceDocuments()">刷新</button>
            </div>
            <div id="knowledge-workspace-docs" class="data-list"><div class="empty-tip">加载中...</div></div>
        </section>
    `;
    await loadKnowledgeWorkspaceDocuments();
}

async function loadKnowledgeWorkspaceDocuments() {
    const area = document.getElementById('knowledge-workspace-docs');
    if (!area) return;
    const resp = await ragAPI.documents(30, 0);
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        area.innerHTML = `<div class="empty-tip">知识文档不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const docs = resp.data.documents || [];
    ragDocumentsCache = docs;
    area.innerHTML = docs.length ? docs.map(doc => `
        <div class="data-row">
            <div class="data-row-main">
                <strong>${escapeHTML(doc.title || doc.source || '知识文档')}</strong>
                <span>${escapeHTML(doc.source || '')}</span>
            </div>
            <div class="data-row-meta">
                <span>${escapeHTML(doc.visibility || 'private')}</span>
                <span>子区块 ${Number(doc.chunk_count || 0)}</span>
                <span>${escapeHTML(doc.created_at || '')}</span>
            </div>
            <div class="data-row-actions">
                <button class="btn-small ghost" onclick="showRAGWorkspace('search', '', ${jsArg(doc.id || 0)})">检索</button>
                <button class="btn-small ghost" onclick="showKnowledgeGraphWorkspace('', ${jsArg(doc.id || 0)})">看图谱</button>
                <button class="btn-small ghost" onclick="deleteRAGDocumentGraph(${jsArg(doc.id || 0)}, ${jsStringArg(doc.title || doc.source || '')})">删图谱</button>
                <button class="btn-small danger-soft" onclick="deleteRAGDocument(${jsArg(doc.id || 0)}, ${jsStringArg(doc.title || doc.source || '')})">删除</button>
            </div>
        </div>
    `).join('') : '<div class="empty-tip">暂无知识文档</div>';
}

async function deleteRAGDocumentGraph(documentID, title = '') {
    if (!documentID) return;
    if (!confirm(`只删除“${title || documentID}”的知识图谱？文档和 RAG 分块会保留。`)) return;
    const resp = await ragAPI.deleteDocumentGraph(documentID);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('文档图谱已删除', 'success');
        await loadKnowledgeWorkspaceDocuments();
        if (document.getElementById('knowledge-graph-canvas')) {
            await loadKnowledgeGraph();
        }
    } else {
        showToast(resp?.message || resp?.data?.msg || '删除文档图谱失败', 'error');
    }
}

async function deleteRAGDocument(documentID, title = '') {
    if (!documentID) return;
    if (!confirm(`确定删除“${title || documentID}”？这会删除文档、分块和该文档贡献的知识图谱。`)) return;
    const resp = await ragAPI.deleteDocument(documentID);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('知识文档已删除', 'success');
        ragDocumentsCache = ragDocumentsCache.filter(doc => !sameID(doc.id, documentID));
        if (sameID(ragSearchDocumentID, documentID)) ragSearchDocumentID = '';
        await loadKnowledgeWorkspaceDocuments();
        if (document.getElementById('knowledge-graph-canvas')) {
            await syncKnowledgeDocumentOptions(0);
            await loadKnowledgeGraph();
        }
    } else {
        showToast(resp?.message || resp?.data?.msg || '删除知识文档失败', 'error');
    }
}

async function showMemoryWorkspace() {
    const body = renderWorkspaceShell(
        'memory',
        'Long-term Context',
        '记忆中心',
        '查看、确认、编辑和关闭长期记忆，让 Agent 只使用真正有价值的上下文。',
        `
            <button class="btn-secondary" onclick="showConversationIntelligencePanel()">会话归档</button>
            <button class="btn-primary" onclick="showCreateMemoryForm()">新增记忆</button>
        `
    );
    if (!body) return;
    body.innerHTML = `
        <div class="settings-tabs workspace-tabs">
            <button id="memory-tab-facts" class="btn-small" onclick="switchMemoryManagerTab('facts')">正式记忆</button>
            <button id="memory-tab-candidates" class="btn-small" onclick="switchMemoryManagerTab('candidates')">候选记忆</button>
        </div>
        <section id="memory-panel-facts" class="memory-panel workspace-section">
            <div class="memory-toolbar">
                <select id="memory-bot-filter" class="form-select">
                    <option value="">全部归属</option>
                    <option value="0">系统 / IM 原生</option>
                    ${botCache.map(b => `<option value="${escapeHTML(String(b.id))}">${escapeHTML(getBotDisplayName(b))}</option>`).join('')}
                </select>
                <select id="memory-scope-filter" class="form-select">
                    <option value="">全部范围</option>
                    <option value="user">个人记忆</option>
                    <option value="group">群画像</option>
                    <option value="conversation">会话记忆</option>
                    <option value="session">本次会话</option>
                </select>
                <button class="btn-small" onclick="loadMemoryList()">刷新</button>
                <button class="btn-small" onclick="showCreateMemoryForm()">+ 新增</button>
            </div>
            <div id="memory-list" class="memory-list">加载中...</div>
        </section>
        <section id="memory-panel-candidates" class="memory-panel workspace-section" style="display:none;">
            <div class="memory-toolbar">
                <select id="memory-candidate-bot-filter" class="form-select">
                    <option value="">全部归属</option>
                    <option value="0">系统 / IM 原生</option>
                    ${botCache.map(b => `<option value="${escapeHTML(String(b.id))}">${escapeHTML(getBotDisplayName(b))}</option>`).join('')}
                </select>
                <select id="memory-candidate-status-filter" class="form-select">
                    <option value="pending">待确认</option>
                    <option value="accepted">已接受</option>
                    <option value="rejected">已拒绝</option>
                    <option value="">全部状态</option>
                </select>
                <button class="btn-small" onclick="loadMemoryCandidates()">刷新</button>
                <button class="btn-small" onclick="showCreateMemoryCandidateForm()">+ 新增候选</button>
            </div>
            <div id="memory-candidate-list" class="memory-list">加载中...</div>
        </section>
    `;
    await switchMemoryManagerTab(showMemoryWorkspace.defaultTab || 'facts');
    showMemoryWorkspace.defaultTab = 'facts';
}

async function showSettingsWorkspace() {
    const body = renderWorkspaceShell(
        'settings',
        'Configuration',
        '系统设置',
        '配置 LLM 预设、Prompt 模板、远程 MCP Server 和工具接入。',
        '<button class="btn-primary" onclick="renderLLMSettings()">新增模型预设</button>'
    );
    if (!body) return;
    body.innerHTML = `
        <div class="settings-tabs workspace-tabs">
            <button class="btn-small active" data-settings-tab="llm" onclick="renderLLMSettings()">LLM 预设</button>
            <button class="btn-small" data-settings-tab="prompt" onclick="renderPromptSettings()">Prompt</button>
            <button class="btn-small" data-settings-tab="mcp" onclick="renderMCPSettings()">MCP 工具</button>
        </div>
        <div id="settings-content" class="settings-content workspace-section">加载中...</div>
    `;
    await renderLLMSettings();
}

let modalStack = [];

function showModal(title, bodyHTML) {
    closeAgentItemMenus();
    const overlay = document.getElementById('modal-overlay');
    if (overlay.style.display === 'flex') {
        modalStack.push({
            title: document.getElementById('modal-title').textContent,
            body: document.getElementById('modal-body').innerHTML
        });
    }
    document.getElementById('modal-title').textContent = title;
    const body = document.getElementById('modal-body');
    body.innerHTML = bodyHTML;
    body.scrollTop = 0;
    overlay.style.display = 'flex';
}

function closeModal() {
    if (modalStack.length > 0) {
        const prev = modalStack.pop();
        document.getElementById('modal-title').textContent = prev.title;
        const body = document.getElementById('modal-body');
        body.innerHTML = prev.body;
        body.scrollTop = 0;
    } else {
        document.getElementById('modal-overlay').style.display = 'none';
    }
}

async function login() {
    const username = document.getElementById('login-username').value.trim();
    const password = document.getElementById('login-password').value.trim();
    if (!username || !password) {
        showToast('请输入用户名和密码', 'warning');
        return;
    }
    const resp = await userAPI.login(username, password);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        const accessToken = resp.data.access_token || resp.data.token || '';
        saveAuthTokens(accessToken, resp.data.refresh_token || '');
        currentUser = { id: resp.data.user_id, username, role: resp.data.role || 'user' };
        userNickCache[currentUser.id] = username;
        localStorage.setItem('claran_user', JSON.stringify(currentUser));
        showToast('登录成功', 'success');
        enterMainPage();
    } else {
        showToast(resp?.data?.msg || '登录失败', 'error');
    }
}

async function register() {
    const username = document.getElementById('reg-username').value.trim();
    const password = document.getElementById('reg-password').value.trim();
    const nickname = document.getElementById('reg-nickname').value.trim();
    if (!username || !password) {
        showToast('请输入用户名和密码', 'warning');
        return;
    }
    const resp = await userAPI.register(username, password, nickname);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('注册成功，请登录', 'success');
        switchAuthTab('login');
    } else {
        showToast(resp?.data?.msg || '注册失败', 'error');
    }
}

async function logout() {
    await userAPI.logout();
    clearAuthTokens();
    currentUser = null;
    currentConversationID = null;
    currentConversationType = '';
    unreadMap = {};
    groupConversationMap = {};
    conversationGroupMap = {};
    friendsCache = [];
    groupsCache = [];
    botCache = [];
    agentUserIDToBot = {};
    groupMembersCache = {};
    userNickCache = {};
    conversationNameCache = {};
    friendRemarkCache = {};
    localStorage.removeItem('claran_user');
    localStorage.removeItem('claran_unread');
    modalStack = [];
    currentBotID = null;
    botChatHistory = {};
    if (ws) {
        ws.close();
        ws = null;
    }
    if (wsReconnectTimer) {
        clearTimeout(wsReconnectTimer);
        wsReconnectTimer = null;
    }
    const noticeBell = document.getElementById('system-notice-bell');
    if (noticeBell) noticeBell.remove();
    systemNoticeCache = [];
    systemNoticeUnread = 0;
    document.getElementById('user-status').textContent = '○ 离线';
    document.getElementById('user-status').className = 'user-status offline';
    updateAdminConsoleEntry();
    document.getElementById('auth-page').classList.add('active');
    document.getElementById('main-page').classList.remove('active');
    showToast('已退出登录', 'info');
}

async function enterMainPage() {
    document.getElementById('auth-page').classList.remove('active');
    document.getElementById('main-page').classList.add('active');

    document.getElementById('welcome-area').style.display = 'flex';
    document.getElementById('chat-area').style.display = 'none';
    currentConversationID = null;
    currentConversationType = '';

    const resp = await userAPI.getInfo();
    if (resp && resp.code === 0 && resp.data && resp.data.user) {
        currentUser = resp.data.user;
        userNickCache[currentUser.id] = currentUser.nickname || currentUser.username;
        localStorage.setItem('claran_user', JSON.stringify(currentUser));
        document.getElementById('user-name').textContent = currentUser.nickname || currentUser.username;
        const avatarEl = document.getElementById('user-avatar');
        if (currentUser.avatar) {
            avatarEl.innerHTML = safeImageHTML(currentUser.avatar);
        } else {
            avatarEl.textContent = (currentUser.nickname || currentUser.username).charAt(0).toUpperCase();
        }
        document.getElementById('user-status').textContent = '● 在线';
        document.getElementById('user-status').className = 'user-status online';
    }
    updateAdminConsoleEntry();

    updateUnreadBadge();
    await loadGroups();
    await refreshAgentCache();
    loadConversations();
    loadFriends();
    connectWS();
    syncAfterOnline();
    loadSystemNotices();
    await renderWorkspace(workspaceFromHash());
}

async function syncAfterOnline() {
    if (!currentUser || !currentUser.id) return;
    const now = Date.now();
    if (now - lastOnlineSyncAt < 3000) return;
    lastOnlineSyncAt = now;
    try {
        const [syncResp, offlineResp, unreadResp] = await Promise.all([
            messageAPI.sync(30),
            messageAPI.offline(),
            messageAPI.unreadCount(),
        ]);
        if (syncResp && syncResp.code === 0 && syncResp.data) {
            applySyncPayload(syncResp.data);
        }
        const offlineMessages = offlineResp?.data?.messages || offlineResp?.data?.Messages || [];
        if (Array.isArray(offlineMessages) && offlineMessages.length) {
            const ids = offlineMessages.map(m => m.message_id || m.MessageId).filter(Boolean);
            if (ids.length) {
                await messageAPI.markOfflineRead(ids);
            }
        }
        const count = unreadResp?.data?.count || unreadResp?.data?.Count || 0;
        renderSyncStatus(count);
        await loadConversations();
    } catch (err) {
        console.warn('上线同步失败:', err);
        renderSyncStatus(0, '同步失败，稍后会自动重试');
    }
}

function applySyncPayload(payload) {
    const convs = payload.conversations || payload.Conversations || [];
    if (Array.isArray(convs)) {
        convs.forEach(c => {
            if (!c) return;
            if (c.participant_ids) conversationParticipantCache[String(c.conversation_id)] = c.participant_ids;
            if (c.target_name) conversationNameCache[c.conversation_id] = c.target_name;
            if (c.group_id) {
                conversationGroupMap[c.conversation_id] = c.group_id;
                groupConversationMap[c.group_id] = c.conversation_id;
            }
        });
    }
    const windows = payload.windows || payload.Windows || [];
    windows.forEach(win => {
        if (!win || !win.success) return;
        const conversationID = win.conversation_id || win.ConversationId;
        const messages = win.messages || win.Messages || [];
        cacheMessages(conversationID, messages);
        if (sameID(conversationID, currentConversationID) && Array.isArray(messages) && messages.length) {
            currentMessages = mergeMessagesByIdentity(currentMessages, messages);
            renderCurrentMessages(false);
        }
    });
}

function mergeMessagesByIdentity(existing = [], incoming = []) {
    const byIdentity = new Map();
    [...existing, ...incoming].forEach(raw => {
        if (!raw) return;
        const msg = { ...raw };
        if (msg.msg_id && !msg.id) msg.id = msg.msg_id;
        const identity = messageIdentity(msg) || `tmp:${msg.created_at || ''}:${msg.sender_id || ''}:${msg.content || ''}`;
        byIdentity.set(identity, { ...(byIdentity.get(identity) || {}), ...msg });
    });
    return Array.from(byIdentity.values()).sort((a, b) => Number(a.id || 0) - Number(b.id || 0));
}

function renderSyncStatus(unreadCount = 0, text = '') {
    const badge = document.getElementById('sync-status-badge');
    if (!badge) return;
    const message = text || (unreadCount > 0 ? `已同步 ${unreadCount} 条离线消息` : '已完成上线同步');
    badge.textContent = message;
    badge.classList.add('visible');
    clearTimeout(renderSyncStatus._timer);
    renderSyncStatus._timer = setTimeout(() => badge.classList.remove('visible'), 3600);
}

async function refreshAgentCache() {
    const resp = await agentAPI.list();
    if (!(resp && resp.code === 0 && resp.data && resp.data.bots)) return;
    botCache = resp.data.bots;
    agentUserIDToBot = {};
    botCache.forEach(bot => {
        if (bot.agent_user_id) {
            agentUserIDToBot[String(bot.agent_user_id)] = bot;
            userNickCache[bot.agent_user_id] = getBotDisplayName(bot);
            if (bot.avatar) userAvatarCache[bot.agent_user_id] = bot.avatar;
        }
    });
}

async function loadConversations() {
    const resp = await messageAPI.getConversations();
    const list = document.getElementById('conversation-list');

    if (resp && resp.code === 0 && resp.data && resp.data.conversations) {
        const convs = resp.data.conversations.filter(c =>
            !isConversationHidden(c.conversation_id) &&
            (c.type !== 'group' || (c.group_id && String(c.group_id) !== '0'))
        );
        if (convs.length === 0) {
            list.innerHTML = '<div class="empty-tip">暂无会话<br><small>点击好友列表的「聊天」或群组的「进入」开始对话</small></div>';
            return;
        }

        const senderIDs = [];
        convs.forEach(c => {
            if (c.last_sender_id) {
                senderIDs.push(c.last_sender_id);
            }
            if (c.participant_ids) {
                conversationParticipantCache[String(c.conversation_id)] = c.participant_ids;
                c.participant_ids.forEach(pid => {
                    senderIDs.push(pid);
                });
            }
        });
        if (senderIDs.length > 0) {
            await resolveUserNames([...new Set(senderIDs)]);
        }

        convs.forEach(c => {
            if (c.group_id && c.group_id > 0) {
                groupConversationMap[c.group_id] = c.conversation_id;
                conversationGroupMap[c.conversation_id] = c.group_id;
                const group = groupsCache.find(g => sameID(g.id, c.group_id));
                if (group && group.name) {
                    conversationNameCache[c.conversation_id] = group.name;
                }
                if (group && group.is_pinned) {
                    c._is_pinned = true;
                }
            }
            if (c.type === 'private' && c.participant_ids) {
                const otherID = c.participant_ids.find(id => !sameID(id, currentUser.id));
                if (otherID) {
                    const name = getUserName(otherID);
                    if (name && name !== '用户' + otherID) {
                        conversationNameCache[c.conversation_id] = name;
                    }
                }
            }
            if (c.target_name && !conversationNameCache[c.conversation_id]) {
                conversationNameCache[c.conversation_id] = c.target_name;
            }
            if (isConversationPinned(c.conversation_id)) {
                c._is_pinned = true;
            }
        });

        const groups = { pinned: [], agent: [], private: [], group: [], other: [] };
        convs.forEach(c => groups[classifyConversation(c)].push(c));
        const order = ['pinned', 'agent', 'private', 'group', 'other'];
        list.innerHTML = order
            .filter(key => groups[key].length > 0)
            .map(key => {
                const collapsed = !!conversationGroupCollapsed[key];
                const unreadTotal = groups[key].reduce((sum, item) => sum + Number(unreadMap[item.conversation_id] || 0), 0);
                return `
                    <section class="conversation-section ${collapsed ? 'collapsed' : ''}">
                        <button class="conversation-section-header" onclick="toggleConversationGroup(${jsStringArg(key)})">
                            <span class="conversation-section-icon">${conversationGroupIcon(key)}</span>
                            <span>${conversationGroupLabel(key)}</span>
                            <span class="conversation-section-count">${groups[key].length}</span>
                            ${unreadTotal > 0 ? `<span class="conversation-section-unread">${unreadTotal > 99 ? '99+' : unreadTotal}</span>` : ''}
                            <span class="conversation-section-caret">${collapsed ? '+' : '-'}</span>
                        </button>
                        <div class="conversation-section-body">
                            ${collapsed ? '' : groups[key].map(renderConversationItem).join('')}
                        </div>
                    </section>
                `;
            }).join('');
    } else {
        list.innerHTML = '<div class="empty-tip">暂无会话</div>';
    }
}

async function loadKnowledgeSidebar() {
    const list = document.getElementById('knowledge-list');
    if (!list) return;
    list.innerHTML = '<div class="empty-tip">加载知识库...</div>';
    const resp = await ragAPI.documents(10, 0);
    if (!(resp && resp.code === 0 && resp.data && resp.data.success)) {
        list.innerHTML = `<div class="empty-tip">知识库不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '请确认 rag-service 已启动')}</small></div>`;
        return;
    }
    ragDocumentsCache = resp.data.documents || [];
    if (!ragDocumentsCache.length) {
        list.innerHTML = '<div class="empty-tip">暂无知识<br><small>录入文档后可以做 Hybrid Search 和 GraphRAG。</small></div>';
        return;
    }
    list.innerHTML = ragDocumentsCache.map(doc => `
        <div class="list-item knowledge-item" onclick="showRAGWorkspace('search', '', ${jsArg(doc.id || 0)})">
            <div class="avatar knowledge-avatar">K</div>
            <div class="list-item-info">
                <div class="list-item-top">
                    <span class="list-item-name">${escapeHTML(doc.title || '未命名知识')}</span>
                    <span class="list-item-type">${escapeHTML(ragVisibilityLabel(doc.visibility))}</span>
                </div>
                <div class="list-item-msg">${escapeHTML(doc.source || doc.source_type || '文本知识')} · ${Number(doc.chunk_count || 0)}块</div>
            </div>
        </div>
    `).join('');
}

function ragVisibilityLabel(value) {
    return {
        private: '仅自己',
        public: '公共',
        shared: '共享',
    }[value] || value || '仅自己';
}

function ragRouteLabel(value) {
    return {
        adaptive: '自适应',
        hybrid: '混合检索',
        document: '文档检索',
        graphrag: '知识图谱',
        direct: '直接回答',
    }[value] || value || '自适应';
}

function responsePayload(resp) {
    if (!(resp && resp.code === 0)) return null;
    if (resp.data && typeof resp.data === 'object' && resp.data.data && typeof resp.data.data === 'object') {
        return resp.data.data;
    }
    return resp.data || null;
}

function responseOK(resp) {
    const data = responsePayload(resp);
    return !!(resp && resp.code === 0 && data && data.success !== false && data.Success !== false);
}

function valueOfAny(obj, ...keys) {
    if (!obj || typeof obj !== 'object') return undefined;
    for (const key of keys) {
        if (Object.prototype.hasOwnProperty.call(obj, key) && obj[key] !== undefined && obj[key] !== null) {
            return obj[key];
        }
    }
    return undefined;
}

function formatElapsed(ms) {
    const total = Math.max(0, Math.floor(Number(ms || 0) / 1000));
    if (total < 60) return `${total}s`;
    const min = Math.floor(total / 60);
    const sec = total % 60;
    return `${min}m ${String(sec).padStart(2, '0')}s`;
}

function shortTime(ts) {
    if (!ts) return '';
    const date = new Date(ts);
    if (Number.isNaN(date.getTime())) return '';
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

function uploadJobID(data) {
    const job = data?.job || data || {};
    return String(job.id || data?.job_id || data?.id || '').trim();
}

function normalizeRAGUploadData(data, patch = {}) {
    const base = data && typeof data === 'object' ? { ...data } : {};
    const job = { ...(base.job || base || {}), ...(patch.job || {}) };
    const files = Array.isArray(job.files) ? job.files : (Array.isArray(base.files) ? base.files : []);
    const id = String(job.id || base.job_id || base.id || patch.job_id || '').trim();
    if (id) {
        job.id = job.id || id;
        base.job_id = base.job_id || id;
    }
    job.files = files;
    job.status = patch.status || job.status || base.status || 'processing';
    job.total = Number(job.total ?? base.total ?? files.length ?? 0);
    job.completed = Number(job.completed ?? base.completed ?? files.filter(f => f.status === 'completed' || f.success).length);
    job.failed = Number(job.failed ?? base.failed ?? files.filter(f => f.status === 'failed').length);
    base.job = job;
    base.files = files;
    base.status = job.status;
    base.poll_error = patch.poll_error ?? base.poll_error;
    base.poll_attempt = patch.poll_attempt ?? base.poll_attempt;
    base.upload_percent = patch.upload_percent ?? base.upload_percent;
    base.upload_loaded = patch.upload_loaded ?? base.upload_loaded;
    base.upload_total = patch.upload_total ?? base.upload_total;
    base.started_at = patch.started_at || base.started_at || Date.now();
    base.updated_at_ms = patch.updated_at_ms || Date.now();
    return base;
}

function persistRAGUploadJobs() {
    const compact = {};
    Object.entries(ragUploadJobs || {}).forEach(([id, job]) => {
        if (String(id).startsWith('upload-')) return;
        const normalized = normalizeRAGUploadData(job);
        compact[id] = {
            id,
            status: normalized.status || normalized.job?.status || 'processing',
            updated_at: normalized.updated_at_ms || Date.now(),
            data: normalized,
        };
    });
    localStorage.setItem('claran_rag_upload_jobs', JSON.stringify(compact));
}

function restoreRAGUploadJobs() {
    try {
        const saved = JSON.parse(localStorage.getItem('claran_rag_upload_jobs') || '{}');
        Object.entries(saved).forEach(([id, item]) => {
            if (!id || Date.now() - Number(item.updated_at || 0) > 24 * 3600 * 1000) return;
            ragUploadJobs[id] = item.data || item;
        });
    } catch (err) {
        ragUploadJobs = {};
    }
    renderRAGUploadBubble();
    Object.entries(ragUploadJobs || {}).forEach(([id, data]) => {
        if (String(id).startsWith('upload-')) return;
        const status = data?.job?.status || data?.status || 'processing';
        if (status !== 'completed' && status !== 'failed') {
            pollRAGUploadJob(id, null, 0);
        }
    });
}

function upsertRAGUploadJob(data) {
    const incoming = normalizeRAGUploadData(data);
    const incomingID = uploadJobID(incoming);
    const existing = incomingID ? ragUploadJobs[incomingID] : null;
    const normalized = existing
        ? normalizeRAGUploadData({ ...incoming, started_at: existing.started_at || incoming.started_at }, { poll_error: incoming.poll_error || '' })
        : incoming;
    const id = uploadJobID(normalized);
    if (!id || id === '0') return;
    ragUploadJobs[id] = normalized;
    persistRAGUploadJobs();
    renderRAGUploadBubble();
}

function patchRAGUploadJob(jobID, patch = {}) {
    const id = String(jobID || '').trim();
    if (!id) return;
    const current = ragUploadJobs[id] || { job: { id, status: 'processing', files: [] }, job_id: id };
    ragUploadJobs[id] = normalizeRAGUploadData(current, patch);
    persistRAGUploadJobs();
    renderRAGUploadBubble();
}

function dismissRAGUploadJob(jobID) {
    delete ragUploadJobs[String(jobID)];
    persistRAGUploadJobs();
    renderRAGUploadBubble();
}

function renderRAGUploadBubble() {
    let bubble = document.getElementById('rag-upload-bubble');
    const jobs = Object.entries(ragUploadJobs || {});
    if (!jobs.length) {
        if (bubble) bubble.remove();
        return;
    }
    if (!bubble) {
        bubble = document.createElement('div');
        bubble.id = 'rag-upload-bubble';
        document.body.appendChild(bubble);
    }
    applyRAGUploadBubblePosition(bubble);
    const active = jobs.filter(([, data]) => {
        const status = data?.job?.status || data?.status;
        return status !== 'completed' && status !== 'failed' && status !== 'stalled';
    }).length;
    bubble.innerHTML = `
        <div class="rag-upload-bubble-head" data-rag-bubble-drag>
            <strong>知识入库</strong>
            <span>${active ? `${active} 个处理中` : '任务已完成'}</span>
        </div>
        <div class="rag-upload-bubble-list">
            ${jobs.slice(-4).map(([id, data]) => {
                const job = data?.job || data || {};
                const files = job.files || data?.files || [];
                const status = job.status || data?.status || 'processing';
                const total = job.total ?? files.length;
                const completed = job.completed ?? files.filter(f => f.status === 'completed' || f.success).length;
                const failed = job.failed ?? files.filter(f => f.status === 'failed').length;
                const progress = status === 'uploading'
                    ? Math.max(0, Math.min(99, Math.round(Number(data.upload_percent || 0))))
                    : (total > 0 ? Math.min(100, Math.max(0, Math.round(((completed + failed) / total) * 100))) : (status === 'completed' ? 100 : 0));
                const elapsed = data?.started_at ? formatElapsed(Date.now() - Number(data.started_at || Date.now())) : '';
                return `
                    <div class="rag-upload-bubble-item ${escapeHTML(status)}">
                        <button class="rag-upload-bubble-main" onclick="showRAGWorkspace('ingest')">
                            <span>${escapeHTML(ragUploadStatusLabel(status))}${elapsed ? ` · ${escapeHTML(elapsed)}` : ''}</span>
                            <strong>${completed}/${total || 1}</strong>
                            <i style="width:${progress}%"></i>
                        </button>
                        ${(status === 'completed' || status === 'failed' || status === 'stalled') ? `<button class="rag-upload-bubble-close" onclick="dismissRAGUploadJob(${jsStringArg(id)})">×</button>` : ''}
                    </div>
                `;
            }).join('')}
        </div>
    `;
    bindRAGUploadBubbleDrag(bubble);
}

function applyRAGUploadBubblePosition(bubble) {
    try {
        const pos = JSON.parse(localStorage.getItem('claran_rag_upload_bubble_pos') || 'null');
        if (pos && Number.isFinite(Number(pos.left)) && Number.isFinite(Number(pos.top))) {
            bubble.style.left = `${Math.max(8, Math.min(window.innerWidth - 260, Number(pos.left)))}px`;
            bubble.style.top = `${Math.max(8, Math.min(window.innerHeight - 120, Number(pos.top)))}px`;
            bubble.style.right = 'auto';
            bubble.style.bottom = 'auto';
        }
    } catch (err) {
        // 位置恢复失败不影响上传任务展示。
    }
}

function bindRAGUploadBubbleDrag(bubble) {
    const handle = bubble.querySelector('[data-rag-bubble-drag]');
    if (!handle || handle.dataset.bound === 'true') return;
    handle.dataset.bound = 'true';
    let start = null;
    handle.addEventListener('click', event => {
        if (start?.dragged) return;
        showRAGWorkspace('ingest');
    });
    handle.addEventListener('pointerdown', event => {
        if (event.button !== 0) return;
        const rect = bubble.getBoundingClientRect();
        start = { x: event.clientX, y: event.clientY, left: rect.left, top: rect.top, dragged: false };
        handle.setPointerCapture?.(event.pointerId);
    });
    handle.addEventListener('pointermove', event => {
        if (!start) return;
        const dx = event.clientX - start.x;
        const dy = event.clientY - start.y;
        if (Math.abs(dx) + Math.abs(dy) > 4) start.dragged = true;
        const left = Math.max(8, Math.min(window.innerWidth - bubble.offsetWidth - 8, start.left + dx));
        const top = Math.max(8, Math.min(window.innerHeight - bubble.offsetHeight - 8, start.top + dy));
        bubble.style.left = `${left}px`;
        bubble.style.top = `${top}px`;
        bubble.style.right = 'auto';
        bubble.style.bottom = 'auto';
    });
    const finish = () => {
        if (!start) return;
        const rect = bubble.getBoundingClientRect();
        localStorage.setItem('claran_rag_upload_bubble_pos', JSON.stringify({ left: rect.left, top: rect.top }));
        setTimeout(() => { start = null; }, 0);
    };
    handle.addEventListener('pointerup', finish);
    handle.addEventListener('pointercancel', finish);
}

function cragLabel(value) {
    return {
        use_internal: '内部知识可用',
        web_fallback: '建议 Web 兜底',
        merge_internal_and_web: '内部 + Web 合并',
        correct: '内部资料充分',
        incorrect: '需要外部兜底',
        ambiguous: '内部 + 外部合并',
        skip_vector: '跳过向量检索',
    }[value] || value || '未判断';
}

async function showRAGWorkspace(defaultTab = 'search', seedQuery = '', seedDocumentID = 0) {
    ragSearchDocumentID = idString(seedDocumentID);
    activateStandaloneWorkspace('knowledge');
    document.getElementById('chat-area').style.display = 'none';
    const welcome = document.getElementById('welcome-area');
    welcome.style.display = 'flex';
    welcome.innerHTML = `
        <div class="rag-workspace">
            <header class="rag-header">
                <div>
                    <h2>知识库与 GraphRAG</h2>
                    <p>把知识写入 RAG 服务，使用 Hybrid Search、CRAG、自检和知识图谱辅助 Agent 工作。</p>
                </div>
                <div class="rag-tabs">
                    <button class="ghost" onclick="showKnowledgeHomeWorkspace()">返回知识工作台</button>
                    <button id="rag-tab-search" onclick="switchRAGTab('search')">检索问答</button>
                    <button id="rag-tab-ingest" onclick="switchRAGTab('ingest')">录入知识</button>
                    <button id="rag-tab-graph" onclick="showKnowledgeGraphWorkspace()">知识图谱</button>
                </div>
            </header>
            <section id="rag-panel-search" class="rag-panel">
                <div class="rag-query-row">
                    <input id="rag-query" type="text" placeholder="输入问题，例如：这个项目中 Agent 与 RAG 的关系是什么？" value="${escapeHTML(seedQuery)}">
                    <select id="rag-mode" class="form-select">
                        <option value="adaptive">自适应</option>
                        <option value="document">文档检索</option>
                        <option value="hybrid">混合检索</option>
                        <option value="graphrag">知识图谱</option>
                    </select>
                    <button class="btn-primary rag-run-btn" onclick="runRAGSearch()">检索</button>
                </div>
                <div class="rag-scope-row">
                    <label>文档范围</label>
                    <select id="rag-document-scope" class="form-select" onchange="onRAGDocumentScopeChange()">
                        <option value="0">全部知识库</option>
                    </select>
                    <span id="rag-document-scope-hint">自适应模式会先判断是否需要检索；文档检索模式会强制检索所选范围。</span>
                </div>
                <div id="rag-search-result" class="rag-result empty-tip">输入问题后查看答案、来源、CRAG 决策和 Self-RAG 检查点。</div>
            </section>
            <section id="rag-panel-ingest" class="rag-panel" style="display:none;">
                <div class="rag-editor-grid">
                    <div class="form-group">
                        <label>标题</label>
                        <input id="rag-title" type="text" placeholder="例如：项目 RAG 架构说明">
                    </div>
                    <div class="form-group">
                        <label>来源</label>
                        <input id="rag-source" type="text" placeholder="文件名、URL、会议或群聊来源">
                    </div>
                    <div class="form-group">
                        <label>可见性</label>
                        <select id="rag-visibility" class="form-select">
                            <option value="private">仅自己可见</option>
                            <option value="public">公共知识</option>
                        </select>
                    </div>
                    <div class="form-group">
                        <label>类型</label>
                        <select id="rag-source-type" class="form-select">
                            <option value="text">文本</option>
                            <option value="markdown">Markdown</option>
                            <option value="conversation">会话摘要</option>
                            <option value="file">文件</option>
                        </select>
                    </div>
                </div>
                <div class="form-group">
                    <label>内容</label>
                    <textarea id="rag-content" rows="12" placeholder="粘贴知识正文。系统会自动分块、抽取实体关系，并写入图谱。"></textarea>
                </div>
                <div class="rag-upload-zone">
                    <input id="rag-file-input" type="file" multiple accept=".txt,.md,.markdown,.mdx,.pdf,.docx,.png,.jpg,.jpeg,.webp,.bmp,.gif,.tif,.tiff,.go,.js,.ts,.tsx,.jsx,.py,.java,.c,.cpp,.cc,.cxx,.h,.hpp,.rs,.sql,.json,.yaml,.yml,.toml,.xml,.html,.css,.scss,.sh,.bat,.ps1,text/plain,text/markdown,text/x-go,application/json,application/pdf,image/png,image/jpeg,image/webp,application/vnd.openxmlformats-officedocument.wordprocessingml.document">
                    <div>
                        <strong>上传文件构建知识库</strong>
                        <p>支持 UTF-8 txt / Markdown / 代码文件 / PDF / docx / 图片。扫描件会尝试使用 GLM-OCR。</p>
                    </div>
                    <button class="btn-secondary" onclick="uploadRAGFiles()">上传并入库</button>
                </div>
                <div class="modal-actions">
                    <button class="btn-secondary" onclick="clearRAGIngestForm()">清空</button>
                    <button class="btn-primary" onclick="ingestRAGDocument()">写入知识库</button>
                </div>
                <div id="rag-ingest-result" class="rag-result"></div>
            </section>
            <section id="rag-panel-graph" class="rag-panel" style="display:none;">
                <div class="rag-query-row">
                    <input id="rag-graph-query" type="text" placeholder="按实体、项目、服务名过滤图谱">
                    <button class="btn-primary rag-run-btn" onclick="loadRAGGraph()">刷新图谱</button>
                </div>
                <div id="rag-graph-summary" class="rag-graph-summary"></div>
                <div id="rag-graph-view" class="rag-graph-view">加载知识图谱...</div>
            </section>
        </div>
    `;
    switchRAGTab(defaultTab || 'search');
    await loadKnowledgeSidebar();
    await syncRAGDocumentScopeOptions(seedDocumentID);
    if (ragSearchDocumentID) {
        const mode = document.getElementById('rag-mode');
        if (mode) mode.value = 'document';
        updateRAGDocumentScopeHint();
    }
    renderActiveRAGUploadIntoIngestPanel();
    if (defaultTab === 'graph') {
        await loadRAGGraph();
    }
}

function renderActiveRAGUploadIntoIngestPanel() {
    const resultEl = document.getElementById('rag-ingest-result');
    if (!resultEl) return;
    const entries = Object.entries(ragUploadJobs || {});
    if (!entries.length) return;
    const [, latest] = entries
        .sort((a, b) => Number(b[1]?.updated_at_ms || b[1]?.started_at || 0) - Number(a[1]?.updated_at_ms || a[1]?.started_at || 0))[0];
    renderRAGUploadJob(latest, resultEl);
}

async function syncRAGDocumentScopeOptions(selectedID = 0) {
    const select = document.getElementById('rag-document-scope');
    if (!select) return;
    const current = idString(selectedID) || idString(ragSearchDocumentID) || idString(select.value) || '0';
    if (!ragDocumentsCache.length || (current !== '0' && !ragDocumentsCache.some(doc => sameID(doc.id, current)))) {
        const resp = await ragAPI.documents(100, 0);
        if (resp && resp.code === 0 && resp.data?.success) {
            ragDocumentsCache = resp.data.documents || [];
        }
    }
    select.innerHTML = `<option value="0">全部知识库</option>` + (ragDocumentsCache || []).map(doc => `
        <option value="${escapeHTML(String(doc.id || 0))}">${escapeHTML(doc.title || doc.source || '未命名文档')} · ${Number(doc.chunk_count || 0)}块</option>
    `).join('');
    select.value = Array.from(select.options).some(opt => opt.value === current) ? current : '0';
    ragSearchDocumentID = idString(select.value);
    updateRAGDocumentScopeHint();
}

function onRAGDocumentScopeChange() {
    const select = document.getElementById('rag-document-scope');
    ragSearchDocumentID = idString(select?.value);
    if (ragSearchDocumentID) {
        const mode = document.getElementById('rag-mode');
        if (mode && mode.value === 'adaptive') mode.value = 'document';
    }
    updateRAGDocumentScopeHint();
}

function selectedRAGDocumentName() {
    const select = document.getElementById('rag-document-scope');
    if (!select || !select.value || select.value === '0') return '全部知识库';
    return select.options[select.selectedIndex]?.textContent || '指定文档';
}

function updateRAGDocumentScopeHint() {
    const hint = document.getElementById('rag-document-scope-hint');
    if (!hint) return;
    hint.textContent = ragSearchDocumentID
        ? `当前只检索：${selectedRAGDocumentName()}。后端会按 document_id 过滤，不再把标题拼进问题。`
        : '当前检索全部可见知识。选择文档后会启用真实 document_id 范围过滤。';
}

function switchRAGTab(tab) {
    ['search', 'ingest', 'graph'].forEach(name => {
        const panel = document.getElementById(`rag-panel-${name}`);
        const btn = document.getElementById(`rag-tab-${name}`);
        if (panel) panel.style.display = name === tab ? 'block' : 'none';
        if (btn) btn.classList.toggle('active', name === tab);
    });
    if (tab === 'graph') loadRAGGraph();
}

async function ingestRAGDocument() {
    const data = {
        title: document.getElementById('rag-title').value.trim(),
        source: document.getElementById('rag-source').value.trim(),
        source_type: document.getElementById('rag-source-type').value,
        visibility: document.getElementById('rag-visibility').value,
        content: document.getElementById('rag-content').value.trim(),
    };
    if (!data.content) {
        showToast('知识内容不能为空', 'warning');
        return;
    }
    const resultEl = document.getElementById('rag-ingest-result');
    resultEl.innerHTML = '<div class="empty-tip">正在写入并构建图谱...</div>';
    const resp = await ragAPI.ingest(data);
    if (resp && resp.code === 0 && resp.data?.success) {
        resultEl.innerHTML = `
            <div class="rag-status-line success">
                已写入：${escapeHTML(resp.data.document?.title || data.title || '未命名知识')}，
                                子区块 ${resp.data.chunk_count || 0}，
                实体 ${resp.data.entity_count || 0}，
                关系 ${resp.data.relation_count || 0}
            </div>
        `;
        showToast('知识已写入', 'success');
        await loadKnowledgeSidebar();
    } else {
        resultEl.innerHTML = `<div class="rag-status-line error">${escapeHTML(resp?.message || resp?.data?.msg || '写入失败')}</div>`;
    }
}

async function uploadRAGFiles() {
    const input = document.getElementById('rag-file-input');
    const files = input?.files || [];
    if (!files.length) {
        showToast('请选择 txt、md、pdf 或 docx 文件', 'warning');
        return;
    }
    const resultEl = document.getElementById('rag-ingest-result');
    const uploadStart = Date.now();
    const tempJobID = `upload-${uploadStart}`;
    const totalBytes = Array.from(files).reduce((sum, file) => sum + Number(file.size || 0), 0);
    ragUploadJobs[tempJobID] = normalizeRAGUploadData({
        job_id: tempJobID,
        job: {
            id: tempJobID,
            status: 'uploading',
            total: files.length,
            completed: 0,
            failed: 0,
            files: Array.from(files).map(file => ({ file_name: file.name, status: 'uploading' })),
        },
        status: 'uploading',
        started_at: uploadStart,
        upload_total: totalBytes,
    });
    renderRAGUploadBubble();
    renderRAGUploadSubmitting(resultEl, {
        phase: 'preparing',
        percent: 0,
        loaded: 0,
        total: totalBytes,
        fileCount: files.length,
        elapsed: 0,
    });
    const resp = await ragAPI.upload({
        fileList: files,
        title: document.getElementById('rag-title')?.value.trim() || '',
        visibility: document.getElementById('rag-visibility')?.value || 'private',
        groupID: currentConversationType === 'group' ? (conversationGroupMap[currentConversationID] || '') : '',
        conversationID: currentConversationID || '',
        onProgress: progress => {
            const next = {
                ...progress,
                total: progress.total || totalBytes,
                fileCount: files.length,
                elapsed: Date.now() - uploadStart,
            };
            renderRAGUploadSubmitting(resultEl, next);
            ragUploadJobs[tempJobID] = normalizeRAGUploadData(ragUploadJobs[tempJobID], {
                status: progress.phase === 'submitted' ? 'processing' : 'uploading',
                upload_percent: next.percent,
                upload_loaded: next.loaded,
                upload_total: next.total,
            });
            renderRAGUploadBubble();
        },
    });
    delete ragUploadJobs[tempJobID];
    renderRAGUploadBubble();
    if (resp && resp.code === 0 && resp.data?.success) {
        const jobID = resp.data.job_id;
        renderRAGUploadJob(resp.data, document.getElementById('rag-ingest-result') || resultEl);
        upsertRAGUploadJob(resp.data);
        if (jobID) {
            showToast('上传任务已提交，后台正在解析入库', 'info');
            pollRAGUploadJob(jobID, resultEl);
        } else {
            showToast('文件处理完成', 'success');
            await loadKnowledgeSidebar();
        }
    } else {
        ragUploadJobs[tempJobID] = normalizeRAGUploadData({
            job_id: tempJobID,
            job: {
                id: tempJobID,
                status: 'failed',
                total: files.length,
                completed: 0,
                failed: files.length,
                files: Array.from(files).map(file => ({ file_name: file.name, status: 'failed' })),
            },
            status: 'failed',
            started_at: uploadStart,
        });
        renderRAGUploadBubble();
        resultEl.innerHTML = `<div class="rag-status-line error">${escapeHTML(resp?.message || resp?.data?.msg || '上传失败')}</div>`;
    }
}

function renderRAGUploadSubmitting(resultEl, progress = {}) {
    resultEl = document.getElementById('rag-ingest-result') || resultEl;
    if (!resultEl) return;
    const phaseLabel = {
        preparing: '准备上传',
        uploading: '正在上传文件',
        submitted: '上传完成，等待后台解析',
    }[progress.phase] || '正在上传文件';
    const percent = Math.max(0, Math.min(100, Number(progress.percent || 0)));
    const loaded = Number(progress.loaded || 0);
    const total = Number(progress.total || 0);
    const elapsed = formatElapsed(progress.elapsed || 0);
    let card = resultEl.querySelector('.rag-upload-submit-card');
    if (!card) {
        resultEl.innerHTML = `
            <div class="rag-upload-submit-card">
                <div class="rag-upload-job-head">
                    <div>
                        <strong data-upload-submit-title>${escapeHTML(phaseLabel)}</strong>
                        <span data-upload-submit-meta></span>
                    </div>
                    <span class="rag-upload-submit-percent" data-upload-submit-percent>0%</span>
                </div>
                <div class="rag-upload-progress">
                    <span data-upload-submit-bar style="width:0%"></span>
                </div>
                <div class="rag-upload-telemetry">
                    <span data-upload-submit-size></span>
                    <span data-upload-submit-files></span>
                    <span data-upload-submit-elapsed></span>
                </div>
            </div>
        `;
        card = resultEl.querySelector('.rag-upload-submit-card');
    }
    card.querySelector('[data-upload-submit-title]').textContent = phaseLabel;
    card.querySelector('[data-upload-submit-meta]').textContent = percent >= 100
        ? '文件已到达网关，接下来进入文档解析、分片、Embedding 和图谱抽取。'
        : '大文件上传期间可切换到其他工作台；右下角任务气泡会持续显示上传和后台解析状态。';
    card.querySelector('[data-upload-submit-percent]').textContent = `${Math.round(percent)}%`;
    card.querySelector('[data-upload-submit-bar]').style.width = `${percent}%`;
    card.querySelector('[data-upload-submit-size]').textContent = total > 0
        ? `${formatAdminBytes(loaded)} / ${formatAdminBytes(total)}`
        : '等待浏览器返回上传大小';
    card.querySelector('[data-upload-submit-files]').textContent = `${Number(progress.fileCount || 0)} 个文件`;
    card.querySelector('[data-upload-submit-elapsed]').textContent = `用时 ${elapsed}`;
}

function ragUploadStatusLabel(status) {
    return {
        uploading: '上传中',
        pending: '等待处理',
        processing: '解析入库中',
        completed: '已完成',
        failed: '失败',
        stalled: '状态异常',
    }[status] || status || '未知';
}

function renderRAGUploadJob(data, resultEl = document.getElementById('rag-ingest-result')) {
    if (!resultEl) return;
    const normalized = normalizeRAGUploadData(data);
    const job = normalized.job || normalized;
    const files = job.files || normalized.files || [];
    const status = job.status || data.status || 'processing';
    const completed = job.completed ?? files.filter(f => f.status === 'completed' || f.success).length;
    const failed = job.failed ?? files.filter(f => f.status === 'failed').length;
    const total = job.total ?? files.length;
    const progress = total > 0 ? Math.min(100, Math.max(0, Math.round(((completed + failed) / total) * 100))) : (status === 'completed' ? 100 : 0);
    const elapsed = normalized.started_at ? formatElapsed(Date.now() - Number(normalized.started_at || Date.now())) : '0s';
    const lastUpdate = shortTime(normalized.updated_at_ms);
    const jobID = String(job.id || data.job_id || '');
    const current = resultEl.querySelector(`.rag-upload-job[data-job-id="${cssEscapeValue(jobID)}"]`);
    if (current && Number(current.dataset.fileCount || 0) === files.length) {
        current.querySelector('[data-upload-meta]').textContent = `${ragUploadStatusLabel(status)} · ${completed}/${total} 完成 · ${failed} 失败 · 用时 ${elapsed}${lastUpdate ? ` · 更新 ${lastUpdate}` : ''}`;
        current.querySelector('[data-upload-progress]').style.width = `${progress}%`;
        current.querySelector('[data-upload-progress-text]').textContent = `进度 ${progress}%`;
        current.querySelector('[data-upload-chunks]').textContent = `子区块 ${files.reduce((sum, item) => sum + Number(item.chunk_count || 0), 0)}`;
        current.querySelector('[data-upload-entities]').textContent = `实体 ${files.reduce((sum, item) => sum + Number(item.entity_count || 0), 0)}`;
        current.querySelector('[data-upload-relations]').textContent = `关系 ${files.reduce((sum, item) => sum + Number(item.relation_count || 0), 0)}`;
        const warning = current.querySelector('[data-upload-warning]');
        if (warning) {
            warning.style.display = normalized.poll_error ? 'block' : 'none';
            warning.textContent = normalized.poll_error ? `轮询暂时失败：${normalized.poll_error}。系统会继续重试，你也可以手动刷新。` : '';
        }
        files.forEach((item, index) => updateRAGUploadFileRow(current, item, index));
        return;
    }
    resultEl.innerHTML = `
        <div class="rag-upload-job" data-job-id="${escapeHTML(jobID)}" data-file-count="${files.length}">
            <div class="rag-upload-job-head">
                <div>
                    <strong>上传任务 #${escapeHTML(jobID)}</strong>
                    <span data-upload-meta>${escapeHTML(ragUploadStatusLabel(status))} · ${completed}/${total} 完成 · ${failed} 失败 · 用时 ${escapeHTML(elapsed)}${lastUpdate ? ` · 更新 ${escapeHTML(lastUpdate)}` : ''}</span>
                </div>
                <button class="btn-small ghost" onclick="refreshRAGUploadJob(${jsArg(job.id || data.job_id || 0)})">刷新状态</button>
            </div>
            <div class="rag-upload-progress" aria-label="知识入库进度">
                <span data-upload-progress style="width:${progress}%"></span>
            </div>
            <div class="rag-upload-telemetry">
                <span data-upload-progress-text>进度 ${progress}%</span>
                <span data-upload-chunks>子区块 ${files.reduce((sum, item) => sum + Number(item.chunk_count || 0), 0)}</span>
                <span data-upload-entities>实体 ${files.reduce((sum, item) => sum + Number(item.entity_count || 0), 0)}</span>
                <span data-upload-relations>关系 ${files.reduce((sum, item) => sum + Number(item.relation_count || 0), 0)}</span>
            </div>
            <div class="rag-status-line warning" data-upload-warning style="${normalized.poll_error ? '' : 'display:none;'}">${normalized.poll_error ? `轮询暂时失败：${escapeHTML(normalized.poll_error)}。系统会继续重试，你也可以手动刷新。` : ''}</div>
            <div class="rag-upload-file-list">
                ${files.map((item, index) => {
                    const fileStatus = item.status || (item.success ? 'completed' : 'pending');
                    const ok = fileStatus === 'completed' || item.success;
                    const failedFile = fileStatus === 'failed';
                    return `
                        <div class="rag-upload-file ${escapeHTML(fileStatus)}" data-file-index="${index}">
                            <div>
                                <strong>${escapeHTML(item.file_name || '')}</strong>
                                <span data-file-status>${escapeHTML(ragUploadStatusLabel(fileStatus))}${item.msg ? ' · ' + escapeHTML(item.msg) : ''}</span>
                            </div>
                            <div class="rag-upload-file-stats">
                                <span data-file-chunks>子区块 ${ok ? Number(item.chunk_count || 0) : '-'}</span>
                                <span data-file-entities>实体 ${ok ? Number(item.entity_count || 0) : '-'}</span>
                                <span data-file-relations>关系 ${ok ? Number(item.relation_count || 0) : '-'}</span>
                                <span data-file-danger class="danger" style="${failedFile ? '' : 'display:none;'}">需要重试</span>
                            </div>
                        </div>
                    `;
                }).join('') || '<div class="empty-tip">没有处理任何文件。</div>'}
            </div>
        </div>
    `;
}

function updateRAGUploadFileRow(root, item, index) {
    const row = root.querySelector(`[data-file-index="${index}"]`);
    if (!row) return;
    const fileStatus = item.status || (item.success ? 'completed' : 'pending');
    const ok = fileStatus === 'completed' || item.success;
    row.className = `rag-upload-file ${fileStatus}`;
    const statusEl = row.querySelector('[data-file-status]');
    if (statusEl) statusEl.textContent = `${ragUploadStatusLabel(fileStatus)}${item.msg ? ' · ' + item.msg : ''}`;
    const chunksEl = row.querySelector('[data-file-chunks]');
    const entitiesEl = row.querySelector('[data-file-entities]');
    const relationsEl = row.querySelector('[data-file-relations]');
    const dangerEl = row.querySelector('[data-file-danger]');
    if (chunksEl) chunksEl.textContent = `子区块 ${ok ? Number(item.chunk_count || 0) : '-'}`;
    if (entitiesEl) entitiesEl.textContent = `实体 ${ok ? Number(item.entity_count || 0) : '-'}`;
    if (relationsEl) relationsEl.textContent = `关系 ${ok ? Number(item.relation_count || 0) : '-'}`;
    if (dangerEl) dangerEl.style.display = fileStatus === 'failed' ? '' : 'none';
}

async function refreshRAGUploadJob(jobID) {
    const resultEl = document.getElementById('rag-ingest-result');
    if (!jobID || !resultEl) return;
    const resp = await ragAPI.uploadStatus(jobID);
    const data = responsePayload(resp);
    if (responseOK(resp)) {
        renderRAGUploadJob(data, resultEl);
        upsertRAGUploadJob(data);
        if (data.job?.status === 'completed') await loadKnowledgeSidebar();
    } else {
        showToast(resp?.message || resp?.data?.msg || '刷新上传任务失败', 'error');
    }
}

async function pollRAGUploadJob(jobID, resultEl, attempt = 0) {
    if (!jobID) return;
    if (attempt > 240) {
        patchRAGUploadJob(jobID, { status: 'stalled', poll_error: '超过最大轮询次数，任务可能仍在后台运行，请手动刷新确认。', poll_attempt: attempt });
        const liveResultEl = document.getElementById('rag-ingest-result') || resultEl;
        if (liveResultEl) renderRAGUploadJob(ragUploadJobs[String(jobID)], liveResultEl);
        return;
    }
    await new Promise(resolve => setTimeout(resolve, attempt < 8 ? 1200 : 2500));
    let resp = null;
    try {
        resp = await ragAPI.uploadStatus(jobID);
    } catch (err) {
        resp = null;
    }
    const data = responsePayload(resp);
    if (!responseOK(resp)) {
        const message = resp?.message || resp?.data?.msg || '网络或服务暂时不可用';
        patchRAGUploadJob(jobID, { poll_error: message, poll_attempt: attempt + 1 });
        const liveResultEl = document.getElementById('rag-ingest-result') || resultEl;
        if (liveResultEl) renderRAGUploadJob(ragUploadJobs[String(jobID)], liveResultEl);
        pollRAGUploadJob(jobID, resultEl, attempt + 1);
        return;
    }
    const liveResultEl = document.getElementById('rag-ingest-result') || resultEl;
    if (liveResultEl) renderRAGUploadJob(data, liveResultEl);
    upsertRAGUploadJob({ ...data, poll_error: '' });
    const status = data.job?.status;
    if (status === 'completed' || status === 'failed') {
        await loadKnowledgeSidebar();
        showToast(status === 'completed' ? 'RAG 文件入库完成' : 'RAG 文件入库任务失败', status === 'completed' ? 'success' : 'error');
        return;
    }
    pollRAGUploadJob(jobID, resultEl, attempt + 1);
}

function clearRAGIngestForm() {
    ['rag-title', 'rag-source', 'rag-content'].forEach(id => {
        const el = document.getElementById(id);
        if (el) el.value = '';
    });
    const fileInput = document.getElementById('rag-file-input');
    if (fileInput) fileInput.value = '';
    const visibility = document.getElementById('rag-visibility');
    if (visibility) visibility.value = 'private';
    const sourceType = document.getElementById('rag-source-type');
    if (sourceType) sourceType.value = 'text';
    const resultEl = document.getElementById('rag-ingest-result');
    if (resultEl) resultEl.innerHTML = '<div class="empty-tip">表单已清空，可以重新录入或上传文件。</div>';
    showToast('知识录入表单已清空', 'info');
}

async function runRAGSearch() {
    const query = document.getElementById('rag-query').value.trim();
    if (!query) {
        showToast('请输入问题', 'warning');
        return;
    }
    const resultEl = document.getElementById('rag-search-result');
    const mode = document.getElementById('rag-mode').value;
    const documentID = idString(document.getElementById('rag-document-scope')?.value) || idString(ragSearchDocumentID);
    renderRAGSearchLoading(resultEl, { query, mode, documentID });
    let resp = null;
    try {
        resp = await ragAPI.search({
            query,
            mode,
            document_id: documentID,
            limit: 8,
        });
    } catch (err) {
        resp = null;
    } finally {
        stopRAGSearchTimer();
    }
    const data = responsePayload(resp);
    if (!responseOK(resp)) {
        resultEl.innerHTML = `<div class="rag-status-line error">${escapeHTML(resp?.message || resp?.data?.msg || '检索失败。如果长时间停在检索中，通常是 rag-service / 模型调用 / 网关超时导致，请查看服务日志。')}</div>`;
        return;
    }
    renderRAGSearchResult(resultEl, data, { mode, documentID });
    const normalized = normalizeRAGSearchResult(data);
    ragGraphCache = { nodes: normalized.graph_nodes || [], edges: normalized.graph_edges || [], communities: [] };
}

function stopRAGSearchTimer() {
    if (ragSearchTimer) {
        clearInterval(ragSearchTimer);
        ragSearchTimer = null;
    }
}

function renderRAGSearchLoading(resultEl, context) {
    stopRAGSearchTimer();
    const start = Date.now();
    const title = context.mode === 'document' ? '正在执行文档检索' : context.mode === 'hybrid' ? '正在执行混合检索' : context.mode === 'graphrag' ? '正在查询知识图谱' : '正在执行 Adaptive RAG';
    resultEl.innerHTML = `
        <div class="rag-loading-card">
            <div class="rag-loading-orbit"><span></span></div>
            <div class="rag-loading-main">
                <strong>${escapeHTML(title)}</strong>
                <p data-rag-loading-stage>正在检索、重排和质检 · 已用时 0s</p>
                <div class="rag-loading-steps">
                    <span class="active" data-step="route">路由</span>
                    <span data-step="retrieve">召回</span>
                    <span data-step="rerank">Rerank</span>
                    <span data-step="answer">总结</span>
                </div>
                <small>问题：${escapeHTML(context.query)}${context.documentID ? ` · 范围：${escapeHTML(selectedRAGDocumentName())}` : ''}</small>
                <em data-rag-loading-hint>如果超过 45 秒，页面会保留状态并提示可能的服务或模型超时。</em>
            </div>
        </div>
    `;
    const paint = () => {
        const elapsed = Date.now() - start;
        const stage = elapsed > 45000 ? '仍在等待服务返回，可能是模型或 RPC 超时' : (elapsed > 15000 ? '正在等待模型总结和自检' : '正在检索、重排和质检');
        const stageEl = resultEl.querySelector('[data-rag-loading-stage]');
        if (stageEl) stageEl.textContent = `${stage} · 已用时 ${formatElapsed(elapsed)}`;
        resultEl.querySelector('[data-step="retrieve"]')?.classList.toggle('active', elapsed > 1200);
        resultEl.querySelector('[data-step="rerank"]')?.classList.toggle('active', elapsed > 3000);
        resultEl.querySelector('[data-step="answer"]')?.classList.toggle('active', elapsed > 5200);
        const hintEl = resultEl.querySelector('[data-rag-loading-hint]');
        if (hintEl && elapsed > 45000) hintEl.textContent = '已等待较久：请检查 rag-service、模型配置或网关 RPC timeout。当前请求仍在等待返回。';
    };
    paint();
    ragSearchTimer = setInterval(paint, 1000);
}

function renderRAGSearchResult(resultEl, data, context) {
    const normalized = normalizeRAGSearchResult(data);
    const check = normalized.self_check || {};
    resultEl.innerHTML = `
        <div class="rag-answer">
            <div class="rag-meta-strip">
                <span>范围：${escapeHTML(context.documentID ? selectedRAGDocumentName() : '全部知识库')}</span>
                <span>模式：${escapeHTML(context.mode === 'document' ? '文档检索' : context.mode === 'hybrid' ? '混合检索' : context.mode === 'graphrag' ? '知识图谱' : '自适应')}</span>
                <span>路线：${escapeHTML(ragRouteLabel(normalized.route))}</span>
                <span>CRAG：${escapeHTML(cragLabel(normalized.crag_action))}</span>
                <span>Retrieve：${check.retrieve ? '是' : '否'}</span>
                <span>IsRel：${check.is_rel ? '通过' : '未通过'}</span>
                <span>IsSup：${check.is_sup ? '通过' : '不足'}</span>
                <span>IsUse：${check.is_use ? '有用' : '不足'}</span>
            </div>
            <div class="rag-answer-label">AI 总结</div>
            <div class="rag-answer-text">${renderMarkdownText(normalized.answer || '当前没有生成模型总结。请检查 RAG 回答模型配置，或查看下方命中来源。')}</div>
            ${check.note ? `<details class="rag-note"><summary>检索诊断</summary><p>${escapeHTML(check.note)}</p></details>` : ''}
        </div>
        <div class="rag-sources">
            <div class="rag-source-head">
                <h3>命中来源</h3>
                <span>${Number((normalized.sources || []).length)} 条，可展开查看原文和命中信息</span>
            </div>
            ${(normalized.sources || []).length ? normalized.sources.map(renderRAGSourceCard).join('') : '<div class="empty-tip">没有命中内部来源。</div>'}
        </div>
    `;
}

function normalizeRAGSearchResult(data = {}) {
    const self = valueOfAny(data, 'self_check', 'SelfCheck') || {};
    return {
        answer: valueOfAny(data, 'answer', 'Answer') || '',
        route: valueOfAny(data, 'route', 'Route') || '',
        crag_action: valueOfAny(data, 'crag_action', 'CragAction') || '',
        self_check: {
            retrieve: !!valueOfAny(self, 'retrieve', 'Retrieve'),
            is_rel: !!valueOfAny(self, 'is_rel', 'IsRel'),
            is_sup: !!valueOfAny(self, 'is_sup', 'IsSup'),
            is_use: !!valueOfAny(self, 'is_use', 'IsUse'),
            note: valueOfAny(self, 'note', 'Note') || '',
        },
        sources: valueOfAny(data, 'sources', 'Sources') || [],
        graph_nodes: valueOfAny(data, 'graph_nodes', 'GraphNodes') || [],
        graph_edges: valueOfAny(data, 'graph_edges', 'GraphEdges') || [],
    };
}

function renderRAGSourceCard(src, index) {
    const documentID = valueOfAny(src, 'document_id', 'DocumentId') || '';
    const chunkID = valueOfAny(src, 'chunk_id', 'ChunkId') || '';
    const title = valueOfAny(src, 'title', 'Title') || '未知文档';
    const source = valueOfAny(src, 'source', 'Source') || '无来源路径';
    const reason = valueOfAny(src, 'reason', 'Reason') || '';
    const content = valueOfAny(src, 'content', 'Content') || '';
    const score = Number(valueOfAny(src, 'score', 'Score') || 0);
    return `
        <article class="rag-source-card">
            <header>
                <div>
                    <span class="rag-source-index">#${index + 1}</span>
                    <strong>${escapeHTML(title)}</strong>
                    <small>${escapeHTML(source)}</small>
                </div>
                <div class="rag-source-score">
                    <span>${score.toFixed(3)}</span>
                    <small>score</small>
                </div>
            </header>
            <div class="rag-source-meta">
                ${documentID ? `<span>文档 ${escapeHTML(String(documentID))}</span>` : ''}
                ${chunkID ? `<span>Chunk ${escapeHTML(String(chunkID))}</span>` : ''}
                ${reason ? `<span>${escapeHTML(reason)}</span>` : ''}
            </div>
            <details>
                <summary>查看命中原文</summary>
                <div class="rag-source-content">${renderMarkdownText(content)}</div>
            </details>
        </article>
    `;
}

async function loadRAGGraph() {
    const view = document.getElementById('rag-graph-view');
    if (!view) return;
    view.innerHTML = '加载知识图谱...';
    const query = document.getElementById('rag-graph-query')?.value || '';
    const resp = await ragAPI.graph(query, 120);
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        view.innerHTML = `<div class="empty-tip">图谱加载失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    ragGraphCache = {
        nodes: resp.data.nodes || [],
        edges: resp.data.edges || [],
        communities: resp.data.communities || [],
    };
    renderRAGGraph();
}

function renderRAGGraph() {
    const view = document.getElementById('rag-graph-view');
    const summary = document.getElementById('rag-graph-summary');
    if (!view) return;
    const nodes = ragGraphCache.nodes || [];
    const edges = ragGraphCache.edges || [];
    const communities = ragGraphCache.communities || [];
    if (summary) {
        summary.innerHTML = `
            <span>实体 ${nodes.length}</span>
            <span>关系 ${edges.length}</span>
            <span>社区 ${communities.length}</span>
            ${communities.slice(0, 4).map(c => `<span>${escapeHTML(c.name || '社区')}：${escapeHTML(c.summary || '')}</span>`).join('')}
        `;
    }
    if (!nodes.length) {
        view.innerHTML = '<div class="empty-tip">暂无图谱节点<br><small>先录入知识，系统会抽取实体和关系。</small></div>';
        return;
    }
    const width = 920;
    const height = 560;
    const centerX = width / 2;
    const centerY = height / 2;
    const radius = Math.min(width, height) * 0.36;
    const positioned = nodes.map((node, idx) => {
        const angle = (Math.PI * 2 * idx) / Math.max(nodes.length, 1);
        const scoreOffset = Math.min(44, Math.max(0, Number(node.score || 0) * 12));
        return {
            ...node,
            x: centerX + Math.cos(angle) * (radius - scoreOffset),
            y: centerY + Math.sin(angle) * (radius - scoreOffset),
        };
    });
    const nodeMap = {};
    positioned.forEach(n => { nodeMap[String(n.id)] = n; });
    const edgeHTML = edges.map(edge => {
        const s = nodeMap[String(edge.source_id)];
        const t = nodeMap[String(edge.target_id)];
        if (!s || !t) return '';
        return `<line x1="${s.x}" y1="${s.y}" x2="${t.x}" y2="${t.y}" class="rag-edge"><title>${escapeHTML(edge.relation || '')}：${escapeHTML(edge.evidence || '')}</title></line>`;
    }).join('');
    const nodeHTML = positioned.map(node => `
        <g class="rag-node" onclick="focusRAGNode(${jsStringArg(node.id)})">
            <circle cx="${node.x}" cy="${node.y}" r="${Math.max(18, Math.min(34, 18 + Number(node.score || 0) * 5))}" class="rag-node-circle"></circle>
            <text x="${node.x}" y="${node.y + 4}" text-anchor="middle">${escapeHTML(String(node.name || '').slice(0, 8))}</text>
            <title>${escapeHTML(node.name || '')} · ${escapeHTML(node.type || 'entity')}\n${escapeHTML(node.summary || '')}</title>
        </g>
    `).join('');
    view.innerHTML = `
        <svg class="rag-graph-svg" viewBox="0 0 ${width} ${height}" role="img" aria-label="知识图谱">
            <g>${edgeHTML}</g>
            <g>${nodeHTML}</g>
        </svg>
        <div id="rag-node-detail" class="rag-node-detail">点击节点查看实体摘要。</div>
    `;
}

function focusRAGNode(nodeID) {
    const node = (ragGraphCache.nodes || []).find(n => sameID(n.id, nodeID));
    const detail = document.getElementById('rag-node-detail');
    if (!node || !detail) return;
    const related = (ragGraphCache.edges || []).filter(e => sameID(e.source_id, nodeID) || sameID(e.target_id, nodeID));
    detail.innerHTML = `
        <strong>${escapeHTML(node.name || '实体')}</strong>
        <span>${escapeHTML(node.type || 'entity')} · score ${Number(node.score || 0).toFixed(2)}</span>
        <p>${escapeHTML(node.summary || '暂无摘要')}</p>
        <small>相关关系：${related.length ? related.map(e => escapeHTML(e.relation || '关联')).join('、') : '暂无'}</small>
    `;
}

const knowledgeTypeOptions = ['Service', 'Data', 'Event', 'Interface', 'Concept'];
const knowledgeRelationOptions = ['CALLS', 'EVENT_FLOW', 'DATA_FLOW', 'DEPENDS_ON', 'RELATED_TO'];
const knowledgeTypeLabels = {
    Service: '服务',
    Data: '数据',
    Event: '事件',
    Interface: '接口/模块',
    PersonOrg: '人/组织',
    DatabaseTable: '数据表',
    EventTopic: '事件主题',
    API: '接口',
    Module: '模块',
    Concept: '概念',
    Person: '人员',
    Organization: '组织',
    Product: '产品',
};
const knowledgeRelationLabels = {
    CALLS: '调用',
    EVENT_FLOW: '事件流',
    DATA_FLOW: '数据流',
    PUBLISHES: '发布',
    CONSUMES: '消费',
    STORES: '存储',
    OWNS: '负责',
    DEPENDS_ON: '依赖',
    CONFIGURES: '配置',
    TRIGGERS: '触发',
    READS: '读取',
    WRITES: '写入',
    RELATED_TO: '相关',
};

function knowledgeTypeLabel(type) {
    return knowledgeTypeLabels[type] || type || '概念';
}

function knowledgeRelationLabel(relation) {
    return knowledgeRelationLabels[relation] || relation || '相关';
}

function knowledgeDisplayType(type) {
    switch (type) {
        case 'API':
        case 'Module':
            return 'Interface';
        case 'DatabaseTable':
        case 'Product':
            return 'Data';
        case 'EventTopic':
            return 'Event';
        case 'Person':
        case 'Organization':
        case 'PersonOrg':
            return 'Concept';
        case 'Service':
        case 'Data':
        case 'Event':
        case 'Interface':
        case 'Concept':
            return type;
        default:
            return 'Concept';
    }
}

function knowledgeDisplayRelation(relation) {
    switch (relation) {
        case 'PUBLISHES':
        case 'CONSUMES':
        case 'TRIGGERS':
            return 'EVENT_FLOW';
        case 'READS':
        case 'WRITES':
        case 'STORES':
            return 'DATA_FLOW';
        case 'CALLS':
        case 'DEPENDS_ON':
        case 'RELATED_TO':
            return relation;
        default:
            return 'RELATED_TO';
    }
}

async function showKnowledgeGraphWorkspace(seedQuery = '', seedDocumentID = 0) {
    activateStandaloneWorkspace('knowledge');
    document.getElementById('chat-area').style.display = 'none';
    const welcome = document.getElementById('welcome-area');
    welcome.style.display = 'flex';
    welcome.innerHTML = `
        <div class="knowledge-graph-shell">
            <section class="knowledge-hero">
                <div class="knowledge-hero-main">
                    <span class="knowledge-kicker">Knowledge Graph</span>
                    <h2>知识图谱实验室</h2>
                    <p>从 GraphRAG 的实体、关系和社区摘要中观察项目结构。搜索服务、表、Topic 或概念，查看它们如何互相调用、写入、发布和消费。</p>
                </div>
                <div class="knowledge-hero-stats" id="knowledge-hero-stats">
                    <span>实体 0</span><span>关系 0</span><span>社区 0</span>
                </div>
                <button class="knowledge-back-btn" onclick="showKnowledgeHomeWorkspace()">返回知识工作台</button>
            </section>
            <section class="knowledge-toolbar">
                <div class="knowledge-search">
                    <input id="knowledge-query" type="text" placeholder="搜索节点，例如 agent_dispatch_records / msg-core-service" value="${escapeHTML(seedQuery)}" onkeydown="if(event.key==='Enter')loadKnowledgeGraph()">
                    <button class="btn-primary" onclick="loadKnowledgeGraph()">搜索图谱</button>
                    <button class="btn-secondary" onclick="resetKnowledgeGraphView()">重置视图</button>
                </div>
                <div class="knowledge-filter-row document-scope">
                    <label>文章范围</label>
                    <select id="knowledge-document-select" class="form-select" onchange="loadKnowledgeGraph()">
                        <option value="0">全部文档</option>
                    </select>
                    <button class="btn-small danger-soft" onclick="deleteSelectedKnowledgeGraph()">删除该文图谱</button>
                </div>
                <div class="knowledge-filter-row">
                    <label>实体类型</label>
                    <div id="knowledge-type-filters" class="knowledge-chip-group">
                        ${knowledgeTypeOptions.map(type => `<button class="knowledge-chip" data-value="${escapeHTML(type)}" onclick="toggleKnowledgeFilter(this)">${escapeHTML(knowledgeTypeLabel(type))}</button>`).join('')}
                    </div>
                </div>
                <div class="knowledge-filter-row">
                    <label>关系类型</label>
                    <div id="knowledge-relation-filters" class="knowledge-chip-group compact">
                        ${knowledgeRelationOptions.map(type => `<button class="knowledge-chip relation" data-value="${escapeHTML(type)}" onclick="toggleKnowledgeFilter(this)">${escapeHTML(knowledgeRelationLabel(type))}</button>`).join('')}
                    </div>
                </div>
                <div class="knowledge-filter-row split">
                    <label>邻居深度</label>
                    <select id="knowledge-hop-select" class="form-select" onchange="loadKnowledgeGraph()">
                        <option value="1">一跳邻居</option>
                        <option value="2">二跳邻居</option>
                    </select>
                    <label>社区</label>
                    <select id="knowledge-community-select" class="form-select" onchange="loadKnowledgeGraph()">
                        <option value="0">全部社区</option>
                    </select>
                </div>
            </section>
            <section class="knowledge-stage">
                <div class="knowledge-canvas-card">
                    <div class="knowledge-canvas-head">
                        <div id="knowledge-graph-summary" class="knowledge-summary-strip"></div>
                        <div class="knowledge-layout-actions">
                            <button class="btn-small ghost" onclick="fitKnowledgeGraph()">适配画布</button>
                            <button class="btn-small ghost" onclick="highlightKnowledgePath()">高亮路径</button>
                        </div>
                    </div>
                    <div id="knowledge-graph-canvas" class="knowledge-graph-canvas">
                        <div class="empty-tip">正在加载知识图谱...</div>
                    </div>
                </div>
                <aside id="knowledge-detail-panel" class="knowledge-detail-panel">
                    <div class="knowledge-detail-empty">
                        <strong>选择节点或关系</strong>
                        <span>点击图中的实体可查看说明、相邻节点、相关关系和证据来源。</span>
                    </div>
                    <div id="knowledge-review-panel" class="knowledge-review-panel"></div>
                </aside>
            </section>
        </div>
    `;
    await loadKnowledgeSidebar();
    await syncKnowledgeDocumentOptions(seedDocumentID);
    await loadKnowledgeGraph();
    await loadKnowledgeReviewCandidates();
}

function toggleKnowledgeFilter(button) {
    button.classList.toggle('active');
    loadKnowledgeGraph();
}

function selectedKnowledgeFilters(id) {
    return Array.from(document.querySelectorAll(`#${id} .knowledge-chip.active`)).map(btn => btn.dataset.value).filter(Boolean);
}

function currentKnowledgeQueryOptions() {
    return {
        query: document.getElementById('knowledge-query')?.value.trim() || '',
        types: selectedKnowledgeFilters('knowledge-type-filters'),
        relations: selectedKnowledgeFilters('knowledge-relation-filters'),
        communityID: document.getElementById('knowledge-community-select')?.value || 0,
        documentID: idString(document.getElementById('knowledge-document-select')?.value),
        hops: Number(document.getElementById('knowledge-hop-select')?.value || 1),
        limit: 120,
    };
}

async function syncKnowledgeDocumentOptions(selectedID = 0) {
    const select = document.getElementById('knowledge-document-select');
    if (!select) return;
    const selectedIDString = String(selectedID || select.value || '0');
    if (!ragDocumentsCache.length || (selectedIDString !== '0' && !ragDocumentsCache.some(doc => sameID(doc.id, selectedIDString)))) {
        const resp = await ragAPI.documents(80, 0);
        if (resp && resp.code === 0 && resp.data?.success) {
            ragDocumentsCache = resp.data.documents || [];
        }
    }
    const current = selectedIDString;
    select.innerHTML = `<option value="0">全部文档</option>` + (ragDocumentsCache || []).map(doc => `
        <option value="${escapeHTML(String(doc.id || 0))}">${escapeHTML(doc.title || doc.source || '未命名文档')} · ${Number(doc.chunk_count || 0)}块</option>
    `).join('');
    select.value = Array.from(select.options).some(opt => opt.value === current) ? current : '0';
}

async function loadKnowledgeGraph() {
    const canvas = document.getElementById('knowledge-graph-canvas');
    if (!canvas) return;
    canvas.innerHTML = '<div class="empty-tip">正在向 knowledge-service 查询图谱视图...</div>';
    const resp = await knowledgeAPI.graph(currentKnowledgeQueryOptions());
    const data = responsePayload(resp);
    if (!responseOK(resp)) {
        canvas.innerHTML = `<div class="empty-tip">图谱加载失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '请确认 knowledge-service / rag-service 已启动')}</small></div>`;
        return;
    }
    knowledgeGraphCache = data;
    syncKnowledgeCommunityOptions(data.communities || []);
    renderKnowledgeGraph();
}

function syncKnowledgeCommunityOptions(communities) {
    const select = document.getElementById('knowledge-community-select');
    if (!select) return;
    const current = select.value || '0';
    select.innerHTML = `<option value="0">全部社区</option>` + communities.map(c => `<option value="${escapeHTML(c.id)}">${escapeHTML(c.name || '未命名社区')}</option>`).join('');
    select.value = Array.from(select.options).some(opt => opt.value === current) ? current : '0';
}

function renderKnowledgeGraph() {
    const data = knowledgeGraphCache || {};
    const nodes = data.nodes || [];
    const edges = data.edges || [];
    const communities = data.communities || [];
    const stats = data.stats || {};
    const canvas = document.getElementById('knowledge-graph-canvas');
    const summary = document.getElementById('knowledge-graph-summary');
    const heroStats = document.getElementById('knowledge-hero-stats');
    if (!canvas) return;
    if (heroStats) {
        heroStats.innerHTML = `<span>实体 ${stats.node_count ?? nodes.length}</span><span>关系 ${stats.edge_count ?? edges.length}</span><span>社区 ${stats.community_count ?? communities.length}</span>`;
    }
    if (summary) {
        const typeLabels = uniqueArray((stats.types || []).map(type => knowledgeTypeLabel(knowledgeDisplayType(type))));
        const relationLabels = uniqueArray((stats.relations || []).map(type => knowledgeRelationLabel(knowledgeDisplayRelation(type))));
        summary.innerHTML = `
            <span>范围：${escapeHTML(selectedKnowledgeDocumentName())}</span>
            <span>类型：${typeLabels.join(' / ') || '暂无'}</span>
            <span>关系：${relationLabels.join(' / ') || '暂无'}</span>
            ${communities.slice(0, 3).map(c => `<span style="--community-color:${escapeHTML(c.color || '#64748b')}">${escapeHTML(c.name || '社区')}</span>`).join('')}
        `;
    }
    if (!nodes.length) {
        const documentID = currentKnowledgeQueryOptions().documentID;
        const reason = data.msg || data.Msg || '';
        canvas.innerHTML = `
            <div class="empty-tip">
                暂无图谱节点
                <br>
                <small>${escapeHTML(reason || (documentID ? '该文档当前没有可展示的实体/关系。可以重新入库、删除该文图谱后重建，或放宽过滤条件。' : '先上传文档，或清空搜索与类型过滤条件。'))}</small>
            </div>
        `;
        renderKnowledgeEmptyDetail();
        return;
    }
    if (knowledgeGraphInstance) {
        knowledgeGraphInstance.destroy();
        knowledgeGraphInstance = null;
    }
    canvas.innerHTML = '<div id="knowledge-g6-container" class="knowledge-g6-container"></div>';
    if (window.G6) {
        renderKnowledgeG6(nodes, edges);
    } else {
        renderKnowledgeSVGFallback(canvas, nodes, edges);
    }
    applyKnowledgePathHighlight();
}

function selectedKnowledgeDocumentName() {
    const select = document.getElementById('knowledge-document-select');
    if (!select || !select.value || select.value === '0') return '全部文档';
    return select.options[select.selectedIndex]?.textContent || '指定文档';
}

async function deleteSelectedKnowledgeGraph() {
    const documentID = idString(document.getElementById('knowledge-document-select')?.value);
    if (!documentID) {
        showToast('请先选择一篇具体文章', 'warning');
        return;
    }
    await deleteRAGDocumentGraph(documentID, selectedKnowledgeDocumentName());
}

function uniqueArray(values) {
    return Array.from(new Set((values || []).filter(Boolean)));
}

function renderKnowledgeG6(nodes, edges) {
    const container = document.getElementById('knowledge-g6-container');
    if (!container) return;
    const width = container.clientWidth || 900;
    const height = container.clientHeight || 620;
    const showEdgeLabels = edges.length <= 18;
    const graphData = {
        nodes: nodes.map(node => ({
            id: String(node.id),
            label: truncateKnowledgeLabel(node.name || '', 22),
            data: node,
            size: node.size || 34,
            style: {
                fill: node.color || '#0ea5e9',
                stroke: '#ffffff',
                lineWidth: 2,
                shadowBlur: 18,
                shadowColor: 'rgba(15,23,42,0.18)',
            },
            labelCfg: {
                position: 'bottom',
                style: { fill: '#21312b', fontSize: 12, fontWeight: 700 },
            },
        })),
        edges: edges.map(edge => ({
            id: String(edge.id),
            source: String(edge.source_id),
            target: String(edge.target_id),
            label: showEdgeLabels ? knowledgeRelationLabel(edge.relation) : '',
            data: edge,
            style: {
                stroke: edge.color || '#64748b',
                lineWidth: Math.max(1.2, Math.min(4, Number(edge.weight || 1) * 2.2)),
                endArrow: true,
                opacity: 0.72,
            },
            labelCfg: {
                autoRotate: true,
                style: { fill: '#52615b', fontSize: 10, background: { fill: '#ffffff', padding: [2, 4, 2, 4], radius: 3 } },
            },
        })),
    };
    knowledgeGraphInstance = new G6.Graph({
        container: 'knowledge-g6-container',
        width,
        height,
        fitView: true,
        fitViewPadding: 42,
        animate: true,
        layout: {
            type: 'force',
            preventOverlap: true,
            nodeStrength: -430,
            edgeStrength: 0.10,
            linkDistance: 220,
        },
        modes: {
            default: ['drag-canvas', 'zoom-canvas', 'drag-node', 'activate-relations'],
        },
        defaultNode: { type: 'circle' },
        defaultEdge: { type: 'quadratic' },
        nodeStateStyles: {
            hover: { lineWidth: 5, shadowBlur: 26 },
            selected: { lineWidth: 6, stroke: '#d97706' },
        },
        edgeStateStyles: {
            hover: { lineWidth: 4, opacity: 1 },
            selected: { lineWidth: 5, stroke: '#d97706', opacity: 1 },
        },
    });
    knowledgeGraphInstance.data(graphData);
    knowledgeGraphInstance.render();
    knowledgeGraphInstance.on('node:mouseenter', evt => {
        knowledgeGraphInstance.setItemState(evt.item, 'hover', true);
        container.classList.add('is-hovering');
    });
    knowledgeGraphInstance.on('node:mouseleave', evt => {
        knowledgeGraphInstance.setItemState(evt.item, 'hover', false);
        container.classList.remove('is-hovering');
    });
    knowledgeGraphInstance.on('edge:mouseenter', evt => knowledgeGraphInstance.setItemState(evt.item, 'hover', true));
    knowledgeGraphInstance.on('edge:mouseleave', evt => knowledgeGraphInstance.setItemState(evt.item, 'hover', false));
    knowledgeGraphInstance.on('node:click', evt => {
        clearKnowledgeGraphSelection();
        knowledgeGraphInstance.setItemState(evt.item, 'selected', true);
        const node = evt.item.getModel().data;
        knowledgeGraphSelected = { type: 'node', id: node.id };
        renderKnowledgeNodeDetail(node.id);
    });
    knowledgeGraphInstance.on('edge:click', evt => {
        clearKnowledgeGraphSelection();
        knowledgeGraphInstance.setItemState(evt.item, 'selected', true);
        const edge = evt.item.getModel().data;
        knowledgeGraphSelected = { type: 'edge', id: edge.id };
        renderKnowledgeEdgeDetail(edge.id);
    });
}

function clearKnowledgeGraphSelection() {
    if (!knowledgeGraphInstance) return;
    knowledgeGraphInstance.getNodes().forEach(item => knowledgeGraphInstance.clearItemStates(item, ['selected']));
    knowledgeGraphInstance.getEdges().forEach(item => knowledgeGraphInstance.clearItemStates(item, ['selected']));
}

function truncateKnowledgeLabel(value, limit = 22) {
    const text = String(value || '').trim();
    if (text.length <= limit) return text;
    return `${text.slice(0, limit)}...`;
}

function knowledgeEntityIntro(node) {
    const type = knowledgeDisplayType(node?.type || 'Concept');
    const label = knowledgeTypeLabel(type);
    const summary = String(node?.summary || '').trim();
    if (summary) return summary;
    const templates = {
        Service: '这是文档中识别出的服务或网关节点，通常代表一个可调用、可部署或负责业务流程的系统组件。',
        Data: '这是文档中识别出的数据节点，通常代表数据表、配置、存储对象或业务数据集合。',
        Event: '这是文档中识别出的事件节点，通常代表消息主题、事件流或异步通知入口。',
        Interface: '这是文档中识别出的接口或模块节点，通常代表可被调用的 API、模块边界或功能入口。',
        Concept: '这是文档中识别出的概念节点，通常代表业务概念、项目术语或讨论主题。',
    };
    return `${label}：${templates[type] || templates.Concept}`;
}

function knowledgeRelationIntro(edge) {
    const relation = knowledgeDisplayRelation(edge?.relation || 'RELATED_TO');
    const label = knowledgeRelationLabel(relation);
    const description = String(edge?.description || '').trim();
    if (description) return description;
    const templates = {
        CALLS: '调用关系：源实体会调用、依赖调用或触发目标实体的能力。',
        EVENT_FLOW: '事件流关系：源实体会发布、消费或触发目标事件，表示异步消息流动。',
        DATA_FLOW: '数据流关系：源实体会读取、写入或存储目标数据，表示数据读写路径。',
        DEPENDS_ON: '依赖关系：源实体需要目标实体提供配置、能力或上下文才能完成工作。',
        RELATED_TO: '相关关系：两个实体在同一段文档上下文中被关联提及，但关系方向或类型不够强。',
    };
    return `${label}：${templates[relation] || templates.RELATED_TO}`;
}

function renderKnowledgeSVGFallback(canvas, nodes, edges) {
    const width = 980;
    const height = 620;
    const radius = Math.min(width, height) * 0.36;
    const cx = width / 2;
    const cy = height / 2;
    const positioned = nodes.map((node, idx) => {
        const angle = Math.PI * 2 * idx / Math.max(nodes.length, 1);
        return { ...node, x: cx + Math.cos(angle) * radius, y: cy + Math.sin(angle) * radius };
    });
    const nodeMap = Object.fromEntries(positioned.map(n => [String(n.id), n]));
    const edgeHTML = edges.map(edge => {
        const s = nodeMap[String(edge.source_id)];
        const t = nodeMap[String(edge.target_id)];
        if (!s || !t) return '';
        return `<line x1="${s.x}" y1="${s.y}" x2="${t.x}" y2="${t.y}" class="knowledge-svg-edge" onclick="renderKnowledgeEdgeDetail(${jsStringArg(edge.id)})"><title>${escapeHTML(knowledgeRelationLabel(edge.relation))}：${escapeHTML(edge.evidence || '')}</title></line>`;
    }).join('');
    const nodeHTML = positioned.map(node => `
        <g class="knowledge-svg-node" onclick="renderKnowledgeNodeDetail(${jsStringArg(node.id)})">
            <circle cx="${node.x}" cy="${node.y}" r="${Math.max(18, Math.min(42, node.size || 28))}" style="fill:${escapeHTML(node.color || '#0ea5e9')}"></circle>
            <text x="${node.x}" y="${node.y + 58}" text-anchor="middle">${escapeHTML(String(node.name || '').slice(0, 18))}</text>
        </g>
    `).join('');
    canvas.innerHTML = `<svg class="knowledge-svg-fallback" viewBox="0 0 ${width} ${height}">${edgeHTML}${nodeHTML}</svg>`;
}

async function renderKnowledgeNodeDetail(nodeID) {
    const panel = document.getElementById('knowledge-detail-panel');
    if (!panel) return;
    panel.innerHTML = '<div class="empty-tip">正在读取节点详情...</div>';
    const resp = await knowledgeAPI.node(nodeID, currentKnowledgeQueryOptions());
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        panel.innerHTML = `<div class="empty-tip">节点详情加载失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const detail = resp.data;
    const node = detail.node || {};
    panel.innerHTML = `
        <div class="knowledge-detail-head">
            <span class="knowledge-node-dot" style="background:${escapeHTML(node.color || '#0ea5e9')}"></span>
            <div>
                <h3>${escapeHTML(node.name || '实体')}</h3>
                <p>${escapeHTML(knowledgeTypeLabel(node.type || 'Concept'))} · 度数 ${node.degree || 0} · score ${Number(node.score || 0).toFixed(2)}</p>
            </div>
        </div>
        <div class="knowledge-detail-section">
            <strong>实体说明</strong>
            <p>${escapeHTML(knowledgeEntityIntro(node))}</p>
        </div>
        <div class="knowledge-detail-actions">
            <button class="btn-small ghost" onclick="loadKnowledgeNeighborhood(${jsStringArg(node.id)})">查看邻域</button>
            <button class="btn-small ghost" onclick="setKnowledgePathPoint('source', ${jsStringArg(node.id)})">设为起点</button>
            <button class="btn-small ghost" onclick="setKnowledgePathPoint('target', ${jsStringArg(node.id)})">设为终点</button>
            <button class="btn-small" onclick="submitKnowledgeReviewCandidate('node', ${jsStringArg(node.id)})">提交审核</button>
        </div>
        <div class="knowledge-detail-section">
            <strong>相邻节点</strong>
            ${(detail.neighbors || []).length ? detail.neighbors.map(n => `<button class="knowledge-neighbor" onclick="renderKnowledgeNodeDetail(${jsStringArg(n.id)})">${escapeHTML(n.name || '')}<span>${escapeHTML(knowledgeTypeLabel(n.type || 'Concept'))}</span></button>`).join('') : '<p>暂无相邻节点</p>'}
        </div>
        <div class="knowledge-detail-section">
            <strong>相关关系</strong>
            ${(detail.relations || []).length ? detail.relations.map(edge => `
                <button class="knowledge-relation-row" onclick="renderKnowledgeEdgeDetail(${jsStringArg(edge.id)})">
                    <span>${escapeHTML(knowledgeRelationLabel(edge.relation || 'RELATED_TO'))}</span>
                    <small>${escapeHTML(edge.evidence || edge.description || '')}</small>
                </button>
            `).join('') : '<p>暂无关系</p>'}
        </div>
        <div id="knowledge-review-panel" class="knowledge-review-panel"></div>
    `;
    loadKnowledgeReviewCandidates();
}

async function renderKnowledgeEdgeDetail(edgeID) {
    const panel = document.getElementById('knowledge-detail-panel');
    if (!panel) return;
    panel.innerHTML = '<div class="empty-tip">正在读取关系详情...</div>';
    const resp = await knowledgeAPI.edge(edgeID, currentKnowledgeQueryOptions());
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        panel.innerHTML = `<div class="empty-tip">关系详情加载失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const detail = resp.data;
    const edge = detail.edge || {};
    panel.innerHTML = `
        <div class="knowledge-detail-head relation">
            <span class="knowledge-node-dot" style="background:${escapeHTML(edge.color || '#64748b')}"></span>
            <div>
                <h3>${escapeHTML(knowledgeRelationLabel(edge.relation || 'RELATED_TO'))}</h3>
                <p>${escapeHTML(detail.source?.name || '源实体')} -> ${escapeHTML(detail.target?.name || '目标实体')}</p>
            </div>
        </div>
        <div class="knowledge-detail-section">
            <strong>关系说明</strong>
            <p>${escapeHTML(knowledgeRelationIntro(edge))}</p>
        </div>
        <div class="knowledge-detail-section">
            <strong>证据来源</strong>
            <pre class="knowledge-evidence">${escapeHTML(edge.evidence || '暂无证据')}</pre>
        </div>
        <div class="knowledge-detail-actions">
            <button class="btn-small" onclick="submitKnowledgeReviewCandidate('edge', ${jsStringArg(edge.id)})">提交审核</button>
        </div>
        <div class="knowledge-detail-section two-cols">
            <button class="knowledge-neighbor" onclick="renderKnowledgeNodeDetail(${jsStringArg(detail.source?.id || 0)})">${escapeHTML(detail.source?.name || '源实体')}<span>${escapeHTML(knowledgeTypeLabel(detail.source?.type || 'Concept'))}</span></button>
            <button class="knowledge-neighbor" onclick="renderKnowledgeNodeDetail(${jsStringArg(detail.target?.id || 0)})">${escapeHTML(detail.target?.name || '目标实体')}<span>${escapeHTML(knowledgeTypeLabel(detail.target?.type || 'Concept'))}</span></button>
        </div>
        <div id="knowledge-review-panel" class="knowledge-review-panel"></div>
    `;
    loadKnowledgeReviewCandidates();
}

function renderKnowledgeEmptyDetail() {
    const panel = document.getElementById('knowledge-detail-panel');
    if (!panel) return;
    panel.innerHTML = `
        <div class="knowledge-detail-empty">
            <strong>没有可展示的实体</strong>
            <span>可以先录入项目文档，或清空实体类型 / 关系类型过滤条件。</span>
        </div>
        <div id="knowledge-review-panel" class="knowledge-review-panel"></div>
    `;
    loadKnowledgeReviewCandidates();
}

async function submitKnowledgeReviewCandidate(itemType, itemID) {
    const reason = prompt('提交到图谱候选审核的原因：', '需要人工确认该图谱事实是否准确');
    if (reason === null) return;
    const resp = await knowledgeAPI.createReviewCandidate({
        itemType,
        itemID,
        reason,
        query: currentKnowledgeQueryOptions().query || '',
    });
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('已提交图谱审核候选', 'success');
        loadKnowledgeReviewCandidates();
    } else {
        showToast(resp?.message || resp?.data?.msg || '提交审核失败', 'error');
    }
}

async function loadKnowledgeReviewCandidates() {
    const panel = document.getElementById('knowledge-review-panel');
    if (!panel) return;
    panel.innerHTML = '<div class="empty-tip small">加载审核候选...</div>';
    const resp = await knowledgeAPI.reviewCandidates({ limit: 20 });
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        panel.innerHTML = `<div class="empty-tip small">审核候选不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const candidates = resp.data.candidates || [];
    panel.innerHTML = `
        <div class="knowledge-review-head">
            <strong>图谱候选审核</strong>
            <span>${candidates.length} / ${resp.data.total || candidates.length}</span>
        </div>
        ${candidates.length ? candidates.map(renderKnowledgeReviewCandidate).join('') : '<div class="empty-tip small">暂无候选</div>'}
    `;
}

function renderKnowledgeReviewCandidate(item) {
    const statusLabel = ({ pending: '待审核', approved: '已通过', rejected: '已拒绝' })[item.status] || item.status || '待审核';
    const canReview = item.status === 'pending';
    return `
        <div class="knowledge-review-item">
            <div>
                <strong>${escapeHTML(item.name || '图谱事实')}</strong>
                <span>${escapeHTML(item.item_type || '')} #${escapeHTML(String(item.item_id || ''))} · ${escapeHTML(statusLabel)}</span>
                <p>${escapeHTML(item.reason || item.summary || '')}</p>
            </div>
            ${canReview ? `
                <div class="knowledge-review-actions">
                    <button class="btn-small" onclick="reviewKnowledgeCandidate(${jsArg(item.id)}, 'approve')">通过</button>
                    <button class="btn-small danger-soft" onclick="reviewKnowledgeCandidate(${jsArg(item.id)}, 'reject')">拒绝</button>
                </div>
            ` : `<small>${escapeHTML(item.review_note || item.reviewed_at || '')}</small>`}
        </div>
    `;
}

async function reviewKnowledgeCandidate(id, action) {
    const note = prompt(action === 'approve' ? '通过说明：' : '拒绝原因：', '');
    if (note === null) return;
    const resp = await knowledgeAPI.reviewCandidate({ id, action, note });
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('审核状态已更新', 'success');
        loadKnowledgeReviewCandidates();
    } else {
        showToast(resp?.message || resp?.data?.msg || '审核失败', 'error');
    }
}

function resetKnowledgeGraphView() {
    const query = document.getElementById('knowledge-query');
    if (query) query.value = '';
    document.querySelectorAll('.knowledge-chip.active').forEach(btn => btn.classList.remove('active'));
    const hops = document.getElementById('knowledge-hop-select');
    if (hops) hops.value = '1';
    const community = document.getElementById('knowledge-community-select');
    if (community) community.value = '0';
    const documentSelect = document.getElementById('knowledge-document-select');
    if (documentSelect) documentSelect.value = '0';
    loadKnowledgeGraph();
}

function fitKnowledgeGraph() {
    if (knowledgeGraphInstance) {
        knowledgeGraphInstance.fitView(42);
    }
}

async function loadKnowledgeNeighborhood(nodeID) {
    const canvas = document.getElementById('knowledge-graph-canvas');
    if (canvas) canvas.innerHTML = '<div class="empty-tip">正在加载节点邻域...</div>';
    const options = currentKnowledgeQueryOptions();
    const resp = await knowledgeAPI.neighborhood(nodeID, options);
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        if (canvas) canvas.innerHTML = `<div class="empty-tip">邻域加载失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    knowledgeGraphCache = resp.data;
    knowledgePathSelection.path = null;
    syncKnowledgeCommunityOptions(resp.data.communities || []);
    renderKnowledgeGraph();
    renderKnowledgeNodeDetail(nodeID);
}

function setKnowledgePathPoint(kind, nodeID) {
    if (kind === 'source') {
        knowledgePathSelection.sourceID = idString(nodeID);
    } else {
        knowledgePathSelection.targetID = idString(nodeID);
    }
    const panel = document.getElementById('knowledge-detail-panel');
    const source = knowledgeGraphCache.nodes?.find(n => sameID(n.id, knowledgePathSelection.sourceID));
    const target = knowledgeGraphCache.nodes?.find(n => sameID(n.id, knowledgePathSelection.targetID));
    if (panel) {
        const notice = document.createElement('div');
        notice.className = 'knowledge-path-notice';
        notice.innerHTML = `路径：${escapeHTML(source?.name || '未选起点')} -> ${escapeHTML(target?.name || '未选终点')}`;
        panel.prepend(notice);
    }
    if (knowledgePathSelection.sourceID && knowledgePathSelection.targetID) {
        highlightKnowledgePath();
    }
}

async function highlightKnowledgePath() {
    const sourceID = idString(knowledgePathSelection.sourceID) || idString(knowledgeGraphSelected?.id);
    const targetID = idString(knowledgePathSelection.targetID);
    if (!sourceID || !targetID || sourceID === targetID) {
        showToast('请先在节点详情中设置不同的起点和终点');
        return;
    }
    const resp = await knowledgeAPI.path({
        sourceID,
        targetID,
        query: document.getElementById('knowledge-query')?.value.trim() || '',
        limit: 120,
        documentID: currentKnowledgeQueryOptions().documentID,
        hops: currentKnowledgeQueryOptions().hops,
    });
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        showToast(resp?.message || resp?.data?.msg || '未找到可见路径');
        return;
    }
    knowledgePathSelection.path = resp.data;
    applyKnowledgePathHighlight();
    renderKnowledgePathDetail(resp.data);
}

function applyKnowledgePathHighlight() {
    const path = knowledgePathSelection.path;
    if (!path || !knowledgeGraphInstance) return;
    const nodeSet = new Set((path.node_ids || []).map(String));
    const edgeSet = new Set((path.edge_ids || []).map(String));
    knowledgeGraphInstance.getNodes().forEach(node => {
        knowledgeGraphInstance.setItemState(node, 'selected', nodeSet.has(String(node.getModel().id)));
    });
    knowledgeGraphInstance.getEdges().forEach(edge => {
        knowledgeGraphInstance.setItemState(edge, 'selected', edgeSet.has(String(edge.getModel().id)));
    });
}

function renderKnowledgePathDetail(path) {
    const panel = document.getElementById('knowledge-detail-panel');
    if (!panel) return;
    const nodes = path.nodes || [];
    const edges = path.edges || [];
    panel.innerHTML = `
        <div class="knowledge-detail-head relation">
            <span class="knowledge-node-dot" style="background:#d97706"></span>
            <div>
                <h3>路径高亮</h3>
                <p>${nodes.map(n => escapeHTML(n.name || '')).join(' -> ')}</p>
            </div>
        </div>
        <div class="knowledge-detail-section">
            <strong>路径关系</strong>
            ${edges.length ? edges.map(edge => `
                <button class="knowledge-relation-row" onclick="renderKnowledgeEdgeDetail(${jsStringArg(edge.id)})">
                    <span>${escapeHTML(knowledgeRelationLabel(edge.relation || 'RELATED_TO'))}</span>
                    <small>${escapeHTML(edge.evidence || edge.description || '')}</small>
                </button>
            `).join('') : '<p>暂无路径关系</p>'}
        </div>
    `;
}

function resetChatView(message = '已删除会话，可通过搜索历史消息继续查找内容') {
    currentConversationID = null;
    currentConversationType = '';
    currentBotID = null;
    currentMessages = [];
    currentConversationRecipientCount = 0;
    clearPendingReply();
    conversationOpenSeq++;
    botChatSeq++;
    document.getElementById('chat-area').style.display = 'none';
    document.getElementById('welcome-area').style.display = 'flex';
    document.getElementById('message-list').innerHTML = '';
    document.getElementById('group-announcement-bar').style.display = 'none';
    document.getElementById('mention-btn').style.display = 'none';
    if (message) showToast(message, 'success');
}

function hideConversation(conversationID) {
    if (!confirm('确定从会话列表删除这个会话吗？历史消息仍可搜索。')) return;
    setConversationHidden(conversationID, true);
    delete unreadMap[conversationID];
    saveUnreadMap();
    updateUnreadBadge();
    if (sameID(currentConversationID, conversationID)) {
        resetChatView();
    } else {
        const msgList = document.getElementById('message-list');
        if (msgList) msgList.innerHTML = '';
    }
    loadConversations();
}

async function loadFriends() {
    const resp = await userAPI.getFriendList();
    const list = document.getElementById('friend-list');

    if (resp && resp.code === 0 && resp.data && resp.data.friends) {
        friendsCache = resp.data.friends;
        renderFriendListFromCache(resp.data.groups || null);
    } else {
        list.innerHTML = '<div class="empty-tip">暂无好友</div>';
    }
}

async function renderFriendListFromCache(preloadedGroups = null) {
    const list = document.getElementById('friend-list');
    if (!list) return;
    const friends = friendsCache || [];
    if (friends.length === 0) {
        list.innerHTML = '<div class="empty-tip">暂无好友<br><small>点击右上角「+ 添加」添加好友</small><br><button class="btn-inline" onclick="showCreateFriendGroup()">新建分组</button></div>';
        return;
    }

    let friendGroups = preloadedGroups;
    if (!Array.isArray(friendGroups)) {
        const groupsResp = await userAPI.getFriendGroups();
        friendGroups = groupsResp && groupsResp.code === 0 && groupsResp.data && groupsResp.data.groups ? groupsResp.data.groups : [];
    }
    const groupNames = { default: '默认分组' };
    friendGroups.forEach(g => { groupNames[String(g.id)] = g.name; });
    friends.forEach(f => {
        if (f.friend_id) {
            userNickCache[f.friend_id] = f.friend_name || '用户' + f.friend_id;
            if (f.remark) {
                friendRemarkCache[f.friend_id] = f.remark;
            } else {
                delete friendRemarkCache[f.friend_id];
            }
            if (f.friend_avatar) {
                userAvatarCache[f.friend_id] = f.friend_avatar;
            }
        }
    });

    const grouped = {};
    friends.forEach(friend => {
        const key = friendGroupKey(friend);
        if (!grouped[key]) grouped[key] = [];
        grouped[key].push(friend);
    });
    const orderedKeys = [
        'default',
        ...friendGroups.map(g => String(g.id)).filter(id => id !== '0'),
        ...Object.keys(grouped).filter(id => id !== 'default' && !friendGroups.some(g => sameID(g.id, id))),
    ].filter((key, idx, arr) => grouped[key] && arr.indexOf(key) === idx);

    const groupsBar = `
        <div class="friend-group-bar">
            <span>好友分组 ${orderedKeys.length}</span>
            <button class="btn-small-outline" onclick="showCreateFriendGroup()">新建分组</button>
        </div>
    `;
    list.innerHTML = groupsBar + orderedKeys.map(key => {
        const collapsed = !!friendGroupCollapsed[key];
        const items = grouped[key] || [];
        return `
            <section class="conversation-section friend-section ${collapsed ? 'collapsed' : ''}">
                <button class="conversation-section-header" onclick="toggleFriendGroup(${jsStringArg(key)})">
                    <span class="conversation-section-icon">F</span>
                    <span>${escapeHTML(groupNames[key] || '未命名分组')}</span>
                    <span class="conversation-section-count">${items.length}</span>
                    <span class="conversation-section-caret">${collapsed ? '+' : '-'}</span>
                </button>
                <div class="conversation-section-body">
                    ${collapsed ? '' : items.map(renderFriendItem).join('')}
                </div>
            </section>
        `;
    }).join('');
}

function renderFriendItem(f) {
    const statusClass = f.friend_status === 'online' ? 'online' : 'offline';
    const statusDot = f.friend_status === 'online' ? '●' : '○';
    const statusText = f.friend_status === 'online' ? '在线' : '离线';
    const displayName = f.remark || f.friend_name || '用户' + f.friend_id;
    const avatarHTML = renderAvatarHTML(f.friend_avatar, displayName.charAt(0).toUpperCase(), '');
    return `
        <div class="list-item friend-item">
            ${avatarHTML}
            <div class="list-item-info">
                <div class="list-item-name">${escapeHTML(displayName)}</div>
                <div class="list-item-msg ${statusClass}">${statusDot} ${statusText}</div>
            </div>
            <div class="friend-actions">
                <button class="btn-chat" onclick="showEditFriend(${jsArg(f.friend_id)}, ${jsStringArg(displayName)}, ${jsStringArg(f.remark || '')})">修改</button>
                <button class="btn-chat" onclick="startPrivateChat(${jsArg(f.friend_id)})">聊天</button>
                <button class="btn-delete-friend" onclick="deleteFriend(${jsArg(f.friend_id)}, ${jsStringArg(displayName)})" title="删除好友">✕</button>
            </div>
        </div>
    `;
}

async function loadGroups() {
    const resp = await groupAPI.list();
    const list = document.getElementById('group-list');

    if (resp && resp.code === 0 && resp.data && resp.data.groups) {
        groupsCache = resp.data.groups;
        const groups = groupsCache;
        if (groups.length === 0) {
            list.innerHTML = '<div class="empty-tip">暂无群组<br><small>点击右上角「加入」输入群号，或「+ 创建」创建群组</small></div>';
            return;
        }

        list.innerHTML = groups.map(g => {
            const avatarHTML = renderAvatarHTML(g.avatar, 'G', 'group-avatar');
            const ownerName = getUserName(g.owner_id);
            const isPinned = g.is_pinned;
            return `
                <div class="list-item group-item ${isPinned ? 'pinned' : ''}">
                    ${avatarHTML}
                    <div class="list-item-info">
                        <div class="list-item-name">${isPinned ? '置顶 · ' : ''}${escapeHTML(g.name)}</div>
                        <div class="list-item-msg">群号: ${escapeHTML(String(g.id))} · 群主: ${escapeHTML(ownerName)}</div>
                    </div>
                    <div class="group-actions">
                        <button class="btn-chat" onclick="openGroupConversation(${jsArg(g.id)})">进入</button>
                        <button class="btn-small-outline" onclick="copyGroupID(${jsArg(g.id)})">复制群号</button>
                        <button class="btn-small-outline" onclick="showGroupMembers(${jsArg(g.id)})">成员</button>
                        <button class="btn-small-outline" onclick="showGroupManage(${jsArg(g.id)})">管理</button>
                    </div>
                </div>
            `;
        }).join('');
    } else {
        list.innerHTML = '<div class="empty-tip">暂无群组</div>';
    }
}

async function resolveConversationName(conversationID, type) {
    if (conversationNameCache[conversationID]) {
        return conversationNameCache[conversationID];
    }

    try {
        if (type === 'private') {
            const convsResp = await messageAPI.getConversations();
            if (convsResp && convsResp.code === 0 && convsResp.data && convsResp.data.conversations) {
                const conv = convsResp.data.conversations.find(c => sameID(c.conversation_id, conversationID));
                if (conv) {
                    if (conv.target_name && !conv.target_name.startsWith('用户')) {
                        conversationNameCache[conversationID] = conv.target_name;
                        return conv.target_name;
                    }
                    if (conv.participant_ids) {
                        const otherID = conv.participant_ids.find(id => !sameID(id, currentUser.id));
                        if (otherID) {
                            await resolveUserNames([otherID]);
                            const name = getUserName(otherID);
                            conversationNameCache[conversationID] = name;
                            return name;
                        }
                    }
                }
            }
            const name = '私聊';
            conversationNameCache[conversationID] = name;
            return name;
        } else {
            const groupID = conversationGroupMap[conversationID];
            if (groupID) {
                const group = groupsCache.find(g => sameID(g.id, groupID));
                if (group) {
                    conversationNameCache[conversationID] = group.name;
                    return group.name;
                }
                const groupResp = await groupAPI.get(groupID);
                if (groupResp && groupResp.code === 0 && groupResp.data && groupResp.data.group && groupResp.data.group.success !== false) {
                    conversationNameCache[conversationID] = groupResp.data.group.name;
                    return groupResp.data.group.name;
                }
            }
            const convsResp = await messageAPI.getConversations();
            if (convsResp && convsResp.code === 0 && convsResp.data && convsResp.data.conversations) {
                const conv = convsResp.data.conversations.find(c => sameID(c.conversation_id, conversationID));
                if (conv && conv.target_name) {
                    conversationNameCache[conversationID] = conv.target_name;
                    return conv.target_name;
                }
                if (conv && conv.group_id && conv.group_id > 0) {
                    groupConversationMap[conv.group_id] = conversationID;
                    conversationGroupMap[conversationID] = conv.group_id;
                    const groupResp = await groupAPI.get(conv.group_id);
                    if (groupResp && groupResp.code === 0 && groupResp.data && groupResp.data.group && groupResp.data.group.success !== false) {
                        conversationNameCache[conversationID] = groupResp.data.group.name;
                        return groupResp.data.group.name;
                    }
                }
            }
            const name = '群聊 #' + conversationID;
            conversationNameCache[conversationID] = name;
            return name;
        }
    } catch (e) {
        return type === 'private' ? '私聊' : '群聊';
    }
}

async function openConversation(conversationID, type, isDeletedGroup = false) {
    activateChatWorkspaceForContent();
    const openSeq = ++conversationOpenSeq;
    botChatSeq++;
    const targetConversationID = conversationID;
    currentConversationID = conversationID;
    currentConversationType = type;
    currentBotID = null;
    currentMessages = [];
    currentConversationRecipientCount = 0;
    clearPendingReply();
    setActiveConversationHighlight(conversationID);

    delete unreadMap[conversationID];
    saveUnreadMap();
    updateUnreadBadge();

    document.getElementById('welcome-area').style.display = 'none';
    document.getElementById('chat-area').style.display = 'flex';

    const sendBtn = document.getElementById('send-btn');
    sendBtn.setAttribute('onclick', 'sendMessage()');
    document.getElementById('msg-input').disabled = false;
    sendBtn.disabled = false;
    document.getElementById('voice-record-btn').disabled = false;
    document.getElementById('msg-input').placeholder = '输入消息...';
    document.getElementById('broadcast-btn').style.display = type === 'group' ? 'inline-flex' : 'none';
    document.getElementById('mention-btn').style.display = type === 'group' ? 'inline-flex' : 'none';

    const convName = await resolveConversationName(conversationID, type);
    if (openSeq !== conversationOpenSeq || !sameID(currentConversationID, targetConversationID) || currentBotID !== null) return;
    document.getElementById('chat-title').textContent = convName;
    const typeLabel = isDeletedGroup ? '群聊已解散' : (type === 'private' ? '私聊' : '群聊');
    document.getElementById('chat-type-badge').textContent = typeLabel;
    document.getElementById('chat-type-badge').className = `chat-type-badge ${type}`;
    document.getElementById('msg-input').disabled = !!isDeletedGroup;
    document.getElementById('send-btn').disabled = !!isDeletedGroup;
    document.getElementById('voice-record-btn').disabled = !!isDeletedGroup;

    if (isDeletedGroup) {
        document.getElementById('group-announcement-bar').style.display = 'none';
        const msgList = document.getElementById('message-list');
        msgList.innerHTML = '<div class="empty-tip deleted-group-tip">此群聊已解散，不能继续发送消息。</div>';
        return;
    }

    await updateCurrentRecipientCount(conversationID);
    if (openSeq !== conversationOpenSeq || !sameID(currentConversationID, targetConversationID) || currentBotID !== null) return;
    if (type === 'private' && conversationHasAgentTarget(conversationID)) {
        document.getElementById('msg-input').placeholder = '向智能助手发送消息...';
    }
    renderAgentNativeStatus();
    renderAgentContextSidebar();

    const announcementBar = document.getElementById('group-announcement-bar');
    if (type === 'group') {
        let groupID = conversationGroupMap[conversationID];
        if (!groupID) {
            const convsResp = await messageAPI.getConversations();
            if (convsResp && convsResp.code === 0 && convsResp.data && convsResp.data.conversations) {
                const conv = convsResp.data.conversations.find(c => sameID(c.conversation_id, conversationID));
                if (conv && conv.group_id && conv.group_id > 0) {
                    groupID = conv.group_id;
                    groupConversationMap[groupID] = conversationID;
                    conversationGroupMap[conversationID] = groupID;
                }
            }
        }
        if (groupID) {
            let group = groupsCache.find(g => sameID(g.id, groupID));
            if (!group) {
                const groupResp = await groupAPI.get(groupID);
                if (openSeq !== conversationOpenSeq || !sameID(currentConversationID, targetConversationID) || currentBotID !== null) return;
                if (groupResp && groupResp.code === 0 && groupResp.data && groupResp.data.group && groupResp.data.group.success !== false) {
                    group = groupResp.data.group;
                }
            }
            if (group && group.announcement) {
                document.getElementById('announcement-text').textContent = group.announcement;
                announcementBar.style.display = 'flex';
            } else {
                announcementBar.style.display = 'none';
            }
        } else {
            announcementBar.style.display = 'none';
        }
    } else {
        announcementBar.style.display = 'none';
    }

    const resp = await messageAPI.getHistory(conversationID);
    const msgList = document.getElementById('message-list');
    if (openSeq !== conversationOpenSeq || !sameID(currentConversationID, targetConversationID) || currentBotID !== null) return;

    if (resp && resp.code === 0 && resp.data && resp.data.messages) {
        const messages = resp.data.messages;
        const cached = getCachedMessages(conversationID);
        currentMessages = cached.length ? mergeMessagesByIdentity(cached, messages) : messages;
        cacheMessages(conversationID, currentMessages);
        const senderIDs = [...new Set(messages.map(m => m.sender_id))];
        await resolveUserNames(senderIDs);
        if (openSeq !== conversationOpenSeq || !sameID(currentConversationID, targetConversationID) || currentBotID !== null) return;

        msgList.innerHTML = messages.map(m => createMessageHTML(m)).join('');
        hydrateMedia(msgList);
        msgList.scrollTop = msgList.scrollHeight;
        const lastMsg = messages[messages.length - 1];
        if (lastMsg && lastMsg.id) {
            await messageAPI.markRead(conversationID, lastMsg.id);
            loadConversations();
        }
        const refreshed = await messageAPI.getHistory(conversationID);
        if (refreshed && refreshed.code === 0 && refreshed.data && refreshed.data.messages) {
            currentMessages = mergeMessagesByIdentity(currentMessages, refreshed.data.messages);
            cacheMessages(conversationID, currentMessages);
        }
        if (openSeq !== conversationOpenSeq || !sameID(currentConversationID, targetConversationID) || currentBotID !== null) return;
        renderCurrentMessages();
    } else {
        currentMessages = [];
        msgList.innerHTML = '<div class="empty-tip">暂无消息，发送第一条消息吧</div>';
        renderAgentNativeStatus();
        renderAgentContextSidebar();
    }

    loadConversations();
}

function createMessageHTML(m) {
    if (m.is_thinking) {
        const thinkingIDAttr = m._thinkingID ? ` data-thinking-id="${m._thinkingID}"` : '';
        const elapsedHTML = m.started_at ? `<span class="agent-thinking-inline" data-started-at="${escapeHTML(String(m.started_at))}">已思考 0.0 秒</span>` : '';
        return `
            <div class="message-item received bot-msg msg-thinking"${thinkingIDAttr}>
                <div class="msg-avatar received agent-avatar">A</div>
                <div class="msg-body">
                    <div class="msg-meta">
                        <span class="message-sender">智能助手</span>
                        ${elapsedHTML}
                    </div>
                    <div class="message-bubble thinking"><div class="spinner"></div> ${escapeHTML(m.content || '智能助手处理中...')}</div>
                </div>
            </div>
        `;
    }
    if (m.is_approval && m.approval) {
        const senderName = '智能助手';
        const time = m.created_at || '';
        return `
            <div class="message-item received bot-msg">
                <div class="msg-avatar received agent-avatar">A</div>
                <div class="msg-body">
                    <div class="msg-meta">
                        <span class="message-sender">${senderName}</span>
                        <span class="message-time">${time}</span>
                    </div>
                    ${renderAgentApprovalCard(m.approval, { action: 'run' })}
                </div>
            </div>
        `;
    }
    const isSent = sameID(m.sender_id, currentUser.id);
    const agentBot = getAgentBotByUserID(m.sender_id);
    const isBot = sameID(m.sender_id, 0) || m.is_bot || !!agentBot;
    const senderName = agentBot ? getBotDisplayName(agentBot) : (isBot ? '智能助手' : (isSent ? '我' : getUserName(m.sender_id)));
    const time = m.created_at || '';
    const avatarContent = agentBot && agentBot.avatar ? safeImageHTML(agentBot.avatar) : (isBot ? 'A' : (isSent
        ? (currentUser.avatar ? safeImageHTML(currentUser.avatar) : (currentUser.nickname || currentUser.username).charAt(0).toUpperCase())
        : getUserAvatarHTML(m.sender_id)));
    const avatarBg = isSent ? '' : 'received';
    const mentionableAvatar = !isSent && currentConversationType === 'group' && m.sender_id && !sameID(m.sender_id, 0);
    const avatarMentionAttrs = mentionableAvatar
        ? ` title="长按或右键 @ ${escapeHTML(senderName)}" onpointerdown="startAvatarMentionPress(event, ${jsArg(m.sender_id)}, ${jsStringArg(senderName)})" onpointerup="cancelAvatarMentionPress()" onpointerleave="cancelAvatarMentionPress()" oncontextmenu="event.preventDefault(); mentionFromAvatar(${jsArg(m.sender_id)}, ${jsStringArg(senderName)})"`
        : '';
    const messageID = m.id || m.msg_id || 0;
    const status = m.status || 'sent';
    const originalContent = status === 'recalled' ? '[消息已撤回]' : (m.content || '');
    const replyMsg = m.reply_to_id ? getMessageByID(m.reply_to_id) : null;
    const replyHTML = replyMsg ? `
        <div class="reply-card">
            <strong>${escapeHTML(getUserName(replyMsg.sender_id))}</strong>
            <span>${escapeHTML((replyMsg.content || '[媒体消息]').slice(0, 80))}</span>
        </div>
    ` : '';
    const editedHTML = m.is_edited ? '<span class="message-edited">已编辑</span>' : '';
    const agentDurationHTML = m.agent_thinking_duration_ms ? `<span class="agent-thinking-duration">思考 ${(Number(m.agent_thinking_duration_ms) / 1000).toFixed(1)} 秒</span>` : '';
    const translation = messageTranslations[String(messageID)];
    const translationHTML = translation ? `<div class="message-translation"><div class="translation-label">译文 · ${escapeHTML(translation.target_language || '中文')}</div>${renderMarkdownText(translation.translated_text || '')}</div>` : '';
    const actionsHTML = (!isBot && status !== 'recalled' && messageID) ? `
        <div class="message-actions">
            <button type="button" onclick="translateMessage(${jsArg(messageID)}, this)">翻译</button>
            <button type="button" onclick="setPendingReply(${jsArg(messageID)})">回复</button>
            <button type="button" onclick="deleteLocalMessage(${jsArg(messageID)})">删除</button>
            ${isSent ? `<button type="button" onclick="editMessage(${jsArg(messageID)})">编辑</button><button type="button" onclick="recallMessage(${jsArg(messageID)})">撤回</button>` : ''}
        </div>
    ` : '';
    const bubbleContent = status === 'recalled' ? escapeHTML(originalContent) : renderMessageContent(originalContent, m.msg_type, { markdown: isBot });
    const errorClass = m.is_error ? 'error-bubble' : '';
    return `
        <div class="message-item ${isSent ? 'sent' : 'received'} ${isBot ? 'bot-msg' : ''}" data-message-id="${messageID}">
            <div class="msg-avatar ${avatarBg} ${mentionableAvatar ? 'mentionable-avatar' : ''}"${avatarMentionAttrs}>${avatarContent}</div>
            <div class="msg-body">
                <div class="msg-meta">
                    <span class="message-sender">${escapeHTML(senderName)}</span>
                    <span class="message-time">${time}</span>
                    ${editedHTML}
                    ${agentDurationHTML}
                    ${readReceiptHTML(m)}
                </div>
                <div class="message-bubble ${errorClass} ${status === 'recalled' ? 'recalled-bubble' : ''}">${replyHTML}${bubbleContent}${translationHTML}</div>
                ${actionsHTML}
            </div>
        </div>
    `;
}

function messageIdentity(m) {
    if (!m) return '';
    const id = m.id || m.msg_id;
    if (id) return `id:${id}`;
    if (m.client_msg_id) return `client:${m.client_msg_id}`;
    return '';
}

function appendMessage(m) {
    if (m.msg_id && !m.id) {
        m.id = m.msg_id;
    }
    if (m.conversation_id && currentConversationID && !sameID(m.conversation_id, currentConversationID)) {
        return;
    }
    const identity = messageIdentity(m);
    if (identity && currentMessages.some(item => messageIdentity(item) === identity)) {
        if (m.id || m.msg_id) {
            currentMessages = currentMessages.map(item => messageIdentity(item) === identity ? { ...item, ...m } : item);
            cacheMessages(currentConversationID, currentMessages);
            renderCurrentMessages();
        }
        return;
    }
    const msgList = document.getElementById('message-list');
    if (msgList.querySelector('.empty-tip')) {
        msgList.innerHTML = '';
    }
    if (m.sender_id && !userNickCache[m.sender_id]) {
        resolveUserNames([m.sender_id]).then(() => {
            currentMessages.push(m);
            cacheMessages(currentConversationID, currentMessages);
            msgList.innerHTML += createMessageHTML(m);
            hydrateMedia(msgList);
            msgList.scrollTop = msgList.scrollHeight;
        });
    } else {
        currentMessages.push(m);
        cacheMessages(currentConversationID, currentMessages);
        msgList.innerHTML += createMessageHTML(m);
        hydrateMedia(msgList);
        msgList.scrollTop = msgList.scrollHeight;
    }
}

async function translateMessage(messageID, buttonEl = null) {
    if (!messageID) return;
    const oldText = buttonEl ? buttonEl.textContent : '';
    if (buttonEl) {
        buttonEl.disabled = true;
        buttonEl.textContent = '翻译中';
    }
    const resp = await messageAPI.translate(messageID, '中文', false);
    if (buttonEl) {
        buttonEl.disabled = false;
        buttonEl.textContent = oldText || '翻译';
    }
    if (resp && resp.code === 0 && resp.data?.success) {
        const translation = resp.data.translation;
        messageTranslations[String(messageID)] = translation;
        renderCurrentMessages();
        showToast(translation.cached ? '已显示缓存译文' : '翻译完成', 'success');
    } else {
        showToast(resp?.message || resp?.data?.msg || '翻译失败', 'error');
    }
}

async function copyGroupID(groupID) {
    try {
        await navigator.clipboard.writeText(String(groupID));
        showToast('群号已复制', 'success');
    } catch (err) {
        showToast('群号: ' + groupID, 'info');
    }
}

function updateAgentThinkingTimers() {
    const now = Date.now();
    document.querySelectorAll('.agent-thinking-inline[data-started-at]').forEach(el => {
        const startedAt = Number(el.dataset.startedAt || 0);
        if (!startedAt) return;
        el.textContent = `已思考 ${((now - startedAt) / 1000).toFixed(1)} 秒`;
    });
}

setInterval(updateAgentThinkingTimers, 250);

function conversationHasAgentTarget(conversationID) {
    if (!conversationID) return false;
    const participants = conversationParticipantCache[String(conversationID)] || [];
    return participants.some(id => !sameID(id, currentUser.id) && isAgentUser(id));
}

function shouldExpectAgentReply(conversationID, content, mentionUserIDs = []) {
    if (!conversationID) return false;
    if (currentConversationType === 'private' && conversationHasAgentTarget(conversationID)) {
        return true;
    }
    return (mentionUserIDs || []).some(id => isAgentUser(id));
}

function addPendingAgentThinking(conversationID, label = '智能助手正在结合会话上下文思考...') {
    const key = String(conversationID || '');
    if (!key) return;
    const startedAt = Date.now();
    const thinkingID = `agent-thinking-${key}-${startedAt}-${Math.random().toString(16).slice(2)}`;
    const pending = { thinkingID, startedAt };
    if (!Array.isArray(pendingAgentThinkingByConversation[key])) {
        pendingAgentThinkingByConversation[key] = [];
    }
    pendingAgentThinkingByConversation[key].push(pending);
    if (sameID(currentConversationID, conversationID) && currentBotID === null) {
        appendMessage({
            sender_id: 0,
            conversation_id: conversationID,
            content: label,
            is_thinking: true,
            _thinkingID: thinkingID,
            started_at: startedAt,
        });
    }
}

function finishPendingAgentThinking(conversationID, agentUserID) {
    const key = String(conversationID || '');
    const queue = pendingAgentThinkingByConversation[key];
    const pending = Array.isArray(queue) ? queue.shift() : queue;
    if (!pending) return 0;
    const durationMs = Date.now() - pending.startedAt;
    if (Array.isArray(queue) && queue.length > 0) {
        pendingAgentThinkingByConversation[key] = queue;
    } else {
        delete pendingAgentThinkingByConversation[key];
    }
    const container = document.getElementById('message-list');
    const thinkingEl = container ? container.querySelector(`[data-thinking-id="${pending.thinkingID}"]`) : null;
    if (thinkingEl) thinkingEl.remove();
    setAgentNativeState(conversationID, 'completed', `Agent 已完成本次处理，用时 ${(durationMs / 1000).toFixed(1)} 秒。`);
    return durationMs;
}

async function sendMessage() {
    const input = document.getElementById('msg-input');
    const content = input.value.trim();
    if (!content || !currentConversationID) return;

    const sendConversationID = currentConversationID;
    const replyToID = pendingReplyMessage ? (pendingReplyMessage.id || pendingReplyMessage.msg_id || 0) : 0;
    const mentionUserIDs = extractMentions(content);
    const mentionAll = content.includes('@all') || content.includes('@所有人');
    input.value = '';
    delete pendingMentionTargets[String(sendConversationID || '')];

    const now = new Date();
    const timeStr = now.getFullYear() + '-' +
        String(now.getMonth() + 1).padStart(2, '0') + '-' +
        String(now.getDate()).padStart(2, '0') + ' ' +
        String(now.getHours()).padStart(2, '0') + ':' +
        String(now.getMinutes()).padStart(2, '0') + ':' +
        String(now.getSeconds()).padStart(2, '0');

    const optimisticMsg = {
        sender_id: currentUser.id,
        conversation_id: sendConversationID,
        content: content,
        msg_type: 'text',
        created_at: timeStr,
        reply_to_id: replyToID,
        mention_user_ids: mentionUserIDs,
        mention_all: mentionAll,
        read_count: 0,
        recipient_count: currentConversationRecipientCount,
    };

    const resp = await messageAPI.send(sendConversationID, content, 'text', {
        reply_to_id: replyToID,
        mention_user_ids: mentionUserIDs,
        mention_all: mentionAll,
    });
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        optimisticMsg.id = resp.data.msg_id;
        clearPendingReply();
        appendMessage(optimisticMsg);
        if (shouldExpectAgentReply(sendConversationID, content, mentionUserIDs)) {
            setAgentNativeState(sendConversationID, 'thinking', '已收到 IM 事件，正在结合会话上下文处理。');
            addPendingAgentThinking(sendConversationID);
        }
        setConversationHidden(sendConversationID, false);
        loadConversations();
    } else {
        const errMsg = resp?.data?.msg || '发送失败';
        showToast(errMsg, 'error');
        if (errMsg.includes('群组不存在') || errMsg.includes('无权访问')) {
            const groupID = conversationGroupMap[sendConversationID];
            if (groupID) {
                delete groupConversationMap[groupID];
                delete conversationGroupMap[sendConversationID];
            }
            await loadGroups();
            loadConversations();
        }
    }
}

async function editMessage(messageID) {
    const msg = getMessageByID(messageID);
    if (!msg || msg.status === 'recalled') return;
    showModal('编辑消息', `
        <div class="form-group">
            <label>消息内容</label>
            <textarea id="edit-message-content" maxlength="2000">${escapeHTML(msg.content || '')}</textarea>
        </div>
        <button class="btn-primary" onclick="saveEditedMessage(${jsArg(messageID)})">保存修改</button>
    `);
    setTimeout(() => document.getElementById('edit-message-content')?.focus(), 0);
}

async function sendBroadcastMessage() {
    const input = document.getElementById('msg-input');
    const content = input.value.trim();
    if (!content || !currentConversationID) return;
    if (currentConversationType !== 'group') {
        showToast('广播消息仅支持群聊', 'warning');
        return;
    }

    const sendConversationID = currentConversationID;
    const replyToID = pendingReplyMessage ? (pendingReplyMessage.id || pendingReplyMessage.msg_id || 0) : 0;
    input.value = '';

    const now = new Date();
    const timeStr = now.getFullYear() + '-' +
        String(now.getMonth() + 1).padStart(2, '0') + '-' +
        String(now.getDate()).padStart(2, '0') + ' ' +
        String(now.getHours()).padStart(2, '0') + ':' +
        String(now.getMinutes()).padStart(2, '0') + ':' +
        String(now.getSeconds()).padStart(2, '0');

    const resp = await messageAPI.send(sendConversationID, content, 'broadcast', {
        reply_to_id: replyToID,
        mention_all: true,
    });
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        clearPendingReply();
        appendMessage({
            id: resp.data.msg_id,
            msg_id: resp.data.msg_id,
            sender_id: currentUser.id,
            conversation_id: sendConversationID,
            content,
            msg_type: 'broadcast',
            created_at: timeStr,
            reply_to_id: replyToID,
            mention_all: true,
            read_count: 0,
            recipient_count: currentConversationRecipientCount,
        });
        setConversationHidden(sendConversationID, false);
        loadConversations();
        showToast('广播已发送', 'success');
    } else {
        showToast(resp?.data?.msg || '广播发送失败', 'error');
    }
}

async function saveEditedMessage(messageID) {
    const input = document.getElementById('edit-message-content');
    const content = input ? input.value.trim() : '';
    if (!content) {
        showToast('消息内容不能为空', 'warning');
        return;
    }
    const resp = await messageAPI.edit(messageID, content);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        applyMessageStateUpdate(resp.data.message || { msg_id: messageID, content, is_edited: true });
        closeModal();
        showToast('消息已编辑', 'success');
    } else {
        showToast(resp?.data?.msg || '编辑失败', 'error');
    }
}

async function recallMessage(messageID) {
    if (!confirm('确定撤回这条消息吗？')) return;
    const resp = await messageAPI.recall(messageID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        applyMessageStateUpdate({ msg_id: messageID, content: '', status: 'recalled' });
        showToast('消息已撤回', 'success');
    } else {
        showToast(resp?.data?.msg || '撤回失败', 'error');
    }
}

async function deleteLocalMessage(messageID) {
    if (!currentConversationID || !messageID) return;
    if (!confirm('只在你的本地消息历史中删除这条消息，其他人仍可看到。继续吗？')) return;
    const resp = await messageAPI.deleteLocal(currentConversationID, messageID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        currentMessages = currentMessages.filter(m => !sameID(m.id || m.msg_id, messageID));
        renderCurrentMessages();
        showToast('本地消息已删除', 'success');
        loadConversations();
    } else {
        showToast(resp?.data?.msg || '删除失败', 'error');
    }
}

async function startPrivateChat(friendID) {
    const resp = await messageAPI.createConversation('private', [currentUser.id, friendID]);
    if (resp && resp.code === 0 && resp.data) {
        const convId = resp.data.conversation_id;
        const friendName = getUserName(friendID);
        conversationNameCache[convId] = friendName;
        openConversation(convId, 'private');
        switchSidebar('conversations', document.querySelector('.sidebar-tab:first-child'));
    } else {
        showToast('创建会话失败', 'error');
    }
}

async function openGroupConversation(groupID) {
    if (groupConversationMap[groupID]) {
        const group = groupsCache.find(g => sameID(g.id, groupID));
        if (group) conversationNameCache[groupConversationMap[groupID]] = group.name;
        openConversation(groupConversationMap[groupID], 'group');
        switchSidebar('conversations', document.querySelector('.sidebar-tab:first-child'));
        return;
    }

    const convsResp = await messageAPI.getConversations();
    if (convsResp && convsResp.code === 0 && convsResp.data && convsResp.data.conversations) {
        const existing = convsResp.data.conversations.find(c => c.type === 'group' && sameID(c.group_id, groupID));
        if (existing) {
            groupConversationMap[groupID] = existing.conversation_id;
            conversationGroupMap[existing.conversation_id] = groupID;
            const group = groupsCache.find(g => sameID(g.id, groupID));
            if (group) conversationNameCache[existing.conversation_id] = group.name;
            openConversation(existing.conversation_id, 'group');
            switchSidebar('conversations', document.querySelector('.sidebar-tab:first-child'));
            return;
        }
    }

    const membersResp = await groupAPI.getMembers(groupID);
    if (!membersResp || membersResp.code !== 0 || !membersResp.data || !membersResp.data.members) {
        showToast('获取群成员失败', 'error');
        return;
    }

    const memberIDs = membersResp.data.members.map(m => m.user_id);
    const resp = await messageAPI.createConversation('group', memberIDs, groupID);
    if (resp && resp.code === 0 && resp.data) {
        const convId = resp.data.conversation_id;
        groupConversationMap[groupID] = convId;
        conversationGroupMap[convId] = groupID;

        const group = groupsCache.find(g => sameID(g.id, groupID));
        if (group) {
            conversationNameCache[convId] = group.name;
        }

        openConversation(convId, 'group');
        switchSidebar('conversations', document.querySelector('.sidebar-tab:first-child'));
    } else {
        showToast('创建群聊会话失败', 'error');
    }
}

async function deleteFriend(friendID, name) {
    if (!confirm(`确定要删除好友「${name}」吗？`)) return;
    const resp = await userAPI.deleteFriend(friendID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已删除好友', 'success');
        loadFriends();
    } else {
        showToast(resp?.data?.msg || '删除失败', 'error');
    }
}

function showEditFriend(friendID, displayName, currentRemark) {
    showModal(`修改好友 - ${escapeHTML(displayName)}`, `
        <div class="form-group">
            <label>好友备注</label>
            <input type="text" id="edit-friend-remark" value="${escapeHTML(currentRemark || displayName || '')}" placeholder="留空则显示好友昵称">
        </div>
        <div class="form-group">
            <label>好友分组</label>
            <select id="edit-friend-group" class="form-select">
                <option value="0">默认分组</option>
            </select>
        </div>
        <button class="btn-primary" onclick="saveFriendRemark(${jsArg(friendID)})">保存</button>
    `);
    loadFriendGroupOptions('edit-friend-group', friendID);
}

async function saveFriendRemark(friendID) {
    const remark = document.getElementById('edit-friend-remark').value.trim();
    const groupID = document.getElementById('edit-friend-group')?.value || 0;
    const resp = await userAPI.updateFriendRemark(friendID, groupID, remark);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        const friend = friendsCache.find(f => sameID(f.friend_id, friendID));
        const fallbackName = friend?.friend_name || userNickCache[friendID] || '用户' + friendID;
        userNickCache[friendID] = fallbackName;
        if (remark) {
            friendRemarkCache[friendID] = remark;
        } else {
            delete friendRemarkCache[friendID];
        }
        Object.keys(conversationNameCache).forEach(convID => {
            delete conversationNameCache[convID];
        });
        showToast('好友信息已更新', 'success');
        closeModal();
        loadFriends();
        loadConversations();
    } else {
        showToast(resp?.data?.msg || '更新好友信息失败', 'error');
    }
}

async function loadFriendGroupOptions(selectID, friendID = 0) {
    const select = document.getElementById(selectID);
    if (!select) return;
    const resp = await userAPI.getFriendGroups();
    if (!(resp && resp.code === 0 && resp.data && resp.data.groups)) return;
    const friend = friendsCache.find(f => sameID(f.friend_id, friendID));
    const currentGroupID = friend ? String(friend.group_id || 0) : '0';
    select.innerHTML = '<option value="0">默认分组</option>' + resp.data.groups.map(g =>
        `<option value="${escapeHTML(String(g.id))}" ${sameID(g.id, currentGroupID) ? 'selected' : ''}>${escapeHTML(g.name)}</option>`
    ).join('');
}

function showAddFriend() {
    showModal('添加好友', `
        <div class="form-group">
            <label>好友用户ID</label>
            <input type="number" id="add-friend-id" placeholder="请输入用户ID">
        </div>
        <div class="form-group">
            <label>好友分组</label>
            <select id="add-friend-group" class="form-select">
                <option value="0">默认分组</option>
            </select>
        </div>
        <div class="form-group">
            <label>备注</label>
            <input type="text" id="add-friend-remark" placeholder="备注（可选）">
        </div>
        <button class="btn-primary" onclick="addFriend()">添加</button>
    `);
    loadFriendGroupOptions('add-friend-group');
}

function showCreateFriendGroup() {
    showModal('新建好友分组', `
        <div class="form-group">
            <label>分组名称</label>
            <input type="text" id="friend-group-name" placeholder="例如：同学、项目组、家人" onkeydown="if(event.key==='Enter')createFriendGroup()">
        </div>
        <button class="btn-primary" onclick="createFriendGroup()">创建分组</button>
    `);
    setTimeout(() => document.getElementById('friend-group-name')?.focus(), 0);
}

async function createFriendGroup() {
    const name = document.getElementById('friend-group-name')?.value.trim() || '';
    if (!name) {
        showToast('请输入分组名称', 'warning');
        return;
    }
    const exists = await friendGroupNameExists(name);
    if (exists) {
        showToast('分组已存在', 'warning');
        return;
    }
    const resp = await userAPI.createFriendGroup(name);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('分组已创建', 'success');
        closeModal();
        loadFriends();
    } else {
        showToast(resp?.data?.msg || '创建失败', 'error');
    }
}

async function friendGroupNameExists(name) {
    const resp = await userAPI.getFriendGroups();
    if (!(resp && resp.code === 0 && resp.data && resp.data.groups)) return false;
    return resp.data.groups.some(g => (g.name || '').trim().toLowerCase() === name.trim().toLowerCase());
}

async function addFriend() {
    const friendID = document.getElementById('add-friend-id').value.trim();
    const remark = document.getElementById('add-friend-remark').value.trim();
    const groupID = document.getElementById('add-friend-group')?.value || 0;
    if (!/^\d{10}$/.test(friendID)) {
        showToast('请输入有效的用户ID', 'warning');
        return;
    }
    const resp = await userAPI.addFriend(friendID, groupID, remark);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        closeModal();
        loadFriends();
        showToast('添加好友成功', 'success');
    } else {
        showToast(resp?.data?.msg || '添加失败', 'error');
    }
}

function showCreateGroup() {
    showModal('创建群组', `
        <div class="form-group">
            <label>群组名称</label>
            <input type="text" id="create-group-name" placeholder="请输入群组名称">
        </div>
        <div class="form-group">
            <label>成员用户ID（多个用逗号分隔）</label>
            <input type="text" id="create-group-members" placeholder="例如: 1,2,3">
        </div>
        <button class="btn-primary" onclick="createGroup()">创建</button>
    `);
}

function showJoinGroup() {
    showModal('通过群号加入', `
        <div class="form-group">
            <label>群号</label>
            <input type="text" id="join-group-id" inputmode="numeric" maxlength="10" placeholder="请输入10位群号">
        </div>
        <button class="btn-primary" onclick="joinGroupByID()">加入群聊</button>
    `);
}

async function joinGroupByID() {
    const input = document.getElementById('join-group-id');
    const groupID = input ? input.value.trim() : '';
    if (!/^\d{10}$/.test(groupID)) {
        showToast('请输入10位群号', 'warning');
        return;
    }
    const resp = await groupAPI.join(groupID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        closeModal();
        await loadGroups();
        showToast(resp.data.msg || '加入群聊成功', 'success');
        openGroupConversation(groupID);
    } else {
        showToast(resp?.data?.msg || resp?.message || '加入群聊失败', 'error');
    }
}

async function createGroup() {
    const name = document.getElementById('create-group-name').value.trim();
    const membersStr = document.getElementById('create-group-members').value.trim();
    const memberIDs = parseIDList(membersStr);
    if (!name) {
        showToast('请输入群组名称', 'warning');
        return;
    }
    if (memberIDs.length < 2) {
        showToast('群聊至少需要3人（包括创建者），2人请使用私聊', 'warning');
        return;
    }
    const resp = await groupAPI.create(name, memberIDs);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        closeModal();
        loadGroups();
        showToast('群组创建成功', 'success');
    } else {
        showToast(resp?.data?.msg || '创建失败', 'error');
    }
}

async function showGroupMembers(groupID) {
    const groupResp = await groupAPI.get(groupID);
    const membersResp = await groupAPI.getMembers(groupID);

    let groupName = '群组';
    let isOwner = false;
    let isAdmin = false;
    let myRole = '';
    if (groupResp && groupResp.code === 0 && groupResp.data && groupResp.data.group) {
        groupName = groupResp.data.group.name;
        isOwner = sameID(groupResp.data.group.owner_id, currentUser.id);
    }

    let membersHTML = '<div class="empty-tip">加载中...</div>';
    if (membersResp && membersResp.code === 0 && membersResp.data && membersResp.data.members) {
        const members = membersResp.data.members;
        groupMembersCache[groupID] = members;
        if (members.length === 0) {
            membersHTML = '<div class="empty-tip">暂无成员</div>';
        } else {
            const memberIDs = members.map(m => m.user_id);
            await resolveUserNames(memberIDs);
            members.forEach(m => {
                if ((m.username || m.nickname) && !friendRemarkCache[m.user_id]) {
                    userNickCache[m.user_id] = m.nickname || m.username;
                }
                if (m.avatar) {
                    userAvatarCache[m.user_id] = m.avatar;
                }
            });

            const myMember = members.find(m => sameID(m.user_id, currentUser.id));
            myRole = myMember ? myMember.role : '';
            isAdmin = myRole === 'admin';
            const canManage = isOwner || isAdmin;

            membersHTML = `
                <div class="section-label">群成员 (${members.length})</div>
                <div class="member-list">
                    ${members.map(m => {
                        const roleClass = m.role === 'owner' ? 'owner' : (m.role === 'admin' ? 'admin' : '');
                        const agent = getAgentBotByUserID(m.user_id);
                        const roleLabel = m.role === 'owner' ? '群主' : (m.role === 'admin' ? '管理员' : '成员');
                        const canKick = !sameID(currentUser.id, m.user_id) && m.role !== 'owner' && (isOwner || (isAdmin && m.role === 'member'));
                        const canMute = canManage && !sameID(currentUser.id, m.user_id) && m.role !== 'owner';
                        const canSetRole = isOwner && m.role !== 'owner';
                        const memberName = agent ? getBotDisplayName(agent) : getUserName(m.user_id);
                        const avatarHTML = agent && agent.avatar ? safeImageHTML(agent.avatar, 'avatar-img small') : getUserAvatarHTML(m.user_id, 'small');
                        const isMuted = m.muted_until && m.muted_until !== '';
                        return `
                            <div class="member-item ${isMuted ? 'muted' : ''}">
                                <div class="avatar small">${avatarHTML}</div>
                                <div class="member-info">
                                    <span class="member-name">${escapeHTML(memberName)}</span>
                                    <span class="member-tag ${roleClass}">${roleLabel}</span>
                                    ${agent ? '<span class="member-tag agent-tag">智能助手</span>' : ''}
                                    ${isMuted ? '<span class="member-tag muted-tag">禁言中</span>' : ''}
                                </div>
                                <div class="member-actions">
                                    <button class="btn-kick" onclick="insertMention(${jsArg(m.user_id)}, ${jsStringArg(memberName)})">@</button>
                                    ${canMute && !isMuted ? `<button class="btn-kick" onclick="showMuteMember(${jsArg(groupID)}, ${jsArg(m.user_id)}, ${jsStringArg(memberName)})">禁言</button>` : ''}
                                    ${canMute && isMuted ? `<button class="btn-kick" onclick="unmuteMember(${jsArg(groupID)}, ${jsArg(m.user_id)})">解禁</button>` : ''}
                                    ${canSetRole ? `<button class="btn-kick" onclick="showSetRole(${jsArg(groupID)}, ${jsArg(m.user_id)}, ${jsStringArg(memberName)})">角色</button>` : ''}
                                    ${canKick ? `<button class="btn-kick" onclick="kickMember(${jsArg(groupID)}, ${jsArg(m.user_id)})">移除</button>` : ''}
                                </div>
                            </div>
                        `;
                    }).join('')}
                </div>
            `;

            if (isOwner) {
                membersHTML += `
                    <div class="section-label" style="margin-top:16px">邀请成员</div>
                    <div class="invite-row">
                        <input type="text" id="invite-user-ids" placeholder="输入用户ID，多个用逗号分隔">
                        <button class="btn-inline btn-primary" onclick="inviteMember(${jsArg(groupID)})">邀请</button>
                    </div>
                `;
            }
        }
    }

    showModal(`群组: ${groupName}`, membersHTML);
}

async function inviteMember(groupID) {
    const idsStr = document.getElementById('invite-user-ids').value.trim();
    const userIDs = parseIDList(idsStr);
    if (userIDs.length === 0) {
        showToast('请输入有效的用户ID', 'warning');
        return;
    }
    const resp = await groupAPI.invite(groupID, userIDs);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('邀请成功', 'success');
        closeModal();
        showGroupMembers(groupID);
    } else {
        showToast(resp?.data?.msg || '邀请失败', 'error');
    }
}

async function kickMember(groupID, userID) {
    if (!confirm('确定要移除该成员吗？')) return;
    const resp = await groupAPI.kick(groupID, userID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已移除', 'success');
        closeModal();
        showGroupMembers(groupID);
    } else {
        showToast(resp?.data?.msg || '移除失败', 'error');
    }
}

function showSearchMessages() {
    if (!currentConversationID) {
        showToast('请先打开一个会话', 'warning');
        return;
    }
    const convName = conversationNameCache[currentConversationID] || '当前会话';
    showModal(`搜索消息 - ${escapeHTML(convName)}`, `
        <div class="search-bar-modal">
            <div class="search-input-wrap">
                <span class="search-icon">🔍</span>
                <input type="text" id="search-keyword" placeholder="输入关键词搜索当前会话消息..." onkeydown="if(event.key==='Enter')doSearch()">
            </div>
            <button class="btn-inline btn-primary" onclick="doSearch()">搜索</button>
        </div>
        <div class="search-filter-row">
            <input type="date" id="search-start-date">
            <input type="date" id="search-end-date">
        </div>
        <div id="search-results" class="search-results"></div>
    `);
    setTimeout(() => document.getElementById('search-keyword')?.focus(), 100);
}

async function doSearch() {
    const keyword = document.getElementById('search-keyword').value.trim();
    const resultsDiv = document.getElementById('search-results');
    if (!keyword) {
        showToast('请输入搜索关键词', 'warning');
        return;
    }

    resultsDiv.innerHTML = '<div class="search-loading"><div class="spinner"></div>搜索中...</div>';
    const startAt = document.getElementById('search-start-date')?.value || '';
    const endAt = document.getElementById('search-end-date')?.value || '';

    const resp = await messageAPI.search(keyword, currentConversationID, 20, startAt, endAt);
    if (resp && resp.code === 0 && resp.data && resp.data.messages) {
        const messages = resp.data.messages;
        if (messages.length === 0) {
            resultsDiv.innerHTML = '<div class="search-empty"><div class="search-empty-icon">🔍</div>未找到相关消息</div>';
            return;
        }

        const senderIDs = [...new Set(messages.map(m => m.sender_id))];
        await resolveUserNames(senderIDs);

        const highlighted = keyword.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
        resultsDiv.innerHTML = `
            <div class="search-count">找到 ${messages.length} 条结果</div>
            ${messages.map(m => {
                const senderName = getUserName(m.sender_id);
                const content = escapeHTML(m.content).replace(new RegExp(highlighted, 'gi'), match => `<mark>${match}</mark>`);
                return `
                    <div class="search-result-item" onclick="jumpToMessage(${jsArg(m.conversation_id)}, ${jsArg(m.id)})">
                        <div class="search-result-header">
                            <span class="search-result-sender">${escapeHTML(senderName)}</span>
                        </div>
                        <div class="search-result-content">${content}</div>
                        <div class="search-result-time">${m.created_at || ''}</div>
                    </div>
                `;
            }).join('')}
        `;
    } else {
        resultsDiv.innerHTML = '<div class="search-empty">搜索失败，请重试</div>';
    }
}

function jumpToMessage(conversationID, messageID) {
    closeModal();
    if (!sameID(currentConversationID, conversationID)) {
        openConversation(conversationID, 'private');
    }
}

function toggleChatMenu() {
    const menu = document.getElementById('chat-menu');
    const isVisible = menu.style.display !== 'none';
    menu.style.display = isVisible ? 'none' : 'block';

    if (!isVisible && currentConversationType === 'group') {
        document.getElementById('menu-group-manage').style.display = 'block';
        document.getElementById('menu-group-members').style.display = 'block';
    } else {
        document.getElementById('menu-group-manage').style.display = 'none';
        document.getElementById('menu-group-members').style.display = 'none';
    }
}

document.addEventListener('click', function(e) {
    const menu = document.getElementById('chat-menu');
    if (menu && menu.style.display !== 'none') {
        if (!e.target.closest('.dropdown-wrapper')) {
            menu.style.display = 'none';
        }
    }
});

function showGroupManageFromMenu() {
    document.getElementById('chat-menu').style.display = 'none';
    const groupID = conversationGroupMap[currentConversationID];
    if (groupID) {
        showGroupManage(groupID, true);
    } else {
        showToast('无法获取群组信息', 'warning');
    }
}

function showGroupMembersFromMenu() {
    document.getElementById('chat-menu').style.display = 'none';
    const groupID = conversationGroupMap[currentConversationID];
    if (groupID) {
        showGroupMembers(groupID);
    } else {
        showToast('无法获取群组信息', 'warning');
    }
}

async function pinConversation() {
    document.getElementById('chat-menu').style.display = 'none';
    if (!currentConversationID) return;

    const nextPinned = !isConversationPinned(currentConversationID);
    setConversationPinned(currentConversationID, nextPinned);

    if (currentConversationType === 'group') {
        const groupID = conversationGroupMap[currentConversationID];
        if (groupID) {
            const resp = await groupAPI.pin(groupID, nextPinned);
            if (resp && resp.code === 0 && resp.data && resp.data.success) {
                await loadGroups();
            }
        }
    }

    showToast(nextPinned ? '已置顶' : '已取消置顶', 'success');
    loadConversations();
}

function closeAnnouncement() {
    document.getElementById('group-announcement-bar').style.display = 'none';
}

function clearCurrentConversationMessages() {
    document.getElementById('chat-menu').style.display = 'none';
    if (!currentConversationID && !currentBotID) {
        showToast('当前没有打开的会话', 'warning');
        return;
    }
    if (!confirm('只清空当前窗口里的消息，后端历史不会删除。继续吗？')) return;
    const msgList = document.getElementById('message-list');
    if (msgList) {
        msgList.innerHTML = '<div class="empty-tip">当前窗口消息已清空</div>';
    }
    if (currentBotID) {
        botChatHistory[currentBotID] = [];
        delete botPendingReplies[currentBotID];
        botChatSeq++;
    }
    showToast('已清空当前窗口消息', 'success');
}

function showConversationInfo() {
    if (!currentConversationID) return;
    const convName = conversationNameCache[currentConversationID] || '会话 #' + currentConversationID;
    const typeLabel = currentConversationType === 'private' ? '私聊' : '群聊';
    showModal('会话详情', `
        <div class="info-row"><span class="info-label">会话名称</span><span>${escapeHTML(convName)}</span></div>
        <div class="info-row"><span class="info-label">会话类型</span><span>${typeLabel}</span></div>
        <div class="info-row"><span class="info-label">会话ID</span><span>${currentConversationID}</span></div>
    `);
}

function showUserProfile() {
    if (!currentUser) return;
    const displayName = currentUser.nickname || currentUser.username || '';
    const avatarHTML = currentUser.avatar
        ? safeImageHTML(currentUser.avatar)
        : escapeHTML(displayName.charAt(0).toUpperCase() || '?');
    const coverStyle = currentUser.cover
        ? ` style="background-image:url(&quot;${escapeHTML(currentUser.cover)}&quot;)"`
        : '';
    showModal('个人信息', `
        <div class="profile-card">
            <div class="profile-cover"${coverStyle}></div>
            <div class="profile-summary">
                <div class="avatar large profile-avatar" id="profile-avatar-display">${avatarHTML}</div>
                <div class="profile-title">
                    <div class="profile-name">${escapeHTML(displayName)}</div>
                    <div class="profile-uid">
                        <span>UID ${currentUser.id}</span>
                        <button class="btn-copy" onclick="copyProfileUID()">复制</button>
                    </div>
                    <div class="profile-signature">${escapeHTML(currentUser.signature || '这个人还没有写个性签名')}</div>
                </div>
            </div>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>昵称</label>
                <input type="text" id="profile-nickname" value="${escapeHTML(currentUser.nickname || '')}">
            </div>
            <div class="form-group">
                <label>性别/身份</label>
                <input type="text" id="profile-gender" value="${escapeHTML(currentUser.gender || '')}" placeholder="例如 保密 / 学生 / 开发者">
            </div>
            <div class="form-group">
                <label>邮箱</label>
                <input type="email" id="profile-email" value="${escapeHTML(currentUser.email || '')}">
            </div>
            <div class="form-group">
                <label>手机</label>
                <input type="tel" id="profile-phone" value="${escapeHTML(currentUser.phone || '')}">
            </div>
            <div class="form-group">
                <label>所在地</label>
                <input type="text" id="profile-location" value="${escapeHTML(currentUser.location || '')}">
            </div>
            <div class="form-group">
                <label>生日</label>
                <input type="date" id="profile-birthday" value="${escapeHTML(currentUser.birthday || '')}">
            </div>
        </div>
        <div class="form-group">
            <label>个性签名</label>
            <input type="text" id="profile-signature" value="${escapeHTML(currentUser.signature || '')}" maxlength="120" placeholder="写一句别人看到你时会记住的话">
        </div>
        <div class="form-group">
            <label>个人简介</label>
            <textarea id="profile-bio" maxlength="500" placeholder="介绍一下你自己">${escapeHTML(currentUser.bio || '')}</textarea>
        </div>
        <div class="form-group">
            <label>个人网站</label>
            <input type="url" id="profile-website" value="${escapeHTML(currentUser.website || '')}" placeholder="https://example.com">
        </div>
        <div class="form-group">
            <label>头像URL</label>
            <input type="text" id="profile-avatar" value="${escapeHTML(currentUser.avatar || '')}" placeholder="输入头像图片URL">
        </div>
        <div class="form-group">
            <label>头图URL</label>
            <input type="text" id="profile-cover" value="${escapeHTML(currentUser.cover || '')}" placeholder="输入个人主页头图URL">
        </div>
        <button class="btn-primary" onclick="saveProfile()">保存修改</button>
    `);
}

async function showAgentConversationTools() {
    if (!currentConversationID) {
        showToast('请先打开一个会话', 'warning');
        return;
    }
    await refreshAgentCache();
    const activeBots = botCache.filter(b => b.is_active !== false);
    if (activeBots.length === 0) {
        showToast('请先创建并启用智能助手', 'warning');
        switchSidebar('bots', document.querySelector('.sidebar-tab:nth-child(4)'));
        return;
    }
    const convName = conversationNameCache[currentConversationID] || '当前会话';
    showModal(`智能助手 - ${escapeHTML(convName)}`, `
        <div class="agent-help-box">
            <strong>选择一个动作</strong>
            <div>先选能力，再填写问题。结果会优先展示可读正文，详细 JSON 可按需展开。</div>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>选择助手</label>
                <select id="conv-agent-id" class="form-select">
                    ${activeBots.map(b => `<option value="${escapeHTML(String(b.id))}">${escapeHTML(getBotDisplayName(b))}</option>`).join('')}
                </select>
            </div>
            <div class="form-group">
                <label>能力</label>
                <select id="conv-agent-action" class="form-select" onchange="selectAgentAction('conv-agent-action', this.value, 'conv-agent-action-hint')">
                    <option value="summarize">会话总结</option>
                    <option value="ask">上下文问答</option>
                    <option value="insights">提取结论</option>
                    <option value="replyCandidates">生成回复</option>
                    <option value="run">执行任务</option>
                </select>
            </div>
        </div>
        ${renderAgentActionGrid('conv-agent-action', 'conv-agent-action-hint', 'summarize')}
        <div class="form-group">
            <label>问题 / 指令</label>
            <textarea id="conv-agent-question" rows="3" placeholder="例如：我错过了什么？有哪些风险和待办？"></textarea>
        </div>
        <div class="btn-row">
            <button class="btn-inline btn-primary" onclick="submitConversationAgentTask(this)">执行</button>
            <button class="btn-inline" onclick="showMentionPicker()">在输入框 @ 成员/助手</button>
        </div>
        <div id="conv-agent-result" class="agent-result-area"></div>
    `);
}

function submitConversationAgentTask(buttonEl = null) {
    const botID = document.getElementById('conv-agent-id').value;
    const action = document.getElementById('conv-agent-action').value;
    const question = document.getElementById('conv-agent-question').value.trim();
    runAgentTask(action, botID, currentConversationID, question, 'conv-agent-result', buttonEl);
}

async function getGroupMembersCached(groupID, force = false) {
    if (!force && groupMembersCache[groupID]) return groupMembersCache[groupID];
    const membersResp = await groupAPI.getMembers(groupID);
    if (membersResp && membersResp.code === 0 && membersResp.data && membersResp.data.members) {
        groupMembersCache[groupID] = membersResp.data.members;
        const ids = groupMembersCache[groupID].map(m => m.user_id);
        await resolveUserNames(ids);
        groupMembersCache[groupID].forEach(m => {
            if ((m.username || m.nickname) && !friendRemarkCache[m.user_id]) {
                userNickCache[m.user_id] = m.nickname || m.username;
            }
            if (m.avatar) userAvatarCache[m.user_id] = m.avatar;
        });
        return groupMembersCache[groupID];
    }
    return [];
}

async function saveProfile() {
    const profile = {
        nickname: document.getElementById('profile-nickname').value.trim(),
        email: document.getElementById('profile-email').value.trim(),
        phone: document.getElementById('profile-phone').value.trim(),
        avatar: document.getElementById('profile-avatar').value.trim(),
        cover: document.getElementById('profile-cover').value.trim(),
        signature: document.getElementById('profile-signature').value.trim(),
        bio: document.getElementById('profile-bio').value.trim(),
        location: document.getElementById('profile-location').value.trim(),
        website: document.getElementById('profile-website').value.trim(),
        gender: document.getElementById('profile-gender').value.trim(),
        birthday: document.getElementById('profile-birthday').value,
    };

    const resp = await userAPI.updateInfo(profile);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        currentUser = { ...currentUser, ...profile };
        userNickCache[currentUser.id] = currentUser.nickname || currentUser.username;
        userAvatarCache[currentUser.id] = currentUser.avatar || '';
    } else {
        showToast(resp?.data?.msg || '更新信息失败', 'error');
        return;
    }

    localStorage.setItem('claran_user', JSON.stringify(currentUser));
    document.getElementById('user-name').textContent = currentUser.nickname || currentUser.username;
    const avatarEl = document.getElementById('user-avatar');
    if (currentUser.avatar) {
        avatarEl.innerHTML = safeImageHTML(currentUser.avatar);
    } else {
        avatarEl.textContent = (currentUser.nickname || currentUser.username).charAt(0).toUpperCase();
    }

    closeModal();
    showToast('个人信息已更新', 'success');
    loadFriends();
    loadConversations();
    if (currentConversationID) {
        openConversation(currentConversationID, currentConversationType || 'private');
    }
}

async function copyProfileUID() {
    if (!currentUser || !currentUser.id) return;
    try {
        await navigator.clipboard.writeText(String(currentUser.id));
        showToast('UID已复制', 'success');
    } catch (err) {
        showToast('复制失败，请手动选择UID', 'error');
    }
}

function escapeHTML(str) {
    if (!str) return '';
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function escapeHTMLKeepText(str) {
    const div = document.createElement('div');
    div.textContent = String(str ?? '');
    return div.innerHTML;
}

function renderInlineMarkdown(text) {
    let html = escapeHTMLKeepText(text);
    html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
    html = html.replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>');
    html = html.replace(/__([^_]+)__/g, '<strong>$1</strong>');
    html = html.replace(/\*([^*\n]+)\*/g, '<em>$1</em>');
    html = html.replace(/_([^_\n]+)_/g, '<em>$1</em>');
    html = html.replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
    return html;
}

function renderMarkdownText(content) {
    const raw = String(content || '').replace(/\r\n/g, '\n');
    if (!raw.trim()) return '';
    const codeBlocks = [];
    let text = raw.replace(/```([a-zA-Z0-9_-]*)\n?([\s\S]*?)```/g, (_, lang, code) => {
        const key = `__CODE_BLOCK_${codeBlocks.length}__`;
        codeBlocks.push(`<pre class="md-code-block"><code>${escapeHTMLKeepText(code.replace(/\n$/, ''))}</code></pre>`);
        return `\n${key}\n`;
    });
    const lines = text.split('\n');
    const blocks = [];
    let paragraph = [];
    let list = [];
    let table = [];
    const flushParagraph = () => {
        if (paragraph.length) {
            blocks.push(`<p>${renderInlineMarkdown(paragraph.join(' '))}</p>`);
            paragraph = [];
        }
    };
    const flushList = () => {
        if (list.length) {
            blocks.push(`<ul>${list.map(item => `<li>${renderInlineMarkdown(item)}</li>`).join('')}</ul>`);
            list = [];
        }
    };
    const isTableDivider = (line) => /^\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)+\|?$/.test(line);
    const parseTableRow = (line) => {
        const normalized = line.trim().replace(/^\|/, '').replace(/\|$/, '');
        return normalized.split('|').map(cell => cell.trim());
    };
    const flushTable = () => {
        if (table.length >= 2 && isTableDivider(table[1])) {
            const headers = parseTableRow(table[0]);
            const rows = table.slice(2).map(parseTableRow).filter(row => row.length > 0);
            blocks.push(`
                <div class="md-table-wrap">
                    <table class="md-table">
                        <thead><tr>${headers.map(h => `<th>${renderInlineMarkdown(h)}</th>`).join('')}</tr></thead>
                        <tbody>${rows.map(row => `<tr>${headers.map((_, idx) => `<td>${renderInlineMarkdown(row[idx] || '')}</td>`).join('')}</tr>`).join('')}</tbody>
                    </table>
                </div>
            `);
        } else if (table.length) {
            paragraph.push(...table);
        }
        table = [];
    };
    for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed) {
            flushTable();
            flushParagraph();
            flushList();
            continue;
        }
        if (/^__CODE_BLOCK_\d+__$/.test(trimmed)) {
            flushTable();
            flushParagraph();
            flushList();
            const idx = Number(trimmed.match(/\d+/)[0]);
            blocks.push(codeBlocks[idx] || '');
            continue;
        }
        if (trimmed.includes('|') && (trimmed.startsWith('|') || table.length > 0)) {
            flushParagraph();
            flushList();
            table.push(trimmed);
            continue;
        }
        const heading = trimmed.match(/^(#{1,3})\s+(.+)$/);
        if (heading) {
            flushTable();
            flushParagraph();
            flushList();
            const level = heading[1].length + 3;
            blocks.push(`<h${level}>${renderInlineMarkdown(heading[2])}</h${level}>`);
            continue;
        }
        const bullet = trimmed.match(/^[-*]\s+(.+)$/);
        if (bullet) {
            flushTable();
            flushParagraph();
            list.push(bullet[1]);
            continue;
        }
        flushTable();
        flushList();
        paragraph.push(trimmed);
    }
    flushTable();
    flushParagraph();
    flushList();
    return `<div class="markdown-message">${blocks.join('')}</div>`;
}

async function showGroupManage(groupID, openedFromMenu = false) {
    let group = groupsCache.find(g => sameID(g.id, groupID));
    if (!group) {
        const groupResp = await groupAPI.get(groupID);
        if (groupResp && groupResp.code === 0 && groupResp.data && groupResp.data.group && groupResp.data.group.success !== false) {
            group = groupResp.data.group;
        }
    }
    if (!group) {
        showToast('无法获取群组信息，可能已被解散或你已不在群中', 'warning');
        return;
    }
    const isOwner = sameID(group.owner_id, currentUser.id);
    let isAdmin = false;
    try {
        const membersResp = await groupAPI.getMembers(groupID);
        if (membersResp && membersResp.code === 0 && membersResp.data && membersResp.data.members) {
            const myMember = membersResp.data.members.find(m => sameID(m.user_id, currentUser.id));
            isAdmin = myMember && myMember.role === 'admin';
        }
    } catch (e) {}
    const canEdit = isOwner || isAdmin;

    if (openedFromMenu && !canEdit) {
        showToast('只有群主或管理员可以管理群聊', 'warning');
    }

    showModal(`群组管理 - ${escapeHTML(group.name)}`, `
        <div class="form-group">
            <label>群名称</label>
            <input type="text" id="mg-name" value="${escapeHTML(group.name)}" ${!canEdit ? 'disabled' : ''}>
        </div>
        <div class="form-group">
            <label>群公告</label>
            <textarea id="mg-announcement" rows="3" ${!canEdit ? 'disabled' : ''}>${escapeHTML(group.announcement || '')}</textarea>
        </div>
        ${canEdit ? `
        <div class="btn-row">
            <button class="btn-primary" onclick="saveGroupInfo(${jsArg(groupID)})">保存修改</button>
            <button class="btn-inline" onclick="pinGroup(${jsArg(groupID)}, ${group.is_pinned ? 'false' : 'true'})">${group.is_pinned ? '取消置顶' : '置顶群聊'}</button>
        </div>
        ` : ''}
        ${isOwner ? `
        <hr style="margin:16px 0;border-color:var(--border);">
        <div class="section-label">高级管理</div>
        <div class="btn-row">
            <button class="btn-inline btn-warning" onclick="showTransferOwner(${jsArg(groupID)})">转让群主</button>
            <button class="btn-inline btn-danger" onclick="deleteGroup(${jsArg(groupID)})">解散群组</button>
        </div>
        ` : ''}
    `);
}

async function saveGroupInfo(groupID) {
    const name = document.getElementById('mg-name').value.trim();
    const announcement = document.getElementById('mg-announcement').value.trim();
    if (!name) { showToast('群名称不能为空', 'warning'); return; }
    const resp = await groupAPI.updateInfo(groupID, name, announcement);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('群信息已更新', 'success');
        closeModal();
        await loadGroups();
        const group = groupsCache.find(g => sameID(g.id, groupID));
        if (group) {
            conversationNameCache[groupConversationMap[groupID]] = group.name;
            if (currentConversationType === 'group' && sameID(conversationGroupMap[currentConversationID], groupID)) {
                document.getElementById('chat-title').textContent = group.name;
                if (group.announcement) {
                    document.getElementById('announcement-text').textContent = group.announcement;
                    document.getElementById('group-announcement-bar').style.display = 'flex';
                } else {
                    document.getElementById('group-announcement-bar').style.display = 'none';
                }
            }
        }
        loadConversations();
    } else {
        showToast(resp?.data?.msg || '更新失败', 'error');
    }
}

async function pinGroup(groupID, isPinned) {
    const resp = await groupAPI.pin(groupID, isPinned);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast(isPinned ? '已置顶' : '已取消置顶', 'success');
        closeModal();
        await loadGroups();
        loadConversations();
    } else {
        showToast(resp?.data?.msg || '操作失败', 'error');
    }
}

function showTransferOwner(groupID) {
    showModal('转让群主', `
        <div class="form-group">
            <label>新群主用户ID</label>
            <input type="number" id="transfer-new-owner" placeholder="输入新群主的用户ID">
        </div>
        <button class="btn-primary btn-warning" onclick="transferOwner(${jsArg(groupID)})">确认转让</button>
    `);
}

async function transferOwner(groupID) {
    const newOwnerID = document.getElementById('transfer-new-owner').value.trim();
    if (!/^\d{10}$/.test(newOwnerID)) { showToast('请输入有效的用户ID', 'warning'); return; }
    if (!confirm('确定要转让群主吗？此操作不可撤销！')) return;
    const resp = await groupAPI.transfer(groupID, newOwnerID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('群主已转让', 'success');
        closeModal();
        await loadGroups();
        if (currentConversationID && currentConversationType === 'group') {
            const convGroupID = conversationGroupMap[currentConversationID];
            if (sameID(convGroupID, groupID)) {
                const groupResp = await groupAPI.get(groupID);
                if (groupResp && groupResp.code === 0 && groupResp.data && groupResp.data.group) {
                    const group = groupResp.data.group;
                    if (group.announcement) {
                        document.getElementById('announcement-text').textContent = group.announcement;
                        document.getElementById('group-announcement-bar').style.display = 'flex';
                    }
                }
            }
        }
    } else {
        showToast(resp?.data?.msg || '转让失败', 'error');
    }
}

async function deleteGroup(groupID) {
    if (!confirm('确定要解散群组吗？此操作不可撤销！')) return;
    const resp = await groupAPI.deleteGroup(groupID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        closeModal();
        const convID = groupConversationMap[groupID];
        if (convID) {
            delete conversationGroupMap[convID];
            delete conversationNameCache[convID];
            setConversationHidden(convID, true);
        }
        delete groupConversationMap[groupID];
        await loadGroups();
        loadConversations();
        if (convID && sameID(currentConversationID, convID)) {
            resetChatView('群组已解散');
        } else {
            showToast('群组已解散', 'success');
        }
    } else {
        showToast(resp?.data?.msg || '解散失败', 'error');
    }
}

function showMuteMember(groupID, userID, userName) {
    showModal(`禁言 - ${escapeHTML(userName)}`, `
        <div class="form-group">
            <label>禁言时长（分钟）</label>
            <input type="number" id="mute-duration" value="10" min="1">
        </div>
        <button class="btn-primary btn-warning" onclick="muteMember(${jsArg(groupID)}, ${jsArg(userID)})">确认禁言</button>
    `);
}

async function muteMember(groupID, userID) {
    const duration = parseInt(document.getElementById('mute-duration').value);
    if (!duration || duration <= 0) { showToast('请输入有效的禁言时长', 'warning'); return; }
    const resp = await groupAPI.mute(groupID, userID, duration);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已禁言', 'success');
        closeModal();
        showGroupMembers(groupID);
    } else {
        showToast(resp?.data?.msg || '禁言失败', 'error');
    }
}

async function unmuteMember(groupID, userID) {
    const resp = await groupAPI.unmute(groupID, userID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已解除禁言', 'success');
        showGroupMembers(groupID);
    } else {
        showToast(resp?.data?.msg || '解除禁言失败', 'error');
    }
}

function showSetRole(groupID, userID, userName) {
    showModal(`设置角色 - ${escapeHTML(userName)}`, `
        <div class="form-group">
            <label>角色</label>
            <select id="set-role-select" class="form-select">
                <option value="member">成员</option>
                <option value="admin">管理员</option>
            </select>
        </div>
        <button class="btn-primary" onclick="setMemberRole(${jsArg(groupID)}, ${jsArg(userID)})">确认</button>
    `);
}

async function setMemberRole(groupID, userID) {
    const role = document.getElementById('set-role-select').value;
    const resp = await groupAPI.setRole(groupID, userID, role);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('角色已更新', 'success');
        closeModal();
        showGroupMembers(groupID);
    } else {
        showToast(resp?.data?.msg || '设置失败', 'error');
    }
}

async function uploadAndSendFile() {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'image/*,audio/*,.pdf,.doc,.docx,.txt,.zip';
    input.onchange = async (e) => {
        const file = e.target.files[0];
        if (!file) return;
        if (!currentConversationID) { showToast('请先打开一个会话', 'warning'); return; }

        showToast('文件上传中...', 'info');
        const resp = await fileAPI.upload(file);
        if (!resp) {
            showToast('文件上传失败', 'error');
            return;
        }
        if (resp.code !== 0 || !resp.data || !resp.data.success) {
            showToast(resp?.data?.msg || resp?.message || '文件上传失败', 'error');
            return;
        }
        const fileURL = resp.data.file_url || '';
        const fileID = resp.data.file_id || '';
        let msgType = 'file';
        if (file.type.startsWith('image/')) msgType = 'image';
        else if (file.type.startsWith('audio/')) msgType = 'voice';

        const payload = makeMediaPayloadWithMeta(fileURL, fileID, file.name, file);
        let content = payload;
        if (msgType === 'image') {
            content = `[img]${payload}[/img]`;
        } else if (msgType === 'voice') {
            content = `[voice]${payload}[/voice]`;
        } else {
            content = `[file]${payload}[/file]`;
        }

        const sendResp = await messageAPI.send(currentConversationID, content, msgType);
        if (sendResp && sendResp.code === 0 && sendResp.data && sendResp.data.success) {
            const now = new Date();
            const timeStr = now.getFullYear() + '-' +
                String(now.getMonth() + 1).padStart(2, '0') + '-' +
                String(now.getDate()).padStart(2, '0') + ' ' +
                String(now.getHours()).padStart(2, '0') + ':' +
                String(now.getMinutes()).padStart(2, '0') + ':' +
                String(now.getSeconds()).padStart(2, '0');
            appendMessage({
                id: sendResp.data.msg_id,
                msg_id: sendResp.data.msg_id,
                conversation_id: currentConversationID,
                sender_id: currentUser.id,
                content,
                msg_type: msgType,
                created_at: timeStr,
            });
            showToast('文件发送成功', 'success');
        } else {
            showToast('消息发送失败', 'error');
        }
    };
    input.click();
}

async function sendVoiceBlob(blob, durationMs) {
    // 语音消息复用图片/文件的媒体协议：先上传二进制文件，再发送轻量的 [voice]url|id|name[/voice] 引用。
    if (!currentConversationID) {
        showToast('请先打开一个会话', 'warning');
        return;
    }
    if (!blob || blob.size === 0) {
        showToast('录音内容为空', 'warning');
        return;
    }

    const ext = blob.type.includes('mp4') ? 'm4a' : blob.type.includes('ogg') ? 'ogg' : 'webm';
    const file = new File([blob], `voice-${Date.now()}.${ext}`, { type: blob.type || 'audio/webm' });
    showToast('语音上传中...', 'info');

    const uploadResp = await fileAPI.upload(file, 'voice');
    if (!uploadResp || uploadResp.code !== 0 || !uploadResp.data || !uploadResp.data.success) {
        showToast(uploadResp?.data?.msg || uploadResp?.message || '语音上传失败', 'error');
        return;
    }

    const fileURL = uploadResp.data.file_url || '';
    const fileID = uploadResp.data.file_id || '';
    const name = `${formatRecordDuration(durationMs)} voice.${ext}`;
    const payload = makeMediaPayloadWithMeta(fileURL, fileID, name, file);
    const content = `[voice]${payload}[/voice]`;
    const sendConversationID = currentConversationID;
    const sendResp = await messageAPI.send(sendConversationID, content, 'voice');
    if (sendResp && sendResp.code === 0 && sendResp.data && sendResp.data.success) {
        const now = new Date();
        const timeStr = now.getFullYear() + '-' +
            String(now.getMonth() + 1).padStart(2, '0') + '-' +
            String(now.getDate()).padStart(2, '0') + ' ' +
            String(now.getHours()).padStart(2, '0') + ':' +
            String(now.getMinutes()).padStart(2, '0') + ':' +
            String(now.getSeconds()).padStart(2, '0');
        appendMessage({
            id: sendResp.data.msg_id,
            msg_id: sendResp.data.msg_id,
            conversation_id: sendConversationID,
            sender_id: currentUser.id,
            content,
            msg_type: 'voice',
            created_at: timeStr,
        });
        showToast('语音已发送', 'success');
    } else {
        showToast(sendResp?.data?.msg || '语音消息发送失败', 'error');
    }
}

async function startVoiceRecording(e) {
    // 长按录音模拟移动端聊天体验：MediaRecorder 先在本地收集音频片段，松开鼠标/手指后再上传最终 Blob。
    if (e) e.preventDefault();
    if (!currentConversationID) {
        showToast('请先打开一个会话', 'warning');
        return;
    }
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia || !window.MediaRecorder) {
        showToast('当前浏览器不支持录音', 'error');
        return;
    }
    if (voiceRecorder && voiceRecorder.state === 'recording') return;

    try {
        voiceRecordStream = await navigator.mediaDevices.getUserMedia({ audio: true });
        const mimeType = getVoiceMimeType();
        voiceRecordChunks = [];
        voiceRecorder = new MediaRecorder(voiceRecordStream, mimeType ? { mimeType } : undefined);
        voiceRecordStartedAt = Date.now();
        voiceRecorder.ondataavailable = event => {
            if (event.data && event.data.size > 0) {
                voiceRecordChunks.push(event.data);
            }
        };
        voiceRecorder.onstop = async () => {
            const durationMs = Date.now() - voiceRecordStartedAt;
            const type = voiceRecorder.mimeType || mimeType || 'audio/webm';
            const blob = new Blob(voiceRecordChunks, { type });
            voiceRecordChunks = [];
            if (voiceRecordStream) {
                voiceRecordStream.getTracks().forEach(track => track.stop());
                voiceRecordStream = null;
            }
            setVoiceRecordingUI(false);
            clearInterval(voiceRecordTimer);
            voiceRecordTimer = null;
            if (durationMs < 700) {
                showToast('录音时间太短', 'warning');
                return;
            }
            await sendVoiceBlob(blob, durationMs);
        };
        voiceRecorder.start();
        updateVoiceRecordTime();
        setVoiceRecordingUI(true);
        voiceRecordTimer = setInterval(updateVoiceRecordTime, 250);
    } catch (err) {
        setVoiceRecordingUI(false);
        showToast('无法访问麦克风: ' + err.message, 'error');
    }
}

function stopVoiceRecording(e) {
    if (e) e.preventDefault();
    if (voiceRecorder && voiceRecorder.state === 'recording') {
        voiceRecorder.stop();
    }
}

function bindVoiceRecorder() {
    // 页面加载后只绑定一次；释放事件挂在 window 上，避免指针滑出录音按钮后无法结束并发送。
    const btn = document.getElementById('voice-record-btn');
    if (!btn || btn.dataset.bound === '1') return;
    btn.dataset.bound = '1';
    btn.addEventListener('mousedown', startVoiceRecording);
    btn.addEventListener('touchstart', startVoiceRecording, { passive: false });
    window.addEventListener('mouseup', stopVoiceRecording);
    window.addEventListener('touchend', stopVoiceRecording, { passive: false });
    window.addEventListener('touchcancel', stopVoiceRecording, { passive: false });
}

async function analyzeImageOCR(fileID, fileName = '图片') {
    if (!fileID) {
        showToast('图片缺少 file_id，无法识别', 'warning');
        return;
    }
    showModal(`图片识别 - ${escapeHTML(fileName)}`, '<div class="empty-tip">正在调用 OCR 识别图片内容...</div>');
    const resp = await fileAPI.ocr(fileID);
    if (resp && resp.code === 0 && resp.data?.success) {
        document.getElementById('modal-body').innerHTML = `
            <div class="agent-help-box">
                <strong>OCR 识别结果</strong>
                <p>这段文本来自图片解析，可复制给 Agent 继续分析；如果图片是复杂截图，结果可能需要人工校对。</p>
            </div>
            <div class="ocr-result-text">${renderMarkdownText(resp.data.text || '未识别到文本')}</div>
        `;
    } else {
        document.getElementById('modal-body').innerHTML = `<div class="empty-tip">图片识别失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '请检查 OCR 配置或稍后重试')}</small></div>`;
    }
}

function renderMessageContent(content, msgType, options = {}) {
    if (msgType === 'broadcast') {
        return `<div class="broadcast-msg"><span class="broadcast-badge">广播</span><span>${options.markdown ? renderMarkdownText(content) : escapeHTML(content)}</span></div>`;
    }
    if (msgType === 'image' || (content && content.startsWith('[img]'))) {
        const media = parseMediaPayload(content, 'img');
        const key = rememberMedia(media);
        const url = resolveMediaURL(media);
        const dataAttrs = media.id ? ` data-media-id="${escapeHTML(media.id)}" data-media-key="${escapeHTML(key)}" data-media-url="${escapeHTML(media.url || '')}" data-media-name="${escapeHTML(media.name || '')}"` : '';
        return `<div class="image-msg-wrap"><img src="${escapeHTML(url)}"${dataAttrs} alt="${escapeHTML(media.name || '图片')}" class="chat-image" onclick="window.open(this.src,'_blank')" onerror="this.closest('.image-msg-wrap').querySelector('.media-error').style.display='inline';"><span class="media-error" style="display:none;">图片加载失败</span>${media.id ? `<button type="button" class="image-ocr-btn" onclick="analyzeImageOCR(${jsStringArg(media.id)}, ${jsStringArg(media.name || '图片')})">识别图片</button>` : ''}</div>`;
    }
    if (msgType === 'voice' || (content && content.startsWith('[voice]'))) {
        const media = parseMediaPayload(content, 'voice');
        const key = rememberMedia(media);
        const url = resolveMediaURL(media);
        const dataAttrs = media.id ? ` data-media-id="${escapeHTML(media.id)}" data-media-key="${escapeHTML(key)}" data-media-url="${escapeHTML(media.url || '')}" data-media-name="${escapeHTML(media.name || '')}"` : '';
        return `<div class="media-msg voice-msg"><span class="voice-icon">VOICE</span><audio controls preload="metadata" src="${escapeHTML(url)}"${dataAttrs}></audio><button type="button" class="voice-download" onclick="downloadMedia(${jsStringArg(media.id || '')}, ${jsStringArg(media.name || 'voice.webm')}, ${jsStringArg(key)})">下载</button><span class="voice-name">${escapeHTML(media.name || 'voice message')}</span></div>`;
    }
    if (msgType === 'file' || (content && content.startsWith('[file]'))) {
        const media = parseMediaPayload(content, 'file');
        const url = resolveDownloadURL(media);
        const key = rememberMedia(media);
        if (media.id) {
            return `<button type="button" class="media-msg file-msg file-download-btn" onclick="downloadMedia(${jsStringArg(media.id)}, ${jsStringArg(media.name || 'download')}, ${jsStringArg(key)})">文件 ${escapeHTML(media.name || '未命名文件')}</button>`;
        }
        return `<a class="media-msg file-msg" href="${escapeHTML(url)}" target="_blank" rel="noopener" download="${escapeHTML(media.name || '')}">文件 ${escapeHTML(media.name || '未命名文件')}</a>`;
    }
    if (options.markdown) {
        const parsedCardPayload = tryParseAgentCardPayload(content);
        if (parsedCardPayload) {
            return renderAgentResult(parsedCardPayload, { action: 'run' });
        }
        return renderMarkdownText(content);
    }
    return renderTextMessage(content);
}

function tryParseAgentCardPayload(content) {
    const raw = String(content || '').trim();
    if (!raw.startsWith('{') && !raw.startsWith('```json')) return null;
    let jsonText = raw;
    const fenced = raw.match(/^```json\s*([\s\S]*?)\s*```$/i);
    if (fenced) jsonText = fenced[1].trim();
    try {
        const parsed = JSON.parse(jsonText);
        if (normalizeAgentActionCards(parsed).length > 0) {
            return parsed;
        }
    } catch (err) {
        return null;
    }
    return null;
}

function renderTextMessage(content) {
    const raw = content || '';
    const placeholders = [];
    const withoutIDs = raw.replace(/@\[(.*?)\]\(\d+\)/g, (_, name) => `@${name}`);
    const withMentions = escapeHTML(withoutIDs).replace(/(^|\s)@([^\s@]+)/g, (all, prefix, name) => {
        const key = `__MENTION_${placeholders.length}__`;
        placeholders.push(`${prefix}<span class="mention-text">@${escapeHTML(name)}</span>`);
        return key;
    });
    return placeholders.reduce((text, html, idx) => text.replace(`__MENTION_${idx}__`, html), withMentions);
}

async function showSystemSettings() {
    showModal('系统设置', `
        <div class="settings-tabs">
            <button class="btn-small active" data-settings-tab="llm" onclick="renderLLMSettings()">LLM 预设</button>
            <button class="btn-small" data-settings-tab="prompt" onclick="renderPromptSettings()">Prompt</button>
            <button class="btn-small" data-settings-tab="mcp" onclick="renderMCPSettings()">MCP 工具</button>
        </div>
        <div id="settings-content" class="settings-content">加载中...</div>
    `);
    await renderLLMSettings();
}

function activateSettingsTab(tab) {
    document.querySelectorAll('.settings-tabs [data-settings-tab]').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.settingsTab === tab);
    });
}

async function renderLLMSettings() {
    activateSettingsTab('llm');
    const area = document.getElementById('settings-content');
    if (!area) return;
    area.innerHTML = '<div class="empty-tip">加载中...</div>';
    const resp = await settingsAPI.listLLMProfiles();
    if (!resp || resp.code !== 0 || !resp.data?.success) {
        area.innerHTML = `<div class="empty-tip">加载失败<br><small>${escapeHTML(resp?.message || '')}</small></div>`;
        return;
    }
    llmProfilesCache = resp.data.profiles || [];
    area.innerHTML = `
            <div class="agent-help-box">
                <strong>LLM 预设</strong>
                <p>这里保存可复用的模型服务配置。创建 Agent、翻译消息、RAG 路由、向量化、OCR 和 Rerank 都可以选择这些预设；也可以测试项目内置默认模型。</p>
            </div>
            <div class="model-test-board">
                <div>
                    <strong>项目内置模型连通测试</strong>
                    <span>使用 .env / yaml 中的默认模型配置，不需要在前端填写 API Key。</span>
                </div>
                <div class="model-test-actions">
                    ${modelCapabilityOptions().map(item => `<button class="btn-small ghost" onclick="testBuiltinLLMSetting(${jsStringArg(item.value)})">${escapeHTML(item.label)}</button>`).join('')}
                </div>
            </div>
            <input id="setting-llm-id" type="hidden" value="">
            <div class="settings-list">
                ${llmProfilesCache.length ? llmProfilesCache.map(renderLLMProfileCard).join('') : '<div class="empty-tip">暂无 LLM 预设</div>'}
            </div>
        <div class="settings-editor">
            <h4>新增 / 修改 LLM 预设</h4>
            <div class="profile-form-grid">
                <div class="form-group">
                    <label>配置名称</label>
                    <input id="setting-llm-name" type="text" placeholder="例如：智谱翻译 / OpenAI 工作模型">
                </div>
                <div class="form-group">
                    <label>用途</label>
                    <select id="setting-llm-usage" class="form-select">
                        ${modelCapabilityOptions().map(item => `<option value="${escapeHTML(item.value)}">${escapeHTML(item.label)}</option>`).join('')}
                    </select>
                    <small class="form-hint">RAG 路由小模型用于判断问题是否需要检索知识库；未配置时使用项目内置默认模型。</small>
                </div>
                <div class="form-group">
                    <label>BaseURL</label>
                    <input id="setting-llm-baseurl" type="text" placeholder="https://api.example.com/v1">
                </div>
                <div class="form-group">
                    <label>模型名</label>
                    <input id="setting-llm-model" type="text" placeholder="例如 glm-4.7 / gpt-4o-mini">
                </div>
            </div>
            <div class="form-group">
                <label>API Key</label>
                <input id="setting-llm-apikey" type="password" placeholder="留空则不写入；保存时会设置为新密钥">
            </div>
            <label class="checkbox-row"><input id="setting-llm-default" type="checkbox"><span>设为该用途默认配置</span></label>
            <div class="modal-actions">
                <button class="btn-secondary" onclick="testLLMSettingForm()">测试连接</button>
                <button class="btn-secondary" onclick="testBuiltinLLMSetting(document.getElementById('setting-llm-usage')?.value || 'general')">测试内置默认</button>
                <button class="btn-primary" onclick="saveLLMSetting()">保存 LLM 预设</button>
            </div>
            <div id="setting-llm-test-result" class="settings-test-result"></div>
        </div>
    `;
}

function modelCapabilityOptions() {
    return [
        { value: 'agent', label: 'Agent 大模型' },
        { value: 'rag_answer', label: 'RAG 回答模型' },
        { value: 'rag_router', label: 'RAG 路由小模型' },
        { value: 'embedding', label: '向量模型' },
        { value: 'ocr', label: 'OCR 模型' },
        { value: 'rerank', label: 'Rerank 模型' },
        { value: 'translation', label: '翻译模型' },
        { value: 'general', label: '通用模型' },
    ];
}

function renderLLMProfileCard(profile) {
    return `
        <div class="data-row">
            <div class="data-row-main">
                <strong>${escapeHTML(profile.name || '未命名')}</strong>
                <span>${escapeHTML(profile.model_name || '未配置模型')} · ${escapeHTML(profile.base_url || '未配置BaseURL')}</span>
            </div>
            <div class="data-row-meta">
                <span>${escapeHTML(llmUsageLabel(profile.usage_type))}${profile.is_default ? ' · 默认' : ''}</span>
                <span>${profile.has_api_key ? '已保存密钥' : '未保存密钥'}</span>
            </div>
            <div class="data-row-actions">
                <button class="btn-small ghost" onclick="testSavedLLMSetting(${jsArg(profile.id)}, ${jsStringArg(profile.usage_type || '')})">测试</button>
                <button class="btn-small" onclick="fillLLMSettingForm(${jsStringArg(JSON.stringify(profile).replace(/</g, '\\u003c'))})">填入表单</button>
                <button class="btn-small danger-soft" onclick="deleteLLMSetting(${jsArg(profile.id)})">删除</button>
            </div>
        </div>
    `;
}

function fillLLMSettingForm(profileJSON) {
    const profile = JSON.parse(profileJSON);
    document.getElementById('setting-llm-id').value = profile.id || '';
    document.getElementById('setting-llm-name').value = profile.name || '';
    document.getElementById('setting-llm-usage').value = profile.usage_type || 'translation';
    document.getElementById('setting-llm-baseurl').value = profile.base_url || '';
    document.getElementById('setting-llm-model').value = profile.model_name || '';
    document.getElementById('setting-llm-default').checked = !!profile.is_default;
}

async function saveLLMSetting() {
    const apiKey = document.getElementById('setting-llm-apikey').value.trim();
    const data = {
        id: Number(document.getElementById('setting-llm-id')?.value || 0),
        name: document.getElementById('setting-llm-name').value.trim(),
        usage_type: document.getElementById('setting-llm-usage').value,
        base_url: document.getElementById('setting-llm-baseurl').value.trim(),
        model_name: document.getElementById('setting-llm-model').value.trim(),
        api_key: apiKey,
        api_key_action: apiKey ? 'set' : 'keep',
        is_default: document.getElementById('setting-llm-default').checked,
        enabled: true,
    };
    const resp = await settingsAPI.saveLLMProfile(data);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('LLM 预设已保存', 'success');
        await renderLLMSettings();
    } else {
        showToast(resp?.message || resp?.data?.msg || '保存失败', 'error');
    }
}

function currentLLMSettingFormPayload(includeKey = true) {
    const apiKey = document.getElementById('setting-llm-apikey')?.value.trim() || '';
    return {
        profile_id: Number(document.getElementById('setting-llm-id')?.value || 0),
        provider_type: 'openai_compatible',
        usage_type: document.getElementById('setting-llm-usage')?.value || 'general',
        base_url: document.getElementById('setting-llm-baseurl')?.value.trim() || '',
        model_name: document.getElementById('setting-llm-model')?.value.trim() || '',
        api_key: includeKey ? apiKey : '',
    };
}

async function testLLMSettingForm() {
    const resultEl = document.getElementById('setting-llm-test-result');
    if (resultEl) resultEl.innerHTML = '<div class="empty-tip small">正在测试连接...</div>';
    const payload = currentLLMSettingFormPayload(true);
    if (!payload.api_key && !payload.profile_id) {
        showToast('测试未保存配置时需要填写 API Key', 'warning');
        if (resultEl) resultEl.innerHTML = '<div class="rag-status-line error">请填写 API Key，或先选择一个已保存配置。</div>';
        return;
    }
    const resp = await settingsAPI.testLLMProfile(payload);
    renderLLMTestResult(resp, resultEl);
}

async function testSavedLLMSetting(profileID, usageType = '') {
    const resp = await settingsAPI.testLLMProfile({ profile_id: profileID, usage_type: usageType });
    const ok = resp && resp.code === 0 && resp.data?.success && resp.data?.result?.ok;
    showToast(ok ? `模型连接正常，延迟 ${resp.data.result.latency_ms || 0}ms` : (resp?.data?.result?.msg || resp?.message || resp?.data?.msg || '模型连接失败'), ok ? 'success' : 'error');
}

async function testBuiltinLLMSetting(usageType = 'general') {
    const resultEl = document.getElementById('setting-llm-test-result');
    if (resultEl) resultEl.innerHTML = `<div class="empty-tip small">正在测试项目内置 ${escapeHTML(llmUsageLabel(usageType))}...</div>`;
    const resp = await settingsAPI.testLLMProfile({ use_builtin: true, usage_type: usageType || 'general' });
    renderLLMTestResult(resp, resultEl);
}

function renderLLMTestResult(resp, resultEl) {
    const result = resp?.data?.result || {};
    const ok = resp && resp.code === 0 && resp.data?.success && result.ok;
    const message = result.msg || resp?.message || resp?.data?.msg || (ok ? '连通测试通过' : '连通测试失败');
    if (resultEl) {
        resultEl.innerHTML = `<div class="rag-status-line ${ok ? 'success' : 'error'}">${escapeHTML(message)}${result.latency_ms ? ` · ${Number(result.latency_ms)}ms` : ''}</div>`;
    }
    showToast(message, ok ? 'success' : 'error');
}

async function deleteLLMSetting(id) {
    if (!confirm('确定删除这个 LLM 预设？')) return;
    const resp = await settingsAPI.deleteLLMProfile(id);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('LLM 预设已删除', 'success');
        await renderLLMSettings();
    } else {
        showToast(resp?.message || resp?.data?.msg || '删除失败', 'error');
    }
}

async function renderPromptSettings() {
    activateSettingsTab('prompt');
    const area = document.getElementById('settings-content');
    if (!area) return;
    const resp = await settingsAPI.listPrompts();
    const prompts = resp?.data?.prompts || [];
    const translationPrompt = prompts.find(p => p.type === 'translation')?.content || '请将下面内容翻译成中文。只输出译文，保留代码、链接、数字、专有名词和 Markdown 结构。';
    area.innerHTML = `
        <div class="agent-help-box">
            <strong>Prompt 设置</strong>
            <p>当前先支持翻译 Prompt，后续摘要、改写、Agent 上下文也会复用同一配置服务。</p>
        </div>
        <div class="form-group">
            <label>翻译 Prompt</label>
            <textarea id="setting-translation-prompt" rows="7">${escapeHTML(translationPrompt)}</textarea>
            <small>可使用 {{text}} 和 {{target_language}} 占位符。</small>
        </div>
        <button class="btn-primary" onclick="saveTranslationPrompt()">保存 Prompt</button>
    `;
}

async function saveTranslationPrompt() {
    const content = document.getElementById('setting-translation-prompt').value.trim();
    const resp = await settingsAPI.savePrompt({ type: 'translation', name: '翻译 Prompt', content, enabled: true, is_default: true });
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('Prompt 已保存', 'success');
    } else {
        showToast(resp?.message || resp?.data?.msg || '保存失败', 'error');
    }
}

async function renderMCPSettings() {
    activateSettingsTab('mcp');
    const area = document.getElementById('settings-content');
    if (!area) return;
    area.innerHTML = '<div class="empty-tip">加载中...</div>';
    const resp = await settingsAPI.listMCPServers({ includeDisabled: true });
    if (!resp || resp.code !== 0 || !resp.data?.success) {
        area.innerHTML = `<div class="empty-tip">加载失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const servers = resp.data.servers || [];
    area.innerHTML = `
        <div class="agent-help-box">
            <strong>MCP 工具接入</strong>
            <p>这里配置外部 MCP Server。Agent 会通过 mcp-gateway-service 发现工具并记录审计；低信任工具默认需要后续审批。stdio 当前只保存配置，不在服务端执行。</p>
        </div>
        <input id="setting-mcp-id" type="hidden" value="">
        <div class="settings-list">
            ${servers.length ? servers.map(renderMCPServerRow).join('') : '<div class="empty-tip">暂无外部 MCP Server</div>'}
        </div>
        <div class="settings-editor">
            <h4>工具发现与审计</h4>
            <div class="profile-form-grid">
                <div class="form-group">
                    <label>Agent ID</label>
                    <input id="mcp-preview-agent-id" type="text" placeholder="为空则只看用户/全局工具">
                </div>
                <div class="form-group">
                    <label>会话 ID</label>
                    <input id="mcp-preview-conversation-id" type="text" placeholder="为空则不使用会话级配置">
                </div>
            </div>
            <div class="btn-row">
                <button class="btn-secondary" onclick="loadMCPToolsPreview()">查看可用工具</button>
                <button class="btn-secondary" onclick="loadMCPTracePreview()">查看调用审计</button>
            </div>
            <div id="mcp-preview-panel" class="data-list compact-list"></div>
        </div>
        <div class="settings-editor">
            <h4>新增 / 修改 MCP Server</h4>
            <div class="profile-form-grid">
                <div class="form-group">
                    <label>名称</label>
                    <input id="setting-mcp-name" type="text" placeholder="例如：GitHub MCP / 内部工单 MCP">
                </div>
                <div class="form-group">
                    <label>作用域</label>
                    <select id="setting-mcp-scope" class="form-select">
                        <option value="global">全局</option>
                        <option value="user">仅本人</option>
                        <option value="agent">指定 Agent</option>
                        <option value="conversation">指定会话</option>
                    </select>
                    <small class="form-hint">Agent/会话级配置需要填写对应 ID；全局和本人配置会自动注入当前用户的 Agent 运行。</small>
                </div>
                <div class="form-group">
                    <label>Agent ID</label>
                    <input id="setting-mcp-agent-id" type="text" placeholder="Agent 级或会话级必填">
                </div>
                <div class="form-group">
                    <label>会话 ID</label>
                    <input id="setting-mcp-conversation-id" type="text" placeholder="会话级必填">
                </div>
                <div class="form-group">
                    <label>传输方式</label>
                    <select id="setting-mcp-transport" class="form-select">
                        <option value="streamable_http">Streamable HTTP / JSON-RPC</option>
                        <option value="sse">SSE / JSON-RPC 兼容</option>
                        <option value="stdio">stdio（仅保存，不执行）</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>信任级别</label>
                    <select id="setting-mcp-trust" class="form-select">
                        <option value="low">低信任，需要审批</option>
                        <option value="normal">普通</option>
                        <option value="high">高信任</option>
                    </select>
                </div>
            </div>
            <div class="form-group">
                <label>Endpoint URL</label>
                <input id="setting-mcp-endpoint" type="text" placeholder="https://example.com/mcp">
            </div>
            <div class="form-group">
                <label>说明</label>
                <input id="setting-mcp-desc" type="text" placeholder="这个 MCP Server 提供什么工具">
            </div>
            <div class="profile-form-grid">
                <div class="form-group">
                    <label>认证方式</label>
                    <select id="setting-mcp-auth-type" class="form-select">
                        <option value="bearer">Bearer Token</option>
                        <option value="api_key">X-API-Key</option>
                        <option value="basic">Basic</option>
                        <option value="">无</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>Secret</label>
                    <input id="setting-mcp-secret" type="password" placeholder="留空则保留原密钥">
                </div>
            </div>
            <div class="profile-form-grid">
                <div class="form-group">
                    <label>允许工具 JSON</label>
                    <textarea id="setting-mcp-allow-tools" rows="3" placeholder='["search","read_file"]'></textarea>
                    <small class="form-hint">为空表示不限制；建议生产环境配置 allowlist。</small>
                </div>
                <div class="form-group">
                    <label>拒绝工具 JSON</label>
                    <textarea id="setting-mcp-deny-tools" rows="3" placeholder='["delete_file","run_shell"]'></textarea>
                </div>
            </div>
            <div class="profile-form-grid">
                <div class="form-group">
                    <label>Headers JSON</label>
                    <textarea id="setting-mcp-headers" rows="3" placeholder='{"X-Workspace":"demo"}'></textarea>
                </div>
                <div class="form-group">
                    <label>stdio Command / Args JSON</label>
                    <input id="setting-mcp-command" type="text" placeholder="node / python / mcp-server">
                    <textarea id="setting-mcp-args" rows="3" placeholder='["server.js"]'></textarea>
                </div>
            </div>
            <label class="checkbox-row"><input id="setting-mcp-enabled" type="checkbox" checked><span>启用该 MCP Server</span></label>
            <div class="btn-row">
                <button class="btn-secondary" onclick="clearMCPSettingForm()">清空表单</button>
                <button class="btn-primary" onclick="saveMCPSetting()">保存 MCP Server</button>
            </div>
        </div>
    `;
}

function renderMCPServerRow(server) {
    const enabledText = server.enabled ? '已启用' : '已停用';
    const scopeText = mcpScopeLabel(server.scope);
    const trustText = mcpTrustLabel(server.trust_level);
    return `
        <div class="data-row mcp-row ${server.enabled ? '' : 'disabled'}">
            <div class="data-row-main">
                <strong>${escapeHTML(server.name || '未命名 MCP')}</strong>
                <span>${escapeHTML(server.description || server.endpoint_url || server.command || '暂无说明')}</span>
            </div>
            <div class="data-row-meta">
                <span>${escapeHTML(scopeText)} · ${escapeHTML(server.transport || 'streamable_http')}</span>
                <span>${escapeHTML(enabledText)} · ${escapeHTML(trustText)} · ${server.has_secret ? '已保存密钥' : '无密钥'}</span>
            </div>
            <div class="data-row-actions">
                <button class="btn-small" onclick="fillMCPSettingForm(${jsStringArg(JSON.stringify(server).replace(/</g, '\\u003c'))})">编辑</button>
                <button class="btn-small danger-soft" onclick="deleteMCPSetting(${jsArg(server.id)})">删除</button>
            </div>
        </div>
    `;
}

async function loadMCPToolsPreview() {
    const panel = document.getElementById('mcp-preview-panel');
    if (!panel) return;
    panel.innerHTML = '<div class="empty-tip">加载工具中...</div>';
    const agentID = Number(document.getElementById('mcp-preview-agent-id')?.value || 0);
    const conversationID = Number(document.getElementById('mcp-preview-conversation-id')?.value || 0);
    const resp = await mcpAPI.tools({ agentID, conversationID });
    const tools = resp?.data?.tools || [];
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        panel.innerHTML = `<div class="empty-tip">${escapeHTML(resp?.message || resp?.data?.msg || '工具发现失败')}</div>`;
        return;
    }
    panel.innerHTML = tools.length ? tools.map(tool => `
        <div class="data-row">
            <div class="data-row-main">
                <strong>${escapeHTML(tool.name || '')}</strong>
                <span>${escapeHTML(tool.description || '暂无说明')}</span>
            </div>
            <div class="data-row-meta">
                <span>${escapeHTML(tool.source || '')} · ${escapeHTML(tool.server_name || '')}</span>
                <span>${tool.requires_approval ? '需要审批' : '可直接调用'}</span>
            </div>
        </div>
    `).join('') : '<div class="empty-tip">当前上下文没有可用 MCP 工具</div>';
}

async function loadMCPTracePreview() {
    const panel = document.getElementById('mcp-preview-panel');
    if (!panel) return;
    panel.innerHTML = '<div class="empty-tip">加载审计中...</div>';
    const agentID = Number(document.getElementById('mcp-preview-agent-id')?.value || 0);
    const conversationID = Number(document.getElementById('mcp-preview-conversation-id')?.value || 0);
    const resp = await mcpAPI.traces({ agentID, conversationID, limit: 20 });
    const traces = resp?.data?.traces || [];
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        panel.innerHTML = `<div class="empty-tip">${escapeHTML(resp?.message || resp?.data?.msg || '审计加载失败')}</div>`;
        return;
    }
    panel.innerHTML = traces.length ? traces.map(trace => `
        <div class="data-row">
            <div class="data-row-main">
                <strong>${escapeHTML(trace.tool_name || '')}</strong>
                <span>${escapeHTML(trace.trace_id || '')}</span>
            </div>
            <div class="data-row-meta">
                <span>${escapeHTML(trace.status || '')} · ${escapeHTML(trace.source || '')} · ${escapeHTML(trace.server_name || '')}</span>
                <span>${escapeHTML(trace.created_at || '')}</span>
            </div>
        </div>
    `).join('') : '<div class="empty-tip">暂无 MCP 工具调用审计</div>';
}

function fillMCPSettingForm(serverJSON) {
    const server = JSON.parse(serverJSON);
    document.getElementById('setting-mcp-id').value = server.id || '';
    document.getElementById('setting-mcp-name').value = server.name || '';
    document.getElementById('setting-mcp-desc').value = server.description || '';
    document.getElementById('setting-mcp-scope').value = server.scope || 'user';
    document.getElementById('setting-mcp-agent-id').value = server.agent_id || '';
    document.getElementById('setting-mcp-conversation-id').value = server.conversation_id || '';
    document.getElementById('setting-mcp-transport').value = server.transport || 'streamable_http';
    document.getElementById('setting-mcp-trust').value = server.trust_level || 'low';
    document.getElementById('setting-mcp-endpoint').value = server.endpoint_url || '';
    document.getElementById('setting-mcp-auth-type').value = server.auth_type || 'bearer';
    document.getElementById('setting-mcp-secret').value = '';
    document.getElementById('setting-mcp-allow-tools').value = server.allow_tools_json || '';
    document.getElementById('setting-mcp-deny-tools').value = server.deny_tools_json || '';
    document.getElementById('setting-mcp-headers').value = server.headers_json || '';
    document.getElementById('setting-mcp-command').value = server.command || '';
    document.getElementById('setting-mcp-args').value = server.args_json || '';
    document.getElementById('setting-mcp-enabled').checked = server.enabled !== false;
}

function clearMCPSettingForm() {
    ['setting-mcp-id', 'setting-mcp-name', 'setting-mcp-desc', 'setting-mcp-agent-id', 'setting-mcp-conversation-id', 'setting-mcp-endpoint', 'setting-mcp-secret', 'setting-mcp-allow-tools', 'setting-mcp-deny-tools', 'setting-mcp-headers', 'setting-mcp-command', 'setting-mcp-args'].forEach(id => {
        const el = document.getElementById(id);
        if (el) el.value = '';
    });
    const scope = document.getElementById('setting-mcp-scope');
    if (scope) scope.value = 'user';
    const transport = document.getElementById('setting-mcp-transport');
    if (transport) transport.value = 'streamable_http';
    const trust = document.getElementById('setting-mcp-trust');
    if (trust) trust.value = 'low';
    const auth = document.getElementById('setting-mcp-auth-type');
    if (auth) auth.value = 'bearer';
    const enabled = document.getElementById('setting-mcp-enabled');
    if (enabled) enabled.checked = true;
}

async function saveMCPSetting() {
    const secret = document.getElementById('setting-mcp-secret')?.value || '';
    const data = {
        id: Number(document.getElementById('setting-mcp-id')?.value || 0),
        name: document.getElementById('setting-mcp-name')?.value?.trim() || '',
        description: document.getElementById('setting-mcp-desc')?.value?.trim() || '',
        scope: document.getElementById('setting-mcp-scope')?.value || 'user',
        agent_id: Number(document.getElementById('setting-mcp-agent-id')?.value || 0),
        conversation_id: Number(document.getElementById('setting-mcp-conversation-id')?.value || 0),
        transport: document.getElementById('setting-mcp-transport')?.value || 'streamable_http',
        endpoint_url: document.getElementById('setting-mcp-endpoint')?.value?.trim() || '',
        command: document.getElementById('setting-mcp-command')?.value?.trim() || '',
        args_json: normalizeOptionalJSONText('setting-mcp-args'),
        headers_json: normalizeOptionalJSONText('setting-mcp-headers'),
        auth_type: document.getElementById('setting-mcp-auth-type')?.value || '',
        secret,
        secret_action: secret.trim() ? 'set' : 'keep',
        enabled: document.getElementById('setting-mcp-enabled')?.checked !== false,
        trust_level: document.getElementById('setting-mcp-trust')?.value || 'low',
        allow_tools_json: normalizeOptionalJSONText('setting-mcp-allow-tools'),
        deny_tools_json: normalizeOptionalJSONText('setting-mcp-deny-tools'),
    };
    if (!data.name) {
        showToast('请填写 MCP 名称', 'warning');
        return;
    }
    const resp = await settingsAPI.saveMCPServer(data);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('MCP Server 已保存', 'success');
        await renderMCPSettings();
    } else {
        showToast(resp?.message || resp?.data?.msg || '保存 MCP Server 失败', 'error');
    }
}

async function deleteMCPSetting(id) {
    if (!confirm('确定删除这个 MCP Server 配置？')) return;
    const resp = await settingsAPI.deleteMCPServer(id);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('MCP Server 已删除', 'success');
        await renderMCPSettings();
    } else {
        showToast(resp?.message || resp?.data?.msg || '删除 MCP Server 失败', 'error');
    }
}

function normalizeOptionalJSONText(id) {
    const value = document.getElementById(id)?.value?.trim() || '';
    if (!value) return '';
    try {
        JSON.parse(value);
        return value;
    } catch (err) {
        showToast(`${id} 不是合法 JSON`, 'warning');
        return value;
    }
}

function mcpScopeLabel(scope) {
    return ({ global: '全局', user: '仅本人', agent: '指定 Agent', conversation: '指定会话' })[scope] || scope || '仅本人';
}

function mcpTrustLabel(trust) {
    return ({ low: '低信任', normal: '普通信任', high: '高信任' })[trust] || trust || '低信任';
}

async function renderSkillSettings() {
    const area = document.getElementById('settings-content');
    if (!area) return;
    area.innerHTML = '<div class="empty-tip">加载中...</div>';
    const resp = await settingsAPI.listSkills('global', 0);
    const skills = resp?.data?.skills || [];
    area.innerHTML = `
        <div class="agent-help-box">
            <strong>全局 Agent Skill</strong>
            <p>这里上传的 Skill 会作为你的全局能力包，在创建 Agent 时可选择注入。支持单个 SKILL.md、zip 包或文件夹。</p>
        </div>
        <div class="settings-list" id="global-skill-list">
            ${skills.length ? skills.map(renderSkillCard).join('') : '<div class="empty-tip">暂无全局 Skill</div>'}
        </div>
        <div class="settings-editor">
            <h4>上传全局 Skill</h4>
            <div class="profile-form-grid">
                <div class="form-group">
                    <label>Skill 名称</label>
                    <input id="setting-skill-name" type="text" placeholder="例如：代码审查 / 资料总结">
                </div>
                <div class="form-group">
                    <label>说明</label>
                    <input id="setting-skill-desc" type="text" placeholder="这个 Skill 会给 Agent 增加什么能力">
                </div>
            </div>
            <div class="form-group">
                <label>上传 SKILL.md 或 zip</label>
                <input id="setting-skill-file" type="file" accept=".md,.zip">
            </div>
            <div class="form-group">
                <label>或上传 Skill 文件夹</label>
                <input id="setting-skill-folder" type="file" webkitdirectory directory multiple>
            </div>
            <label class="checkbox-row"><input id="setting-skill-default" type="checkbox"><span>设为默认全局 Skill</span></label>
            <button class="btn-primary" onclick="uploadGlobalSkill()">上传 Skill</button>
        </div>
    `;
}

function renderSkillCard(skill) {
    return `
        <div class="data-row skill-row">
            <div class="data-row-main">
                <strong>${escapeHTML(skill.name || '未命名Skill')}</strong>
                <span>${escapeHTML(skill.summary || skill.description || '暂无摘要')}</span>
            </div>
            <div class="data-row-meta">
                <span>${skill.scope === 'agent' ? 'Agent 专属' : '全局'}${skill.is_default ? ' · 默认' : ''}</span>
                <span>${escapeHTML(skill.entry_file || 'SKILL.md')}</span>
            </div>
            <div class="data-row-actions">
                <button class="btn-small" onclick="editSkillContent(${jsArg(skill.id)})">编辑</button>
                <button class="btn-small" onclick="copyText(${jsStringArg(skill.skills_dir || '')})">复制目录</button>
                <button class="btn-small danger-soft" onclick="deleteSkillSetting(${jsArg(skill.id)}, ${jsStringArg(skill.scope || 'global')}, ${jsArg(skill.agent_id || 0)})">删除</button>
            </div>
        </div>
    `;
}

async function showSkillManager() {
    await showSkillWorkspace();
}

async function loadSkillManagerList() {
    const list = document.getElementById('global-skill-list');
    if (!list) return;
    list.innerHTML = '<div class="empty-tip">加载中...</div>';
    const resp = await settingsAPI.listSkills('global', 0);
    const skills = resp?.data?.skills || [];
    list.innerHTML = skills.length ? skills.map(renderSkillCard).join('') : '<div class="empty-tip">暂无全局 Skill</div>';
}

async function editSkillContent(skillID) {
    const resp = await settingsAPI.getSkill(skillID);
    const skill = resp?.data?.skill;
    if (!(resp && resp.code === 0 && resp.data?.success && skill)) {
        showToast(resp?.message || resp?.data?.msg || '读取 Skill 失败', 'error');
        return;
    }
    showModal(`编辑 Skill - ${escapeHTML(skill.name || '')}`, `
        <div class="agent-help-box">
            <strong>${escapeHTML(skill.summary || '暂无摘要')}</strong>
            <p>修改后会覆盖该 Skill 的 SKILL.md。已选择该目录的 Agent 下次重新初始化后会使用新内容。</p>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>Skill 名称</label>
                <input id="skill-edit-name" type="text" value="${escapeHTML(skill.name || '')}">
            </div>
            <div class="form-group">
                <label>说明</label>
                <input id="skill-edit-desc" type="text" value="${escapeHTML(skill.description || '')}">
            </div>
        </div>
        <div class="form-group">
            <label>SKILL.md</label>
            <textarea id="skill-edit-content" class="code-editor-textarea" rows="18">${escapeHTML(skill.content || '')}</textarea>
        </div>
        <div class="btn-row">
            <button class="btn-secondary" onclick="showSkillManager()">返回 Skill 管理</button>
            <button class="btn-primary" onclick="saveSkillContent(${jsArg(skill.id)})">保存 Skill</button>
        </div>
    `);
}

async function saveSkillContent(skillID) {
    const data = {
        name: document.getElementById('skill-edit-name')?.value?.trim() || '',
        description: document.getElementById('skill-edit-desc')?.value?.trim() || '',
        content: document.getElementById('skill-edit-content')?.value || '',
    };
    if (!data.content.trim()) {
        showToast('SKILL.md 内容不能为空', 'warning');
        return;
    }
    const resp = await settingsAPI.updateSkillContent(skillID, data);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('Skill 已保存', 'success');
        await showSkillManager();
    } else {
        showToast(resp?.message || resp?.data?.msg || '保存 Skill 失败', 'error');
    }
}

function selectedSkillFiles(fileInputID, folderInputID) {
    const fileInput = document.getElementById(fileInputID);
    const folderInput = document.getElementById(folderInputID);
    if (folderInput?.files?.length) return folderInput.files;
    if (fileInput?.files?.length) return fileInput.files;
    return [];
}

async function uploadGlobalSkill() {
    const files = selectedSkillFiles('setting-skill-file', 'setting-skill-folder');
    if (!files.length) {
        showToast('请上传 SKILL.md、zip 或 Skill 文件夹', 'warning');
        return;
    }
    const resp = await settingsAPI.uploadSkill({
        fileList: files,
        name: document.getElementById('setting-skill-name')?.value?.trim() || '',
        description: document.getElementById('setting-skill-desc')?.value?.trim() || '',
        scope: 'global',
        isDefault: document.getElementById('setting-skill-default')?.checked || false,
    });
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('全局 Skill 已上传', 'success');
        await loadSkillManagerList();
    }
}

async function deleteSkillSetting(id, scope = 'global', agentID = 0) {
    if (!confirm('确定删除这个 Skill 配置？已落盘文件会保留，避免影响正在运行的 Agent。')) return;
    const resp = await settingsAPI.deleteSkill(id);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('Skill 已删除', 'success');
        if (scope === 'agent' && agentID) {
            await loadAgentSkillPanel(agentID);
        } else if (document.getElementById('global-skill-list')) {
            await loadSkillManagerList();
        } else {
            await showSkillManager();
        }
    } else {
        showToast(resp?.message || resp?.data?.msg || '删除失败', 'error');
    }
}

async function loadBotSidebar() {
    const list = document.getElementById('bot-list');
    const resp = await agentAPI.list();
    if (resp && resp.code === 0 && resp.data && resp.data.bots) {
        cleanupDetachedAgentMenus();
        botCache = resp.data.bots;
        agentUserIDToBot = {};
        botCache.forEach(bot => {
            if (bot.agent_user_id) {
                agentUserIDToBot[String(bot.agent_user_id)] = bot;
                userNickCache[bot.agent_user_id] = getBotDisplayName(bot);
                if (bot.avatar) userAvatarCache[bot.agent_user_id] = bot.avatar;
            }
        });
        const bots = botCache;
        if (bots.length === 0) {
            list.innerHTML = '<div class="empty-tip">暂无智能助手<br><small>点击右上角「+ 创建」添加</small></div>';
            return;
        }
        list.innerHTML = bots.map(b => `
            <div class="list-item" data-bot-id="${escapeHTML(String(b.id))}" onclick="chatWithBot(${jsArg(b.id)})">
                ${renderAvatarHTML(b.avatar, 'A', 'conv-avatar agent-avatar')}
                <div class="list-item-info">
                    <div class="list-item-top">
                        <span class="list-item-name">${escapeHTML(getBotDisplayName(b))}</span>
                        <span class="list-item-type ${b.type}">${b.is_active ? '已启用' : '已停用'}</span>
                    </div>
                    <div class="list-item-msg">${escapeHTML(b.signature || b.description || '可运行、可入群、可被 @')}</div>
                    <div class="list-item-msg">${escapeHTML(agentSourceLabel(b.type))} · UID: ${escapeHTML(String(b.agent_user_id || '未绑定'))}</div>
                </div>
                <div class="bot-item-actions" onclick="event.stopPropagation()">
                    <button class="btn-agent-pill primary" onclick="showAgentRunModal(${jsArg(b.id)}, ${jsStringArg(getBotDisplayName(b))})">运行</button>
                    <div class="agent-menu-wrapper">
                        <button class="btn-agent-pill" onclick="toggleAgentItemMenu(${jsStringArg('agent-menu-' + b.id)}, this)">管理</button>
                        <div id="agent-menu-${escapeHTML(String(b.id))}" class="agent-item-menu">
                            <button onclick="showEditAgentForm(${jsArg(b.id)})">编辑配置</button>
                            <button onclick="showAgentPermissions(${jsArg(b.id)}, ${jsStringArg(getBotDisplayName(b))})">协作权限</button>
                            <button onclick="showMemoryManager()">记忆管理</button>
                            <button onclick="addAgentFriend(${jsArg(b.id)}, ${jsStringArg(getBotDisplayName(b))})">加为好友</button>
                            <button onclick="startAgentPrivateChat(${jsArg(b.agent_user_id)})">私聊</button>
                            <button onclick="copyAgentUID(${jsArg(b.agent_user_id)})">复制 UID</button>
                            <button onclick="showBotRoutes(${jsArg(b.id)}, ${jsStringArg(b.name)})">路由规则</button>
                            <button onclick="showBotBilling(${jsArg(b.id)}, ${jsStringArg(b.name)})">计费记录</button>
                            <button onclick="toggleBot(${jsArg(b.id)}, ${!b.is_active})">${b.is_active ? '停用助手' : '启用助手'}</button>
                            <button class="danger" onclick="deleteBot(${jsArg(b.id)})">删除助手</button>
                        </div>
                    </div>
                </div>
            </div>
        `).join('');
    } else {
        list.innerHTML = '<div class="empty-tip">加载失败</div>';
    }
}

function showCreateBotForm() {
    showModal('创建智能助手', `
        <div class="form-group">
            <label>助手昵称</label>
            <input type="text" id="bot-name" placeholder="例如: 项目助手">
        </div>
        <div class="form-group">
            <label>类型</label>
            <select id="bot-type" class="form-select" onchange="onBotTypeChange()">
                <option value="internal">系统模型（使用默认模型服务）</option>
                <option value="custom">自定义模型（填写自己的密钥）</option>
            </select>
        </div>
        <div class="form-group">
            <label>使用已保存的 LLM 预设</label>
            <select id="bot-llm-profile" class="form-select" onchange="onAgentLLMProfileChange()">
                <option value="">不使用预设</option>
            </select>
            <small class="form-hint">可在系统设置中预设 BaseURL、模型和 API Key；选择后创建 Agent 时会优先使用该预设。</small>
        </div>
        <div class="form-group">
            <label>描述</label>
            <input type="text" id="bot-desc" placeholder="这个助手能帮你做什么">
        </div>
        <div class="form-group">
            <label>头像 URL</label>
            <input type="text" id="bot-avatar" placeholder="/files/agent.png 或 https://...">
        </div>
        <div class="form-group">
            <label>个性签名</label>
            <input type="text" id="bot-signature" placeholder="展示在成员列表和助手列表中">
        </div>
        <div class="form-group">
            <label>模型名称</label>
            <input type="text" id="bot-model" placeholder="例如: gpt-4o-mini（系统模型留空使用默认）">
        </div>
        <div id="custom-bot-fields" style="display:none;">
            <div class="form-group">
                <label>模型密钥 <span style="color:var(--danger);">*必填</span></label>
                <input type="password" id="bot-apikey" placeholder="你的模型服务密钥">
            </div>
            <div class="form-group">
                <label>模型服务地址 <span style="color:var(--danger);">*必填</span></label>
                <input type="text" id="bot-baseurl" placeholder="例如: https://api.openai.com/v1">
            </div>
        </div>
        <div class="form-group">
            <label>系统提示词</label>
            <textarea id="bot-prompt" rows="3" placeholder="助手的身份、边界和工作方式"></textarea>
        </div>
        <div class="form-group">
            <label>工作目录</label>
            <input type="text" id="bot-workspace" placeholder="留空则使用 storage/agent/files/{bot_id}">
            <small class="form-hint">相对路径会放在 Agent 文件根目录下；生产环境可以为每个 Agent 配置独立工作目录。</small>
        </div>
        <div class="form-group">
            <label>注入全局 Skill</label>
            <select id="bot-skill-dir" class="form-select">
                <option value="">不注入 Skill</option>
            </select>
            <small class="form-hint">全局 Skill 在系统设置中上传；创建后也可以给单个 Agent 上传专属 Skill。</small>
        </div>
        <div class="form-group">
            <label>工具策略</label>
            <select id="bot-tool-policy" class="form-select">
                <option value="safe">常规模式</option>
                <option value="approval_required">操作前确认</option>
                <option value="readonly">只读模式</option>
                <option value="disabled">禁用工具</option>
            </select>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>会话上下文条数</label>
                <input type="number" id="bot-context-limit" min="10" max="500" value="80">
                <small class="form-hint">总结、问答、洞察会读取最近多少条会话消息。</small>
            </div>
            <div class="form-group">
                <label>记忆召回条数</label>
                <input type="number" id="bot-memory-limit" min="1" max="50" value="12">
                <small class="form-hint">每轮对话最多注入多少条长期记忆。</small>
            </div>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>最大输出 Token</label>
                <input type="number" id="bot-max-output-tokens" min="0" max="32768" value="0">
                <small class="form-hint">0 表示使用模型默认值。</small>
            </div>
            <div class="form-group">
                <label>创造性</label>
                <input type="number" id="bot-temperature" min="0" max="2" step="0.1" value="0.7">
                <small class="form-hint">越低越稳定，越高越发散。</small>
            </div>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>群聊触发方式</label>
                <select id="bot-group-trigger-mode" class="form-select">
                    <option value="mention">仅 @ 或命令</option>
                    <option value="keyword">关键词触发</option>
                    <option value="command">命令触发</option>
                    <option value="all">全部消息判断</option>
                    <option value="silent">只记录不主动回复</option>
                </select>
            </div>
            <label class="form-check-row">
                <input type="checkbox" id="bot-auto-reply-enabled" checked>
                <span>允许按规则自动回复</span>
            </label>
        </div>
        <button id="create-bot-submit" class="btn-primary" onclick="createBot()">创建智能助手</button>
    `);
    loadLLMProfilesForAgentCreate();
    loadGlobalSkillsForAgentCreate();
}

function onBotTypeChange() {
    const type = document.getElementById('bot-type').value;
    const customFields = document.getElementById('custom-bot-fields');
    customFields.style.display = type === 'custom' ? 'block' : 'none';
}

async function loadLLMProfilesForAgentCreate() {
    const select = document.getElementById('bot-llm-profile');
    if (!select) return;
    try {
        const resp = await settingsAPI.listLLMProfiles();
        const profiles = resp?.data?.profiles || [];
        llmProfilesCache = profiles;
        select.innerHTML = '<option value="">不使用预设</option>' + profiles.map(profile => {
            const label = `${profile.name || '未命名预设'} · ${profile.model_name || '未设置模型'}`;
            return `<option value="${escapeHTML(String(profile.id))}">${escapeHTML(label)}</option>`;
        }).join('');
    } catch (err) {
        select.innerHTML = '<option value="">预设加载失败</option>';
    }
}

async function loadGlobalSkillsForAgentCreate() {
    const select = document.getElementById('bot-skill-dir');
    if (!select) return;
    try {
        const resp = await settingsAPI.listSkills('global', 0);
        const skills = resp?.data?.skills || [];
        select.innerHTML = '<option value="">不注入 Skill</option>' + skills.map(skill => {
            const label = `${skill.name || '未命名Skill'}${skill.is_default ? ' · 默认' : ''}`;
            return `<option value="${escapeHTML(skill.skills_dir || '')}" ${skill.is_default ? 'selected' : ''}>${escapeHTML(label)}</option>`;
        }).join('');
    } catch (err) {
        select.innerHTML = '<option value="">Skill加载失败</option>';
    }
}

function onAgentLLMProfileChange() {
    const profileID = document.getElementById('bot-llm-profile')?.value || '';
    const typeSelect = document.getElementById('bot-type');
    const modelInput = document.getElementById('bot-model');
    const baseURLInput = document.getElementById('bot-baseurl');
    const customFields = document.getElementById('custom-bot-fields');
    if (!profileID) {
        onBotTypeChange();
        return;
    }
    const profile = llmProfilesCache.find(item => String(item.id) === String(profileID));
    if (typeSelect) typeSelect.value = 'custom';
    if (customFields) customFields.style.display = 'none';
    if (modelInput && profile?.model_name) modelInput.value = profile.model_name;
    if (baseURLInput && profile?.base_url) baseURLInput.value = profile.base_url;
}

async function loadGlobalSkillsForAgentEdit(currentSkillsDir = '') {
    const select = document.getElementById('edit-agent-global-skill');
    if (!select) return;
    try {
        const resp = await settingsAPI.listSkills('global', 0);
        const skills = resp?.data?.skills || [];
        select.innerHTML = '<option value="">不切换</option>' + skills.map(skill => {
            const selected = currentSkillsDir && skill.skills_dir === currentSkillsDir ? 'selected' : '';
            return `<option value="${escapeHTML(skill.skills_dir || '')}" ${selected}>${escapeHTML(skill.name || '未命名Skill')}</option>`;
        }).join('');
    } catch (err) {
        select.innerHTML = '<option value="">Skill加载失败</option>';
    }
}

function applySelectedGlobalSkillToAgent() {
    const value = document.getElementById('edit-agent-global-skill')?.value || '';
    if (value) {
        document.getElementById('edit-agent-skills-dir').value = value;
    }
}

async function loadLLMProfilesForAgentEdit() {
    const select = document.getElementById('edit-agent-llm-profile');
    if (!select) return;
    try {
        const resp = await settingsAPI.listLLMProfiles();
        const profiles = resp?.data?.profiles || [];
        llmProfilesCache = profiles;
        select.innerHTML = '<option value="">不切换预设</option>' + profiles.map(profile => {
            const label = `${profile.name || '未命名预设'} · ${llmUsageLabel(profile.usage_type)} · ${profile.model_name || '未设置模型'}`;
            return `<option value="${escapeHTML(String(profile.id))}">${escapeHTML(label)}</option>`;
        }).join('');
    } catch (err) {
        select.innerHTML = '<option value="">预设加载失败</option>';
    }
}

function applySelectedLLMProfileToAgent() {
    const profileID = document.getElementById('edit-agent-llm-profile')?.value || '';
    const profile = llmProfilesCache.find(item => String(item.id) === String(profileID));
    if (!profile) return;
    const modelInput = document.getElementById('edit-agent-model');
    const baseURLInput = document.getElementById('edit-agent-baseurl');
    if (modelInput && profile.model_name) modelInput.value = profile.model_name;
    if (baseURLInput && profile.base_url) baseURLInput.value = profile.base_url;
    showToast('已选择 LLM 预设，保存后将通过后端注入密钥', 'info');
}

async function loadAgentSkillPanel(botID) {
    const panel = document.getElementById('edit-agent-skill-panel');
    if (!panel) return;
    panel.innerHTML = '<div class="empty-tip">加载 Skill...</div>';
    const resp = await settingsAPI.listSkills('agent', botID);
    const skills = resp?.data?.skills || [];
    panel.innerHTML = skills.length ? skills.map(renderSkillCard).join('') : '<div class="empty-tip">该 Agent 暂无专属 Skill</div>';
}

async function uploadAgentSkill(botID) {
    const files = selectedSkillFiles('edit-agent-skill-file', 'edit-agent-skill-folder');
    if (!files.length) {
        showToast('请上传 SKILL.md、zip 或 Skill 文件夹', 'warning');
        return;
    }
    const resp = await settingsAPI.uploadSkill({
        fileList: files,
        name: document.getElementById('edit-agent-skill-name')?.value?.trim() || '',
        description: document.getElementById('edit-agent-skill-desc')?.value?.trim() || '',
        scope: 'agent',
        agentID: botID,
        isDefault: true,
    });
    if (resp && resp.code === 0 && resp.data?.success && resp.data.skill) {
        document.getElementById('edit-agent-skills-dir').value = resp.data.skill.skills_dir || '';
        showToast('专属 Skill 已上传并填入配置，保存后生效', 'success');
        await loadAgentSkillPanel(botID);
    }
}

function closeAgentItemMenus(exceptID = '') {
    document.querySelectorAll('.agent-item-menu.open').forEach(menu => {
        if (!exceptID || menu.id !== exceptID) {
            menu.classList.remove('open');
            menu.style.left = '';
            menu.style.top = '';
        }
    });
}

function cleanupDetachedAgentMenus() {
    document.querySelectorAll('body > .agent-item-menu').forEach(menu => menu.remove());
}

function toggleAgentItemMenu(menuID, triggerEl = null) {
    const menu = document.getElementById(menuID);
    if (!menu) return;
    const shouldOpen = !menu.classList.contains('open');
    closeAgentItemMenus(menuID);
    if (!shouldOpen) {
        menu.classList.remove('open');
        return;
    }
    const rect = triggerEl ? triggerEl.getBoundingClientRect() : { right: window.innerWidth - 12, bottom: 0 };
    if (menu.parentElement !== document.body) {
        document.body.appendChild(menu);
    }
    menu.classList.add('open');
    const menuWidth = menu.offsetWidth || 150;
    const menuHeight = menu.offsetHeight || 260;
    const left = Math.max(8, Math.min(window.innerWidth - menuWidth - 8, rect.right - menuWidth));
    const preferredTop = rect.bottom + 6;
    const fallbackTop = rect.top - menuHeight - 6;
    menu.style.left = `${left}px`;
    menu.style.top = `${preferredTop + menuHeight > window.innerHeight - 8 ? Math.max(8, fallbackTop) : preferredTop}px`;
}

async function copyText(text, successMessage = '已复制') {
    const value = String(text || '').trim();
    if (!value) {
        showToast('没有可复制的内容', 'warning');
        return;
    }
    try {
        if (navigator.clipboard && window.isSecureContext) {
            await navigator.clipboard.writeText(value);
        } else {
            const textarea = document.createElement('textarea');
            textarea.value = value;
            textarea.setAttribute('readonly', '');
            textarea.style.position = 'fixed';
            textarea.style.left = '-9999px';
            document.body.appendChild(textarea);
            textarea.select();
            document.execCommand('copy');
            textarea.remove();
        }
        showToast(successMessage, 'success');
    } catch (err) {
        writeLocalLog('warn', '复制文本失败', { message: err.message });
        showToast('复制失败，请手动选择文本', 'warning');
    }
}

function memoryScopeLabel(scope) {
    return {
        user: '个人记忆',
        group: '群画像',
        conversation: '会话记忆',
        session: '本次会话'
    }[scope] || scope || '记忆';
}

function memoryTypeLabel(type) {
    return {
        preference: '偏好',
        speaking_style: '表达习惯',
        long_term_goal: '长期目标',
        group_profile: '群背景',
        project_state: '项目状态',
        chat_summary: '聊天摘要',
        agent_run_summary: '运行摘要',
        learning_state: '学习状态',
        repeated_issue: '反复困惑'
    }[type] || type || '事实';
}

function memoryOwnerLabel(botID) {
    if (String(botID || '0') === '0') return '系统 / IM 原生';
    const bot = botCache.find(b => sameID(b.id, botID));
    return bot ? getBotDisplayName(bot) : `Agent ${botID}`;
}

function memoryStatusLabel(status) {
    return { pending: '待确认', accepted: '已接受', rejected: '已拒绝' }[status] || status || '待确认';
}

function conversationArtifactLabel(type) {
    return {
        conversation_summary: '会话摘要',
        decision: '决策',
        task: '任务',
        topic_chunk: '话题块',
        quote: '关键表述',
        memory_candidate: '候选记忆'
    }[type] || type || '归档产物';
}

async function showMemoryManager(defaultTab = 'facts') {
    showMemoryWorkspace.defaultTab = defaultTab || 'facts';
    await showMemoryWorkspace();
}

async function switchMemoryManagerTab(tab) {
    ['facts', 'candidates'].forEach(name => {
        const panel = document.getElementById(`memory-panel-${name}`);
        const btn = document.getElementById(`memory-tab-${name}`);
        if (panel) panel.style.display = name === tab ? 'block' : 'none';
        if (btn) btn.classList.toggle('active', name === tab);
    });
    if (tab === 'candidates') {
        await loadMemoryCandidates();
    } else {
        await loadMemoryList();
    }
}

async function loadMemoryList() {
    const list = document.getElementById('memory-list');
    if (!list) return;
    list.innerHTML = '<div class="empty-tip">加载中...</div>';
    const botID = document.getElementById('memory-bot-filter')?.value || '';
    const scope = document.getElementById('memory-scope-filter')?.value || '';
    const resp = await memoryAPI.list({ bot_id: botID, scope, include_disabled: true, limit: 80 });
    if (!resp || resp.code !== 0 || !resp.data?.success) {
        list.innerHTML = `<div class="empty-tip">加载失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const memories = resp.data.memories || [];
    if (!memories.length) {
        list.innerHTML = '<div class="empty-tip">暂无记忆<br><small>可以手动添加，也可以让 Agent 对话后自动沉淀运行摘要。</small></div>';
        return;
    }
    list.innerHTML = memories.map(m => {
        return `
            <div class="data-row memory-row ${m.enabled ? '' : 'disabled'}">
                <div class="data-row-main">
                    <strong>${escapeHTML(m.title || memoryTypeLabel(m.type))}</strong>
                    <span>${renderMarkdownText(m.content || '')}</span>
                </div>
                <div class="data-row-meta">
                    <span>${escapeHTML(memoryScopeLabel(m.scope))} · ${escapeHTML(memoryTypeLabel(m.type))}</span>
                    <span>${escapeHTML(memoryOwnerLabel(m.bot_id))}</span>
                    <span>${m.visibility === 'shared' ? '共享' : '仅自己可见'}</span>
                    <span>${m.enabled ? '已启用' : '已关闭'}</span>
                </div>
                <div class="data-row-actions">
                    <button class="btn-small" onclick="showEditMemoryForm(${jsStringArg(JSON.stringify(m).replace(/</g, '\\u003c'))})">编辑</button>
                    <button class="btn-small" onclick="toggleMemoryEnabled(${jsArg(m.id)}, ${!m.enabled})">${m.enabled ? '关闭' : '启用'}</button>
                    <button class="btn-small danger-soft" onclick="deleteMemoryFact(${jsArg(m.id)})">删除</button>
                </div>
            </div>
        `;
    }).join('');
}

async function loadMemoryCandidates() {
    const list = document.getElementById('memory-candidate-list');
    if (!list) return;
    list.innerHTML = '<div class="empty-tip">加载中...</div>';
    const botID = document.getElementById('memory-candidate-bot-filter')?.value || '';
    const status = document.getElementById('memory-candidate-status-filter')?.value || '';
    const resp = await memoryAPI.candidates({ bot_id: botID, status, limit: 80 });
    if (!resp || resp.code !== 0 || !resp.data?.success) {
        list.innerHTML = `<div class="empty-tip">加载失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const candidates = resp.data.candidates || [];
    if (!candidates.length) {
        list.innerHTML = '<div class="empty-tip">暂无候选记忆<br><small>会话归档和 Agent 对话可以把长期有用的信息先放到这里。</small></div>';
        return;
    }
    list.innerHTML = candidates.map(item => `
        <div class="data-row memory-row ${item.status !== 'pending' ? 'disabled' : ''}">
            <div class="data-row-main">
                <strong>${escapeHTML(item.title || memoryTypeLabel(item.type))}</strong>
                <span>${renderMarkdownText(item.content || '')}</span>
                ${item.evidence ? `<small class="memory-evidence">证据：${escapeHTML(item.evidence)}</small>` : ''}
            </div>
            <div class="data-row-meta">
                <span>${escapeHTML(memoryOwnerLabel(item.bot_id))}</span>
                <span>${escapeHTML(memoryScopeLabel(item.scope))} · ${escapeHTML(memoryTypeLabel(item.type))}</span>
                <span>${escapeHTML(memoryStatusLabel(item.status))}</span>
                <span>置信 ${Number(item.confidence || 0).toFixed(2)}</span>
            </div>
            <div class="data-row-actions">
                ${item.status === 'pending' ? `
                    <button class="btn-small" onclick="acceptMemoryCandidate(${jsArg(item.id)})">接受</button>
                    <button class="btn-small danger-soft" onclick="rejectMemoryCandidate(${jsArg(item.id)})">拒绝</button>
                ` : `<span class="muted-small">${escapeHTML(memoryStatusLabel(item.status))}</span>`}
            </div>
        </div>
    `).join('');
}

function showCreateMemoryCandidateForm() {
    showModal('新增候选记忆', `
        <div class="agent-help-box">
            <strong>候选记忆不会立刻注入 Agent</strong>
            <p>它会先进入待确认区，确认后才会成为正式长期记忆。</p>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>归属</label>
                <select id="candidate-edit-bot" class="form-select">
                    <option value="0">系统 / IM 原生</option>
                    ${botCache.map(b => `<option value="${escapeHTML(String(b.id))}">${escapeHTML(getBotDisplayName(b))}</option>`).join('')}
                </select>
            </div>
            <div class="form-group">
                <label>范围</label>
                <select id="candidate-edit-scope" class="form-select">
                    ${['user','group','conversation','session'].map(v => `<option value="${v}">${memoryScopeLabel(v)}</option>`).join('')}
                </select>
            </div>
            <div class="form-group">
                <label>类型</label>
                <select id="candidate-edit-type" class="form-select">
                    ${['preference','speaking_style','long_term_goal','group_profile','project_state','chat_summary','agent_run_summary','learning_state','repeated_issue'].map(v => `<option value="${v}">${memoryTypeLabel(v)}</option>`).join('')}
                </select>
            </div>
            <div class="form-group">
                <label>来源</label>
                <input id="candidate-edit-source" type="text" value="user_manual" placeholder="例如 conversation-intelligence">
            </div>
        </div>
        <div class="form-group">
            <label>标题</label>
            <input id="candidate-edit-title" type="text" placeholder="例如：用户学习状态">
        </div>
        <div class="form-group">
            <label>内容</label>
            <textarea id="candidate-edit-content" rows="5" placeholder="写入一条待确认的长期事实"></textarea>
        </div>
        <div class="form-group">
            <label>证据</label>
            <textarea id="candidate-edit-evidence" rows="3" placeholder="可选：来源消息、摘要或判断依据"></textarea>
        </div>
        <div class="modal-actions">
            <button class="btn-secondary" onclick="showMemoryManager('candidates')">返回</button>
            <button class="btn-primary" onclick="saveMemoryCandidate()">创建候选</button>
        </div>
    `);
}

async function saveMemoryCandidate() {
    const data = {
        bot_id: apiID(document.getElementById('candidate-edit-bot').value),
        scope: document.getElementById('candidate-edit-scope').value,
        type: document.getElementById('candidate-edit-type').value,
        source: document.getElementById('candidate-edit-source').value.trim() || 'user_manual',
        title: document.getElementById('candidate-edit-title').value.trim(),
        content: document.getElementById('candidate-edit-content').value.trim(),
        evidence: document.getElementById('candidate-edit-evidence').value.trim(),
    };
    if (!data.content) {
        showToast('候选记忆内容不能为空', 'warning');
        return;
    }
    const resp = await memoryAPI.createCandidate(data);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('候选记忆已创建', 'success');
        await showMemoryManager('candidates');
    } else {
        showToast(resp?.message || resp?.data?.msg || '创建候选记忆失败', 'error');
    }
}

async function acceptMemoryCandidate(id) {
    const resp = await memoryAPI.acceptCandidate(id);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('候选记忆已写入正式记忆', 'success');
        await loadMemoryCandidates();
    } else {
        showToast(resp?.message || resp?.data?.msg || '接受候选记忆失败', 'error');
    }
}

async function rejectMemoryCandidate(id) {
    const resp = await memoryAPI.rejectCandidate(id);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('候选记忆已拒绝', 'success');
        await loadMemoryCandidates();
    } else {
        showToast(resp?.message || resp?.data?.msg || '拒绝候选记忆失败', 'error');
    }
}

function showCreateMemoryForm() {
    showMemoryEditorModal(null);
}

function showEditMemoryForm(memoryJSON) {
    try {
        showMemoryEditorModal(JSON.parse(memoryJSON));
    } catch (err) {
        showToast('记忆数据解析失败', 'error');
    }
}

function showMemoryEditorModal(memory) {
    const isEdit = !!memory;
    showModal(isEdit ? '编辑记忆' : '新增记忆', `
        <div class="profile-form-grid">
            <div class="form-group">
                <label>所属助手</label>
                <select id="memory-edit-bot" class="form-select">
                    <option value="0" ${String(memory?.bot_id || '0') === '0' ? 'selected' : ''}>系统 / IM 原生</option>
                    ${botCache.map(b => `<option value="${escapeHTML(String(b.id))}" ${sameID(b.id, memory?.bot_id) ? 'selected' : ''}>${escapeHTML(getBotDisplayName(b))}</option>`).join('')}
                </select>
            </div>
            <div class="form-group">
                <label>范围</label>
                <select id="memory-edit-scope" class="form-select">
                    ${['user','group','conversation','session'].map(v => `<option value="${v}" ${memory?.scope === v ? 'selected' : ''}>${memoryScopeLabel(v)}</option>`).join('')}
                </select>
            </div>
            <div class="form-group">
                <label>类型</label>
                <select id="memory-edit-type" class="form-select">
                    ${['preference','speaking_style','long_term_goal','group_profile','project_state','chat_summary','agent_run_summary','learning_state','repeated_issue'].map(v => `<option value="${v}" ${memory?.type === v ? 'selected' : ''}>${memoryTypeLabel(v)}</option>`).join('')}
                </select>
            </div>
            <div class="form-group">
                <label>可见性</label>
                <select id="memory-edit-visibility" class="form-select">
                    <option value="private" ${memory?.visibility !== 'shared' ? 'selected' : ''}>仅自己可见</option>
                    <option value="shared" ${memory?.visibility === 'shared' ? 'selected' : ''}>可共享给协作上下文</option>
                </select>
            </div>
        </div>
        <div class="form-group">
            <label>标题</label>
            <input id="memory-edit-title" type="text" value="${escapeHTML(memory?.title || '')}" placeholder="例如：回答偏好">
        </div>
        <div class="form-group">
            <label>内容</label>
            <textarea id="memory-edit-content" rows="6" placeholder="写入一条明确、可被 Agent 使用的事实">${escapeHTML(memory?.content || '')}</textarea>
        </div>
        <label class="checkbox-row">
            <input id="memory-edit-enabled" type="checkbox" ${memory?.enabled === false ? '' : 'checked'}>
            <span>启用这条记忆</span>
        </label>
        <div class="modal-actions">
            <button class="btn-secondary" onclick="showMemoryManager()">返回</button>
            <button class="btn-primary" onclick="saveMemoryFact(${isEdit ? jsArg(memory.id) : '0'})">${isEdit ? '保存' : '创建'}</button>
        </div>
    `);
}

async function saveMemoryFact(memoryID = 0) {
    const data = {
        bot_id: apiID(document.getElementById('memory-edit-bot').value),
        scope: document.getElementById('memory-edit-scope').value,
        type: document.getElementById('memory-edit-type').value,
        visibility: document.getElementById('memory-edit-visibility').value,
        title: document.getElementById('memory-edit-title').value.trim(),
        content: document.getElementById('memory-edit-content').value.trim(),
        enabled: document.getElementById('memory-edit-enabled').checked,
    };
    if (!data.content) {
        showToast('记忆内容不能为空', 'warning');
        return;
    }
    const resp = memoryID ? await memoryAPI.update(memoryID, data) : await memoryAPI.create(data);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast(memoryID ? '记忆已更新' : '记忆已创建', 'success');
        await showMemoryManager();
    } else {
        showToast(resp?.message || resp?.data?.msg || '保存记忆失败', 'error');
    }
}

async function toggleMemoryEnabled(memoryID, enabled) {
    const resp = await memoryAPI.update(memoryID, { enabled });
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast(enabled ? '记忆已启用' : '记忆已关闭', 'success');
        loadMemoryList();
    } else {
        showToast(resp?.message || resp?.data?.msg || '操作失败', 'error');
    }
}

async function deleteMemoryFact(memoryID) {
    if (!confirm('确定删除这条记忆？删除后 Agent 不会再召回它。')) return;
    const resp = await memoryAPI.delete(memoryID);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('记忆已删除', 'success');
        loadMemoryList();
    } else {
        showToast(resp?.message || resp?.data?.msg || '删除失败', 'error');
    }
}

function webSearchSourceRows(sources = []) {
    if (!sources.length) return '<div class="empty-tip">没有可展示的网页来源。</div>';
    return sources.map(src => `
        <div class="data-row web-source-row">
            <div class="data-row-main">
                <strong>${escapeHTML(src.title || src.url || '未知来源')}</strong>
                <span>${escapeHTML(src.snippet || '')}</span>
                ${(src.passages || []).length ? `<div class="web-passages">${src.passages.map(p => `<p>${renderInlineMarkdown(p)}</p>`).join('')}</div>` : ''}
            </div>
            <div class="data-row-meta">
                <span>${src.trusted ? '高可信' : '普通来源'}</span>
                <span>${escapeHTML(src.fetch_status || src.source || '')}</span>
                <span>score ${Number(src.score || 0).toFixed(2)}</span>
            </div>
            <div class="data-row-actions">
                ${src.url ? `<a class="btn-small ghost-link" href="${escapeHTML(src.url)}" target="_blank" rel="noopener noreferrer">打开</a>` : ''}
            </div>
        </div>
    `).join('');
}

function showWebSearchPanel(seedQuery = '') {
    const body = renderWorkspaceShell(
        'web-search',
        'Web Search Augmentation',
        '联网搜索增强',
        '搜索可信网页、抓取正文、清洗相关段落，并把一次性上下文交给用户或 Agent。',
        `
            <button class="btn-secondary" onclick="showKnowledgeHomeWorkspace()">返回知识工作台</button>
            <button class="btn-primary" onclick="runWebSearchAugment()">联网增强</button>
        `
    );
    if (!body) return;
    body.innerHTML = `
        <section class="workspace-section web-search-page">
            <div class="web-search-toolbar">
                <input id="web-search-query" type="text" placeholder="输入需要查询的实时问题，例如：Milvus 最新 standalone 部署方式" value="${escapeHTML(seedQuery)}">
                <select id="web-search-limit" class="form-select">
                    <option value="5">5 条</option>
                    <option value="8">8 条</option>
                    <option value="10">10 条</option>
                </select>
                <button class="btn-secondary" onclick="runWebSearchOnly()">只搜索</button>
                <button class="btn-primary" onclick="runWebSearchAugment()">联网增强</button>
            </div>
            <div class="web-search-note">
                <strong>临时增强，不入库</strong>
                <span>适合查实时版本、官方文档、价格、API 变更。结果会显示来源，默认不写入 Milvus 或长期记忆。</span>
            </div>
            <div id="web-search-result" class="data-list web-search-result">
                <div class="empty-tip">输入问题后可查看搜索结果、正文片段和增强上下文。</div>
            </div>
        </section>
    `;
}

async function runWebSearchOnly() {
    const query = document.getElementById('web-search-query')?.value.trim();
    const limit = Number(document.getElementById('web-search-limit')?.value || 5);
    const area = document.getElementById('web-search-result');
    if (!query) {
        showToast('请输入搜索关键词', 'warning');
        return;
    }
    area.innerHTML = '<div class="empty-tip">正在搜索...</div>';
    const resp = await webSearchAPI.search(query, limit);
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        area.innerHTML = `<div class="empty-tip">搜索失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    area.innerHTML = webSearchSourceRows(resp.data.results || []);
}

async function runWebSearchAugment() {
    const query = document.getElementById('web-search-query')?.value.trim();
    const limit = Number(document.getElementById('web-search-limit')?.value || 5);
    const area = document.getElementById('web-search-result');
    if (!query) {
        showToast('请输入搜索问题', 'warning');
        return;
    }
    area.innerHTML = '<div class="empty-tip">正在搜索并抓取网页正文...</div>';
    const resp = await webSearchAPI.augment({ query, limit, max_fetch: Math.min(limit, 5), max_passages: 3 });
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        area.innerHTML = `<div class="empty-tip">联网增强失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    area.innerHTML = `
        <div class="web-answer-context">
            <strong>可注入上下文</strong>
            ${renderMarkdownText(resp.data.answer_context || '')}
        </div>
        ${webSearchSourceRows(resp.data.sources || [])}
    `;
}

async function showConversationIntelligencePanel() {
    const conversationID = currentConversationID || '';
    showModal('会话智能归档', `
        <div class="agent-help-box">
            <strong>聊天记录 RAG 归档</strong>
            <p>最近消息会被提炼成会话摘要、决策、任务、话题块和候选记忆。任务状态会显示调度、失败原因和可重试次数。</p>
        </div>
        <div class="conversation-intel-toolbar">
            <input id="ci-conversation-id" type="text" placeholder="会话 ID" value="${escapeHTML(String(conversationID || ''))}">
            <select id="ci-agent-id" class="form-select">
                <option value="0">不绑定 Agent</option>
                ${botCache.map(b => `<option value="${escapeHTML(String(b.id))}">${escapeHTML(getBotDisplayName(b))}</option>`).join('')}
            </select>
            <button class="btn-secondary" onclick="loadConversationArtifacts()">查看归档</button>
            <button class="btn-secondary" onclick="loadConversationDigestJobs()">任务状态</button>
            <button class="btn-primary" onclick="createAndProcessConversationDigest()">立即归档</button>
        </div>
        <div id="conversation-intel-job" class="rag-result"></div>
        <div id="conversation-digest-job-list" class="data-list"></div>
        <div id="conversation-artifact-list" class="data-list">
            <div class="empty-tip">选择会话后可查看已有归档产物。</div>
        </div>
    `);
    if (conversationID) {
        await loadConversationDigestJobs();
        await loadConversationArtifacts();
    }
}

async function showAdminWorkspace() {
    activateStandaloneWorkspace('admin');
    document.getElementById('chat-area').style.display = 'none';
    const welcome = document.getElementById('welcome-area');
    welcome.style.display = 'flex';
    welcome.innerHTML = `
        <div class="admin-console">
            <div class="admin-console-hero">
                <div>
                    <span class="eyebrow">Admin Console</span>
                    <h2>系统管理台</h2>
                    <p>面向运营和排障的控制台：用户封禁、群聊治理、媒体预览、成本观察、知识候选审核和审计追踪集中处理。</p>
                </div>
                <div class="admin-hero-actions">
                    <button class="btn-secondary" onclick="renderAdminUsers()">查找用户</button>
                    <button class="btn-secondary" onclick="renderAdminMedia()">媒体预览</button>
                    <button class="btn-primary" onclick="renderAdminDashboard()">刷新总览</button>
                </div>
            </div>
            <div class="admin-console-tabs">
                <button class="active" data-admin-tab="dashboard" onclick="renderAdminDashboard()">总览</button>
                <button data-admin-tab="users" onclick="renderAdminUsers()">用户</button>
                <button data-admin-tab="groups" onclick="renderAdminGroups()">群聊</button>
                <button data-admin-tab="media" onclick="renderAdminMedia()">媒体</button>
                <button data-admin-tab="agents" onclick="renderAdminAgents()">Agent</button>
                <button data-admin-tab="reviews" onclick="renderAdminKnowledgeCandidates()">审核</button>
                <button data-admin-tab="mcp" onclick="renderAdminMCPAudit()">MCP</button>
                <button data-admin-tab="notices" onclick="renderAdminNotices()">公告</button>
                <button data-admin-tab="billing" onclick="renderAdminBilling()">成本</button>
                <button data-admin-tab="audits" onclick="renderAdminAudits()">审计</button>
            </div>
            <div id="admin-workspace-content" class="admin-console-content">加载中...</div>
        </div>
    `;
    await renderAdminDashboard();
}

function activateAdminTab(tab) {
    document.querySelectorAll('[data-admin-tab]').forEach(btn => {
        btn.classList.toggle('active', btn.dataset.adminTab === tab);
    });
}

async function renderAdminMedia() {
    activateAdminTab('media');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    const type = document.getElementById('admin-media-type')?.value || '';
    const uploader = document.getElementById('admin-media-uploader')?.value || '';
    area.innerHTML = '<div class="empty-tip">加载媒体文件...</div>';
    const resp = await adminAPI.files({ limit: '80', file_type: type, uploader_id: uploader });
    const files = resp?.data?.files || resp?.data?.Files || [];
    if (!(resp && resp.code === 0) || !Array.isArray(files)) {
        area.innerHTML = `<div class="empty-tip">媒体文件不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    area.innerHTML = `
        <div class="admin-section-head">
            <div>
                <h3>媒体资产</h3>
                <p>支持图片直接预览，音频内嵌播放，其他文件保留预览和下载入口。</p>
            </div>
            <span>${files.length} 个文件</span>
        </div>
        <div class="admin-filter-bar admin-media-filter">
            <select id="admin-media-type" class="form-select" onchange="renderAdminMedia()">
                <option value="">全部类型</option>
                <option value="image" ${type === 'image' ? 'selected' : ''}>图片</option>
                <option value="video" ${type === 'video' ? 'selected' : ''}>视频</option>
                <option value="audio" ${type === 'audio' ? 'selected' : ''}>音频</option>
                <option value="voice" ${type === 'voice' ? 'selected' : ''}>语音</option>
                <option value="file" ${type === 'file' ? 'selected' : ''}>普通文件</option>
            </select>
            <input id="admin-media-uploader" placeholder="上传者 ID" value="${escapeHTML(uploader)}" onkeydown="if(event.key==='Enter') renderAdminMedia()">
            <button class="btn-small" onclick="renderAdminMedia()">筛选</button>
        </div>
        <div class="admin-grid">
            ${files.length ? files.map(renderAdminFileItem).join('') : '<div class="empty-tip">暂无文件</div>'}
        </div>
    `;
}

async function renderAdminDashboard() {
    activateAdminTab('dashboard');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    area.innerHTML = '<div class="empty-tip">加载系统总览...</div>';
    const resp = await adminAPI.dashboard();
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        area.innerHTML = `<div class="empty-tip">总览不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const metrics = resp.data.metrics || [];
    const notices = resp.data.notices || [];
    const audits = resp.data.recent_audits || [];
    area.innerHTML = `
        <div class="admin-dashboard-grid">
            <section class="admin-dashboard-main">
                <div class="admin-section-head">
                    <div>
                        <h3>系统总览</h3>
                        <p>关键对象数量和最近治理动作。</p>
                    </div>
                    <span>实时读取</span>
                </div>
                <div class="admin-metric-grid">
            ${metrics.map(m => `
                <div class="admin-metric">
                    <span>${escapeHTML(m.label || m.key)}</span>
                    <strong>${escapeHTML(String(m.value || '0'))}</strong>
                    <small>${escapeHTML(m.hint || '')}</small>
                </div>
            `).join('')}
                </div>
            </section>
            <section class="admin-governance-panel">
                <h3>治理入口</h3>
                <button onclick="renderAdminUsers()"><strong>用户封禁 / 解封</strong><span>按昵称、用户名、邮箱或手机号查找</span></button>
                <button onclick="renderAdminGroups()"><strong>群聊治理</strong><span>查看群聊，可封禁/解封群聊发送能力</span></button>
                <button onclick="renderAdminKnowledgeCandidates()"><strong>知识候选审核</strong><span>处理记忆和图谱候选</span></button>
                <button onclick="renderAdminBilling()"><strong>成本分析</strong><span>按模型和日期观察调用成本</span></button>
            </section>
        </div>
        <div class="admin-two-col">
            <section class="admin-panel-card">
                <h3>近期公告</h3>
                ${notices.length ? notices.map(renderAdminNoticeRow).join('') : '<div class="empty-tip small">暂无公告</div>'}
            </section>
            <section class="admin-panel-card">
                <h3>管理审计</h3>
                ${audits.length ? audits.map(renderAdminAuditRow).join('') : '<div class="empty-tip small">暂无审计记录</div>'}
            </section>
        </div>
    `;
}

async function renderAdminUsers() {
    activateAdminTab('users');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    area.innerHTML = `
        <div class="admin-section-head">
            <div>
                <h3>用户治理</h3>
                <p>查找用户后可执行封禁/解封。封禁用户会阻止其继续登录。</p>
            </div>
            <span>真实写入 user-service</span>
        </div>
        <div class="admin-filter-bar">
            <input id="admin-user-keyword" placeholder="搜索用户名、昵称、邮箱或手机号" onkeydown="if(event.key==='Enter') renderAdminUsersList()">
            <select id="admin-user-status" class="form-select" onchange="renderAdminUsersList()">
                <option value="">全部状态</option>
                <option value="online">在线</option>
                <option value="offline">离线</option>
                <option value="banned">已封禁</option>
            </select>
            <select id="admin-user-role" class="form-select" onchange="renderAdminUsersList()">
                <option value="">全部角色</option>
                <option value="user">普通用户</option>
                <option value="admin">管理员</option>
            </select>
            <button class="btn-small" onclick="renderAdminUsersList()">搜索</button>
        </div>
        <div id="admin-user-list" class="admin-table-wrap"></div>
    `;
    await renderAdminUsersList();
}

async function renderAdminUsersList() {
    const keyword = document.getElementById('admin-user-keyword')?.value || '';
    const status = document.getElementById('admin-user-status')?.value || '';
    const role = document.getElementById('admin-user-role')?.value || '';
    const resp = await adminAPI.users({ keyword, status, role, include_system: 'true', limit: '80' });
    const list = document.getElementById('admin-user-list');
    if (!list) return;
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        list.innerHTML = `<div class="empty-tip">用户列表不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const users = resp?.data?.users || [];
    list.innerHTML = renderAdminTable(['用户', '角色', '状态', '系统用户', '创建时间', '操作'], users.map(u => [
        `<strong>${escapeHTML(u.nickname || u.username || '')}</strong><small>#${escapeHTML(entityID(u))} · ${escapeHTML(u.username || '')}</small>`,
        escapeHTML(u.role || 'user'),
        adminStatusBadge(u.status || ''),
        u.is_system ? '是' : '否',
        escapeHTML(u.created_at || ''),
        renderAdminUserActions(u),
    ]), resp?.data?.total);
}

async function renderAdminGroups() {
    activateAdminTab('groups');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    area.innerHTML = `
        <div class="admin-section-head">
            <div>
                <h3>群聊治理</h3>
                <p>查看和搜索群聊，可执行封禁/解封。封禁后群历史仍保留，但 msg-core 会拒绝新消息写入。</p>
            </div>
            <span>真实写入 group-service</span>
        </div>
        ${adminFilterBar('admin-group-keyword', '搜索群名、群号或公告', 'renderAdminGroupsList')}
        <div id="admin-group-list" class="admin-table-wrap"></div>
    `;
    await renderAdminGroupsList();
}

async function renderAdminGroupsList() {
    const keyword = document.getElementById('admin-group-keyword')?.value || '';
    const resp = await adminAPI.groups({ keyword, limit: '80' });
    const list = document.getElementById('admin-group-list');
    if (!list) return;
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        list.innerHTML = `<div class="empty-tip">群聊列表不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const groups = resp?.data?.groups || [];
    list.innerHTML = renderAdminTable(['群聊', '群主', '状态', '公告', '创建时间', '操作'], groups.map(g => [
        `<strong>${escapeHTML(g.name || '')}</strong><small>#${escapeHTML(entityID(g))}</small>`,
        escapeHTML(String(g.owner_id || '')),
        adminGroupStatusBadge(g.status || 'active'),
        escapeHTML((g.announcement || '').slice(0, 80)),
        escapeHTML(g.created_at || ''),
        `${renderAdminGroupActions(g)}<button class="btn-small ghost" onclick="copyText(${jsStringArg(entityID(g))}, '群号已复制')">复制群号</button>`,
    ]), resp?.data?.total);
}

async function renderAdminAgents() {
    activateAdminTab('agents');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    area.innerHTML = '<div class="empty-tip">加载 Agent...</div>';
    const resp = await adminAPI.agents({ limit: '100' });
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        area.innerHTML = `<div class="empty-tip">Agent 列表不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const agents = resp?.data?.agents || [];
    area.innerHTML = renderAdminTable(['Agent', '模型', '创建者', '状态', '工具策略'], agents.map(a => [
        `<strong>${escapeHTML(a.name || '')}</strong><small>#${escapeHTML(entityID(a))} · user ${escapeHTML(String(a.agent_user_id || ''))}</small>`,
        escapeHTML(a.model_name || ''),
        escapeHTML(String(a.owner_id || '')),
        a.is_active ? '启用' : '停用',
        escapeHTML(a.tool_policy || ''),
    ]), resp?.data?.total);
}

function renderAdminFileItem(file) {
    const id = file.id || file.file_id;
    const name = file.file_name || file.name || `文件 #${id}`;
    const contentType = file.content_type || '';
    const rawType = file.file_type || file.type || '';
    const type = normalizeMediaKind(rawType, contentType);
    const preview = id ? fileAPI.previewURL(id) : '';
    const previewHTML = type === 'image'
        ? `<img src="${escapeHTML(preview)}" alt="${escapeHTML(name)}" loading="lazy">`
        : (type === 'video'
            ? `<video muted controls preload="metadata" src="${escapeHTML(preview)}"></video>`
            : (type === 'voice' || type === 'audio'
            ? `<audio controls preload="metadata" src="${escapeHTML(preview)}"></audio>`
            : `<div class="admin-file-icon">${escapeHTML((type || 'FILE').toUpperCase())}</div>`));
    return `
        <div class="admin-file-card">
            <div class="admin-file-preview">${previewHTML}</div>
            <strong title="${escapeHTML(name)}">${escapeHTML(name)}</strong>
            <span>${escapeHTML(type || 'file')} · ${escapeHTML(contentType || rawType || '未知 MIME')} · ${escapeHTML(formatAdminBytes(file.size || file.file_size || 0))} · 上传者 ${escapeHTML(String(file.uploader_id || ''))}</span>
            <div class="btn-row">
                <button class="btn-small ghost" onclick="window.open(${jsStringArg(preview)}, '_blank')">预览</button>
                <button class="btn-small ghost" onclick="downloadMedia(${jsStringArg(id)}, ${jsStringArg(name)}, '')">下载</button>
            </div>
        </div>
    `;
}

function normalizeMediaKind(fileType = '', contentType = '') {
    const type = String(fileType || '').toLowerCase();
    const mime = String(contentType || '').toLowerCase();
    if (type === 'image' || mime.startsWith('image/')) return 'image';
    if (type === 'video' || mime.startsWith('video/')) return 'video';
    if (type === 'audio' || type === 'voice' || mime.startsWith('audio/')) return type === 'voice' ? 'voice' : 'audio';
    return type || 'file';
}

async function renderAdminAgentApprovals() {
    activateAdminTab('agent');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    area.innerHTML = '<div class="empty-tip">加载 Agent 审批...</div>';
    const resp = await agentAPI.listApprovals();
    const approvals = resp?.data?.approvals || [];
    area.innerHTML = approvals.length ? approvals.map(item => `
        <div class="data-row">
            <div>
                <strong>${escapeHTML(item.title || item.action || 'Agent 审批')}</strong>
                <span>${escapeHTML(item.status || 'pending')} · ${escapeHTML(item.created_at || '')}</span>
            </div>
            <div class="btn-row">
                <button class="btn-small" onclick="agentAPI.confirmApproval(${jsArg(item.id)}, '治理台通过').then(renderAdminAgentApprovals)">通过</button>
                <button class="btn-small danger-soft" onclick="agentAPI.rejectApproval(${jsArg(item.id)}).then(renderAdminAgentApprovals)">拒绝</button>
            </div>
        </div>
    `).join('') : '<div class="empty-tip">暂无 Agent 审批</div>';
}

async function renderAdminKnowledgeCandidates() {
    activateAdminTab('reviews');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    area.innerHTML = '<div class="empty-tip">加载知识候选...</div>';
    const resp = await adminAPI.reviews({ source: 'all', status: 'pending', limit: '80' });
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        area.innerHTML = `<div class="empty-tip">审核候选不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const items = resp?.data?.items || [];
    area.innerHTML = `
        <h3>待审核候选</h3>
        ${items.length ? items.map(item => `
            <div class="data-row">
                <div><strong>${escapeHTML(item.title || '候选')}</strong><span>${escapeHTML(item.source || '')} · ${escapeHTML(item.content || '')}</span></div>
                <div class="btn-row">
                    <button class="btn-small" onclick="adminReviewAction(${jsArg(item.source)}, ${jsArg(entityID(item))}, 'approve')">通过</button>
                    <button class="btn-small danger-soft" onclick="adminReviewAction(${jsArg(item.source)}, ${jsArg(entityID(item))}, 'reject')">拒绝</button>
                </div>
            </div>
        `).join('') : '<div class="empty-tip small">暂无待审候选</div>'}
    `;
}

async function adminReviewAction(source, id, action) {
    const note = prompt(action === 'approve' ? '通过说明：' : '拒绝原因：', '');
    if (note === null) return;
    const resp = await adminAPI.reviewAction({ source, item_id: id, action, note });
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('审核完成', 'success');
        renderAdminKnowledgeCandidates();
    } else {
        showToast(resp?.message || resp?.data?.msg || '审核失败', 'error');
    }
}

async function renderAdminMCPAudit() {
    activateAdminTab('mcp');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    area.innerHTML = '<div class="empty-tip">加载 MCP 审计...</div>';
    const resp = await adminAPI.mcpTraces({ limit: '80' });
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        adminMCPTraceCache = [];
        area.innerHTML = `<div class="empty-tip">MCP 审计不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const traces = resp?.data?.traces || [];
    adminMCPTraceCache = Array.isArray(traces) ? traces : [];
    area.innerHTML = adminMCPTraceCache.length ? adminMCPTraceCache.map((trace, index) => `
        <div class="data-row">
            <div>
                <strong>${escapeHTML(trace.tool_name || 'MCP Tool')}</strong>
                <span>${escapeHTML(trace.status || '')} · user ${escapeHTML(String(trace.user_id || ''))} · ${escapeHTML(trace.created_at || '')}</span>
            </div>
            <button class="btn-small ghost" onclick="showAdminMCPTraceDetail(${index})">详情</button>
        </div>
    `).join('') : '<div class="empty-tip">暂无 MCP 调用审计</div>';
}

async function renderAdminNotices() {
    activateAdminTab('notices');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    area.innerHTML = `
        <div class="admin-notice-editor">
            <input id="admin-notice-title" placeholder="公告标题">
            <select id="admin-notice-level">
                <option value="info">普通</option>
                <option value="warning">提醒</option>
                <option value="critical">重要</option>
            </select>
            <textarea id="admin-notice-content" rows="4" placeholder="公告内容"></textarea>
            <button class="btn-primary" onclick="saveAdminNotice()">发布公告</button>
        </div>
        <div id="admin-notice-list"></div>
    `;
    await loadAdminNotices();
}

async function saveAdminNotice() {
    const resp = await adminAPI.saveNotice({
        title: document.getElementById('admin-notice-title')?.value || '',
        content: document.getElementById('admin-notice-content')?.value || '',
        level: document.getElementById('admin-notice-level')?.value || 'info',
        audience: 'all',
        enabled: true,
    });
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast('公告已发布', 'success');
        renderAdminNotices();
    } else {
        showToast(resp?.message || resp?.data?.msg || '发布失败', 'error');
    }
}

async function loadAdminNotices() {
    const resp = await adminAPI.notices({ include_disabled: 'true', limit: '50' });
    const list = document.getElementById('admin-notice-list');
    if (!list) return;
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        list.innerHTML = `<div class="empty-tip">公告列表不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const notices = resp?.data?.notices || [];
    list.innerHTML = notices.length ? notices.map(renderAdminNoticeRow).join('') : '<div class="empty-tip small">暂无公告</div>';
}

async function renderAdminBilling() {
    activateAdminTab('billing');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    area.innerHTML = '<div class="empty-tip">加载成本记录...</div>';
    const resp = await adminAPI.billing({ limit: '80' });
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        area.innerHTML = `<div class="empty-tip">成本记录不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const records = resp?.data?.records || [];
    area.innerHTML = `
        <div class="admin-section-head">
            <div>
                <h3>成本看板</h3>
                <p>按日期和模型聚合当前页调用成本，明细默认折叠，避免表格挤占视线。</p>
            </div>
            <span>${records.length} 条记录</span>
        </div>
        ${renderAdminCostVisuals(records, resp?.data?.total_cost || 0, resp?.data?.total || records.length)}
        <details class="admin-cost-details">
            <summary>展开调用明细</summary>
            ${renderAdminTable(['Agent', '用户', 'Token', '费用', '模型', '时间'], records.map(r => [
                escapeHTML(String(r.bot_id || '')),
                escapeHTML(String(r.user_id || '')),
                `${r.input_tokens || 0} / ${r.output_tokens || 0}`,
                `¥${Number(r.cost || 0).toFixed(6)}`,
                escapeHTML(r.model_name || ''),
                escapeHTML(r.created_at || ''),
            ]))}
        </details>
    `;
}

function adminStatusBadge(status) {
    const label = ({ online: '在线', offline: '离线', banned: '已封禁' })[status] || status || '未知';
    return `<span class="admin-status-badge ${escapeHTML(status || 'unknown')}">${escapeHTML(label)}</span>`;
}

function adminGroupStatusBadge(status) {
    const normalized = status || 'active';
    const label = ({ active: '正常', banned: '已封禁' })[normalized] || normalized;
    return `<span class="admin-status-badge ${escapeHTML(normalized)}">${escapeHTML(label)}</span>`;
}

function renderAdminUserActions(user) {
    const id = entityID(user);
    if (user.is_system) {
        return '<span class="admin-action-muted">系统用户</span>';
    }
    if (user.status === 'banned') {
        return `<button class="btn-small ghost" onclick="updateAdminUserStatus(${jsArg(id)}, 'offline')">解封</button>`;
    }
    return `<button class="btn-small danger-soft" onclick="updateAdminUserStatus(${jsArg(id)}, 'banned')">封禁</button>`;
}

async function updateAdminUserStatus(userID, status) {
    const action = status === 'banned' ? '封禁' : '解封';
    const reason = prompt(`${action}用户 #${userID} 的原因：`, status === 'banned' ? '管理员手动封禁' : '管理员解除封禁');
    if (reason === null) return;
    const resp = await adminAPI.updateUserStatus(userID, status, reason);
    if (resp && resp.code === 0 && (resp.data?.success !== false)) {
        showToast(`${action}完成`, 'success');
        renderAdminUsersList();
    } else {
        showToast(resp?.message || resp?.data?.msg || `${action}失败`, 'error');
    }
}

function renderAdminGroupActions(group) {
    const id = entityID(group);
    const status = group.status || 'active';
    if (status === 'banned') {
        return `<button class="btn-small ghost" onclick="updateAdminGroupStatus(${jsArg(id)}, 'active')">解封群聊</button>`;
    }
    return `<button class="btn-small danger-soft" onclick="updateAdminGroupStatus(${jsArg(id)}, 'banned')">封禁群聊</button>`;
}

async function updateAdminGroupStatus(groupID, status) {
    const action = status === 'banned' ? '封禁' : '解封';
    const reason = prompt(`${action}群聊 #${groupID} 的原因：`, status === 'banned' ? '管理员手动封禁群聊' : '管理员解除群封禁');
    if (reason === null) return;
    const resp = await adminAPI.updateGroupStatus(groupID, status, reason);
    if (resp && resp.code === 0 && resp.data?.success) {
        showToast(`${action}群聊完成`, 'success');
        renderAdminGroupsList();
    } else {
        showToast(resp?.message || resp?.data?.msg || `${action}群聊失败`, 'error');
    }
}

function formatAdminBytes(value) {
    const bytes = Number(value || 0);
    if (!bytes) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB'];
    let size = bytes;
    let unit = 0;
    while (size >= 1024 && unit < units.length - 1) {
        size /= 1024;
        unit++;
    }
    return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function renderAdminCostVisuals(records, totalCost, totalCount) {
    const byDay = {};
    const byModel = {};
    records.forEach(record => {
        const day = String(record.created_at || '').slice(0, 10) || '未知日期';
        const model = record.model_name || '未知模型';
        byDay[day] = (byDay[day] || 0) + Number(record.cost || 0);
        byModel[model] = (byModel[model] || 0) + Number(record.cost || 0);
    });
    const dayEntries = Object.entries(byDay).slice(-7);
    const modelEntries = Object.entries(byModel).sort((a, b) => b[1] - a[1]).slice(0, 6);
    const maxDay = Math.max(0.000001, ...dayEntries.map(([, value]) => value));
    const modelTotal = Math.max(0.000001, modelEntries.reduce((sum, [, value]) => sum + value, 0));
    const topModel = modelEntries[0]?.[0] || '暂无';
    const conicStops = modelEntries.length ? modelEntries.reduce((state, [model, value], index) => {
        const pct = (value / modelTotal) * 100;
        const start = state.cursor;
        const end = start + pct;
        const color = ['#0f766e', '#2563eb', '#f59e0b', '#ef4444', '#8b5cf6', '#14b8a6'][index % 6];
        state.parts.push(`${color} ${start}% ${end}%`);
        state.cursor = end;
        return state;
    }, { cursor: 0, parts: [] }).parts.join(', ') : '#e2e8f0 0 100%';
    return `
        <div class="admin-cost-summary">
            <strong>当前页成本 ¥${Number(totalCost || 0).toFixed(6)}</strong>
            <span>调用 ${totalCount || records.length} 次 · 主要模型 ${escapeHTML(topModel)}</span>
        </div>
        <div class="admin-cost-visuals">
            <section>
                <h3>近几日成本</h3>
                <div class="admin-cost-bars">
                    ${dayEntries.length ? dayEntries.map(([day, value]) => `
                        <div class="admin-cost-bar" title="${escapeHTML(day)} 成本 ¥${Number(value).toFixed(6)}" data-tooltip="${escapeHTML(day)} · ¥${Number(value).toFixed(6)}">
                            <span style="height:${Math.max(8, (value / maxDay) * 100)}%"></span>
                            <small>${escapeHTML(day.slice(5))}</small>
                        </div>
                    `).join('') : '<div class="empty-tip small">暂无成本数据</div>'}
                </div>
            </section>
            <section>
                <h3>模型占比</h3>
                <div class="admin-cost-donut" style="--cost-donut:${escapeHTML(conicStops)}" title="当前页总成本 ¥${Number(totalCost || 0).toFixed(6)}，主要模型 ${escapeHTML(topModel)}" data-tooltip="总成本 ¥${Number(totalCost || 0).toFixed(6)}">
                    <strong>${modelEntries.length}</strong>
                    <span>模型</span>
                </div>
                <div class="admin-cost-models">
                    ${modelEntries.length ? modelEntries.map(([model, value]) => `
                        <div title="${escapeHTML(model)} 成本 ¥${Number(value).toFixed(6)}，占比 ${((value / modelTotal) * 100).toFixed(1)}%" data-tooltip="${escapeHTML(model)} · ¥${Number(value).toFixed(6)}">
                            <strong>${escapeHTML(model)}</strong>
                            <span><i style="width:${Math.max(4, (value / modelTotal) * 100)}%"></i></span>
                            <small>¥${Number(value).toFixed(6)}</small>
                        </div>
                    `).join('') : '<div class="empty-tip small">暂无模型成本</div>'}
                </div>
            </section>
        </div>
    `;
}

async function renderAdminAudits() {
    activateAdminTab('audits');
    const area = document.getElementById('admin-workspace-content');
    if (!area) return;
    area.innerHTML = '<div class="empty-tip">加载管理审计...</div>';
    const resp = await adminAPI.audits({ limit: '80' });
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        area.innerHTML = `<div class="empty-tip">管理审计不可用<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    const logs = resp?.data?.logs || [];
    area.innerHTML = logs.length ? logs.map(renderAdminAuditRow).join('') : '<div class="empty-tip small">暂无审计记录</div>';
}

function renderAdminNoticeRow(item) {
    return `
        <div class="data-row admin-notice-row">
            <div><strong>${escapeHTML(item.title || '')}</strong><span>${escapeHTML(item.level || 'info')} · ${escapeHTML(item.audience || 'all')} · ${escapeHTML(item.created_at || '')}</span></div>
            <p>${escapeHTML(item.content || '')}</p>
        </div>
    `;
}

function renderAdminAuditRow(item) {
    return `
        <div class="data-row">
            <div><strong>${escapeHTML(item.action || '')}</strong><span>${escapeHTML(item.target_type || '')} #${escapeHTML(item.target_id || '')} · ${escapeHTML(item.created_at || '')}</span></div>
            <small>${escapeHTML(item.detail || '')}</small>
        </div>
    `;
}

function showAdminMCPTraceDetail(index) {
    const trace = adminMCPTraceCache[index];
    if (!trace) {
        showToast('MCP 审计记录不存在', 'warning');
        return;
    }
    showModal('MCP 调用审计', `
        <div class="trace-detail-grid">
            <label>Trace ID</label><span>${escapeHTML(String(trace.trace_id || trace.id || ''))}</span>
            <label>用户</label><span>${escapeHTML(String(trace.user_id || ''))}</span>
            <label>Agent</label><span>${escapeHTML(String(trace.agent_id || ''))}</span>
            <label>会话</label><span>${escapeHTML(String(trace.conversation_id || ''))}</span>
            <label>工具</label><span>${escapeHTML(trace.tool_name || '')}</span>
            <label>来源</label><span>${escapeHTML([trace.source, trace.server_name].filter(Boolean).join(' / '))}</span>
            <label>状态</label><span>${escapeHTML(trace.status || '')}</span>
            <label>耗时</label><span>${escapeHTML(String(trace.latency_ms || 0))} ms</span>
            <label>时间</label><span>${escapeHTML(trace.created_at || '')}</span>
        </div>
        ${trace.error_message ? `<h4>错误信息</h4><pre class="trace-json">${escapeHTML(trace.error_message)}</pre>` : ''}
        <div class="empty-tip small">当前 MCP 审计表保存调用摘要；请求参数和工具返回正文未持久化，避免在管理台泄露敏感内容。</div>
    `);
}

function adminFilterBar(inputID, placeholder, fnName) {
    return `
        <div class="admin-filter-bar admin-filter-simple">
            <input id="${inputID}" placeholder="${escapeHTML(placeholder)}" onkeydown="if(event.key==='Enter') ${fnName}()">
            <button class="btn-small" onclick="${fnName}()">搜索</button>
        </div>
    `;
}

function renderAdminTable(headers, rows, total = null) {
    if (!rows.length) return '<div class="empty-tip small">暂无数据</div>';
    return `
        ${total !== null && total !== undefined ? `<div class="admin-table-total">共 ${total} 条</div>` : ''}
        <table class="admin-table">
            <thead><tr>${headers.map(h => `<th>${escapeHTML(h)}</th>`).join('')}</tr></thead>
            <tbody>
                ${rows.map(row => `<tr>${row.map(cell => `<td>${cell}</td>`).join('')}</tr>`).join('')}
            </tbody>
        </table>
    `;
}

async function showMCPTraceDetail(traceID) {
    if (!traceID) {
        showToast('缺少 MCP trace_id', 'warning');
        return;
    }
    showModal('MCP 调用详情', '<div class="empty-tip">正在读取审计详情...</div>');
    const resp = await mcpAPI.trace(traceID);
    const trace = resp?.data?.trace || resp?.data || {};
    if (!(resp && resp.code === 0) || !Object.keys(trace).length) {
        document.getElementById('modal-body').innerHTML = `
            <div class="empty-tip">读取失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>
        `;
        return;
    }
    document.getElementById('modal-body').innerHTML = `
        <div class="trace-detail-grid">
            <label>Trace ID</label><span>${escapeHTML(String(trace.trace_id || trace.id || traceID))}</span>
            <label>工具</label><span>${escapeHTML(trace.tool_name || trace.tool || '')}</span>
            <label>状态</label><span>${escapeHTML(trace.status || '')}</span>
            <label>耗时</label><span>${escapeHTML(String(trace.duration_ms || trace.latency_ms || ''))} ms</span>
            <label>创建时间</label><span>${escapeHTML(trace.created_at || '')}</span>
        </div>
        <h4>请求参数</h4>
        <pre class="trace-json">${escapeHTML(formatTraceJSON(trace.arguments_json || trace.arguments || trace.request || {}))}</pre>
        <h4>执行结果</h4>
        <pre class="trace-json">${escapeHTML(formatTraceJSON(trace.result_json || trace.result || trace.response || trace.error || {}))}</pre>
    `;
}

function formatTraceJSON(value) {
    if (typeof value === 'string') {
        try {
            return JSON.stringify(JSON.parse(value), null, 2);
        } catch (_) {
            return value;
        }
    }
    return JSON.stringify(value || {}, null, 2);
}

async function createAndProcessConversationDigest() {
    const conversationID = document.getElementById('ci-conversation-id')?.value.trim();
    const agentID = document.getElementById('ci-agent-id')?.value || '0';
    const jobArea = document.getElementById('conversation-intel-job');
    if (!conversationID) {
        showToast('请输入会话 ID', 'warning');
        return;
    }
    jobArea.innerHTML = '<div class="empty-tip">正在创建归档任务...</div>';
    const createResp = await conversationIntelligenceAPI.createJob({
        conversation_id: apiID(conversationID),
        agent_id: apiID(agentID),
        reason: 'frontend_manual',
    });
    if (!(createResp && createResp.code === 0 && createResp.data?.success)) {
        jobArea.innerHTML = `<div class="rag-status-line error">${escapeHTML(createResp?.message || createResp?.data?.msg || '创建任务失败')}</div>`;
        return;
    }
    const job = createResp.data.job || {};
    const jobID = entityID(job);
    if (!jobID) {
        jobArea.innerHTML = '<div class="rag-status-line error">任务已创建，但响应中缺少任务 ID，无法继续处理。</div>';
        await loadConversationDigestJobs();
        return;
    }
    jobArea.innerHTML = `<div class="rag-status-line">任务已创建 #${escapeHTML(jobID)}，正在处理...</div>`;
    const processResp = await conversationIntelligenceAPI.processJob(jobID);
    if (!(processResp && processResp.code === 0 && processResp.data?.success)) {
        jobArea.innerHTML = `<div class="rag-status-line error">${escapeHTML(processResp?.message || processResp?.data?.msg || '处理任务失败')}</div>`;
        return;
    }
    const doneJob = processResp.data.job || {};
    jobArea.innerHTML = `<div class="rag-status-line success">归档完成：消息 ${doneJob.message_count || 0}，有效 ${doneJob.valuable_count || 0}，状态 ${escapeHTML(doneJob.status || '')}</div>`;
    renderConversationArtifacts(processResp.data.artifacts || []);
    await loadConversationDigestJobs();
}

async function loadConversationDigestJobs() {
    const conversationID = document.getElementById('ci-conversation-id')?.value.trim();
    const list = document.getElementById('conversation-digest-job-list');
    if (!list) return;
    list.innerHTML = '<div class="empty-tip">正在读取归档任务...</div>';
    const params = { limit: 50 };
    if (conversationID) params.conversation_id = conversationID;
    const resp = await conversationIntelligenceAPI.jobs(params);
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        list.innerHTML = `<div class="empty-tip">任务读取失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    renderConversationDigestJobs(resp.data.jobs || []);
}

function renderConversationDigestJobs(jobs = []) {
    const list = document.getElementById('conversation-digest-job-list');
    if (!list) return;
    if (!jobs.length) {
        list.innerHTML = '<div class="empty-tip">暂无归档任务</div>';
        return;
    }
    const counts = jobs.reduce((acc, job) => {
        const status = job.status || 'unknown';
        acc[status] = (acc[status] || 0) + 1;
        return acc;
    }, {});
    const summary = Object.entries(counts).map(([status, count]) => `${conversationJobStatusLabel(status)} ${count}`).join(' · ');
    list.innerHTML = `
        <div class="rag-status-line">${escapeHTML(summary)}</div>
        ${jobs.map(job => `
            <div class="data-row conversation-job-row">
                <div class="data-row-main">
                    <strong>#${escapeHTML(entityID(job))} · ${escapeHTML(conversationJobStatusLabel(job.status))}</strong>
                    <span>会话 ${escapeHTML(String(job.conversation_id || ''))} · 消息 ${job.message_count || 0} / 有效 ${job.valuable_count || 0}</span>
                    ${job.error_message ? `<small class="memory-evidence">失败原因：${escapeHTML(job.error_message)}</small>` : ''}
                    ${job.next_run_at ? `<small class="memory-evidence">下次自动重试：${escapeHTML(formatDateTime(job.next_run_at))}</small>` : ''}
                </div>
                <div class="data-row-meta">
                    <span>重试 ${job.retry_count || 0}/${job.max_retries || 0}</span>
                    ${job.completed_at ? `<span>${escapeHTML(formatDateTime(job.completed_at))}</span>` : ''}
                </div>
                <div class="data-row-actions">
                    ${job.status === 'failed' ? `<button class="btn-small" onclick="retryConversationDigestJob(${jsArg(entityID(job))})">重试</button>` : ''}
                </div>
            </div>
        `).join('')}
    `;
}

function conversationJobStatusLabel(status) {
    return {
        pending: '待处理',
        processing: '处理中',
        completed: '已完成',
        skipped: '已跳过',
        failed: '失败',
    }[status] || status || '未知';
}

function formatDateTime(value) {
    if (!value) return '';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
        return String(value);
    }
    return date.toLocaleString('zh-CN', { hour12: false });
}

async function retryConversationDigestJob(jobID) {
    const area = document.getElementById('conversation-intel-job');
    if (area) area.innerHTML = `<div class="rag-status-line">正在重试归档任务 #${escapeHTML(String(jobID))}...</div>`;
    const resp = await conversationIntelligenceAPI.retryJob(jobID);
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        if (area) area.innerHTML = `<div class="rag-status-line error">${escapeHTML(resp?.message || resp?.data?.msg || '重试失败')}</div>`;
        await loadConversationDigestJobs();
        return;
    }
    const job = resp.data.job || {};
    if (area) area.innerHTML = `<div class="rag-status-line success">重试完成：状态 ${escapeHTML(conversationJobStatusLabel(job.status))}</div>`;
    renderConversationArtifacts(resp.data.artifacts || []);
    await loadConversationDigestJobs();
}

async function loadConversationArtifacts() {
    const conversationID = document.getElementById('ci-conversation-id')?.value.trim();
    const list = document.getElementById('conversation-artifact-list');
    if (!list) return;
    if (!conversationID) {
        list.innerHTML = '<div class="empty-tip">请输入会话 ID</div>';
        return;
    }
    list.innerHTML = '<div class="empty-tip">正在读取归档产物...</div>';
    const resp = await conversationIntelligenceAPI.artifacts({ conversation_id: conversationID, limit: 80 });
    if (!(resp && resp.code === 0 && resp.data?.success)) {
        list.innerHTML = `<div class="empty-tip">读取失败<br><small>${escapeHTML(resp?.message || resp?.data?.msg || '')}</small></div>`;
        return;
    }
    renderConversationArtifacts(resp.data.artifacts || []);
}

function renderConversationArtifacts(artifacts = []) {
    const list = document.getElementById('conversation-artifact-list');
    if (!list) return;
    if (!artifacts.length) {
        list.innerHTML = '<div class="empty-tip">暂无归档产物<br><small>当前窗口可能没有足够有价值的消息，或服务还未处理。</small></div>';
        return;
    }
    list.innerHTML = artifacts.map(item => `
        <div class="data-row conversation-artifact-row">
            <div class="data-row-main">
                <strong>${escapeHTML(item.title || conversationArtifactLabel(item.type))}</strong>
                <span>${renderMarkdownText(item.content || '')}</span>
                ${item.source_message_ids?.length ? `<small class="memory-evidence">来源消息：${item.source_message_ids.map(escapeHTML).join(', ')}</small>` : ''}
            </div>
            <div class="data-row-meta">
                <span>${escapeHTML(conversationArtifactLabel(item.type))}</span>
                <span>置信 ${Number(item.confidence || 0).toFixed(2)}</span>
            </div>
            <div class="data-row-actions">
                ${item.type === 'memory_candidate' ? `<button class="btn-small" onclick="showMemoryManager('candidates')">去确认</button>` : ''}
            </div>
        </div>
    `).join('');
}

async function addAgentFriend(botID, botName) {
    const resp = await agentAPI.addFriend(botID, 0, botName || '');
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已添加到好友列表', 'success');
        loadFriends();
    } else {
        showToast(resp?.data?.msg || resp?.message || '添加好友失败', 'error');
    }
}

function startAgentPrivateChat(agentUserID) {
    if (!agentUserID) {
        showToast('这个智能助手还没有绑定 UID', 'warning');
        return;
    }
    startPrivateChat(agentUserID);
}

async function copyAgentUID(agentUserID) {
    if (!agentUserID) {
        showToast('这个智能助手还没有绑定 UID', 'warning');
        return;
    }
    try {
        await navigator.clipboard.writeText(String(agentUserID));
        showToast('已复制 UID', 'success');
    } catch (e) {
        showToast(String(agentUserID), 'info');
    }
}

function showBotRoutes(botID, botName) {
    showModal(`Agent 触发规则 - ${botName}`, `
        <div class="agent-help-box">
            <strong>低打扰原则</strong>
            <div>群聊默认只响应 @。这里可以增加关键词、命令或静默记录规则，让 Agent 像 IM 原生成员一样按事件工作。</div>
        </div>
        <div class="form-group" style="display:flex;gap:8px;align-items:flex-end;">
            <div style="flex:2;">
                <label>触发内容 / 事件类型</label>
                <input type="text" id="route-pattern" placeholder="例如: 报错 / /amiya / file.uploaded">
            </div>
            <div style="flex:1;">
                <label>规则类型</label>
                <select id="route-type" class="form-select">
                    <option value="agent_keyword">关键词触发并回复</option>
                    <option value="agent_command">命令触发并回复</option>
                    <option value="agent_record">静默记录不回复</option>
                </select>
            </div>
            <button class="btn-primary" onclick="createBotRoute(${jsArg(botID)})">添加</button>
        </div>
        <div id="route-list-area" class="data-list">加载中...</div>
    `);
    loadBotRoutes(botID);
}

function showBotBilling(botID, botName) {
    showModal(`计费记录 - ${botName}`, `
        <div id="billing-list-area" class="bot-list-area">加载中...</div>
    `);
    loadBotBilling(botID);
}

async function createBot() {
    if (botCreateSubmitting) return;
    const name = document.getElementById('bot-name').value.trim();
    const type = document.getElementById('bot-type').value;
    const description = document.getElementById('bot-desc').value.trim();
    const modelName = document.getElementById('bot-model').value.trim();
    const apiKey = document.getElementById('bot-apikey')?.value?.trim() || '';
    const baseURL = document.getElementById('bot-baseurl')?.value?.trim() || '';
    const systemPrompt = document.getElementById('bot-prompt').value.trim();
    const avatar = document.getElementById('bot-avatar')?.value?.trim() || '';
    const signature = document.getElementById('bot-signature')?.value?.trim() || '';
    const workspaceRoot = document.getElementById('bot-workspace')?.value?.trim() || '';
    const skillsDir = document.getElementById('bot-skill-dir')?.value?.trim() || '';
    const toolPolicy = document.getElementById('bot-tool-policy')?.value || 'safe';
    const llmProfileID = document.getElementById('bot-llm-profile')?.value || '';
    const contextMessageLimit = Number(document.getElementById('bot-context-limit')?.value || 80);
    const memoryRecallLimit = Number(document.getElementById('bot-memory-limit')?.value || 12);
    const maxOutputTokens = Number(document.getElementById('bot-max-output-tokens')?.value || 0);
    const temperature = Number(document.getElementById('bot-temperature')?.value || 0.7);
    const groupTriggerMode = document.getElementById('bot-group-trigger-mode')?.value || 'mention';
    const autoReplyEnabled = !!document.getElementById('bot-auto-reply-enabled')?.checked;
    if (!name) { showToast('请填写助手名称', 'warning'); return; }
    if (!llmProfileID && type === 'custom' && !apiKey) { showToast('自定义模型必须填写模型密钥', 'warning'); return; }
    if (!llmProfileID && type === 'custom' && !baseURL) { showToast('自定义模型必须填写模型服务地址', 'warning'); return; }
    botCreateSubmitting = true;
    const submitBtn = document.getElementById('create-bot-submit');
    if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.textContent = '创建中...';
    }
    try {
        const resp = await agentAPI.create(name, type, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, '', {
            avatar,
            signature,
            workspace_root: workspaceRoot,
            tool_policy: toolPolicy,
            llm_profile_id: llmProfileID ? Number(llmProfileID) : 0,
            context_message_limit: contextMessageLimit,
            memory_recall_limit: memoryRecallLimit,
            max_output_tokens: maxOutputTokens,
            temperature,
            group_trigger_mode: groupTriggerMode,
            auto_reply_enabled: autoReplyEnabled,
        });
        if (resp && resp.code === 0 && resp.data && resp.data.success) {
            showToast('智能助手创建成功', 'success');
            closeModal();
            loadBotSidebar();
        } else {
            showToast(resp?.data?.msg || '创建失败', 'error');
        }
    } finally {
        botCreateSubmitting = false;
        if (submitBtn && document.body.contains(submitBtn)) {
            submitBtn.disabled = false;
            submitBtn.textContent = '创建智能助手';
        }
    }
}

async function toggleBot(botID, isActive) {
    const resp = await agentAPI.update(botID, { is_active: isActive });
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast(isActive ? '已启用' : '已停用', 'success');
        loadBotSidebar();
    } else {
        showToast(resp?.data?.msg || '操作失败', 'error');
    }
}

async function showEditAgentForm(botID) {
    const resp = await agentAPI.get(botID);
    if (!(resp && resp.code === 0 && resp.data && resp.data.bot)) {
        showToast(resp?.data?.msg || '加载智能助手失败', 'error');
        return;
    }
    const b = resp.data.bot;
    showModal(`编辑智能助手 - ${escapeHTML(getBotDisplayName(b))}`, `
        <div class="form-group">
            <label>使用已保存的 LLM 预设</label>
            <select id="edit-agent-llm-profile" class="form-select" onchange="applySelectedLLMProfileToAgent()">
                <option value="">不切换预设</option>
            </select>
            <small class="form-hint">选择预设后，后端会解析并注入 API Key；浏览器不会读取密钥明文。也可以在下方手动填写自己的 BaseURL/API Key。</small>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>昵称</label>
                <input type="text" id="edit-agent-name" value="${escapeHTML(b.name || '')}">
            </div>
            <div class="form-group">
                <label>模型</label>
                <input type="text" id="edit-agent-model" value="${escapeHTML(b.model_name || '')}">
            </div>
        </div>
        <div class="form-group">
            <label>头像 URL</label>
            <input type="text" id="edit-agent-avatar" value="${escapeHTML(b.avatar || '')}">
        </div>
        <div class="form-group">
            <label>个性签名</label>
            <input type="text" id="edit-agent-signature" value="${escapeHTML(b.signature || '')}">
        </div>
        <div class="form-group">
            <label>模型服务地址</label>
            <input type="text" id="edit-agent-baseurl" value="${escapeHTML(b.base_url || '')}">
        </div>
        <div class="form-group">
            <label>模型密钥</label>
            <input type="password" id="edit-agent-apikey" placeholder="留空表示不修改">
        </div>
        <div class="form-group">
            <label>系统提示词</label>
            <textarea id="edit-agent-prompt" rows="4">${escapeHTML(b.system_prompt || '')}</textarea>
        </div>
        <div class="form-group">
            <label>工作目录</label>
            <input type="text" id="edit-agent-workspace" value="${escapeHTML(b.workspace_root || '')}" placeholder="storage/agent/files/${escapeHTML(String(botID))}">
            <small class="form-hint">留空时使用默认 Agent 文件目录；修改后新的工具调用会在该目录内执行。</small>
        </div>
        <div class="form-group">
            <label>当前 Skill 目录</label>
            <input type="text" id="edit-agent-skills-dir" value="${escapeHTML(b.skills_dir || '')}" placeholder="可选择全局 Skill 或上传专属 Skill">
            <small class="form-hint">该目录由 settings-service 上传校验后生成；不建议手动填写项目外路径。</small>
        </div>
        <div class="form-group">
            <label>选择全局 Skill</label>
            <select id="edit-agent-global-skill" class="form-select" onchange="applySelectedGlobalSkillToAgent()">
                <option value="">不切换</option>
            </select>
        </div>
        <div id="edit-agent-skill-panel" class="settings-list">加载 Skill...</div>
        <div class="settings-editor">
            <h4>上传该 Agent 专属 Skill</h4>
            <div class="profile-form-grid">
                <div class="form-group">
                    <label>Skill 名称</label>
                    <input id="edit-agent-skill-name" type="text" placeholder="例如：项目工作流">
                </div>
                <div class="form-group">
                    <label>说明</label>
                    <input id="edit-agent-skill-desc" type="text" placeholder="这个 Skill 会给该 Agent 增加什么能力">
                </div>
            </div>
            <div class="form-group">
                <label>上传 SKILL.md 或 zip</label>
                <input id="edit-agent-skill-file" type="file" accept=".md,.zip">
            </div>
            <div class="form-group">
                <label>或上传 Skill 文件夹</label>
                <input id="edit-agent-skill-folder" type="file" webkitdirectory directory multiple>
            </div>
            <button class="btn-inline" onclick="uploadAgentSkill(${jsArg(botID)})">上传并注入该 Agent</button>
        </div>
        <div class="form-group">
            <label>工具策略</label>
            <select id="edit-agent-tool-policy" class="form-select">
                ${['safe', 'approval_required', 'readonly', 'disabled'].map(v => `<option value="${v}" ${b.tool_policy === v ? 'selected' : ''}>${toolPolicyLabel(v)}</option>`).join('')}
            </select>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>会话上下文条数</label>
                <input type="number" id="edit-agent-context-limit" min="10" max="500" value="${escapeHTML(String(b.context_message_limit || 80))}">
                <small class="form-hint">总结、问答和洞察读取的最近消息数量。</small>
            </div>
            <div class="form-group">
                <label>记忆召回条数</label>
                <input type="number" id="edit-agent-memory-limit" min="1" max="50" value="${escapeHTML(String(b.memory_recall_limit || 12))}">
                <small class="form-hint">每轮 Agent 对话最多注入的长期记忆数量。</small>
            </div>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>最大输出 Token</label>
                <input type="number" id="edit-agent-max-output-tokens" min="0" max="32768" value="${escapeHTML(String(b.max_output_tokens || 0))}">
            </div>
            <div class="form-group">
                <label>创造性</label>
                <input type="number" id="edit-agent-temperature" min="0" max="2" step="0.1" value="${escapeHTML(String(b.temperature || 0.7))}">
            </div>
        </div>
        <div class="profile-form-grid">
            <div class="form-group">
                <label>群聊触发方式</label>
                <select id="edit-agent-group-trigger-mode" class="form-select">
                    ${[
                        ['mention', '仅 @ 或命令'],
                        ['keyword', '关键词触发'],
                        ['command', '命令触发'],
                        ['all', '全部消息判断'],
                        ['silent', '只记录不主动回复'],
                    ].map(([value, label]) => `<option value="${value}" ${(b.group_trigger_mode || 'mention') === value ? 'selected' : ''}>${label}</option>`).join('')}
                </select>
            </div>
            <label class="form-check-row">
                <input type="checkbox" id="edit-agent-auto-reply-enabled" ${b.auto_reply_enabled === false ? '' : 'checked'}>
                <span>允许按规则自动回复</span>
            </label>
        </div>
        <div class="btn-row">
            <button class="btn-inline btn-primary" onclick="saveAgentConfig(${jsArg(botID)})">保存</button>
            <button class="btn-inline" onclick="showAgentPermissions(${jsArg(botID)}, ${jsStringArg(getBotDisplayName(b))})">权限管理</button>
        </div>
    `);
    loadLLMProfilesForAgentEdit();
    loadGlobalSkillsForAgentEdit(b.skills_dir || '');
    loadAgentSkillPanel(botID);
}

async function saveAgentConfig(botID) {
    const data = {
        name: document.getElementById('edit-agent-name').value.trim(),
        model_name: document.getElementById('edit-agent-model').value.trim(),
        avatar: document.getElementById('edit-agent-avatar').value.trim(),
        signature: document.getElementById('edit-agent-signature').value.trim(),
        base_url: document.getElementById('edit-agent-baseurl').value.trim(),
        api_key: document.getElementById('edit-agent-apikey').value.trim(),
        system_prompt: document.getElementById('edit-agent-prompt').value.trim(),
        skills_dir: document.getElementById('edit-agent-skills-dir').value.trim(),
        workspace_root: document.getElementById('edit-agent-workspace').value.trim(),
        tool_policy: document.getElementById('edit-agent-tool-policy').value,
        context_message_limit: Number(document.getElementById('edit-agent-context-limit')?.value || 80),
        memory_recall_limit: Number(document.getElementById('edit-agent-memory-limit')?.value || 12),
        max_output_tokens: Number(document.getElementById('edit-agent-max-output-tokens')?.value || 0),
        temperature: Number(document.getElementById('edit-agent-temperature')?.value || 0.7),
        group_trigger_mode: document.getElementById('edit-agent-group-trigger-mode')?.value || 'mention',
        auto_reply_enabled: !!document.getElementById('edit-agent-auto-reply-enabled')?.checked,
        llm_profile_id: Number(document.getElementById('edit-agent-llm-profile')?.value || 0),
    };
    if (!data.name) {
        showToast('助手昵称不能为空', 'warning');
        return;
    }
    const resp = await agentAPI.update(botID, data);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('智能助手已更新', 'success');
        closeModal();
        loadBotSidebar();
    } else {
        showToast(resp?.data?.msg || '保存失败', 'error');
    }
}

async function deleteBot(botID) {
    if (!confirm('确定要删除该智能助手吗？')) return;
    const resp = await agentAPI.delete(botID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已删除', 'success');
        loadBotSidebar();
    } else {
        showToast(resp?.data?.msg || '删除失败', 'error');
    }
}

function normalizeAgentPayload(resp) {
    if (!(resp && resp.code === 0 && resp.data)) return null;
    return resp.data.result || resp.data.result_ || resp.data.reply || resp.data.summary || resp.data.insights || resp.data.candidates || resp.data;
}

function parseAgentResultString(value) {
    if (typeof value !== 'string') return value;
    const raw = value.trim();
    if (!raw) return value;
    const fenced = raw.match(/^```json\s*([\s\S]*?)\s*```$/i);
    const jsonText = fenced ? fenced[1].trim() : raw;
    if (!jsonText.startsWith('{') && !jsonText.startsWith('[')) return value;
    try {
        return JSON.parse(jsonText);
    } catch (e) {
        return value;
    }
}

function normalizeAgentResultForView(value) {
    value = parseAgentResultString(value);
    if (value === null || value === undefined || value === '') {
        return { text: '', detail: null };
    }
    if (typeof value === 'object') {
        const textFields = ['answer', 'summary', 'content', 'reply', 'result', 'message', 'text'];
        const text = textFields.map(k => value[k]).find(v => typeof v === 'string' && v.trim()) || agentObjectToReadableText(value);
        return { text, detail: value };
    }
    return { text: String(value), detail: null };
}

function agentObjectToReadableText(value) {
    if (!value || typeof value !== 'object') return '';
    if (Array.isArray(value)) {
        return value.map((item, idx) => {
            if (typeof item === 'string') return `${idx + 1}. ${item}`;
            return `${idx + 1}. ${agentObjectToReadableText(item) || JSON.stringify(item)}`;
        }).join('\n');
    }
    const labels = {
        key_information: '关键信息',
        conclusions: '结论',
        todos: '待办',
        risks: '风险',
        reply_candidates: '回复候选',
        candidates: '候选回复',
        summary: '摘要',
        answer: '回答',
        content: '内容',
    };
    const sections = [];
    Object.entries(value).forEach(([key, val]) => {
        if (val === null || val === undefined || val === '' || key === 'metadata') return;
        const title = labels[key] || key;
        if (Array.isArray(val)) {
            if (val.length === 0) return;
            sections.push(`${title}:\n${val.map((item, idx) => {
                if (typeof item === 'string') return `${idx + 1}. ${item}`;
                if (item && typeof item === 'object') {
                    const content = item.content || item.description || item.title || item.text || item.name || JSON.stringify(item);
                    const status = item.status ? ` [${item.status}]` : '';
                    return `${idx + 1}. ${content}${status}`;
                }
                return `${idx + 1}. ${String(item)}`;
            }).join('\n')}`);
        } else if (typeof val === 'object') {
            const nested = agentObjectToReadableText(val);
            if (nested) sections.push(`${title}:\n${nested}`);
        } else {
            sections.push(`${title}: ${String(val)}`);
        }
    });
    return sections.join('\n\n');
}

function normalizeAgentActionCards(value) {
    value = parseAgentResultString(value);
    const source = value && value.data ? value.data : value;
    if (!source || typeof source !== 'object') return [];
    const direct = source.cards || source.action_cards || source.actionCards || source.action_decisions || source.decisions || source.actions;
    if (!Array.isArray(direct)) return [];
    return direct.filter(card => card && typeof card === 'object').map((card, idx) => ({
        version: card.version || '1.0',
        type: card.type || card.card_type || 'info',
        title: card.title || agentActionCardTitle(card.type || card.card_type || 'info'),
        summary: card.summary || card.description || card.content || '',
        source: card.source || card.source_ref || '',
        status: card.status || 'pending',
        actions: Array.isArray(card.actions) ? card.actions : [],
        raw: card,
        id: card.id || card.action_id || `card-${idx}`,
    }));
}

function agentActionCardTitle(type) {
    const titles = {
        approval: '等待确认',
        task: '任务候选',
        knowledge: '知识引用',
        diagnostic: '诊断结果',
        file: '文件处理',
        info: 'Agent 卡片',
    };
    return titles[type] || 'Agent 卡片';
}

function renderAgentActionCard(card) {
    const actionButtons = (card.actions || []).map(action => {
        const label = action.label || action.name || action.type || '操作';
        return `<button type="button" onclick="showToast('卡片操作已记录，后续接入持久化审批流', 'info')">${escapeHTML(label)}</button>`;
    }).join('');
    return `
        <div class="agent-action-decision-card ${escapeHTML(card.type)}">
            <div class="agent-action-decision-head">
                <strong>${escapeHTML(card.title)}</strong>
                <span>${escapeHTML(card.status)}</span>
            </div>
            <div class="agent-action-decision-body">${renderMarkdownText(card.summary || 'Agent 返回了一个结构化动作。')}</div>
            ${card.source ? `<div class="agent-action-decision-source">来源：${escapeHTML(card.source)}</div>` : ''}
            ${actionButtons ? `<div class="agent-action-decision-actions">${actionButtons}</div>` : ''}
        </div>
    `;
}

function renderAgentResult(value, meta = {}) {
    const normalized = normalizeAgentResultForView(value);
    const cards = normalizeAgentActionCards(value);
    if (!normalized.text && !normalized.detail && cards.length === 0) return '<div class="empty-tip">暂无返回内容</div>';
    const detailID = `agent-detail-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const elapsed = meta.elapsedMs !== undefined ? `<span>耗时 ${(meta.elapsedMs / 1000).toFixed(1)} 秒</span>` : '';
    const action = meta.action ? `<span>${escapeHTML(agentActionLabel(meta.action))}</span>` : '';
    const cardsHTML = cards.length ? `<div class="agent-action-decision-list">${cards.map(renderAgentActionCard).join('')}</div>` : '';
    const detailHTML = normalized.detail
        ? `
            <button type="button" class="agent-detail-toggle" onclick="toggleAgentDetail(${jsStringArg(detailID)}, this)">查看详细 JSON</button>
            <pre id="${detailID}" class="agent-result-json" style="display:none;">${escapeHTML(JSON.stringify(normalized.detail, null, 2))}</pre>
        `
        : '';
    return `
        <div class="agent-result-card">
            <div class="agent-result-meta">${action}${elapsed}</div>
            <div class="agent-result-text">${renderMarkdownText(normalized.text || JSON.stringify(normalized.detail, null, 2))}</div>
            ${cardsHTML}
            ${detailHTML}
        </div>
    `;
}

function toggleAgentDetail(detailID, btn) {
    const el = document.getElementById(detailID);
    if (!el) return;
    const showing = el.style.display !== 'none';
    el.style.display = showing ? 'none' : 'block';
    if (btn) btn.textContent = showing ? '查看详细 JSON' : '收起详细 JSON';
}

function extractAgentApproval(resp) {
    if (!(resp && resp.code === 0 && resp.data)) return null;
    const data = resp.data;
    if (data.status !== 'pending_approval' && data.msg !== 'pending_user_approval') return null;
    return {
        id: data.approval_id || data.approval?.approval_id || data.approval?.ID || '',
        botID: data.bot_id || data.approval?.bot_id || 0,
        conversationID: data.conversation_id || data.approval?.conversation_id || 0,
        description: data.reply || data.approval?.description || '这个操作需要你确认后才会继续。',
    };
}

function renderAgentApprovalCard(approval, meta = {}) {
    const detailID = `approval-note-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const resultID = `approval-result-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    const elapsed = meta.elapsedMs !== undefined ? `<span>耗时 ${(meta.elapsedMs / 1000).toFixed(1)} 秒</span>` : '';
    return `
        <div class="agent-result-card agent-approval-card">
            <div class="agent-result-meta">
                <span>等待你确认</span>
                ${meta.action ? `<span>${escapeHTML(agentActionLabel(meta.action))}</span>` : ''}
                ${elapsed}
            </div>
            <div class="agent-result-text">${renderMarkdownText(approval.description || 'Agent 请求继续执行一个需要确认的操作。')}</div>
            <textarea id="${detailID}" class="approval-note-input" rows="2" placeholder="可选：补充限制或说明，例如只读检查、不要修改文件"></textarea>
            <div class="btn-row">
                <button class="btn-inline btn-primary" onclick="confirmAgentApproval(${jsStringArg(approval.id)}, ${jsStringArg(resultID)}, ${jsStringArg(detailID)})">允许执行</button>
                <button class="btn-inline" onclick="rejectAgentApproval(${jsStringArg(approval.id)}, ${jsStringArg(resultID)})">拒绝</button>
            </div>
            <div id="${resultID}" class="agent-approval-followup"></div>
        </div>
    `;
}

async function confirmAgentApproval(approvalID, resultElID, noteElID) {
    const area = document.getElementById(resultElID);
    const message = document.getElementById(noteElID)?.value.trim() || '';
    if (area) area.innerHTML = '<div class="search-loading"><div class="spinner"></div>已允许，Agent 正在继续执行...</div>';
    const resp = await agentAPI.confirmApproval(approvalID, message);
    const pending = extractAgentApproval(resp);
    if (pending) {
        if (area) area.innerHTML = renderAgentApprovalCard(pending, { action: 'run' });
        return;
    }
    if (resp && resp.code === 0 && resp.data && resp.data.success !== false) {
        if (area) area.innerHTML = renderAgentResult(normalizeAgentPayload(resp), { action: 'run' });
    } else if (area) {
        area.innerHTML = `<div class="agent-error-card"><strong>继续执行失败</strong><span>${escapeHTML(resp?.data?.msg || resp?.message || 'Agent 执行失败')}</span></div>`;
    }
}

async function rejectAgentApproval(approvalID, resultElID) {
    const area = document.getElementById(resultElID);
    const resp = await agentAPI.rejectApproval(approvalID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        if (area) area.innerHTML = '<div class="empty-tip">已拒绝，本次操作不会执行。</div>';
    } else if (area) {
        area.innerHTML = `<div class="agent-error-card"><strong>拒绝失败</strong><span>${escapeHTML(resp?.data?.msg || resp?.message || '审批记录处理失败')}</span></div>`;
    }
}

async function runAgentTask(action, botID, conversationID, question, resultElID, buttonEl = null) {
    const area = document.getElementById(resultElID);
    const startedAt = Date.now();
    let timer = null;
    if (buttonEl) {
        buttonEl.disabled = true;
        buttonEl.dataset.originalText = buttonEl.textContent;
        buttonEl.textContent = '执行中...';
    }
    if (area) {
        area.innerHTML = `
            <div class="agent-running">
                <div class="spinner"></div>
                <div>
                    <strong>${escapeHTML(agentActionLabel(action))}中</strong>
                    <span id="${resultElID}-timer">已思考 0.0 秒</span>
                </div>
            </div>
        `;
        timer = setInterval(() => {
            const timerEl = document.getElementById(`${resultElID}-timer`);
            if (timerEl) timerEl.textContent = `已思考 ${((Date.now() - startedAt) / 1000).toFixed(1)} 秒`;
        }, 100);
    }
    const apiMap = {
        run: agentAPI.run,
        summarize: agentAPI.summarize,
        ask: agentAPI.ask,
        insights: agentAPI.insights,
        replyCandidates: agentAPI.replyCandidates,
    };
    try {
        const resp = await apiMap[action](botID, conversationID, question || '');
        const result = normalizeAgentPayload(resp);
        const elapsedMs = Date.now() - startedAt;
        const approval = extractAgentApproval(resp);
        if (approval) {
            if (area) area.innerHTML = renderAgentApprovalCard(approval, { action, elapsedMs });
        } else if (resp && resp.code === 0 && resp.data && resp.data.success !== false) {
            if (area) area.innerHTML = renderAgentResult(result, { action, elapsedMs });
        } else if (area) {
            area.innerHTML = `<div class="agent-error-card"><strong>执行失败</strong><span>${escapeHTML(resp?.data?.msg || resp?.message || '智能助手执行失败')}</span><small>请检查助手权限、模型配置、工具策略或后端运行日志。</small></div>`;
        }
    } finally {
        if (timer) clearInterval(timer);
        if (buttonEl) {
            buttonEl.disabled = false;
            buttonEl.textContent = buttonEl.dataset.originalText || '执行';
        }
    }
}

async function showAgentRunModal(botID, botName) {
    const defaultConversationID = currentConversationID || 0;
    const historyKey = String(botID);
    if (!agentRunHistories[historyKey]) {
        agentRunHistories[historyKey] = [{
            role: 'agent',
            content: '可以连续和我对话，也可以选择一个会话作为上下文来源。需要总结、问答、提取结论或执行任务时，直接写在下面。',
            time: new Date().toLocaleTimeString(),
        }];
    }
    showModal(`运行智能助手 - ${escapeHTML(botName)}`, `
        <div class="agent-run-shell">
            <div class="agent-run-toolbar">
                <div class="agent-run-field">
                    <label>上下文会话</label>
                    <select id="agent-run-conversation" class="form-select">
                        <option value="${escapeHTML(String(defaultConversationID || 0))}">正在加载会话...</option>
                    </select>
                </div>
                <div class="agent-run-field">
                    <label>任务类型</label>
                    <select id="agent-run-action" class="form-select" onchange="selectAgentAction('agent-run-action', this.value, 'agent-run-action-hint')">
                        <option value="ask">连续对话</option>
                        <option value="summarize">会话总结</option>
                        <option value="insights">提取结论</option>
                        <option value="replyCandidates">生成回复</option>
                        <option value="run">执行任务</option>
                    </select>
                </div>
            </div>
            ${renderAgentActionGrid('agent-run-action', 'agent-run-action-hint', 'ask')}
            <div id="agent-run-history" class="agent-chat-history"></div>
            <div class="agent-chat-composer">
                <textarea id="agent-run-question" rows="2" placeholder="输入给 Agent 的指令，Enter 发送，Shift+Enter 换行" onkeydown="handleAgentComposerKeydown(event, ${jsArg(botID)})"></textarea>
                <button id="agent-run-submit" class="btn-send" onclick="submitAgentRun(${jsArg(botID)}, this)">发送</button>
            </div>
            <div class="agent-run-footer">
                <button class="btn-inline" onclick="loadAgentSessions(${jsArg(botID)}, document.getElementById('agent-run-conversation')?.value || 0, 'agent-run-history')">查看会话记录</button>
            </div>
        </div>
    `);
    renderAgentRunHistory(botID);
    const options = await fetchAgentContextConversations(defaultConversationID);
    const select = document.getElementById('agent-run-conversation');
    if (select) {
        select.innerHTML = options.map(o => `<option value="${escapeHTML(String(o.id))}" ${sameID(o.id, defaultConversationID) ? 'selected' : ''}>${escapeHTML(o.label)}</option>`).join('');
    }
}

function renderAgentRunHistory(botID) {
    const area = document.getElementById('agent-run-history');
    if (!area) return;
    const shouldStickToBottom = area.scrollHeight - area.scrollTop - area.clientHeight < 48;
    const history = agentRunHistories[String(botID)] || [];
    area.innerHTML = history.map(item => {
        if (item.kind === 'approval' && item.approval) {
            return `<div class="agent-chat-turn agent">${renderAgentApprovalCard(item.approval, { action: item.action || 'run' })}</div>`;
        }
        if (item.kind === 'sessions') {
            return `<div class="agent-chat-turn agent"><div class="agent-session-list">${item.html}</div></div>`;
        }
        const resultCardsHTML = item.result && normalizeAgentActionCards(item.result).length
            ? `<div class="agent-chat-cards">${normalizeAgentActionCards(item.result).map(renderAgentActionCard).join('')}</div>`
            : '';
        const thinkingHTML = item.kind === 'thinking'
            ? `<span class="agent-thinking-timer">已思考 ${(((Date.now() - (item.startedAt || Date.now())) / 1000)).toFixed(1)} 秒</span>`
            : '';
        const durationHTML = item.durationMs
            ? `<span class="agent-thinking-duration">思考 ${(item.durationMs / 1000).toFixed(1)} 秒</span>`
            : '';
        return `
            <div class="agent-chat-turn ${item.role === 'user' ? 'user' : 'agent'}">
                <div class="agent-chat-avatar">${item.role === 'user' ? '我' : 'A'}</div>
                <div class="agent-chat-bubble">
                    <div class="agent-chat-meta">${item.role === 'user' ? '你' : '智能助手'} · ${escapeHTML(item.time || '')}${thinkingHTML}${durationHTML}</div>
                    <div class="agent-chat-text">${item.role === 'agent' ? renderMarkdownText(item.content || '') : escapeHTML(item.content || '')}</div>
                    ${resultCardsHTML}
                </div>
            </div>
        `;
    }).join('');
    if (shouldStickToBottom) {
        area.scrollTop = area.scrollHeight;
    }
}

function pushAgentRunHistory(botID, item) {
    const key = String(botID);
    if (!agentRunHistories[key]) agentRunHistories[key] = [];
    agentRunHistories[key].push({ time: new Date().toLocaleTimeString(), ...item });
    renderAgentRunHistory(botID);
}

function handleAgentComposerKeydown(event, botID) {
    if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault();
        submitAgentRun(botID, document.getElementById('agent-run-submit'));
    }
}

async function submitAgentRun(botID, buttonEl = null) {
    const conversationID = document.getElementById('agent-run-conversation').value || 0;
    const questionEl = document.getElementById('agent-run-question');
    const question = questionEl.value.trim();
    const action = document.getElementById('agent-run-action').value;
    if (!question) {
        showToast('请输入要交给 Agent 的内容', 'warning');
        return;
    }
    pushAgentRunHistory(botID, { role: 'user', content: question });
    questionEl.value = '';
    if (buttonEl) {
        buttonEl.disabled = true;
        buttonEl.dataset.originalText = buttonEl.textContent;
        buttonEl.textContent = '发送中';
    }
    const startedAt = Date.now();
    pushAgentRunHistory(botID, { role: 'agent', kind: 'thinking', content: `${agentActionLabel(action)}中...`, startedAt });
    const key = String(botID);
    const thinkingIndex = agentRunHistories[key].length - 1;
    const timer = setInterval(() => renderAgentRunHistory(botID), 100);
    const apiMap = {
        run: agentAPI.run,
        summarize: agentAPI.summarize,
        ask: agentAPI.ask,
        insights: agentAPI.insights,
        replyCandidates: agentAPI.replyCandidates,
    };
    try {
        const resp = await apiMap[action](botID, conversationID, question);
        const approval = extractAgentApproval(resp);
        const durationMs = Date.now() - startedAt;
        if (approval) {
            agentRunHistories[key][thinkingIndex] = { role: 'agent', kind: 'approval', approval, action, durationMs, time: new Date().toLocaleTimeString() };
        } else if (resp && resp.code === 0 && resp.data && resp.data.success !== false) {
            const payload = normalizeAgentPayload(resp);
            const normalized = normalizeAgentResultForView(payload);
            agentRunHistories[key][thinkingIndex] = { role: 'agent', content: normalized.text || '执行完成，但没有返回文本。', result: payload, durationMs, time: new Date().toLocaleTimeString() };
        } else {
            agentRunHistories[key][thinkingIndex] = { role: 'agent', content: resp?.data?.msg || resp?.message || '智能助手执行失败', durationMs, time: new Date().toLocaleTimeString() };
        }
        renderAgentRunHistory(botID);
    } finally {
        clearInterval(timer);
        if (buttonEl) {
            buttonEl.disabled = false;
            buttonEl.textContent = buttonEl.dataset.originalText || '发送';
        }
    }
}

async function loadAgentSessions(botID, conversationID, resultElID) {
    const area = document.getElementById(resultElID);
    if (area) area.innerHTML = '<div class="search-loading"><div class="spinner"></div>正在加载会话记录...</div>';
    const resp = await agentAPI.listSessions(botID, conversationID || 0);
    if (!(resp && resp.code === 0 && resp.data && resp.data.success)) {
        const msg = resp?.data?.msg || '会话记录加载失败';
        if (resultElID === 'agent-run-history') {
            pushAgentRunHistory(botID, { role: 'agent', content: msg });
        } else if (area) area.innerHTML = `<div class="empty-tip">${escapeHTML(msg)}</div>`;
        return;
    }
    const sessions = resp.data.sessions || [];
    if (sessions.length === 0) {
        if (resultElID === 'agent-run-history') {
            pushAgentRunHistory(botID, { role: 'agent', content: '暂无会话记录。' });
        } else if (area) area.innerHTML = '<div class="empty-tip">暂无会话记录</div>';
        return;
    }
    const sessionHTML = sessions.map(s => {
        const title = s.title || '未命名会话';
        const shortID = String(s.session_id || '').replace(/^agent_/, '');
        return `
            <div class="agent-session-card">
                <div class="agent-session-main">
                    <strong>${escapeHTML(title)}</strong>
                    <span>${escapeHTML(s.created_at || '未知时间')}</span>
                </div>
                <code>${escapeHTML(shortID)}</code>
            </div>
        `;
    }).join('');
    if (resultElID === 'agent-run-history') {
        pushAgentRunHistory(botID, { role: 'agent', kind: 'sessions', html: sessionHTML });
    } else if (area) {
        area.innerHTML = `<div class="agent-session-list">${sessionHTML}</div>`;
    }
}

async function showAgentPermissions(botID, botName) {
    showModal(`协作权限 - ${escapeHTML(botName)}`, `
        <div class="form-group" style="display:flex;gap:8px;align-items:flex-end;">
            <div style="flex:1;">
                <label>用户 UID</label>
                <input type="text" id="agent-perm-user" placeholder="输入用户 UID">
            </div>
            <div style="width:130px;">
                <label>权限</label>
                <select id="agent-perm-role" class="form-select">
                    <option value="viewer">查看者</option>
                    <option value="operator">使用者</option>
                    <option value="admin">协管员</option>
                </select>
            </div>
            <button class="btn-inline btn-primary" onclick="grantAgentPermission(${jsArg(botID)}, ${jsStringArg(botName)})">授权</button>
        </div>
        <div id="agent-permissions-list" class="bot-list-area">加载中...</div>
    `);
    loadAgentPermissions(botID, botName);
}

async function loadAgentPermissions(botID, botName) {
    const area = document.getElementById('agent-permissions-list');
    if (!area) return;
    area.innerHTML = '<div class="search-loading"><div class="spinner"></div>加载中...</div>';
    const resp = await agentAPI.listPermissions(botID);
    const perms = resp?.data?.permissions || [];
    if (!(resp && resp.code === 0)) {
        area.innerHTML = `<div class="empty-tip">${escapeHTML(resp?.data?.msg || '权限加载失败')}</div>`;
        return;
    }
    if (perms.length === 0) {
        area.innerHTML = '<div class="empty-tip">暂无授权用户</div>';
        return;
    }
    area.innerHTML = perms.map(p => `
        <div class="bot-item">
            <div class="bot-info">
                <span class="bot-name">UID ${escapeHTML(String(p.user_id))}</span>
                <span class="bot-type internal">${escapeHTML(agentRoleLabel(p.role))}</span>
                <span class="bot-status active">${escapeHTML(p.created_at || '')}</span>
            </div>
            <div class="bot-actions">
                ${p.role === 'owner' ? '' : `<button class="btn-inline btn-danger" onclick="revokeAgentPermission(${jsArg(botID)}, ${jsArg(p.user_id)}, ${jsStringArg(botName)})">撤销</button>`}
            </div>
        </div>
    `).join('');
}

async function grantAgentPermission(botID, botName) {
    const userID = document.getElementById('agent-perm-user').value.trim();
    const role = document.getElementById('agent-perm-role').value;
    if (!/^\d+$/.test(userID)) {
        showToast('请输入有效 UID', 'warning');
        return;
    }
    const resp = await agentAPI.grantPermission(botID, userID, role);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('授权成功', 'success');
        loadAgentPermissions(botID, botName);
    } else {
        showToast(resp?.data?.msg || '授权失败', 'error');
    }
}

async function revokeAgentPermission(botID, userID, botName) {
    if (!confirm('确定撤销该用户的智能助手权限吗？')) return;
    const resp = await agentAPI.revokePermission(botID, userID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('权限已撤销', 'success');
        loadAgentPermissions(botID, botName);
    } else {
        showToast(resp?.data?.msg || '撤销失败', 'error');
    }
}

let currentBotID = null;
let botChatHistory = {};
let botThinkingID = null;
let botPendingReplies = {};

function chatWithBot(botID) {
    activateChatWorkspaceForContent();
    closeModal();
    conversationOpenSeq++;
    botChatSeq++;
    document.querySelectorAll('.msg-thinking').forEach(el => el.remove());
    currentBotID = botID;
    currentConversationID = null;
    currentConversationType = '';

    const botEl = Array.from(document.querySelectorAll('[data-bot-id]')).find(el => sameID(el.dataset.botId, botID));
    const botName = botEl ? botEl.querySelector('.list-item-name')?.textContent || '智能助手' : '智能助手';

    document.getElementById('welcome-area').style.display = 'none';
    document.getElementById('chat-area').style.display = 'flex';
    document.getElementById('chat-title').textContent = botName;
    document.getElementById('chat-type-badge').textContent = '智能助手';
    document.getElementById('chat-type-badge').className = 'chat-type-badge group';
    document.getElementById('group-announcement-bar').style.display = 'none';
    document.getElementById('message-list').innerHTML = '';
    document.getElementById('broadcast-btn').style.display = 'none';
    document.getElementById('mention-btn').style.display = 'none';
    document.getElementById('msg-input').disabled = false;
    document.getElementById('send-btn').disabled = false;
    document.getElementById('voice-record-btn').disabled = false;

    if (botChatHistory[botID]) {
        botChatHistory[botID].forEach(m => {
            document.getElementById('message-list').innerHTML += createMessageHTML(m);
        });
        document.getElementById('message-list').scrollTop = document.getElementById('message-list').scrollHeight;
    }
    if (botPendingReplies[botID]) {
        document.getElementById('message-list').innerHTML += createMessageHTML(botPendingReplies[botID]);
        document.getElementById('message-list').scrollTop = document.getElementById('message-list').scrollHeight;
    }

    document.getElementById('msg-input').placeholder = `向 ${botName} 提问...`;

    const sendBtn = document.getElementById('send-btn');
    sendBtn.setAttribute('onclick', 'sendBotChatMsg()');
}

async function sendBotChatMsg() {
    const input = document.getElementById('msg-input');
    const content = input.value.trim();
    if (!content || !currentBotID) return;

    input.value = '';
    const activeBotID = currentBotID;
    const activeBotSeq = botChatSeq;
    const botConversationID = Number(currentUser.id) * 1000000 + Number(activeBotID);

    const now = new Date();
    const timeStr = now.getFullYear() + '-' +
        String(now.getMonth() + 1).padStart(2, '0') + '-' +
        String(now.getDate()).padStart(2, '0') + ' ' +
        String(now.getHours()).padStart(2, '0') + ':' +
        String(now.getMinutes()).padStart(2, '0') + ':' +
        String(now.getSeconds()).padStart(2, '0');

    const userMsg = { sender_id: currentUser.id, content: content, created_at: timeStr };
    appendMessage(userMsg);
    if (!botChatHistory[activeBotID]) botChatHistory[activeBotID] = [];
    botChatHistory[activeBotID].push(userMsg);

    const thinkingID = 'thinking-' + Date.now();
    botThinkingID = thinkingID;
    const startedAt = Date.now();
    const thinkingMsg = { sender_id: 0, content: '智能助手处理中...', created_at: timeStr, is_thinking: true, _thinkingID: thinkingID, started_at: startedAt };
    appendMessage(thinkingMsg);
    botPendingReplies[activeBotID] = thinkingMsg;

    const resp = await agentAPI.chat(activeBotID, content, botConversationID);

    delete botPendingReplies[activeBotID];
    const isStillActive = currentBotID === activeBotID && activeBotSeq === botChatSeq;
    if (isStillActive) {
        const container = document.getElementById('message-list');
        const thinkingEl = container.querySelector(`[data-thinking-id="${thinkingID}"]`) || container.querySelector('.msg-thinking');
        if (thinkingEl) thinkingEl.remove();
    }

    const approval = extractAgentApproval(resp);
    if (approval) {
        const durationMs = Date.now() - startedAt;
        const approvalMsg = { sender_id: 0, content: approval.description, created_at: timeStr, is_bot: true, is_approval: true, approval, agent_thinking_duration_ms: durationMs };
        botChatHistory[activeBotID].push(approvalMsg);
        if (isStillActive) {
            appendMessage(approvalMsg);
        }
    } else if (resp && resp.code === 0 && resp.data && resp.data.success) {
        const replyTime = new Date();
        const replyTimeStr = replyTime.getFullYear() + '-' +
            String(replyTime.getMonth() + 1).padStart(2, '0') + '-' +
            String(replyTime.getDate()).padStart(2, '0') + ' ' +
            String(replyTime.getHours()).padStart(2, '0') + ':' +
            String(replyTime.getMinutes()).padStart(2, '0') + ':' +
            String(replyTime.getSeconds()).padStart(2, '0');

        const durationMs = Date.now() - startedAt;
        const botMsg = { sender_id: 0, content: resp.data.reply, created_at: replyTimeStr, is_bot: true, agent_thinking_duration_ms: durationMs };
        botChatHistory[activeBotID].push(botMsg);
        if (isStillActive) {
            appendMessage(botMsg);
        }
    } else {
        const errMsg = { sender_id: 0, content: `❌ 对话失败: ${resp?.data?.msg || '未知错误'}`, created_at: timeStr, is_error: true };
        botChatHistory[activeBotID].push(errMsg);
        if (isStillActive) {
            appendMessage(errMsg);
        }
    }
    if (isStillActive) {
        botThinkingID = null;
    }
}

async function createBotRoute(botID) {
    const pattern = document.getElementById('route-pattern').value.trim();
    const routeType = document.getElementById('route-type').value;
    if (!pattern) { showToast('请填写路由模式', 'warning'); return; }
    const resp = await agentAPI.createRoute(botID, pattern, routeType, 0);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('Agent 触发规则已添加', 'success');
        loadBotRoutes(botID);
    } else {
        showToast(resp?.data?.msg || '添加失败', 'error');
    }
}

async function loadBotRoutes(botID) {
    const area = document.getElementById('route-list-area');
    if (!botID) { area.innerHTML = '<div class="empty-tip">请输入Agent ID</div>'; return; }
    area.innerHTML = '<div class="search-loading"><div class="spinner"></div>加载中...</div>';
    const resp = await agentAPI.listRoutes(botID);
    if (resp && resp.code === 0 && resp.data && resp.data.routes) {
        const routes = resp.data.routes;
        if (routes.length === 0) {
            area.innerHTML = '<div class="empty-tip">暂无 Agent 触发规则</div>';
            return;
        }
        area.innerHTML = routes.map(r => `
            <div class="data-row route-row">
                <div class="data-row-main">
                    <strong>${escapeHTML(r.route_pattern)}</strong>
                    <span>${escapeHTML(routeTypeLabel(r.route_type))}</span>
                </div>
                <div class="data-row-meta">
                    <span>优先级 ${r.priority || 0}</span>
                </div>
                <div class="data-row-actions">
                    <button class="btn-inline btn-danger" onclick="deleteBotRoute(${jsArg(r.id)}, ${jsArg(botID)})">删除</button>
                </div>
            </div>
        `).join('');
    } else {
        area.innerHTML = '<div class="empty-tip">加载失败</div>';
    }
}

async function deleteBotRoute(routeID, botID) {
    if (!confirm('确定删除该 Agent 触发规则？')) return;
    const resp = await agentAPI.deleteRoute(routeID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已删除', 'success');
        loadBotRoutes(botID);
    } else {
        showToast(resp?.data?.msg || '删除失败', 'error');
    }
}

async function loadBotBilling(botID) {
    const area = document.getElementById('billing-list-area');
    if (!botID) { area.innerHTML = '<div class="empty-tip">参数错误</div>'; return; }
    area.innerHTML = '<div class="search-loading"><div class="spinner"></div>加载中...</div>';
    const resp = await agentAPI.getBilling(botID);
    if (resp && resp.code === 0 && resp.data && resp.data.records) {
        const records = resp.data.records;
        if (records.length === 0) {
            area.innerHTML = '<div class="empty-tip">暂无计费记录</div>';
            return;
        }
        area.innerHTML = records.map(r => `
            <div class="bot-item">
                <div class="bot-info">
                    <span class="bot-name">计费 ${escapeHTML(r.model_name || '')}</span>
                    <span class="bot-status active">Token: ${(r.input_tokens || 0) + (r.output_tokens || 0)}</span>
                    <span class="bot-type internal">费用: ¥${(r.cost || 0).toFixed(6)}</span>
                </div>
                <div class="bot-desc">输入 ${r.input_tokens || 0} / 输出 ${r.output_tokens || 0} · ${escapeHTML(r.created_at || '')}</div>
            </div>
        `).join('');
    } else {
        area.innerHTML = '<div class="empty-tip">加载失败</div>';
    }
}

window.addEventListener('hashchange', () => {
    if (token && currentUser) {
        renderWorkspace(workspaceFromHash());
    }
});

window.onload = function () {
    bindVoiceRecorder();
    restoreRAGUploadJobs();
    if (token && currentUser) {
        enterMainPage();
    }
};


