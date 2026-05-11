// 页面切换
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

function switchSidebar(panel) {
    document.querySelectorAll('.sidebar-tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.sidebar-panel').forEach(p => p.classList.remove('active'));

    event.target.classList.add('active');
    document.getElementById(`${panel}-panel`).classList.add('active');

    if (panel === 'conversations') loadConversations();
    if (panel === 'friends') loadFriends();
    if (panel === 'groups') loadGroups();
}

// Toast通知
function showToast(msg, type = 'info') {
    const container = document.getElementById('toast-container');
    const toast = document.createElement('div');
    toast.className = `toast ${type}`;
    toast.textContent = msg;
    container.appendChild(toast);
    setTimeout(() => toast.remove(), 3000);
}

// 模态框
function showModal(title, bodyHTML) {
    document.getElementById('modal-title').textContent = title;
    document.getElementById('modal-body').innerHTML = bodyHTML;
    document.getElementById('modal-overlay').style.display = 'flex';
}

function closeModal() {
    document.getElementById('modal-overlay').style.display = 'none';
}

// 登录
async function login() {
    const username = document.getElementById('login-username').value.trim();
    const password = document.getElementById('login-password').value.trim();

    if (!username || !password) {
        showToast('请输入用户名和密码', 'error');
        return;
    }

    const resp = await userAPI.login(username, password);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        token = resp.data.token;
        currentUser = { id: resp.data.user_id, username };
        localStorage.setItem('claran_token', token);
        localStorage.setItem('claran_user', JSON.stringify(currentUser));
        showToast('登录成功', 'success');
        enterMainPage();
    } else {
        showToast(resp?.data?.msg || '登录失败', 'error');
    }
}

