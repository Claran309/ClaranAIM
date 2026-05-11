// API基础配置
const API_BASE = 'http://localhost:8080/api/v1';
const WS_BASE = 'ws://localhost:8081/ws';

let token = localStorage.getItem('claran_token') || '';
let currentUser = JSON.parse(localStorage.getItem('claran_user') || 'null');
let currentConversationID = null;
let ws = null;

// 通用请求方法
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

// 用户API
const userAPI = {
    register: (username, password, nickname) =>
        request('POST', '/user/register', { username, password, nickname }, false),

    login: (username, password) =>
        request('POST', '/user/login', { username, password }, false),

    getInfo: () => request('GET', '/user/info'),

    updateInfo: (nickname, email, phone) =>
        request('PUT', '/user/info', { nickname, email, phone }),

    addFriend: (friendID, groupID, remark) =>
        request('POST', '/user/friend/add', { friend_id: friendID, group_id: groupID || 0, remark: remark || '' }),

    deleteFriend: (friendID) =>
        request('POST', '/user/friend/delete', { friend_id: friendID }),

    getFriendList: () => request('GET', '/user/friend/list'),

    getFriendGroups: () => request('GET', '/user/friend/groups'),
};

// 群组API
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
};

// 消息API
const messageAPI = {
    createConversation: (type, participantIDs) =>
        request('POST', '/message/conversation', { type, participant_ids: participantIDs }),

    send: (conversationID, content, msgType = 'text') =>
        request('POST', '/message/send', { conversation_id: conversationID, content, msg_type: msgType }),

    getHistory: (conversationID, limit = 50, beforeID = 0) =>
        request('GET', `/message/history/${conversationID}?limit=${limit}&before_id=${beforeID}`),

    search: (keyword, limit = 20) =>
        request('GET', `/message/search?keyword=${encodeURIComponent(keyword)}&limit=${limit}`),

    getConversations: () => request('GET', '/message/conversations'),
};

// WebSocket连接
let wsReconnectTimer = null;

function connectWS() {
    if (!token) return;

    if (ws) {
        ws.close();
        ws = null;
    }

    ws = new WebSocket(`${WS_BASE}?token=${token}`);

    ws.onopen = () => {
        showToast('WebSocket连接已建立', 'success');
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
        showToast('WebSocket连接已断开', 'info');
        ws = null;
        wsReconnectTimer = setTimeout(connectWS, 3000);
    };

    ws.onerror = (err) => {
        console.error('WebSocket错误:', err);
    };
}

let lastSentMsgIds = new Set();

function handleWSMessage(msg) {
    if (msg.type === 'new_message' && msg.data) {
        if (msg.data.conversation_id === currentConversationID) {
            if (msg.data.msg_id && lastSentMsgIds.has(msg.data.msg_id)) {
                lastSentMsgIds.delete(msg.data.msg_id);
                return;
            }
            appendMessage(msg.data);
        }
        loadConversations();
    }
}
