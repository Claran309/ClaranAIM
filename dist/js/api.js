const API_BASE = 'http://localhost:8080/api/v1';
const WS_BASE = 'ws://localhost:8081/ws';
const API_LOG_KEY = 'claran_frontend_logs';
const API_LOG_LIMIT = 500;

function apiLocalLog(level, message, details = null) {
    const entry = {
        time: new Date().toISOString(),
        level,
        message: String(message || ''),
        details
    };
    try {
        const logs = JSON.parse(localStorage.getItem(API_LOG_KEY) || '[]');
        logs.push(entry);
        localStorage.setItem(API_LOG_KEY, JSON.stringify(logs.slice(-API_LOG_LIMIT)));
    } catch (err) {
        console.warn('写入API本地日志失败:', err);
    }
}

let token = localStorage.getItem('claran_token') || '';
let refreshToken = localStorage.getItem('claran_refresh_token') || '';
let currentUser = JSON.parse(localStorage.getItem('claran_user') || 'null');
let currentConversationID = null;
let currentConversationType = '';
let ws = null;
let wsReconnectTimer = null;
let unreadMap = JSON.parse(localStorage.getItem('claran_unread') || '{}');
let localMsgIdCounter = 0;
let pendingMessages = {};
let friendsCache = [];
let groupsCache = [];
let userNickCache = {};
let userAvatarCache = {};
let friendRemarkCache = {};
let conversationNameCache = {};
let refreshPromise = null;

function parseJSONSafeInt(text) {
    return JSON.parse(text.replace(/:\s*(-?\d{16,})(?=[,}\]])/g, ':"$1"'));
}

if (currentUser && currentUser.id) {
    userNickCache[currentUser.id] = currentUser.nickname || currentUser.username;
}

function saveUnreadMap() {
    localStorage.setItem('claran_unread', JSON.stringify(unreadMap));
}

function saveAuthTokens(accessToken, nextRefreshToken) {
    if (accessToken) {
        token = accessToken;
        localStorage.setItem('claran_token', token);
    }
    if (nextRefreshToken !== undefined) {
        refreshToken = nextRefreshToken || '';
        if (refreshToken) {
            localStorage.setItem('claran_refresh_token', refreshToken);
        } else {
            localStorage.removeItem('claran_refresh_token');
        }
    }
}

function clearAuthTokens() {
    token = '';
    refreshToken = '';
    localStorage.removeItem('claran_token');
    localStorage.removeItem('claran_refresh_token');
}

async function refreshAccessToken() {
    if (!refreshToken) return false;
    if (refreshPromise) return refreshPromise;
    refreshPromise = (async () => {
        try {
            const resp = await fetch(`${API_BASE}/user/token/refresh`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ refresh_token: refreshToken }),
            });
            const text = await resp.text();
            const result = text ? parseJSONSafeInt(text) : null;
            const accessToken = result?.data?.access_token || result?.data?.token || '';
            if (resp.ok && result?.code === 0 && accessToken) {
                saveAuthTokens(accessToken, refreshToken);
                return true;
            }
        } catch (err) {
            console.warn('刷新Token失败:', err);
        }
        clearAuthTokens();
        return false;
    })();
    try {
        return await refreshPromise;
    } finally {
        refreshPromise = null;
    }
}

function authHeaders(extra = {}) {
    const headers = { ...extra };
    if (token) headers['Authorization'] = `Bearer ${token}`;
    return headers;
}

async function requestOnce(method, path, data = null, auth = true) {
    const headers = { 'Content-Type': 'application/json' };
    if (auth && token) {
        headers['Authorization'] = `Bearer ${token}`;
    }
    const options = { method, headers };
    if (data && method !== 'GET') {
        options.body = JSON.stringify(data);
    }
    apiLocalLog('info', 'HTTP请求', { method, path, auth });
    const resp = await fetch(`${API_BASE}${path}`, options);
    const text = await resp.text();
    const result = text ? parseJSONSafeInt(text) : null;
    if (!resp.ok || result?.code !== 0) {
        apiLocalLog('warn', 'HTTP响应异常', {
            method,
            path,
            status: resp.status,
            code: result?.code,
            message: result?.message || result?.data?.msg || ''
        });
    }
    return result;
}

async function request(method, path, data = null, auth = true) {
    try {
        let result = await requestOnce(method, path, data, auth);
        if (auth && result?.code === 401 && path !== '/user/token/refresh' && await refreshAccessToken()) {
            result = await requestOnce(method, path, data, auth);
        }
        return result;
    } catch (err) {
        apiLocalLog('error', 'HTTP请求失败', { method, path, message: err.message, stack: err.stack || '' });
        showToast('网络请求失败: ' + err.message, 'error');
        return null;
    }
}

function apiID(value) {
    return value === undefined || value === null || value === '' ? 0 : String(value);
}