// 注册
async function register() {
    const username = document.getElementById('reg-username').value.trim();
    const password = document.getElementById('reg-password').value.trim();
    const nickname = document.getElementById('reg-nickname').value.trim();

    if (!username || !password) {
        showToast('请输入用户名和密码', 'error');
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

// 退出登录
function logout() {
    token = '';
    currentUser = null;
    currentConversationID = null;
    localStorage.removeItem('claran_token');
    localStorage.removeItem('claran_user');
    if (ws) ws.close();
    document.getElementById('auth-page').classList.add('active');
    document.getElementById('main-page').classList.remove('active');
}

// 进入主页面
async function enterMainPage() {
    document.getElementById('auth-page').classList.remove('active');
    document.getElementById('main-page').classList.add('active');

    // 获取用户信息
    const resp = await userAPI.getInfo();
    if (resp && resp.code === 0 && resp.data && resp.data.user) {
        currentUser = resp.data.user;
        document.getElementById('user-name').textContent = currentUser.nickname || currentUser.username;
        document.getElementById('user-avatar').textContent = (currentUser.nickname || currentUser.username).charAt(0).toUpperCase();
    }

    // 加载数据
    loadConversations();
    connectWS();
}

// 加载会话列表
async function loadConversations() {
    const resp = await messageAPI.getConversations();
    const list = document.getElementById('conversation-list');

    if (resp && resp.code === 0 && resp.data && resp.data.conversations) {
        const convs = resp.data.conversations;
        if (convs.length === 0) {
            list.innerHTML = '<div style="padding:20px;text-align:center;color:#999;">暂无会话</div>';
            return;
        }

        list.innerHTML = convs.map(c => `
            <div class="list-item ${currentConversationID === c.conversation_id ? 'active' : ''}"
                 onclick="openConversation(${c.conversation_id}, '${c.type}')">
                <div class="avatar">${c.type === 'private' ? 'P' : 'G'}</div>
                <div class="list-item-info">
                    <div class="list-item-name">会话 #${c.conversation_id} (${c.type === 'private' ? '私聊' : '群聊'})</div>
                    <div class="list-item-msg">${c.last_message || '暂无消息'}</div>
                </div>
            </div>
        `).join('');
    } else {
        list.innerHTML = '<div style="padding:20px;text-align:center;color:#999;">暂无会话</div>';
    }
}

// 加载好友列表
async function loadFriends() {
    const resp = await userAPI.getFriendList();
    const list = document.getElementById('friend-list');

    if (resp && resp.code === 0 && resp.data && resp.data.friends) {
        const friends = resp.data.friends;
        if (friends.length === 0) {
            list.innerHTML = '<div style="padding:20px;text-align:center;color:#999;">暂无好友</div>';
            return;
        }

        list.innerHTML = friends.map(f => `
            <div class="list-item">
                <div class="avatar">${(f.friend_name || '?').charAt(0).toUpperCase()}</div>
                <div class="list-item-info">
                    <div class="list-item-name">${f.friend_name || '用户' + f.friend_id}</div>
                    <div class="list-item-msg">${f.remark || f.friend_status || ''}</div>
                </div>
                <button class="btn-small" onclick="startPrivateChat(${f.friend_id})">聊天</button>
            </div>
        `).join('');
    } else {
        list.innerHTML = '<div style="padding:20px;text-align:center;color:#999;">暂无好友</div>';
    }
}

// 加载群组列表
async function loadGroups() {
    const resp = await groupAPI.list();
    const list = document.getElementById('group-list');

    if (resp && resp.code === 0 && resp.data && resp.data.groups) {
        const groups = resp.data.groups;
        if (groups.length === 0) {
            list.innerHTML = '<div style="padding:20px;text-align:center;color:#999;">暂无群组</div>';
            return;
        }

        list.innerHTML = groups.map(g => `
            <div class="list-item">
                <div class="avatar">G</div>
                <div class="list-item-info">
                    <div class="list-item-name">${g.name}</div>
                    <div class="list-item-msg">群主: ${g.owner_id}</div>
                </div>
                <button class="btn-small" onclick="openGroupConversation(${g.id})">进入</button>
            </div>
        `).join('');
    } else {
        list.innerHTML = '<div style="padding:20px;text-align:center;color:#999;">暂无群组</div>';
    }
}

// 打开会话
async function openConversation(conversationID, type) {
    currentConversationID = conversationID;

    document.getElementById('welcome-area').style.display = 'none';
    document.getElementById('chat-area').style.display = 'flex';
    document.getElementById('chat-title').textContent = `会话 #${conversationID} (${type === 'private' ? '私聊' : '群聊'})`;

    // 加载消息历史
    const resp = await messageAPI.getHistory(conversationID);
    const msgList = document.getElementById('message-list');

    if (resp && resp.code === 0 && resp.data && resp.data.messages) {
        msgList.innerHTML = resp.data.messages.map(m => createMessageHTML(m)).join('');
        msgList.scrollTop = msgList.scrollHeight;
    } else {
        msgList.innerHTML = '<div style="text-align:center;color:#999;padding:20px;">暂无消息</div>';
    }

    // 更新会话列表高亮
    loadConversations();
}

// 创建消息HTML
function createMessageHTML(m) {
    const isSent = m.sender_id === currentUser.id;
    return `
        <div class="message-item ${isSent ? 'sent' : 'received'}">
            <span class="message-sender">${isSent ? '我' : '用户' + m.sender_id}</span>
            <div class="message-bubble">${escapeHTML(m.content)}</div>
            <span class="message-time">${m.created_at || ''}</span>
        </div>
    `;
}

// 追加消息
function appendMessage(m) {
    const msgList = document.getElementById('message-list');
    msgList.innerHTML += createMessageHTML(m);
    msgList.scrollTop = msgList.scrollHeight;
}

// 发送消息
async function sendMessage() {
    const input = document.getElementById('msg-input');
    const content = input.value.trim();

    if (!content || !currentConversationID) return;

    const resp = await messageAPI.send(currentConversationID, content);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        input.value = '';
        if (resp.data.msg_id) {
            lastSentMsgIds.add(resp.data.msg_id);
        }
        appendMessage({
            sender_id: currentUser.id,
            content: content,
            created_at: new Date().toLocaleString('zh-CN')
        });
    } else {
        showToast(resp?.data?.msg || '发送失败', 'error');
    }
}

// 开始私聊
async function startPrivateChat(friendID) {
    const resp = await messageAPI.createConversation('private', [currentUser.id, friendID]);
    if (resp && resp.code === 0 && resp.data) {
        currentConversationID = resp.data.conversation_id;
        openConversation(resp.data.conversation_id, 'private');
        showToast('私聊会话已创建', 'success');
    } else {
        showToast('创建会话失败', 'error');
    }
}

// 打开群聊会话
async function openGroupConversation(groupID) {
    // 先获取群成员
    const membersResp = await groupAPI.getMembers(groupID);
    if (membersResp && membersResp.code === 0 && membersResp.data && membersResp.data.members) {
        const memberIDs = membersResp.data.members.map(m => m.user_id);
        const resp = await messageAPI.createConversation('group', memberIDs);
        if (resp && resp.code === 0 && resp.data) {
            openConversation(resp.data.conversation_id, 'group');
        }
    }
}

// 显示创建会话对话框
function showCreateConversation() {
    showModal('创建新会话', `
        <div class="form-group">
            <label>会话类型</label>
            <select id="conv-type" style="width:100%;padding:8px;border:1px solid #d9d9d9;border-radius:6px;">
                <option value="private">私聊</option>
                <option value="group">群聊</option>
            </select>
        </div>
        <div class="form-group">
            <label>对方用户ID（多个用逗号分隔）</label>
            <input type="text" id="conv-participants" placeholder="例如: 1,2,3">
        </div>
        <button class="btn-primary" onclick="createConversation()">创建</button>
    `);
}

async function createConversation() {
    const type = document.getElementById('conv-type').value;
    const participantsStr = document.getElementById('conv-participants').value.trim();
    const participantIDs = participantsStr.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n));

    if (participantIDs.length === 0) {
        showToast('请输入有效的用户ID', 'error');
        return;
    }

    const resp = await messageAPI.createConversation(type, participantIDs);
    if (resp && resp.code === 0 && resp.data) {
        closeModal();
        openConversation(resp.data.conversation_id, type);
        showToast('会话创建成功', 'success');
    } else {
        showToast(resp?.data?.msg || '创建失败', 'error');
    }
}

