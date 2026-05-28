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
let friendGroupCollapsed = JSON.parse(localStorage.getItem('claran_friend_group_collapsed') || '{}');
let agentContextSidebarVisible = false;
let agentNativeStateByConversation = {};
let llmProfilesCache = [];
let messageTranslations = {};
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
    if (!event.target.closest('.agent-menu-wrapper')) {
        closeAgentItemMenus();
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

function escapeRegExp(value) {
    return String(value || '').replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
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
    return [url || '', id || '', name || ''].map(part => encodeURIComponent(part)).join('|');
}

function parseMediaPayload(content, tag) {
    const match = (content || '').match(new RegExp(`\\[${tag}\\]([\\s\\S]*?)\\[\\/${tag}\\]`));
    const raw = match ? match[1] : (content || '');
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

function resolveMediaURL(media) {
    if (!media) return '';
    if (media.id) return fileAPI.previewURL(media.id);
    if (media.url && media.url.startsWith('/')) return `http://localhost:8080${media.url}`;
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
    if (media.url.startsWith('/files/')) return `http://localhost:8080${media.url}`;
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

function renderCurrentMessages() {
    const msgList = document.getElementById('message-list');
    msgList.innerHTML = currentMessages.map(m => createMessageHTML(m)).join('');
    hydrateMedia(msgList);
    msgList.scrollTop = msgList.scrollHeight;
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
    const recent = currentMessages.slice(-8).filter(m => m && (m.content || m.msg_type));
    if (!recent.length) return '<div class="agent-context-empty">暂无可分析消息</div>';
    return recent.map(m => {
        const name = getAgentBotByUserID(m.sender_id) ? getBotDisplayName(getAgentBotByUserID(m.sender_id)) : getUserName(m.sender_id);
        const content = (m.content || `[${m.msg_type || '消息'}]`).replace(/\s+/g, ' ').slice(0, 90);
        return `<li><strong>${escapeHTML(name)}</strong><span>${escapeHTML(content)}</span></li>`;
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
    side.style.display = 'flex';
    side.innerHTML = `
        <div class="agent-context-head">
            <strong>Agent 上下文</strong>
            <button type="button" onclick="toggleAgentContextSidebar(false)">×</button>
        </div>
        <section>
            <label>原生状态</label>
            <p>${escapeHTML(agentNativeStatusLabel(state.status))} · ${escapeHTML(state.detail || '当前没有正在运行的 Agent 任务')}</p>
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

let modalStack = [];

function showModal(title, bodyHTML) {
    const overlay = document.getElementById('modal-overlay');
    if (overlay.style.display === 'flex') {
        modalStack.push({
            title: document.getElementById('modal-title').textContent,
            body: document.getElementById('modal-body').innerHTML
        });
    }
    document.getElementById('modal-title').textContent = title;
    document.getElementById('modal-body').innerHTML = bodyHTML;
    overlay.style.display = 'flex';
}

function closeModal() {
    if (modalStack.length > 0) {
        const prev = modalStack.pop();
        document.getElementById('modal-title').textContent = prev.title;
        document.getElementById('modal-body').innerHTML = prev.body;
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
    document.getElementById('user-status').textContent = '○ 离线';
    document.getElementById('user-status').className = 'user-status offline';
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

    updateUnreadBadge();
    await loadGroups();
    await refreshAgentCache();
    loadConversations();
    loadFriends();
    connectWS();
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
        currentMessages = messages;
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
            currentMessages = refreshed.data.messages;
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
            msgList.innerHTML += createMessageHTML(m);
            hydrateMedia(msgList);
            msgList.scrollTop = msgList.scrollHeight;
        });
    } else {
        currentMessages.push(m);
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
    if (!key || pendingAgentThinkingByConversation[key]) return;
    const startedAt = Date.now();
    const thinkingID = `agent-thinking-${key}-${startedAt}`;
    pendingAgentThinkingByConversation[key] = { thinkingID, startedAt };
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
    const pending = pendingAgentThinkingByConversation[key];
    if (!pending) return 0;
    const durationMs = Date.now() - pending.startedAt;
    delete pendingAgentThinkingByConversation[key];
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

        const payload = makeMediaPayload(fileURL, fileID, file.name);
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
    const payload = makeMediaPayload(fileURL, fileID, name);
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

function renderMessageContent(content, msgType, options = {}) {
    if (msgType === 'broadcast') {
        return `<div class="broadcast-msg"><span class="broadcast-badge">广播</span><span>${options.markdown ? renderMarkdownText(content) : escapeHTML(content)}</span></div>`;
    }
    if (msgType === 'image' || (content && content.startsWith('[img]'))) {
        const media = parseMediaPayload(content, 'img');
        const key = rememberMedia(media);
        const url = resolveMediaURL(media);
        const dataAttrs = media.id ? ` data-media-id="${escapeHTML(media.id)}" data-media-key="${escapeHTML(key)}" data-media-url="${escapeHTML(media.url || '')}" data-media-name="${escapeHTML(media.name || '')}"` : '';
        return `<div class="image-msg-wrap"><img src="${escapeHTML(url)}"${dataAttrs} alt="${escapeHTML(media.name || '图片')}" class="chat-image" onclick="window.open(this.src,'_blank')" onerror="this.closest('.image-msg-wrap').querySelector('.media-error').style.display='inline';"><span class="media-error" style="display:none;">图片加载失败</span></div>`;
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
            <button class="btn-small active" onclick="renderLLMSettings()">LLM 预设</button>
            <button class="btn-small" onclick="renderPromptSettings()">Prompt</button>
        </div>
        <div id="settings-content" class="settings-content">加载中...</div>
    `);
    await renderLLMSettings();
}

async function renderLLMSettings() {
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
                <p>这里保存可复用的模型服务配置。创建 Agent 或翻译消息时可以直接选择这些预设，API Key 不会回显。</p>
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
                        <option value="translation">翻译</option>
                        <option value="agent">Agent</option>
                        <option value="general">通用</option>
                    </select>
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
            <button class="btn-primary" onclick="saveLLMSetting()">保存 LLM 预设</button>
        </div>
    `;
}

function renderLLMProfileCard(profile) {
    return `
        <div class="memory-card">
            <div class="memory-card-head">
                <strong>${escapeHTML(profile.name || '未命名')}</strong>
                <span>${escapeHTML(profile.usage_type || 'general')} ${profile.is_default ? '· 默认' : ''}</span>
            </div>
            <div class="memory-card-meta">
                <span>${escapeHTML(profile.model_name || '未配置模型')}</span>
                <span>${escapeHTML(profile.base_url || '未配置BaseURL')}</span>
                <span>${profile.has_api_key ? '已保存密钥' : '未保存密钥'}</span>
            </div>
            <div class="memory-card-actions">
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

async function loadBotSidebar() {
    const list = document.getElementById('bot-list');
    const resp = await agentAPI.list();
    if (resp && resp.code === 0 && resp.data && resp.data.bots) {
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
            <label>工具策略</label>
            <select id="bot-tool-policy" class="form-select">
                <option value="safe">常规模式</option>
                <option value="approval_required">操作前确认</option>
                <option value="readonly">只读模式</option>
                <option value="disabled">禁用工具</option>
            </select>
        </div>
        <button id="create-bot-submit" class="btn-primary" onclick="createBot()">创建智能助手</button>
    `);
    loadLLMProfilesForAgentCreate();
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

function closeAgentItemMenus(exceptID = '') {
    document.querySelectorAll('.agent-item-menu.open').forEach(menu => {
        if (!exceptID || menu.id !== exceptID) {
            menu.classList.remove('open');
            menu.style.left = '';
            menu.style.top = '';
        }
    });
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
        agent_run_summary: '运行摘要'
    }[type] || type || '事实';
}

async function showMemoryManager() {
    showModal('记忆管理', `
        <div class="agent-help-box">
            <strong>个人可控记忆</strong>
            <p>这里展示当前账号拥有的 Agent 记忆。表达习惯和用户画像默认只对本人可见，你可以随时修改、关闭或删除。</p>
        </div>
        <div class="memory-toolbar">
            <select id="memory-bot-filter" class="form-select">
                <option value="">全部助手</option>
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
    `);
    await loadMemoryList();
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
        const bot = botCache.find(b => sameID(b.id, m.bot_id));
        return `
            <div class="memory-card ${m.enabled ? '' : 'disabled'}">
                <div class="memory-card-head">
                    <strong>${escapeHTML(m.title || memoryTypeLabel(m.type))}</strong>
                    <span>${escapeHTML(memoryScopeLabel(m.scope))} · ${escapeHTML(memoryTypeLabel(m.type))}</span>
                </div>
                <div class="memory-card-content">${renderMarkdownText(m.content || '')}</div>
                <div class="memory-card-meta">
                    <span>${escapeHTML(bot ? getBotDisplayName(bot) : ('Agent ' + m.bot_id))}</span>
                    <span>${m.visibility === 'shared' ? '共享' : '仅自己可见'}</span>
                    <span>${m.enabled ? '已启用' : '已关闭'}</span>
                    <span>向量: ${escapeHTML(m.vector_status || 'pending')}</span>
                </div>
                <div class="memory-card-actions">
                    <button class="btn-small" onclick="showEditMemoryForm(${jsStringArg(JSON.stringify(m).replace(/</g, '\\u003c'))})">编辑</button>
                    <button class="btn-small" onclick="toggleMemoryEnabled(${jsArg(m.id)}, ${!m.enabled})">${m.enabled ? '关闭' : '启用'}</button>
                    <button class="btn-small danger-soft" onclick="deleteMemoryFact(${jsArg(m.id)})">删除</button>
                </div>
            </div>
        `;
    }).join('');
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
                    ${['preference','speaking_style','long_term_goal','group_profile','project_state','chat_summary','agent_run_summary'].map(v => `<option value="${v}" ${memory?.type === v ? 'selected' : ''}>${memoryTypeLabel(v)}</option>`).join('')}
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
    if (!data.bot_id || data.bot_id === '0') {
        showToast('请选择所属助手', 'warning');
        return;
    }
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
        <div id="route-list-area" class="bot-list-area">加载中...</div>
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
    const toolPolicy = document.getElementById('bot-tool-policy')?.value || 'safe';
    const llmProfileID = document.getElementById('bot-llm-profile')?.value || '';
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
        const resp = await agentAPI.create(name, type, description, modelName, apiKey, baseURL, systemPrompt, '', '', {
            avatar,
            signature,
            workspace_root: workspaceRoot,
            tool_policy: toolPolicy,
            llm_profile_id: llmProfileID ? Number(llmProfileID) : 0,
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
            <label>工具策略</label>
            <select id="edit-agent-tool-policy" class="form-select">
                ${['safe', 'approval_required', 'readonly', 'disabled'].map(v => `<option value="${v}" ${b.tool_policy === v ? 'selected' : ''}>${toolPolicyLabel(v)}</option>`).join('')}
            </select>
        </div>
        <div class="btn-row">
            <button class="btn-inline btn-primary" onclick="saveAgentConfig(${jsArg(botID)})">保存</button>
            <button class="btn-inline" onclick="showAgentPermissions(${jsArg(botID)}, ${jsStringArg(getBotDisplayName(b))})">权限管理</button>
        </div>
    `);
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
        workspace_root: document.getElementById('edit-agent-workspace').value.trim(),
        tool_policy: document.getElementById('edit-agent-tool-policy').value,
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
            <div class="bot-item">
                <div class="bot-info">
                    <span class="bot-name">${escapeHTML(r.route_pattern)}</span>
                    <span class="bot-type ${r.route_type}">${escapeHTML(routeTypeLabel(r.route_type))}</span>
                    <span class="bot-status active">优先级: ${r.priority || 0}</span>
                </div>
                <div class="bot-actions">
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

window.onload = function () {
    bindVoiceRecorder();
    if (token && currentUser) {
        enterMainPage();
    }
};


