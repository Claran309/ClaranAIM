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
    const re = /@(\d+)/g;
    let match;
    while ((match = re.exec(content || '')) !== null) {
        ids.push(match[1]);
    }
    return [...new Set(ids)];
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
            avatarEl.innerHTML = `<img src="${currentUser.avatar}" style="width:100%;height:100%;border-radius:50%;object-fit:cover;">`;
        } else {
            avatarEl.textContent = (currentUser.nickname || currentUser.username).charAt(0).toUpperCase();
        }
        document.getElementById('user-status').textContent = '● 在线';
        document.getElementById('user-status').className = 'user-status online';
    }

    updateUnreadBadge();
    await loadGroups();
    loadConversations();
    loadFriends();
    connectWS();
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

        convs.sort((a, b) => {
            if (a._is_pinned && !b._is_pinned) return -1;
            if (!a._is_pinned && b._is_pinned) return 1;
            return 0;
        });

        list.innerHTML = convs.map(c => {
            const unread = unreadMap[c.conversation_id] || 0;
            const isActive = sameID(currentConversationID, c.conversation_id);
            const typeLabel = c.is_deleted_group ? '已解散' : (c.type === 'private' ? '私聊' : '群聊');
            const pinnedPrefix = c._is_pinned ? '📌 ' : '';
            let displayName = conversationNameCache[c.conversation_id] || c.target_name;
            if (!displayName || displayName.startsWith('用户') || displayName.startsWith('群聊#')) {
                if (c.type === 'private' && c.participant_ids) {
                    const otherID = c.participant_ids.find(id => !sameID(id, currentUser.id));
                    if (otherID) displayName = getUserName(otherID);
                }
            }
            if (!displayName) displayName = '会话 #' + c.conversation_id;
            conversationNameCache[c.conversation_id] = displayName;

            let avatarHTML;
            if (c.type === 'private' && c.participant_ids) {
                const otherID = c.participant_ids.find(id => !sameID(id, currentUser.id));
                if (otherID) {
                    avatarHTML = getUserAvatarHTML(otherID, 'conv-avatar');
                } else {
                    avatarHTML = '👤';
                }
            } else {
                avatarHTML = '👥';
            }

            return `
                <div class="list-item ${isActive ? 'active' : ''} ${c._is_pinned ? 'pinned' : ''} ${c.is_deleted_group ? 'deleted-group' : ''}" data-conversation-id="${escapeHTML(String(c.conversation_id))}" onclick="openConversation(${jsArg(c.conversation_id)}, ${jsStringArg(c.type)}, ${c.is_deleted_group ? 'true' : 'false'})">
                    <div class="avatar conv-avatar">${avatarHTML}</div>
                    <div class="list-item-info">
                        <div class="list-item-top">
                            <span class="list-item-name">${pinnedPrefix}${escapeHTML(displayName)}</span>
                            <span class="list-item-type">${typeLabel}</span>
                        </div>
                        <div class="list-item-msg">${escapeHTML(c.last_message || '暂无消息')}</div>
                    </div>
                    <button class="btn-icon-sm" onclick="event.stopPropagation(); hideConversation(${jsArg(c.conversation_id)})" title="删除会话">删除</button>
                    ${unread > 0 ? `<span class="item-unread">${unread > 99 ? '99+' : unread}</span>` : ''}
                </div>
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
        const friends = friendsCache;
        if (friends.length === 0) {
            list.innerHTML = '<div class="empty-tip">暂无好友<br><small>点击右上角「+ 添加」添加好友</small></div>';
            return;
        }

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

        list.innerHTML = friends.map(f => {
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
        }).join('');
    } else {
        list.innerHTML = '<div class="empty-tip">暂无好友</div>';
    }
}

async function loadGroups() {
    const resp = await groupAPI.list();
    const list = document.getElementById('group-list');

    if (resp && resp.code === 0 && resp.data && resp.data.groups) {
        groupsCache = resp.data.groups;
        const groups = groupsCache;
        if (groups.length === 0) {
            list.innerHTML = '<div class="empty-tip">暂无群组<br><small>点击右上角「+ 创建」创建群组</small></div>';
            return;
        }

        list.innerHTML = groups.map(g => {
            const avatarHTML = renderAvatarHTML(g.avatar, '👥', 'group-avatar');
            const ownerName = getUserName(g.owner_id);
            const isPinned = g.is_pinned;
            return `
                <div class="list-item group-item ${isPinned ? 'pinned' : ''}">
                    ${avatarHTML}
                    <div class="list-item-info">
                        <div class="list-item-name">${isPinned ? '📌 ' : ''}${escapeHTML(g.name)}</div>
                        <div class="list-item-msg">群主: ${escapeHTML(ownerName)}</div>
                    </div>
                    <div class="group-actions">
                        <button class="btn-chat" onclick="openGroupConversation(${jsArg(g.id)})">进入</button>
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

    const convName = await resolveConversationName(conversationID, type);
    if (openSeq !== conversationOpenSeq || !sameID(currentConversationID, targetConversationID) || currentBotID !== null) return;
    document.getElementById('chat-title').textContent = convName;
    const typeLabel = isDeletedGroup ? '群聊已解散' : (type === 'private' ? '👤 私聊' : '👥 群聊');
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
    }

    loadConversations();
}