// 显示添加好友对话框
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
    const friendID = parseInt(document.getElementById('add-friend-id').value);
    const remark = document.getElementById('add-friend-remark').value.trim();

    if (!friendID) {
        showToast('请输入有效的用户ID', 'error');
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

// 显示创建群组对话框
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
    const memberIDs = membersStr.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n));

    if (!name) {
        showToast('请输入群组名称', 'error');
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

// 显示会话详情
async function showConversationInfo() {
    if (!currentConversationID) return;

    const resp = await messageAPI.getHistory(currentConversationID, 1, 0);
    let info = `<p>会话ID: ${currentConversationID}</p>`;

    showModal('会话详情', `
        ${info}
        <div class="form-group" style="margin-top:16px;">
            <label>搜索消息</label>
            <input type="text" id="search-keyword" placeholder="输入关键词搜索">
            <button class="btn-primary" style="margin-top:8px;" onclick="searchMessages()">搜索</button>
        </div>
        <div id="search-results"></div>
    `);
}

async function searchMessages() {
    const keyword = document.getElementById('search-keyword').value.trim();
    if (!keyword) return;

    const resp = await messageAPI.search(keyword);
    const container = document.getElementById('search-results');

    if (resp && resp.code === 0 && resp.data && resp.data.messages) {
        const msgs = resp.data.messages;
        if (msgs.length === 0) {
            container.innerHTML = '<p style="color:#999;margin-top:8px;">未找到相关消息</p>';
        } else {
            container.innerHTML = msgs.map(m => `
                <div style="padding:8px;border-bottom:1px solid #f0f0f0;">
                    <span style="color:#1677ff;">用户${m.sender_id}:</span>
                    <span>${escapeHTML(m.content)}</span>
                    <span style="color:#999;font-size:12px;">${m.created_at}</span>
                </div>
            `).join('');
        }
    }
}

// HTML转义
function escapeHTML(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

// 页面初始化
window.onload = function () {
    if (token && currentUser) {
        enterMainPage();
    }
};