function apiIDs(values) {
    return (values || []).map(apiID);
}

const userAPI = {
    register: (username, password, nickname) =>
        request('POST', '/user/register', { username, password, nickname }, false),
    login: (username, password) =>
        request('POST', '/user/login', { username, password }, false),
    refreshToken: () =>
        request('POST', '/user/token/refresh', { refresh_token: refreshToken }, false),
    getInfo: () => request('GET', '/user/info'),
    updateInfo: (profile) =>
        request('PUT', '/user/info', profile),
    updateAvatar: (avatar) =>
        request('POST', '/user/avatar', { avatar }),
    logout: () => request('POST', '/user/logout'),
    addFriend: (friendID, groupID, remark) =>
        request('POST', '/user/friend/add', { friend_id: apiID(friendID), group_id: apiID(groupID || 0), remark: remark || '' }),
    deleteFriend: (friendID) =>
        request('POST', '/user/friend/delete', { friend_id: apiID(friendID) }),
    updateFriendRemark: (friendID, groupID, remark) =>
        request('PUT', '/user/friend/remark', { friend_id: apiID(friendID), group_id: apiID(groupID || 0), remark: remark || '' }),
    getFriendList: () => request('GET', '/user/friend/list'),
    createFriendGroup: (name) => request('POST', '/user/friend/group', { name }),
    getFriendGroups: () => request('GET', '/user/friend/groups'),
    batchGetInfo: (ids) => request('GET', `/user/batch?ids=${ids.join(',')}`),
};

const groupAPI = {
    create: (name, memberIDs) =>
        request('POST', '/group/create', { name, member_ids: apiIDs(memberIDs) }),
    get: (id) => request('GET', `/group/${id}`),
    list: () => request('GET', '/group/list'),
    invite: (groupID, userIDs) =>
        request('POST', '/group/invite', { group_id: apiID(groupID), user_ids: apiIDs(userIDs) }),
    kick: (groupID, userID) =>
        request('POST', '/group/kick', { group_id: apiID(groupID), user_id: apiID(userID) }),
    getMembers: (id) => request('GET', `/group/${id}/members`),
    transfer: (groupID, newOwnerID) =>
        request('POST', '/group/transfer', { group_id: apiID(groupID), new_owner_id: apiID(newOwnerID) }),
    updateInfo: (groupID, name, announcement) =>
        request('PUT', '/group/info', { group_id: apiID(groupID), name, announcement }),
    pin: (groupID, isPinned) =>
        request('POST', '/group/pin', { group_id: apiID(groupID), is_pinned: isPinned }),
    mute: (groupID, userID, durationMinutes) =>
        request('POST', '/group/mute', { group_id: apiID(groupID), user_id: apiID(userID), duration_minutes: durationMinutes }),
    unmute: (groupID, userID) =>
        request('POST', '/group/unmute', { group_id: apiID(groupID), user_id: apiID(userID) }),
    setRole: (groupID, userID, role) =>
        request('POST', '/group/role', { group_id: apiID(groupID), user_id: apiID(userID), role }),
    deleteGroup: (groupID) =>
        request('POST', '/group/delete', { group_id: apiID(groupID) }),
};

const messageAPI = {
    createConversation: (type, participantIDs, groupID = 0) =>
        request('POST', '/message/conversation', { type, participant_ids: apiIDs(participantIDs), group_id: apiID(groupID) }),
    send: (conversationID, content, msgType = 'text', options = {}) =>
        request('POST', '/message/send', {
            conversation_id: apiID(conversationID),
            content,
            msg_type: msgType,
            reply_to_id: apiID(options.reply_to_id || 0),
            mention_user_ids: (options.mention_user_ids || []).map(apiID),
            mention_all: !!options.mention_all,
        }),
    markRead: (conversationID, messageID = 0) =>
        request('POST', '/message/read', { conversation_id: apiID(conversationID), message_id: apiID(messageID) }),
    edit: (messageID, content) =>
        request('PUT', '/message/edit', { message_id: apiID(messageID), content }),
    recall: (messageID) =>
        request('POST', '/message/recall', { message_id: apiID(messageID) }),
    deleteLocal: (conversationID, messageID) =>
        request('POST', '/message/delete-local', { conversation_id: apiID(conversationID), message_id: apiID(messageID) }),
    getHistory: (conversationID, limit = 50, beforeID = 0) =>
        request('GET', `/message/history/${conversationID}?limit=${limit}&before_id=${beforeID}`),
    search: (keyword, conversationID = 0, limit = 20, startAt = '', endAt = '') =>
        request('GET', `/message/search?keyword=${encodeURIComponent(keyword)}&limit=${limit}&conversation_id=${conversationID}&start_at=${encodeURIComponent(startAt)}&end_at=${encodeURIComponent(endAt)}`),
    getConversations: () => request('GET', '/message/conversations'),
};