function createMessageHTML(m) {
    if (m.is_thinking) {
        const thinkingIDAttr = m._thinkingID ? ` data-thinking-id="${m._thinkingID}"` : '';
        return `
            <div class="message-item received bot-msg msg-thinking"${thinkingIDAttr}>
                <div class="msg-avatar received">🤖</div>
                <div class="msg-body">
                    <div class="msg-meta">
                        <span class="message-sender">🤖 AI助手</span>
                    </div>
                    <div class="message-bubble thinking"><div class="spinner"></div> AI思考中...</div>
                </div>
            </div>
        `;
    }
    const isSent = sameID(m.sender_id, currentUser.id);
    const isBot = sameID(m.sender_id, 0) || m.is_bot;
    const senderName = isBot ? '🤖 AI助手' : (isSent ? '我' : getUserName(m.sender_id));
    const time = m.created_at || '';
    const avatarContent = isBot ? '🤖' : (isSent
        ? (currentUser.avatar ? `<img src="${currentUser.avatar}" class="avatar-img">` : (currentUser.nickname || currentUser.username).charAt(0).toUpperCase())
        : getUserAvatarHTML(m.sender_id));
    const avatarBg = isSent ? '' : 'received';
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
    const actionsHTML = (!isBot && status !== 'recalled' && messageID) ? `
        <div class="message-actions">
            <button type="button" onclick="setPendingReply(${jsArg(messageID)})">回复</button>
            ${isSent ? `<button type="button" onclick="editMessage(${jsArg(messageID)})">编辑</button><button type="button" onclick="recallMessage(${jsArg(messageID)})">撤回</button>` : ''}
        </div>
    ` : '';
    const bubbleContent = status === 'recalled' ? escapeHTML(originalContent) : renderMessageContent(originalContent, m.msg_type);
    const errorClass = m.is_error ? 'error-bubble' : '';
    return `
        <div class="message-item ${isSent ? 'sent' : 'received'} ${isBot ? 'bot-msg' : ''}" data-message-id="${messageID}">
            <div class="msg-avatar ${avatarBg}">${avatarContent}</div>
            <div class="msg-body">
                <div class="msg-meta">
                    <span class="message-sender">${escapeHTML(senderName)}</span>
                    <span class="message-time">${time}</span>
                    ${editedHTML}
                    ${readReceiptHTML(m)}
                </div>
                <div class="message-bubble ${errorClass} ${status === 'recalled' ? 'recalled-bubble' : ''}">${replyHTML}${bubbleContent}</div>
                ${actionsHTML}
            </div>
        </div>
    `;
}

