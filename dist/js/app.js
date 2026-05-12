let groupConversationMap = {};

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

function showModal(title, bodyHTML) {
    document.getElementById('modal-title').textContent = title;
    document.getElementById('modal-body').innerHTML = bodyHTML;
    document.getElementById('modal-overlay').style.display = 'flex';
}

function closeModal() {
    document.getElementById('modal-overlay').style.display = 'none';
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
        token = resp.data.token;
        currentUser = { id: resp.data.user_id, username };
        userNickCache[currentUser.id] = username;
        localStorage.setItem('claran_token', token);
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
    token = '';
    currentUser = null;
    currentConversationID = null;
    currentConversationType = '';
    unreadMap = {};
    groupConversationMap = {};
    friendsCache = [];
    groupsCache = [];
    userNickCache = {};
    conversationNameCache = {};
    localStorage.removeItem('claran_token');
    localStorage.removeItem('claran_user');
    localStorage.removeItem('claran_unread');
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
    loadConversations();
    loadFriends();
    loadGroups();
    connectWS();
}

async function loadConversations() {
    const resp = await messageAPI.getConversations();
    const list = document.getElementById('conversation-list');

    if (resp && resp.code === 0 && resp.data && resp.data.conversations) {
        const convs = resp.data.conversations;
        if (convs.length === 0) {
            list.innerHTML = '<div class="empty-tip">暂无会话<br><small>点击好友列表的「聊天」或群组的「进入」开始对话</small></div>';
            return;
        }

        const senderIDs = [];
        convs.forEach(c => {
            if (c.last_sender_id && !userNickCache[c.last_sender_id]) {
                senderIDs.push(c.last_sender_id);
            }
        });
        if (senderIDs.length > 0) {
            await resolveUserNames([...new Set(senderIDs)]);
        }

        list.innerHTML = convs.map(c => {
            const unread = unreadMap[c.conversation_id] || 0;
            const isActive = currentConversationID === c.conversation_id;
            const typeIcon = c.type === 'private' ? '👤' : '👥';
            const typeLabel = c.type === 'private' ? '私聊' : '群聊';
            const displayName = conversationNameCache[c.conversation_id] || c.target_name || '会话 #' + c.conversation_id;
            return `
                <div class="list-item ${isActive ? 'active' : ''}" onclick="openConversation(${c.conversation_id}, '${c.type}')">
                    <div class="avatar conv-avatar">${typeIcon}</div>
                    <div class="list-item-info">
                        <div class="list-item-top">
                            <span class="list-item-name">${escapeHTML(displayName)}</span>
                            <span class="list-item-type">${typeLabel}</span>
                        </div>
                        <div class="list-item-msg">${escapeHTML(c.last_message || '暂无消息')}</div>
                    </div>
                    ${unread > 0 ? `<span class="item-unread">${unread > 99 ? '99+' : unread}</span>` : ''}
                </div>
            `;
        }).join('');
    } else {
        list.innerHTML = '<div class="empty-tip">暂无会话</div>';
    }
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
                userNickCache[f.friend_id] = f.friend_name || f.remark || '用户' + f.friend_id;
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
                        <button class="btn-chat" onclick="startPrivateChat(${f.friend_id})">聊天</button>
                        <button class="btn-delete-friend" onclick="deleteFriend(${f.friend_id}, '${escapeHTML(displayName)}')" title="删除好友">✕</button>
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
            const ownerName = userNickCache[g.owner_id] || '用户' + g.owner_id;
            const isPinned = g.is_pinned;
            return `
                <div class="list-item group-item ${isPinned ? 'pinned' : ''}">
                    ${avatarHTML}
                    <div class="list-item-info">
                        <div class="list-item-name">${isPinned ? '📌 ' : ''}${escapeHTML(g.name)}</div>
                        <div class="list-item-msg">群主: ${escapeHTML(ownerName)}</div>
                    </div>
                    <div class="group-actions">
                        <button class="btn-chat" onclick="openGroupConversation(${g.id})">进入</button>
                        <button class="btn-small-outline" onclick="showGroupMembers(${g.id})">成员</button>
                        <button class="btn-small-outline" onclick="showGroupManage(${g.id})">管理</button>
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
        const participantsResp = await messageAPI.getConversations();
        if (type === 'private') {
            const convResp = await messageAPI.getHistory(conversationID, 1, 0);
            if (convResp && convResp.code === 0 && convResp.data && convResp.data.messages && convResp.data.messages.length > 0) {
                const otherSenderID = convResp.data.messages[0].sender_id;
                const otherID = otherSenderID === currentUser.id ? convResp.data.messages[0].sender_id : otherSenderID;
            }

            const convsResp = await messageAPI.getConversations();
            if (convsResp && convsResp.code === 0 && convsResp.data && convsResp.data.conversations) {
                const conv = convsResp.data.conversations.find(c => c.conversation_id === conversationID);
                if (conv && conv.participant_ids) {
                    const otherID = conv.participant_ids.find(id => id !== currentUser.id);
                    if (otherID) {
                        await resolveUserNames([otherID]);
                        const name = getUserName(otherID);
                        conversationNameCache[conversationID] = name;
                        return name;
                    }
                }
            }

            const name = friendsCache.find(f => true)?.friend_name || '私聊';
            conversationNameCache[conversationID] = name;
            return name;
        } else {
            for (const [groupID, convID] of Object.entries(groupConversationMap)) {
                if (convID === conversationID) {
                    const group = groupsCache.find(g => g.id === parseInt(groupID));
                    if (group) {
                        conversationNameCache[conversationID] = group.name;
                        return group.name;
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

async function openConversation(conversationID, type) {
    currentConversationID = conversationID;
    currentConversationType = type;

    delete unreadMap[conversationID];
    saveUnreadMap();
    updateUnreadBadge();

    document.getElementById('welcome-area').style.display = 'none';
    document.getElementById('chat-area').style.display = 'flex';

    const convName = await resolveConversationName(conversationID, type);
    document.getElementById('chat-title').textContent = convName;
    const typeLabel = type === 'private' ? '👤 私聊' : '👥 群聊';
    document.getElementById('chat-type-badge').textContent = typeLabel;
    document.getElementById('chat-type-badge').className = `chat-type-badge ${type}`;

    const resp = await messageAPI.getHistory(conversationID);
    const msgList = document.getElementById('message-list');

    if (resp && resp.code === 0 && resp.data && resp.data.messages) {
        const messages = resp.data.messages;
        const senderIDs = [...new Set(messages.map(m => m.sender_id))];
        await resolveUserNames(senderIDs);

        msgList.innerHTML = messages.map(m => createMessageHTML(m)).join('');
        msgList.scrollTop = msgList.scrollHeight;
    } else {
        msgList.innerHTML = '<div class="empty-tip">暂无消息，发送第一条消息吧 💬</div>';
    }

    loadConversations();
}

function createMessageHTML(m) {
    const isSent = m.sender_id === currentUser.id;
    const isBot = m.sender_id === 0;
    const senderName = isBot ? '🤖 AI助手' : (isSent ? '我' : getUserName(m.sender_id));
    const time = m.created_at || '';
    const avatarChar = isBot ? '🤖' : (isSent
        ? (currentUser.nickname || currentUser.username).charAt(0).toUpperCase()
        : getUserAvatarChar(m.sender_id));
    const avatarBg = isSent ? '' : 'received';
    const bubbleContent = renderMessageContent(m.content, m.msg_type);
    return `
        <div class="message-item ${isSent ? 'sent' : 'received'} ${isBot ? 'bot-msg' : ''}">
            <div class="msg-avatar ${avatarBg}">${avatarChar}</div>
            <div class="msg-body">
                <div class="msg-meta">
                    <span class="message-sender">${escapeHTML(senderName)}</span>
                    <span class="message-time">${time}</span>
                </div>
                <div class="message-bubble">${bubbleContent}</div>
            </div>
        </div>
    `;
}

function appendMessage(m) {
    const msgList = document.getElementById('message-list');
    if (msgList.querySelector('.empty-tip')) {
        msgList.innerHTML = '';
    }
    if (m.sender_id && !userNickCache[m.sender_id]) {
        resolveUserNames([m.sender_id]).then(() => {
            msgList.innerHTML += createMessageHTML(m);
            msgList.scrollTop = msgList.scrollHeight;
        });
    } else {
        msgList.innerHTML += createMessageHTML(m);
        msgList.scrollTop = msgList.scrollHeight;
    }
}

async function sendMessage() {
    const input = document.getElementById('msg-input');
    const content = input.value.trim();
    if (!content || !currentConversationID) return;

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
        content: content,
        created_at: timeStr,
    };
    appendMessage(optimisticMsg);

    const resp = await messageAPI.send(currentConversationID, content);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        // success
    } else {
        showToast(resp?.data?.msg || '发送失败', 'error');
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
        openConversation(groupConversationMap[groupID], 'group');
        switchSidebar('conversations', document.querySelector('.sidebar-tab:first-child'));
        return;
    }

    const membersResp = await groupAPI.getMembers(groupID);
    if (!membersResp || membersResp.code !== 0 || !membersResp.data || !membersResp.data.members) {
        showToast('获取群成员失败', 'error');
        return;
    }

    const memberIDs = membersResp.data.members.map(m => m.user_id);
    const resp = await messageAPI.createConversation('group', memberIDs);
    if (resp && resp.code === 0 && resp.data) {
        const convId = resp.data.conversation_id;
        groupConversationMap[groupID] = convId;

        const group = groupsCache.find(g => g.id === groupID);
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

function showCreateConversation() {
    showModal('创建新会话', `
        <div class="form-group">
            <label>会话类型</label>
            <select id="conv-type" class="form-select">
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
        showToast('请输入有效的用户ID', 'warning');
        return;
    }

    const allIDs = type === 'private' ? participantIDs : [currentUser.id, ...participantIDs];
    const resp = await messageAPI.createConversation(type, allIDs);
    if (resp && resp.code === 0 && resp.data) {
        closeModal();
        openConversation(resp.data.conversation_id, type);
        showToast('会话创建成功', 'success');
    } else {
        showToast(resp?.data?.msg || '创建失败', 'error');
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
    const friendID = parseInt(document.getElementById('add-friend-id').value);
    const remark = document.getElementById('add-friend-remark').value.trim();
    if (!friendID) {
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
    const memberIDs = membersStr.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n));
    if (!name) {
        showToast('请输入群组名称', 'warning');
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
    if (groupResp && groupResp.code === 0 && groupResp.data && groupResp.data.group) {
        groupName = groupResp.data.group.name;
        isOwner = groupResp.data.group.owner_id === currentUser.id;
    }

    let membersHTML = '<div class="empty-tip">加载中...</div>';
    if (membersResp && membersResp.code === 0 && membersResp.data && membersResp.data.members) {
        const members = membersResp.data.members;
        if (members.length === 0) {
            membersHTML = '<div class="empty-tip">暂无成员</div>';
        } else {
            const memberIDs = members.map(m => m.user_id);
            await resolveUserNames(memberIDs);

            membersHTML = `
                <div class="section-label">群成员 (${members.length})</div>
                <div class="member-list">
                    ${members.map(m => {
                        const roleClass = m.role === 'owner' ? 'owner' : (m.role === 'admin' ? 'admin' : '');
                        const roleLabel = m.role === 'owner' ? '群主' : (m.role === 'admin' ? '管理员' : '成员');
                        const canKick = currentUser.id !== m.user_id && m.role !== 'owner';
                        const canManage = isOwner && m.role !== 'owner';
                        const memberName = getUserName(m.user_id);
                        return `
                            <div class="member-item">
                                <div class="avatar small">${memberName.charAt(0).toUpperCase()}</div>
                                <div class="member-info">
                                    <span class="member-name">${escapeHTML(memberName)}</span>
                                    <span class="member-tag ${roleClass}">${roleLabel}</span>
                                </div>
                                <div class="member-actions">
                                    ${canManage ? `<button class="btn-kick" onclick="showMuteMember(${groupID}, ${m.user_id}, '${escapeHTML(memberName)}')">禁言</button>` : ''}
                                    ${canManage ? `<button class="btn-kick" onclick="showSetRole(${groupID}, ${m.user_id}, '${escapeHTML(memberName)}')">角色</button>` : ''}
                                    ${canKick ? `<button class="btn-kick" onclick="kickMember(${groupID}, ${m.user_id})">移除</button>` : ''}
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
                        <button class="btn-inline btn-primary" onclick="inviteMember(${groupID})">邀请</button>
                    </div>
                `;
            }
        }
    }

    showModal(`群组: ${groupName}`, membersHTML);
}

async function inviteMember(groupID) {
    const idsStr = document.getElementById('invite-user-ids').value.trim();
    const userIDs = idsStr.split(',').map(s => parseInt(s.trim())).filter(n => !isNaN(n));
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

    const resp = await messageAPI.search(keyword, currentConversationID);
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
                    <div class="search-result-item" onclick="jumpToMessage(${m.conversation_id}, ${m.id})">
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
    if (currentConversationID !== conversationID) {
        openConversation(conversationID, 'private');
    }
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
    showModal('个人信息', `
        <div style="text-align:center;margin-bottom:20px;">
            <div class="avatar large" id="profile-avatar-display">${currentUser.avatar ? `<img src="${currentUser.avatar}" style="width:100%;height:100%;border-radius:50%;object-fit:cover;">` : (currentUser.nickname || currentUser.username).charAt(0).toUpperCase()}</div>
            <div style="font-size:18px;font-weight:600;margin-top:8px;">${escapeHTML(currentUser.nickname || currentUser.username)}</div>
        </div>
        <div class="form-group">
            <label>昵称</label>
            <input type="text" id="profile-nickname" value="${escapeHTML(currentUser.nickname || '')}">
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
            <label>头像URL</label>
            <input type="text" id="profile-avatar" value="${escapeHTML(currentUser.avatar || '')}" placeholder="输入头像图片URL">
        </div>
        <button class="btn-primary" onclick="saveProfile()">保存修改</button>
    `);
}

async function saveProfile() {
    const nickname = document.getElementById('profile-nickname').value.trim();
    const email = document.getElementById('profile-email').value.trim();
    const phone = document.getElementById('profile-phone').value.trim();
    const avatar = document.getElementById('profile-avatar').value.trim();

    if (nickname || email || phone) {
        const resp = await userAPI.updateInfo(nickname, email, phone);
        if (resp && resp.code === 0 && resp.data && resp.data.success) {
            currentUser.nickname = nickname || currentUser.nickname;
            currentUser.email = email;
            currentUser.phone = phone;
            userNickCache[currentUser.id] = currentUser.nickname || currentUser.username;
        } else {
            showToast(resp?.data?.msg || '更新信息失败', 'error');
            return;
        }
    }

    if (avatar && avatar !== (currentUser.avatar || '')) {
        const resp = await userAPI.updateAvatar(avatar);
        if (resp && resp.code === 0 && resp.data && resp.data.success) {
            currentUser.avatar = avatar;
        } else {
            showToast(resp?.data?.msg || '更新头像失败', 'error');
            return;
        }
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
}

function escapeHTML(str) {
    if (!str) return '';
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
}

function showGroupManage(groupID) {
    const group = groupsCache.find(g => g.id === groupID);
    if (!group) return;
    const isOwner = group.owner_id === currentUser.id;
    const isAdmin = isOwner;

    showModal(`群组管理 - ${escapeHTML(group.name)}`, `
        <div class="form-group">
            <label>群名称</label>
            <input type="text" id="mg-name" value="${escapeHTML(group.name)}" ${!isAdmin ? 'disabled' : ''}>
        </div>
        <div class="form-group">
            <label>群公告</label>
            <textarea id="mg-announcement" rows="3" ${!isAdmin ? 'disabled' : ''}>${escapeHTML(group.announcement || '')}</textarea>
        </div>
        ${isAdmin ? `
        <div class="btn-row">
            <button class="btn-primary" onclick="saveGroupInfo(${groupID})">保存修改</button>
            <button class="btn-inline" onclick="pinGroup(${groupID}, ${group.is_pinned ? 'false' : 'true'})">${group.is_pinned ? '取消置顶' : '置顶群聊'}</button>
        </div>
        <hr style="margin:16px 0;border-color:var(--border);">
        <div class="section-label">高级管理</div>
        <div class="btn-row">
            <button class="btn-inline btn-warning" onclick="showTransferOwner(${groupID})">转让群主</button>
            <button class="btn-inline btn-danger" onclick="deleteGroup(${groupID})">解散群组</button>
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
        loadGroups();
    } else {
        showToast(resp?.data?.msg || '更新失败', 'error');
    }
}

async function pinGroup(groupID, isPinned) {
    const resp = await groupAPI.pin(groupID, isPinned);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast(isPinned ? '已置顶' : '已取消置顶', 'success');
        closeModal();
        loadGroups();
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
        <button class="btn-primary btn-warning" onclick="transferOwner(${groupID})">确认转让</button>
    `);
}

async function transferOwner(groupID) {
    const newOwnerID = parseInt(document.getElementById('transfer-new-owner').value);
    if (!newOwnerID) { showToast('请输入有效的用户ID', 'warning'); return; }
    if (!confirm('确定要转让群主吗？此操作不可撤销！')) return;
    const resp = await groupAPI.transfer(groupID, newOwnerID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('群主已转让', 'success');
        closeModal();
        loadGroups();
    } else {
        showToast(resp?.data?.msg || '转让失败', 'error');
    }
}

async function deleteGroup(groupID) {
    if (!confirm('确定要解散群组吗？此操作不可撤销！')) return;
    const resp = await groupAPI.deleteGroup(groupID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('群组已解散', 'success');
        closeModal();
        loadGroups();
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
        <button class="btn-primary btn-warning" onclick="muteMember(${groupID}, ${userID})">确认禁言</button>
    `);
}

async function muteMember(groupID, userID) {
    const duration = parseInt(document.getElementById('mute-duration').value);
    if (!duration || duration <= 0) { showToast('请输入有效的禁言时长', 'warning'); return; }
    const resp = await groupAPI.mute(groupID, userID, duration);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已禁言', 'success');
        closeModal();
    } else {
        showToast(resp?.data?.msg || '禁言失败', 'error');
    }
}

async function unmuteMember(groupID, userID) {
    const resp = await groupAPI.unmute(groupID, userID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已解除禁言', 'success');
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
        <button class="btn-primary" onclick="setMemberRole(${groupID}, ${userID})">确认</button>
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
        if (resp && resp.code === 0 && resp.data && resp.data.success) {
            const fileURL = resp.data.file_url || '';
            const fileID = resp.data.file_id || '';
            let msgType = 'file';
            if (file.type.startsWith('image/')) msgType = 'image';
            else if (file.type.startsWith('audio/')) msgType = 'voice';

            let content = fileURL || fileID;
            if (msgType === 'image') {
                content = `[img]${fileURL || fileID}[/img]`;
            } else if (msgType === 'voice') {
                content = `[voice]${file.name}[/voice]`;
            } else {
                content = `[file]${file.name}[/file]`;
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
                appendMessage({ sender_id: currentUser.id, content, created_at: timeStr });
                showToast('文件发送成功', 'success');
            } else {
                showToast('消息发送失败', 'error');
            }
        } else {
            showToast(resp?.data?.msg || '文件上传失败', 'error');
        }
    };
    input.click();
}

function renderMessageContent(content, msgType) {
    if (msgType === 'image' || (content && content.startsWith('[img]'))) {
        const urlMatch = content.match(/\[img\](.*?)\[\/img\]/);
        const url = urlMatch ? urlMatch[1] : content;
        return `<img src="${escapeHTML(url)}" style="max-width:260px;max-height:200px;border-radius:8px;cursor:pointer;" onclick="window.open('${escapeHTML(url)}','_blank')" onerror="this.outerHTML='[图片加载失败]'">`;
    }
    if (msgType === 'voice' || (content && content.startsWith('[voice]'))) {
        const nameMatch = content.match(/\[voice\](.*?)\[\/voice\]/);
        const name = nameMatch ? nameMatch[1] : '语音消息';
        return `<div class="media-msg voice-msg">🎤 ${escapeHTML(name)}</div>`;
    }
    if (msgType === 'file' || (content && content.startsWith('[file]'))) {
        const nameMatch = content.match(/\[file\](.*?)\[\/file\]/);
        const name = nameMatch ? nameMatch[1] : '文件';
        return `<div class="media-msg file-msg">📎 ${escapeHTML(name)}</div>`;
    }
    return escapeHTML(content);
}

function showBotPanel() {
    showModal('AI 助手管理', `
        <div class="section-label">我的 AI 助手</div>
        <div id="bot-list-area" class="bot-list-area">加载中...</div>
        <hr style="margin:16px 0;border-color:var(--border);">
        <div class="section-label">创建新助手</div>
        <div class="form-group">
            <label>助手名称</label>
            <input type="text" id="bot-name" placeholder="例如: Amiya">
        </div>
        <div class="form-group">
            <label>类型</label>
            <select id="bot-type" class="form-select">
                <option value="internal">内部Bot</option>
                <option value="custom">自部署Bot</option>
            </select>
        </div>
        <div class="form-group">
            <label>描述</label>
            <input type="text" id="bot-desc" placeholder="助手功能描述">
        </div>
        <div class="form-group">
            <label>模型名称</label>
            <input type="text" id="bot-model" placeholder="例如: gpt-4o-mini" value="gpt-4o-mini">
        </div>
        <div class="form-group">
            <label>API Key</label>
            <input type="password" id="bot-apikey" placeholder="LLM API Key">
        </div>
        <div class="form-group">
            <label>Base URL</label>
            <input type="text" id="bot-baseurl" placeholder="例如: https://api.openai.com/v1">
        </div>
        <div class="form-group">
            <label>系统提示词</label>
            <textarea id="bot-prompt" rows="3" placeholder="助手的系统提示词"></textarea>
        </div>
        <button class="btn-primary" onclick="createBot()">创建助手</button>
    `);
    loadBotList();
}

async function loadBotList() {
    const area = document.getElementById('bot-list-area');
    const resp = await botAPI.list();
    if (resp && resp.code === 0 && resp.data && resp.data.bots) {
        const bots = resp.data.bots;
        if (bots.length === 0) {
            area.innerHTML = '<div class="empty-tip">暂无AI助手</div>';
            return;
        }
        area.innerHTML = bots.map(b => `
            <div class="bot-item">
                <div class="bot-info">
                    <span class="bot-name">🤖 ${escapeHTML(b.name)}</span>
                    <span class="bot-type ${b.type}">${b.type === 'internal' ? '内部' : '自部署'}</span>
                    <span class="bot-status ${b.is_active ? 'active' : 'inactive'}">${b.is_active ? '运行中' : '已停用'}</span>
                </div>
                <div class="bot-desc">${escapeHTML(b.description || '无描述')}</div>
                <div class="bot-actions">
                    <button class="btn-inline" onclick="chatWithBot(${b.id})">对话</button>
                    <button class="btn-inline" onclick="toggleBot(${b.id}, ${!b.is_active})">${b.is_active ? '停用' : '启用'}</button>
                    <button class="btn-inline btn-danger" onclick="deleteBot(${b.id})">删除</button>
                </div>
            </div>
        `).join('');
    } else {
        area.innerHTML = '<div class="empty-tip">加载失败</div>';
    }
}

async function createBot() {
    const name = document.getElementById('bot-name').value.trim();
    const type = document.getElementById('bot-type').value;
    const description = document.getElementById('bot-desc').value.trim();
    const modelName = document.getElementById('bot-model').value.trim();
    const apiKey = document.getElementById('bot-apikey').value.trim();
    const baseURL = document.getElementById('bot-baseurl').value.trim();
    const systemPrompt = document.getElementById('bot-prompt').value.trim();
    if (!name || !modelName) { showToast('请填写助手名称和模型', 'warning'); return; }
    const resp = await botAPI.create(name, type, description, modelName, apiKey, baseURL, systemPrompt, '', '');
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('助手创建成功', 'success');
        loadBotList();
    } else {
        showToast(resp?.data?.msg || '创建失败', 'error');
    }
}

async function toggleBot(botID, isActive) {
    const resp = await botAPI.update(botID, { is_active: isActive });
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast(isActive ? '已启用' : '已停用', 'success');
        loadBotList();
    } else {
        showToast(resp?.data?.msg || '操作失败', 'error');
    }
}

async function deleteBot(botID) {
    if (!confirm('确定要删除该AI助手吗？')) return;
    const resp = await botAPI.delete(botID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        showToast('已删除', 'success');
        loadBotList();
    } else {
        showToast(resp?.data?.msg || '删除失败', 'error');
    }
}

function chatWithBot(botID) {
    if (!currentConversationID) {
        showToast('请先打开一个会话', 'warning');
        return;
    }
    showModal('AI 助手对话', `
        <div class="form-group">
            <label>输入消息</label>
            <input type="text" id="bot-chat-msg" placeholder="向AI助手提问..." onkeydown="if(event.key==='Enter')sendBotChat(${botID})">
        </div>
        <button class="btn-primary" onclick="sendBotChat(${botID})">发送</button>
        <div id="bot-chat-reply" style="margin-top:12px;"></div>
    `);
    setTimeout(() => document.getElementById('bot-chat-msg')?.focus(), 100);
}

async function sendBotChat(botID) {
    const msg = document.getElementById('bot-chat-msg').value.trim();
    if (!msg) return;
    const replyDiv = document.getElementById('bot-chat-reply');
    replyDiv.innerHTML = '<div class="search-loading"><div class="spinner"></div>AI思考中...</div>';
    const resp = await botAPI.chat(botID, msg, currentConversationID);
    if (resp && resp.code === 0 && resp.data && resp.data.success) {
        replyDiv.innerHTML = `<div class="bot-reply"><strong>AI回复:</strong><br>${escapeHTML(resp.data.reply)}</div>`;
        if (resp.data.reply && currentConversationID) {
            const now = new Date();
            const timeStr = now.getFullYear() + '-' +
                String(now.getMonth() + 1).padStart(2, '0') + '-' +
                String(now.getDate()).padStart(2, '0') + ' ' +
                String(now.getHours()).padStart(2, '0') + ':' +
                String(now.getMinutes()).padStart(2, '0') + ':' +
                String(now.getSeconds()).padStart(2, '0');
            appendMessage({ sender_id: 0, content: `[AI] ${resp.data.reply}`, created_at: timeStr });
        }
    } else {
        replyDiv.innerHTML = `<div class="bot-reply error">对话失败: ${escapeHTML(resp?.data?.msg || '未知错误')}</div>`;
    }
}

window.onload = function () {
    if (token && currentUser) {
        enterMainPage();
    }
};