const fileAPI = {
    upload: async (file, fileType = 'file') => {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('file_type', fileType);
        const uploadOnce = () => fetch(`${API_BASE}/file/upload`, {
            method: 'POST',
            headers: authHeaders(),
            body: formData,
        });
        try {
            apiLocalLog('info', '文件上传请求', { fileType, filename: file && file.name, size: file && file.size });
            let resp = await uploadOnce();
            if (resp.status === 401 && await refreshAccessToken()) {
                resp = await uploadOnce();
            }
            if (!resp.ok) {
                const text = await resp.text();
                let errMsg = '上传失败';
                try { errMsg = parseJSONSafeInt(text).message || errMsg; } catch(e) {}
                apiLocalLog('warn', '文件上传响应异常', { status: resp.status, message: errMsg });
                showToast('文件上传失败: ' + errMsg, 'error');
                return null;
            }
            const result = parseJSONSafeInt(await resp.text());
            if (result.code !== 0) {
                apiLocalLog('warn', '文件上传业务失败', { code: result.code, message: result.message || '' });
                showToast('文件上传失败: ' + (result.message || '未知错误'), 'error');
                return result;
            }
            return result;
        } catch (err) {
            apiLocalLog('error', '文件上传网络错误', { message: err.message, stack: err.stack || '' });
            showToast('文件上传网络错误: ' + err.message, 'error');
            return null;
        }
    },
    get: (id) => request('GET', `/file/${id}`),
    previewURL: (id) => `http://localhost:8080/file/preview/${id}`,
    downloadURL: (id) => `${API_BASE}/file/download/${id}`,
    fetchBlob: async (id) => {
        const downloadOnce = () => fetch(`${API_BASE}/file/download/${id}`, { headers: authHeaders() });
        let resp = await downloadOnce();
        if (resp.status === 401 && await refreshAccessToken()) {
            resp = await downloadOnce();
        }
        if (!resp.ok) {
            let errMsg = '文件加载失败';
            try {
                const result = parseJSONSafeInt(await resp.text());
                errMsg = result.message || errMsg;
            } catch (e) {}
            throw new Error(errMsg);
        }
        return await resp.blob();
    },
    delete: (id) => request('DELETE', `/file/${id}`, { file_id: id }),
    list: (fileType = '', limit = 20, offset = 0) =>
        request('GET', `/file/list?file_type=${fileType}&limit=${limit}&offset=${offset}`),
};

const botAPI = {
    create: (name, type, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot, extra = {}) =>
        request('POST', '/bot/create', { name, type, description, model_name: modelName, api_key: apiKey, base_url: baseURL, system_prompt: systemPrompt, skills_dir: skillsDir, agent_root: agentRoot, ...extra }),
    update: (botID, data) =>
        request('PUT', '/bot/update', { bot_id: apiID(botID), ...data }),
    get: (id) => request('GET', `/bot/${id}`),
    list: (type = '') => request('GET', `/bot/list?type=${type}`),
    delete: (botID) => request('DELETE', '/bot/delete', { bot_id: apiID(botID) }),
    chat: (botID, message, conversationID = 0) =>
        request('POST', '/bot/chat', { bot_id: apiID(botID), message, conversation_id: apiID(conversationID) }),
    createRoute: (botID, routePattern, routeType, priority) =>
        request('POST', '/bot/route/create', { bot_id: apiID(botID), route_pattern: routePattern, route_type: routeType, priority }),
    listRoutes: (botID) => request('GET', `/bot/${botID}/routes`),
    deleteRoute: (routeID) => request('DELETE', '/bot/route/delete', { route_id: apiID(routeID) }),
    getBilling: (botID, limit = 20, offset = 0) =>
        request('GET', `/bot/${botID}/billing?limit=${limit}&offset=${offset}`),
};

const agentAPI = {
    run: (botID, conversationID, question = '') =>
        request('POST', '/agent/run', { bot_id: apiID(botID), conversation_id: apiID(conversationID), question, message: question }),
    summarize: (botID, conversationID, question = '') =>
        request('POST', '/agent/summarize', { bot_id: apiID(botID), conversation_id: apiID(conversationID), question }),
    ask: (botID, conversationID, question) =>
        request('POST', '/agent/ask', { bot_id: apiID(botID), conversation_id: apiID(conversationID), question }),
    insights: (botID, conversationID, question = '') =>
        request('POST', '/agent/insights', { bot_id: apiID(botID), conversation_id: apiID(conversationID), question }),
    replyCandidates: (botID, conversationID, question = '') =>
        request('POST', '/agent/reply-candidates', { bot_id: apiID(botID), conversation_id: apiID(conversationID), question }),
    grantPermission: (botID, userID, role) =>
        request('POST', '/agent/permission/grant', { bot_id: apiID(botID), user_id: apiID(userID), role }),
    revokePermission: (botID, userID) =>
        request('POST', '/agent/permission/revoke', { bot_id: apiID(botID), user_id: apiID(userID) }),
    listPermissions: (botID) => request('GET', `/agent/${botID}/permissions`),
    listSessions: (botID, conversationID = 0) =>
        request('GET', `/agent/${botID}/sessions?conversation_id=${apiID(conversationID)}`),
};