function appendMessage(m) {
    if (m.msg_id && !m.id) {
        m.id = m.msg_id;
    }
    if (m.conversation_id && currentConversationID && !sameID(m.conversation_id, currentConversationID)) {
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

async function sendMessage() {
    const input = document.getElementById('msg-input');
    const content = input.value.trim();
    if (!content || !currentConversationID) return;

    const sendConversationID = currentConversationID;
    const replyToID = pendingReplyMessage ? (pendingReplyMessage.id || pendingReplyMessage.msg_id || 0) : 0;
    const mentionUserIDs = extractMentions(content);
    const mentionAll = content.includes('@all') || content.includes('@所有人');
    input.value = '';

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
        <button class="btn-primary" onclick="saveFriendRemark(${jsArg(friendID)})">保存</button>
    `);
}

async function saveFriendRemark(friendID) {
    const remark = document.getElementById('edit-friend-remark').value.trim();
    const resp = await userAPI.updateFriendRemark(friendID, 0, remark);
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

function showAddFriend() {
    showModal('添加好友', `
        <div class="form-group">
            <label>好友用户ID</label>
            <input type="number" id="add-friend-id" placeholder="请输入用户ID">
        </div>
        <div class="form-group">
            <label>备注</label>
            <input type="text" id="add-friend-remark" placeholder="备注（可选）">
        </div>
        <button class="btn-primary" onclick="addFriend()">添加</button>
    `);
}

async function addFriend() {
    const friendID = document.getElementById('add-friend-id').value.trim();
    const remark = document.getElementById('add-friend-remark').value.trim();
    if (!/^\d{10}$/.test(friendID)) {
        showToast('请输入有效的用户ID', 'warning');
        return;
    }
    const resp = await userAPI.addFriend(friendID, 0, remark);
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
                        const roleLabel = m.role === 'owner' ? '群主' : (m.role === 'admin' ? '管理员' : '成员');
                        const canKick = !sameID(currentUser.id, m.user_id) && m.role !== 'owner' && (isOwner || (isAdmin && m.role === 'member'));
                        const canMute = canManage && !sameID(currentUser.id, m.user_id) && m.role !== 'owner';
                        const canSetRole = isOwner && m.role !== 'owner';
                        const memberName = getUserName(m.user_id);
                        const avatarHTML = getUserAvatarHTML(m.user_id, 'small');
                        const isMuted = m.muted_until && m.muted_until !== '';
                        return `
                            <div class="member-item ${isMuted ? 'muted' : ''}">
                                <div class="avatar small">${avatarHTML}</div>
                                <div class="member-info">
                                    <span class="member-name">${escapeHTML(memberName)}</span>
                                    <span class="member-tag ${roleClass}">${roleLabel}</span>
                                    ${isMuted ? '<span class="member-tag muted-tag">🔇 禁言中</span>' : ''}
                                </div>
                                <div class="member-actions">
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
        ? `<img src="${escapeHTML(currentUser.avatar)}" class="avatar-img">`
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
        avatarEl.innerHTML = `<img src="${currentUser.avatar}" style="width:100%;height:100%;border-radius:50%;object-fit:cover;">`;
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
    // Voice messages use the same media contract as images/files: upload the
    // binary first, then send a lightweight [voice]url|id|name[/voice] message.
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
    // Long-press recording mirrors mobile chat apps. MediaRecorder streams audio
    // chunks locally until mouse/touch release, then the final Blob is uploaded.
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
    // Bind once on page load. Release listeners live on window so sending still
    // happens if the pointer leaves the circular record button before mouseup.
    const btn = document.getElementById('voice-record-btn');
    if (!btn || btn.dataset.bound === '1') return;
    btn.dataset.bound = '1';
    btn.addEventListener('mousedown', startVoiceRecording);
    btn.addEventListener('touchstart', startVoiceRecording, { passive: false });
    window.addEventListener('mouseup', stopVoiceRecording);
    window.addEventListener('touchend', stopVoiceRecording, { passive: false });
    window.addEventListener('touchcancel', stopVoiceRecording, { passive: false });
}

function renderMessageContent(content, msgType) {
    if (msgType === 'broadcast') {
        return `<div class="broadcast-msg"><span class="broadcast-badge">广播</span><span>${escapeHTML(content)}</span></div>`;
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
    return escapeHTML(content);
}

async function loadBotSidebar() {
    const list = document.getElementById('bot-list');
    const resp = await botAPI.list();
    if (resp && resp.code === 0 && resp.data && resp.data.bots) {
        const bots = resp.data.bots;
        if (bots.length === 0) {
            list.innerHTML = '<div class="empty-tip">暂无AI助手<br><small>点击右上角「+ 创建」添加</small></div>';
            return;
        }
        list.innerHTML = bots.map(b => `
            <div class="list-item" data-bot-id="${escapeHTML(String(b.id))}" onclick="chatWithBot(${jsArg(b.id)})">
                <div class="avatar conv-avatar">🤖</div>
                <div class="list-item-info">
                    <div class="list-item-top">
                        <span class="list-item-name">${escapeHTML(b.name)}</span>
                        <span class="list-item-type ${b.type}">${b.type === 'internal' ? '内部' : '自部署'}</span>
                    </div>
                    <div class="list-item-msg">${escapeHTML(b.description || '无描述')}</div>
                </div>
                <div class="bot-item-actions" onclick="event.stopPropagation()">
                    <button class="btn-icon-sm" onclick="showBotRoutes(${jsArg(b.id)}, ${jsStringArg(b.name)})" title="路由管理">🔗</button>
                    <button class="btn-icon-sm" onclick="showBotBilling(${jsArg(b.id)}, ${jsStringArg(b.name)})" title="计费记录">💰</button>
                    <button class="btn-icon-sm" onclick="toggleBot(${jsArg(b.id)}, ${!b.is_active})" title="${b.is_active ? '停用' : '启用'}">${b.is_active ? '⏸' : '▶'}</button>
                    <button class="btn-icon-sm" onclick="deleteBot(${jsArg(b.id)})" title="删除">🗑</button>
                </div>
            </div>
        `).join('');
    } else {
        list.innerHTML = '<div class="empty-tip">加载失败</div>';
    }
}

function showCreateBotForm() {
    showModal('创建 AI 助手', `
        <div class="form-group">
            <label>助手名称</label>
            <input type="text" id="bot-name" placeholder="例如: Amiya">
        </div>
        <div class="form-group">
            <label>类型</label>
            <select id="bot-type" class="form-select" onchange="onBotTypeChange()">
                <option value="internal">内部Bot（使用系统默认API）</option>
                <option value="custom">自部署Bot（需要自己的API Key）</option>
            </select>
        </div>
        <div class="form-group">
            <label>描述</label>
            <input type="text" id="bot-desc" placeholder="助手功能描述">
        </div>
        <div class="form-group">
            <label>模型名称</label>
            <input type="text" id="bot-model" placeholder="例如: gpt-4o-mini（内部Bot留空使用默认）">
        </div>
        <div id="custom-bot-fields" style="display:none;">
            <div class="form-group">
                <label>API Key <span style="color:var(--danger);">*必填</span></label>
                <input type="password" id="bot-apikey" placeholder="你的 LLM API Key">
            </div>
            <div class="form-group">
                <label>Base URL <span style="color:var(--danger);">*必填</span></label>
                <input type="text" id="bot-baseurl" placeholder="例如: https://api.openai.com/v1">
            </div>
        </div>
        <div class="form-group">
            <label>系统提示词</label>
            <textarea id="bot-prompt" rows="3" placeholder="助手的系统提示词"></textarea>
        </div>
        <button class="btn-primary" onclick="createBot()">创建助手</button>
    `);
}

function onBotTypeChange() {
    const type = document.getElementById('bot-type').value;
    const customFields = document.getElementById('custom-bot-fields');
    customFields.style.display = type === 'custom' ? 'block' : 'none';
}

function showBotRoutes(botID, botName) {
    showModal(`路由管理 - ${botName}`, `
        <div class="form-group" style="display:flex;gap:8px;align-items:flex-end;">
            <div style="flex:2;">
                <label>路由模式</label>
                <input type="text" id="route-pattern" placeholder="例如: /chat/*">
            </div>
            <div style="flex:1;">
                <label>类型</label>
                <select id="route-type" class="form-select">
                    <option value="exact">精确匹配</option>
                    <option value="prefix">前缀匹配</option>
                    <option value="regex">正则匹配</option>
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
    const name = document.getElementById('bot-name').value.trim();
    const type = document.getElementById('bot-type').value;
    const description = document.getElementById('bot-desc').value.trim();
    const modelName = document.getElementById('bot-model').value.trim();
    const apiKey = document.getElementById('bot-apikey')?.value?.trim() || '';
    const baseURL = document.getElementById('bot-baseurl')?.value?.trim() || '';
    const systemPrompt = document.getElementById('bot-prompt').value.trim();
    if (!name) { showToast('请填写助手名称', 'warning'); return; }
    if (type === 'custom' && !apiKey) { showToast('自部署Bot必须提供API Key', 'warning'); return; }
    if (type === 'custom' && !baseURL) { showToast('自部署Bot必须提供Base URL', 'warning'); return; }
    const resp = await botAPI.create(name, type, description, modelName, apiKey, baseURL, systemPrompt, '', '');
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('助手创建成功', 'success');
        closeModal();
        loadBotSidebar();
    } else {
        showToast(resp?.data?.msg || '创建失败', 'error');
    }
}

async function toggleBot(botID, isActive) {
    const resp = await botAPI.update(botID, { is_active: isActive });
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast(isActive ? '已启用' : '已停用', 'success');
        loadBotSidebar();
    } else {
        showToast(resp?.data?.msg || '操作失败', 'error');
    }
}

async function deleteBot(botID) {
    if (!confirm('确定要删除该AI助手吗？')) return;
    const resp = await botAPI.delete(botID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已删除', 'success');
        loadBotSidebar();
    } else {
        showToast(resp?.data?.msg || '删除失败', 'error');
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
    const botName = botEl ? botEl.querySelector('.list-item-name')?.textContent || 'AI助手' : 'AI助手';

    document.getElementById('welcome-area').style.display = 'none';
    document.getElementById('chat-area').style.display = 'flex';
    document.getElementById('chat-title').textContent = `🤖 ${botName}`;
    document.getElementById('chat-type-badge').textContent = '🤖 AI助手';
    document.getElementById('chat-type-badge').className = 'chat-type-badge group';
    document.getElementById('group-announcement-bar').style.display = 'none';
    document.getElementById('message-list').innerHTML = '';
    document.getElementById('broadcast-btn').style.display = 'none';
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
    const thinkingMsg = { sender_id: 0, content: '🤔 AI思考中...', created_at: timeStr, is_thinking: true, _thinkingID: thinkingID };
    appendMessage(thinkingMsg);
    botPendingReplies[activeBotID] = thinkingMsg;

    const resp = await botAPI.chat(activeBotID, content, botConversationID);

    delete botPendingReplies[activeBotID];
    const isStillActive = currentBotID === activeBotID && activeBotSeq === botChatSeq;
    if (isStillActive) {
        const container = document.getElementById('message-list');
        const thinkingEl = container.querySelector(`[data-thinking-id="${thinkingID}"]`) || container.querySelector('.msg-thinking');
        if (thinkingEl) thinkingEl.remove();
    }

    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        const replyTime = new Date();
        const replyTimeStr = replyTime.getFullYear() + '-' +
            String(replyTime.getMonth() + 1).padStart(2, '0') + '-' +
            String(replyTime.getDate()).padStart(2, '0') + ' ' +
            String(replyTime.getHours()).padStart(2, '0') + ':' +
            String(replyTime.getMinutes()).padStart(2, '0') + ':' +
            String(replyTime.getSeconds()).padStart(2, '0');

        const botMsg = { sender_id: 0, content: resp.data.reply, created_at: replyTimeStr, is_bot: true };
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
    const resp = await botAPI.createRoute(botID, pattern, routeType, 0);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('路由添加成功', 'success');
        loadBotRoutes(botID);
    } else {
        showToast(resp?.data?.msg || '添加失败', 'error');
    }
}

async function loadBotRoutes(botID) {
    const area = document.getElementById('route-list-area');
    if (!botID) { area.innerHTML = '<div class="empty-tip">请输入Bot ID</div>'; return; }
    area.innerHTML = '<div class="search-loading"><div class="spinner"></div>加载中...</div>';
    const resp = await botAPI.listRoutes(botID);
    if (resp && resp.code === 0 && resp.data && resp.data.routes) {
        const routes = resp.data.routes;
        if (routes.length === 0) {
            area.innerHTML = '<div class="empty-tip">暂无路由</div>';
            return;
        }
        area.innerHTML = routes.map(r => `
            <div class="bot-item">
                <div class="bot-info">
                    <span class="bot-name">🔗 ${escapeHTML(r.route_pattern)}</span>
                    <span class="bot-type ${r.route_type}">${r.route_type}</span>
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
    if (!confirm('确定删除该路由？')) return;
    const resp = await botAPI.deleteRoute(routeID);
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
    const resp = await botAPI.getBilling(botID);
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
