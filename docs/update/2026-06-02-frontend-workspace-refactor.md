# 2026-06-02 前端工作区重构

## 背景

原前端把聊天、Agent、知识库、记忆、设置和治理能力都塞在同一个主界面里，入口多、层级浅，新增功能后会显得拥挤，也不利于后续继续扩展管理端和配置端。

本次重构没有引入 React/Vue，也没有拆散现有登录、WebSocket 和 API 状态，而是在静态前端上增加 Hash Workspace，让主要能力拥有独立工作台。

## 本次修改

- 新增顶部产品级工作区导航：
  - `#/chat`：IM 聊天主界面。
  - `#/agents`：Agent 工作台。
  - `#/knowledge`：知识工作台，承接 RAG、GraphRAG 和 Web Search。
  - `#/memory`：记忆中心。
  - `#/settings`：系统设置。
  - `#/admin`：系统治理台，仅管理员可见。
- 新增工作区渲染框架：
  - `workspaceFromHash`
  - `navigateWorkspace`
  - `renderWorkspace`
  - `renderWorkspaceShell`
  - `activateChatWorkspaceForContent`
  - `activateStandaloneWorkspace`
- 将原来的侧边栏入口保留为快捷入口，但用户也可以通过工作区导航进入独立页面。
- 新增聊天首页欢迎页，提供 Agent、知识和记忆的快速入口。
- 新增 Agent 工作台首页：
  - 创建 Agent。
  - 打开 Skill 管理。
  - 打开会话归档。
  - 查看我的 Agent。
  - 查看待确认动作。
- 新增知识工作台首页：
  - RAG 检索。
  - 知识录入。
  - GraphRAG 可视化。
  - Web Search 增强。
  - 最近知识文档列表。
- 新增记忆中心首页：
  - 正式记忆。
  - 候选记忆。
  - 新增记忆。
  - 会话归档入口。
- 新增系统设置首页：
  - LLM 预设。
  - Prompt 配置。
  - MCP Server 配置。
- 管理台从侧边栏按钮扩展为独立 `#/admin` 工作区，并继续使用原有 admin-service API。

## 体验优化

- 新增深色窄导航栏，区分“全局工作区”和“聊天侧栏”。
- 工作区页面增加独立 Header、动作按钮区、命令卡片和内容区。
- 增加页面进入动画、欢迎页环形动效、卡片 hover 反馈和响应式布局。
- 移动端下导航自动变为横向滚动，聊天工作区保留侧栏能力。
- 管理员入口按 `currentUser.role === 'admin'` 控制展示，普通用户不会看到治理台入口。

## Bug 修复

- 修复切换到 RAG / 知识图谱页面时错误清空当前聊天会话的问题。
  - 知识工作台现在不会重置 `currentConversationID` 和 `currentConversationType`。
  - 用户从会话跳到知识库查资料，再回到聊天页时，前端仍能恢复原会话视图。
- 保持 Agent 独立对话和普通 IM 会话的发送按钮切换：
  - 普通会话使用 `sendMessage()`。
  - Agent 独立对话使用 `sendBotChatMsg()`。

## 涉及文件

- `dist/index.html`
- `dist/js/app.js`
- `dist/css/style.css`

## 验证

- `node --check dist/js/app.js`
- `node --check dist/js/api.js`
- `go test ./...`

## 后续建议

- 后续可以把工作区进一步拆成真正的多页面构建产物，但目前静态前端依赖大量共享全局状态，Hash Workspace 是风险更低的过渡方案。
- 管理端、配置端、知识端可以继续沿当前工作区壳子深化页面，而不是继续往聊天侧边栏里堆按钮。