async function connectWS() {
    if (!token && refreshToken) {
        await refreshAccessToken();
    }
    if (!token) return;
    if (ws) {
        ws.close();
        ws = null;
    }

    ws = new WebSocket(`${WS_BASE}?token=${token}`);

    ws.onopen = () => {
        showToast('实时连接已建立', 'success');
        if (wsReconnectTimer) {
            clearTimeout(wsReconnectTimer);
            wsReconnectTimer = null;
        }
    };

    ws.onmessage = (event) => {
        try {
            const msg = parseJSONSafeInt(event.data);
            handleWSMessage(msg);
        } catch (e) {
            console.log('收到消息:', event.data);
        }
    };

    ws.onclose = () => {
        showToast('实时连接已断开，3秒后重连', 'warning');
        ws = null;
        wsReconnectTimer = setTimeout(connectWS, 3000);
    };

    ws.onerror = (err) => {
        console.error('WebSocket错误:', err);
    };
}

function handleWSMessage(msg) {
    if (msg.type === 'new_message' && msg.data) {
        const data = msg.data;

        if (data.sender_id === currentUser.id) {
            return;
        }

        if (String(data.conversation_id) === String(currentConversationID)) {
            appendMessage(data);
        } else {
            const convId = data.conversation_id;
            setConversationHidden(convId, false);
            unreadMap[convId] = (unreadMap[convId] || 0) + 1;
            saveUnreadMap();
            updateUnreadBadge();
        }
        loadConversations();
    } else if ((msg.type === 'message_edited' || msg.type === 'message_recalled') && msg.data) {
        applyMessageStateUpdate(msg.data);
        loadConversations();
    } else if (msg.type === 'message_read' && msg.data) {
        updateMessageReadReceipt(msg.data);
    }
}

function updateUnreadBadge() {
    let total = 0;
    for (const key in unreadMap) {
        total += unreadMap[key];
    }
    const badge = document.getElementById('conv-unread');
    if (total > 0) {
        badge.textContent = total > 99 ? '99+' : total;
        badge.style.display = 'inline-block';
    } else {
        badge.style.display = 'none';
    }
}

async function resolveUserNames(userIDs) {
    const unknownIDs = userIDs.filter(id => !userNickCache[id]);
    if (unknownIDs.length === 0) return;

    try {
        const resp = await userAPI.batchGetInfo(unknownIDs);
        if (resp && resp.code === 0 && resp.data && resp.data.users) {
            for (const u of resp.data.users) {
                userNickCache[u.id] = u.nickname || u.username;
                if (u.avatar) {
                    userAvatarCache[u.id] = u.avatar;
                }
            }
        }
    } catch (e) {
        console.error('批量获取用户信息失败:', e);
    }
}

function getUserName(userID) {
    if (userID === currentUser.id) {
        return currentUser.nickname || currentUser.username || '我';
    }
    if (friendRemarkCache[userID]) {
        return friendRemarkCache[userID];
    }
    return userNickCache[userID] || '用户' + userID;
}

function getUserAvatarChar(userID) {
    const name = getUserName(userID);
    return name.charAt(0).toUpperCase();
}

function getUserAvatarHTML(userID, extraClass) {
    const avatarURL = userAvatarCache[userID];
    if (userID === currentUser.id && currentUser.avatar) {
        return `<img src="${currentUser.avatar}" class="avatar-img ${extraClass || ''}">`;
    }
    if (avatarURL) {
        return `<img src="${avatarURL}" class="avatar-img ${extraClass || ''}">`;
    }
    const char = getUserAvatarChar(userID);
    return char;
}

function renderAvatarHTML(avatarURL, fallbackChar, extraClass) {
    if (avatarURL) {
        return `<div class="avatar ${extraClass || ''}"><img src="${avatarURL}" style="width:100%;height:100%;border-radius:50%;object-fit:cover;"></div>`;
    }
    return `<div class="avatar ${extraClass || ''}">${fallbackChar}</div>`;
}

function isAgentUser(userID) {
    return !!agentUserIDToBot[String(userID)];
}

function getAgentBotByUserID(userID) {
    return agentUserIDToBot[String(userID)] || null;
}

function getBotDisplayName(bot) {
    return bot ? (bot.nickname || bot.name || ('助手 ' + bot.id)) : '智能助手';
}
