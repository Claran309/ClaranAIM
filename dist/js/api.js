const API_BASE = 'http://localhost:8080/api/v1';
const WS_BASE = 'ws://localhost:8081/ws';

let token = localStorage.getItem('claran_token') || '';
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

if (currentUser && currentUser.id) {
    userNickCache[currentUser.id] = currentUser.nickname || currentUser.username;
}

function saveUnreadMap() {
    localStorage.setItem('claran_unread', JSON.stringify(unreadMap));
}

async function request(method, path, data = null, auth = true) {
    const headers = { 'Content-Type': 'application/json' };
    if (auth && token) {
        headers['Authorization'] = `Bearer ${token}`;
    }
    const options = { method, headers };
    if (data && method !== 'GET') {
        options.body = JSON.stringify(data);
    }
    try {
        const resp = await fetch(`${API_BASE}${path}`, options);
        const result = await resp.json();
        return result;
    } catch (err) {
        showToast('网络请求失败: ' + err.message, 'error');
        return null;
    }
}

const userAPI = {
    register: (username, password, nickname) =>
        request('POST', '/user/register', { username, password, nickname }, false),
    login: (username, password) =>
        request('POST', '/user/login', { username, password }, false),
    getInfo: () => request('GET', '/user/info'),
    updateInfo: (nickname, email, phone) =>
        request('PUT', '/user/info', { nickname, email, phone }),
    updateAvatar: (avatar) =>
        request('POST', '/user/avatar', { avatar }),
    logout: () => request('POST', '/user/logout'),
    addFriend: (friendID, groupID, remark) =>
        request('POST', '/user/friend/add', { friend_id: friendID, group_id: groupID || 0, remark: remark || '' }),
    deleteFriend: (friendID) =>
        request('POST', '/user/friend/delete', { friend_id: friendID }),
    updateFriendRemark: (friendID, groupID, remark) =>
        request('PUT', '/user/friend/remark', { friend_id: friendID, group_id: groupID || 0, remark: remark || '' }),
    getFriendList: () => request('GET', '/user/friend/list'),
    getFriendGroups: () => request('GET', '/user/friend/groups'),
    batchGetInfo: (ids) => request('GET', `/user/batch?ids=${ids.join(',')}`),
};

const groupAPI = {
    create: (name, memberIDs) =>
        request('POST', '/group/create', { name, member_ids: memberIDs }),
    get: (id) => request('GET', `/group/${id}`),
    list: () => request('GET', '/group/list'),
    invite: (groupID, userIDs) =>
        request('POST', '/group/invite', { group_id: groupID, user_ids: userIDs }),
    kick: (groupID, userID) =>
        request('POST', '/group/kick', { group_id: groupID, user_id: userID }),
    getMembers: (id) => request('GET', `/group/${id}/members`),
    transfer: (groupID, newOwnerID) =>
        request('POST', '/group/transfer', { group_id: groupID, new_owner_id: newOwnerID }),
    updateInfo: (groupID, name, announcement) =>
        request('PUT', '/group/info', { group_id: groupID, name, announcement }),
    pin: (groupID, isPinned) =>
        request('POST', '/group/pin', { group_id: groupID, is_pinned: isPinned }),
    mute: (groupID, userID, durationMinutes) =>
        request('POST', '/group/mute', { group_id: groupID, user_id: userID, duration_minutes: durationMinutes }),
    unmute: (groupID, userID) =>
        request('POST', '/group/unmute', { group_id: groupID, user_id: userID }),
    setRole: (groupID, userID, role) =>
        request('POST', '/group/role', { group_id: groupID, user_id: userID, role }),
    deleteGroup: (groupID) =>
        request('POST', '/group/delete', { group_id: groupID }),
};

const messageAPI = {
    createConversation: (type, participantIDs, groupID = 0) =>
        request('POST', '/message/conversation', { type, participant_ids: participantIDs, group_id: groupID }),
    send: (conversationID, content, msgType = 'text', options = {}) =>
        request('POST', '/message/send', {
            conversation_id: conversationID,
            content,
            msg_type: msgType,
            reply_to_id: options.reply_to_id || 0,
            mention_user_ids: options.mention_user_ids || [],
            mention_all: !!options.mention_all,
        }),
    markRead: (conversationID, messageID = 0) =>
        request('POST', '/message/read', { conversation_id: conversationID, message_id: messageID }),
    edit: (messageID, content) =>
        request('PUT', '/message/edit', { message_id: messageID, content }),
    recall: (messageID) =>
        request('POST', '/message/recall', { message_id: messageID }),
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
        const headers = {};
        if (token) headers['Authorization'] = `Bearer ${token}`;
        try {
            const resp = await fetch(`${API_BASE}/file/upload`, {
                method: 'POST',
                headers,
                body: formData,
            });
            if (!resp.ok) {
                const text = await resp.text();
                let errMsg = '上传失败';
                try { errMsg = JSON.parse(text).message || errMsg; } catch(e) {}
                showToast('文件上传失败: ' + errMsg, 'error');
                return null;
            }
            const result = await resp.json();
            if (result.code !== 0) {
                showToast('文件上传失败: ' + (result.message || '未知错误'), 'error');
                return result;
            }
            return result;
        } catch (err) {
            showToast('文件上传网络错误: ' + err.message, 'error');
            return null;
        }
    },
    get: (id) => request('GET', `/file/${id}`),
    previewURL: (id) => `${API_BASE}/file/download/${id}`,
    downloadURL: (id) => `${API_BASE}/file/download/${id}`,
    fetchBlob: async (id) => {
        const headers = {};
        if (token) headers['Authorization'] = `Bearer ${token}`;
        const resp = await fetch(`${API_BASE}/file/download/${id}`, { headers });
        if (!resp.ok) {
            let errMsg = '文件加载失败';
            try {
                const result = await resp.json();
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
    create: (name, type, description, modelName, apiKey, baseURL, systemPrompt, skillsDir, agentRoot) =>
        request('POST', '/bot/create', { name, type, description, model_name: modelName, api_key: apiKey, base_url: baseURL, system_prompt: systemPrompt, skills_dir: skillsDir, agent_root: agentRoot }),
    update: (botID, data) =>
        request('PUT', '/bot/update', { bot_id: botID, ...data }),
    get: (id) => request('GET', `/bot/${id}`),
    list: (type = '') => request('GET', `/bot/list?type=${type}`),
    delete: (botID) => request('DELETE', '/bot/delete', { bot_id: botID }),
    chat: (botID, message, conversationID = 0) =>
        request('POST', '/bot/chat', { bot_id: botID, message, conversation_id: conversationID }),
    createRoute: (botID, routePattern, routeType, priority) =>
        request('POST', '/bot/route/create', { bot_id: botID, route_pattern: routePattern, route_type: routeType, priority }),
    listRoutes: (botID) => request('GET', `/bot/${botID}/routes`),
    deleteRoute: (routeID) => request('DELETE', '/bot/route/delete', { route_id: routeID }),
    getBilling: (botID, limit = 20, offset = 0) =>
        request('GET', `/bot/${botID}/billing?limit=${limit}&offset=${offset}`),
};

function connectWS() {
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
            const msg = JSON.parse(event.data);
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
