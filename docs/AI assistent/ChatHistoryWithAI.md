# Phase1 MVP

## 1.1 Trae
请阅读我项目中除了learn文件夹以外的项目结构，这是项目初始化后的基本结构。 请阅读我项目中的readme和我给出的项目成品要求，了解我做该项目的目的和应该呈现的效果与各阶段应该完成的任务

现阶段你不需要写代码

AIM

AIM 是一个面向多人在线的即时通讯系统，内置可自部署的 AI 助手，将大模型能力深度集成到聊天场景中，实现"通讯 + AI"的深度融合。

基本要求：

基于 TCP/WebSocket 的实时消息收发，支持单聊、群聊、广播消息；消息类型支持文本、图片、文件、语音。

消息已读回执、输入状态提示、在线状态管理；消息本地存储与云端漫游，支持按关键词、时间范围搜索历史消息。

好友关系管理：添加、删除、分组、备注。

群组管理：创建、邀请、踢出、禁言、转让群主、群公告。

内置聊天 Agent，接入多家厂商模型接口，用户可直接 @Agent 对话；支持用户通过 OpenAPI 自行部署机器人，也可使用 AIM 平台内置 Agent，这里有两个选择，一个是自己提供 API Key，也可以使用平台的模型，要求需要做好计费管理。

一键总结群聊/单聊历史消息，生成要点摘要与待办提取；根据上下文生成回复候选，用户可一键选用。

分布式架构，不限制框架，需要合理划分模块，要求至少需要使用 docker 打包部署。

进阶要求：

RAG 知识库 Bot：用户可上传多种格式的文档（pdf、md、doc、ppt）构建私有知识库，Bot 可基于知识库回答问题。

MCP 工具集成：Bot 可调用外部工具（天气查询、代码执行、Web 搜索等），扩展能力边界。

多 Agent 协作：支持配置多个不同人设/能力的 Bot，按场景自动路由或由用户指定。

记忆能力：Bot 记住用户偏好与历史交互，跨会话保持上下文。

消息引用与回复、限时撤回与编辑、离线推送与上线同步。

多端消息同步：同一账号多端登录，消息实时同步，已读状态一致。

实时多语言翻译；接入模型对消息内容进行实时审核与过滤。

集成主流可观测性组件，包括分布式日志、链路追踪；集成 Prometheus + Grafana 监控仪表盘：在线人数、消息吞吐量、Agent 响应延迟。

提供 CLI 客户端（TUI 界面），支持 Markdown 渲染与流式输出；可选提供 Web 前端或移动端客户端。

服务治理，包括超时，限流，熔断，重试等。

## 1.2
现在请熟悉我项目中的learn文件夹中的示例项目，熟悉我的码风和项目结构，并阅读APIdoc_plan.md文件，了解我第一阶段需要开发的接口

现阶段你不用写代码

## 1.3

现在，请以我的码风，以我的项目目的和结构为准，写出整个项目第一阶段的所有代码，要求接口在APIdoc里有，核心代码分为cmd中的main和internal中的handler/service/dao/model等，要求整个项目是分布式架构，比如说各个服务之间的表结构应当在各自的服务中完成初始化与自动迁移，各服务之间必须是松耦合的。

注意在代码中 添加适量注释

另外，请写出相应的前端代码和页面（在dist中），并告诉我启动方法（我不会前端），注意我需要能在前端页面直接对项目进行测试以省区我用apifox等软件的必要

另外，项目中所有需要用到的第三方组件（如mysql，redis等）我都会运行在docker中，你要保证代码能正常连接这些在docker中的组件，以下为组件docker信息：
Mysql：端口3306，服务名MySQL，dsn为claran:chr070309@tcp(localhost:3306)/ClaranCloudDisk?charset=utf8mb4&parseTime=True&loc=Local
Redis：端口6379，服务名Redis
JWT_SECRET_KEY=MySuperSecureJWTKey@2025!Claran_work

## 1.4
现在有以下问题：

- 在线状态有问题，没有登出->切换离线状态功能

- group服务没有做redis缓存，为什么？如果必要就加上

- user服务虽然有新用户信息以及管理头像系列逻辑，但并没有相关路由，前端页面也不能修改信息和头像

- 前端页面弹出消息框部分（比如用户成功登录，websocket已连接等）不要再右上角，放在显眼的位置

- 前端的查找消息历史系列功能模块做得比较丑且不直观

- 告诉我现在消息是怎么落库的？表结构是怎样的？

- 每次主动发送消息，前端都会看到发两次消息，实际上接收到1条消息

- 多次点击进入群聊聊天，前端会弹出多个不同的会话列表

- 前端没有未读消息提示（红点和条数）

- 当前并未完全实现用户信息管理系统或前端未适配相应接口，待调试

- 前端页面设计一般（如各种提示引导框在不起眼的位置），待修改

- 每次需要手动切换群组或其他按钮才显示有某人好友或者群聊

- 群聊无邀请成员和管理成员的前端模块

## 1.5
- 在好友界面显示的好友无法显示头像和群聊头像

- 好友登出后还是显示在线

- 聊天气泡长度和文本长度不贴合

- 聊天界面仍然显示“用户2”而非设定的昵称

- 重新登录停留在上次会话界面，希望在无界面

- 在会话外收到消息有消息提示和红点，但是重新登录没有

- 请支持群聊能改会话标配的功能，私聊就消失用户昵称即可

- 聊天历史搜索功能应该只在当前会话搜索

## 1.6
- 修改个人信息界面头像没居中

- 添加不存在的用户为好友也能成功并创建聊天回话，这是非法的

- 当用户创建一个未注册用户为好友时能成功，并且用户注册并占据用户ID后显示出非法创建者为好友，这是不合理的

- 好友的在线状态不正常，查看他人的在线状态只会在添加好友或者登录时才更新

- 可以和已删除用户对话，这是错误的

- 可以非法创建不合理的会话，如创建一个不存在的用户会话，这是不合理的

- 气泡位置不正确，应该贴合人物头像位置

- 聊天界面用户不显示头像，这是不正确的

# Phase2 bot service & other basic function

## 2.1
现在请添加一下功能，注意每个功能都应该有相应前端页面和按钮，涉及到本地保存的部分请用minio+fileservice，并把文件保存在项目根目录里特定的文件夹下（新建），涉及到bot和aillm部分请用我在bot服务写好的轮子对其进行封装即可和重构，写完后请为我生成拓展后的APIdoc：

- 群聊管理

- [x] 创建/注销

- [ ] 转让群主

- [ ] 群公告

- [ ] 置顶

- 群成员管理

- [x] 成员校验

- [x] 邀请/踢出

- [ ] 禁言

- [ ] 管理员

- [ ] 消息发送（图片、文件、语音）

file-service

- [ ] 保存多媒体消息（图片、文件、语音）

- [ ] 传输多媒体消息

agent-manager-service

bot类型

- [ ] 内部 Agent

- [ ] 自部署 Agent

- [ ] 计费管理

- [ ] 配置管理

- [ ] 路由管理

我的minio的docker信息：

minio

c68e84d8f78a

minio/minio:latest

9000:9000

9001:9001

## 2.2
- 修改个人信息界面头像没居中

- 添加不存在的用户为好友也能成功并创建聊天回话，这是非法的

- 上传附件时显示上传文件失败

- 最好统一输出失败信息日志以方便排查错误

- bot的计费管理、路由管理、配置管理都没有相应功能和前端页面

- 当用户创建一个未注册用户为好友时能成功，并且用户注册并占据用户ID后显示出非法创建者为好友，这是不合理的

- 好友的在线状态不正常，现在登出后可以正常显示离线，但是重新登录后仍然显示离线

- 在前端页面的二级输入框中，我鼠标一滑动窗口就会自己关闭

- bot部分不正常，内部的bot怎么能让用户自己配置apikey和baseurl呢？直接调用我之前写好的agent就行了，只有自部署的bot才要用户自己提供相关信息

- 对话失败: 对话失败: [NodeRunError] failed to create chat completion: invalid character '<' looking for beginning of value / node path: [node_1, ChatModel]

- bot对话界面不应该放在会话中，应该视为一个单独会话或者用户

- 群公告应在会话的最顶端常态显示（用户可以自己关掉）

- 群聊管理应该放在会话右上角的三点里

- 群聊可以邀请不存在的用户进群

- 群聊禁言功能未正常生效

- 可以和已删除用户对话，这是错误的

- 可以非法创建不合理的会话，如创建一个不存在的用户会话，这是不合理的

- 气泡位置不正确，应该贴合人物头像位置

- 聊天界面用户不显示头像，这是不正确的

- 根据新增功能和接口扩写TechArch.md文件

- .env.example 未更新

- readme的架构图未更新

## 2.3
- 修改个人信息界面，头像显示区域应该在提示框中部

- 不应该能成功创建人数小于等于2两人的群聊

- 现在好友列表里能成功显示用户头像了，但是在聊天界面中不能正常显示用户头像（聊天气泡旁）

- 用户上传文件时，显示文件上传失败，但是日志显示成功上传到minio里了，并没看到错误日志，请把错误日志一起收集到一个地方

- 上传的文件保存在storage的source里

- 聊天会话界面有问题，不同会话应该有不同前端页面，现在全部都在一个前端页面里

- 有时后注册的用户添加好友后显示好友id而非昵称

- 前端不能切换会话，并且由于这个原因所有跟会话相关的内容都有问题，包括群公告

- 前端不能置顶会话

- 通过右上角三点管理群聊时显示无法获取群信息

- agent其他配置：
> SESSION_DIR="./storage/agent/sessionStore"  会话历史保存位置
>
> AGENT_ROOT="." agent操作根目录
>
> COZELOOP_API_TOKEN="sat_aZRCmtRHG147ECDECWc17tcyv4yE0ZzJE9lmAkNCvBCdW6xIixFxkeVj42Xw7ntf"
>
> COZELOOP_WORKSPACE_ID="7629700170703552518"
>
> SKILLS_DIR="./skills"  技能存储位置

- 在运行各个服务时先检查各个模块是否正常，并输出日志

- Agent 路由管理页面和配置管理页面又丑又不好用

- Agent 创建不应该在按钮处，应该在侧面菜单，并且可以直接选择以 Agent 为对象创建新会话

- agent-manager-service 遇到错误时前端不会返回错误信息

- 业务描述应统一为 Agent

- Agent 输出错误：[trace] ChatModel/ChatModel error: failed to create chat completion: error, status code: 404, status: 404 Not Found, message: , body: {"timestamp":"2026-05-13T11:19:46.456+00:00","status":404,"error":"Not Found","path":"/v4/v1/chat/completions"}
> 这时我给的环境变量是
>
> LM_DEFAULT_API_KEY=3dce3f52b18249a3a25d25c8be3e77fd.vYBHUh7vdMsFmBPO
>
> LLM_DEFAULT_BASE_URL=https://open.bigmodel.cn/api/paas/v4
>
> LLM_DEFAULT_MODEL=GLM-4.7

## 2.4 Now Codex

### 还是Codex和gpt5.5好用太多了，比Trae+glm5.1好用十倍有人懂吗，虽然装了一堆skill，token消耗更多了qwq

- 会话切换还是有问题，会话范围后端应该没问题，能够正常收发消息，问题是前端无法正常显示不同会话的消息，会把所有消息杂糅在一个会话里

- 群聊成员界面无法正常显示用户头像

- 用户点击进入群聊后，除了正常的群聊会话列表外，会错误地生成两个额外的无群聊名的会话列表

- 群聊列表前端有很大问题

- 转让群主后，新群主或管理员不能修改群聊信息

- 无法点击会话列表切换会话，但是 Agent 对话部分没有这个问题，请按照 Agent 部分写

- 由于会话部分有过多问题，导致前端无法正常显示会话列表，只能通过点击会话列表切换会话，但是会话列表会显示所有会话，包括群聊会话，导致用户无法正常切换会话

- 群公告不显示

- 无法禁言他人

- 私聊时会话列表应为对方用户头像

- 切换 Agent 会话后，再次进入用户会话会导致显示的消息记录消失

- Agent 正常对话没有问题，但是我之前写 Agent 时有写过 Agent 想请求某些工具时会要求用户在命令行审批的功能（toolcall），请你做相应的前端接口，或者对该功能进行修改
> 详情：finish_reason: tool_calls
>
> usage: &{7473 {7466} 60 7533 {47}} map[] map[] [call_-7636677595769070032]}, LayerSpecificPayload=<nil>, SubsLen=1
>
> [trace] Graph/ReAct error: interrupt happened, info: &{State:0xc001a23c70 BeforeNodes:[] AfterNodes:[] RerunNodes:[ToolNode] RerunNodesExtra:map[ToolNode:0xc001a921e0] SubGraphs:map[] InterruptContexts:[]}

- Agent 对话部分，当 Agent 在思考时切换会话，Agent 思考完后会错误地在其他会话里输出内容

- 检查一下，现在的 Agent 好像没有记忆功能，我之前写过 session 的记忆功能，请在不同 Agent 中实现对应记忆的功能，注意记忆不要弄混了，一个 agent_id 对应一个 session_id 的记忆

- 不同的 Agent 应该对应不同的 prompt 或 skill 等，如有必要请重构 storage/agent 中的文件，并完善不同 Agent 的个性化功能

## 2.5 

- 能够发送图片和文件，但是图片不能正常预览在会话里（显示图片加载失败），文件不能下载

- 需要能预览图片（会话）和查看完整图片（直接点击图片）

- 私聊也应该能置顶

- 需要能修改好友信息

- 创建群聊时会创建两个相同的会话列表

- 不能接受群成员群聊消息

- 创建群聊回话会出现三个异常会话列表

- 禁言功能正常，但是被禁言的人点击发送按钮时应该被阻止，现在是能发送但拦截

- 聊天时需要点一下会话才能显示新出现的消息，不对

- 不能正常踢人，现在被踢掉的人能够正常发消息，而且发消息后又会出现在群成员里，此时踢掉他会显示不在群里

- 设置群公告的人看不到群公告

- 与bot对话时显示：❌ 对话失败: 对话失败: [NodeRunError] failed to create chat completion: Post "https://open.bigmodel.cn/api/paas/v4/chat/completions": dial tcp 129.227.65.212:443: connectex: A connection attempt failed because the connected party did not properly respond after a period of time, or established connection failed because connected host has failed to respond. ------------------------ node path: [node_1, ChatModel]

- 我想加上管理员可上传群头像功能

- 我想把创建的ai也作为一个user对象，可以正常私聊，或者加好友和拉进群聊天，且同一个bot实例对不同的用户和好友应该是独立的

- 还有很多其他bug，请自查

## 2.6 
现在有两个任务：

1-生成一份技术文档，要求包含项目详细解析，技术实现，架构设计讲解，分布式架构下不同表和数据怎么传递信息，以及未来可以添加的新功能的规划（readme中有），注意围绕AIM这个核心（AI+IM，IM是聊天室）

2-为项目代码添加详细注释

3-聊天可发送语音消息，就像常规的聊天软件一样，长按录音，松手发送，可以在会话里直接播放

4-优化前端UI，使其更美观

## 2.7
我们必须深入探讨一下IM中AI的生态位，既然你说是让 AI 能在真实 IM 会话上下文里工作，那么我们就需要把AI嵌入真实会话并执行相应工作中，例如AI总结群聊消息，AI根据群聊消息总结RAG知识库，AI点评用户发言习惯，以及更多能在IM系统里展示Agent特有能力的功能。与此同时，AIM中 的A也不仅仅是AI，更是Agent，由此而言，AIM就是在IM中塞入Agent智能体辅助用户日常工作与生活，配置Tool、skill等，更重要的事我们需要在Agent盛行的当下探讨Agent在IM系统中的重要程度。诚然，这些部分时未来plan，请你深入思考并在readme中开一条标题FuturePlan总结

现在，我们对aim有了更为深刻的认知，现在我们该分阶段为该项目实现哪些功能内容呢？请从readme的FuturePlan和开发阶段，以及docs/APIdoc_plan.md总结，并梳理在Readme最底部的Plan中

现在请把项目内所有有关plan和未来规划的内容都像这样总结起来，并放在docs里的plan.md里

## 2.8 
我又测试了一遍，发现了一些新的问题，在readme的to fix list中，请修复，另外请考虑future to fix 的项目是否合理，不必针对future to fix的建议对项目进行修改，只需要考虑这样做需要涉及到的模块的影响，对项目的影响程度有多大，这样做是否合理即可


- 发送语音后不能正常收听语音
- 不能正常下载文件：{"code":401,"message":"缺少认证信息","data":null}
- 无法预览图片：[图片加载失败]
- 可以错误地在已解散的群聊中发送消息
- 上传文件有时候会出现fail to fetch的错误信息
- 当用户改变个人信息后，历史消息中的用户也要改变信息（头像和昵称）
- 修改的备注同理
- 不正确的是，需要重新登录才能看到好友修改过的个人信息
- 非管理员和群主成员点击群管理没反应，要求要有反馈
- 非群主显示的群聊名称不正确
- 群公告发起者不能查看群公告
- 被禁言的人虽然不能发送消息了，但是在本人前端页面显示发送了消息的弹窗
- 需要能删除会话列表的功能，并且删除会话列表后前端显示的历史消息应该清空，但是能搜索历史消息
- 不应该能在已解散的群聊中发送讯息
- 切换会话后，之前会话中正在思考的机器人会被错误地停止
- 计费记录中不能正常计费token，但是能计费金钱，请修复，另外，这个金钱-token的汇率是怎么界定的？

future to fix

- 请把bot更改作为用户实例，创建后可以正常加好友（有特殊id）和邀请群聊。可以在私聊/群聊里对话，记忆机制等就像现在这样就行，也就是说不同机器人的记忆独立，对不同用户的记忆独立，但是同一机器人在不同会话中对同一用户的记忆保持

## 2.9
补全phase1&2和高级im功能

我们注意到，在常规的im应用（如qq）中，接收人接受消息后，消息会保存在接收人本地系统里，此时接收人对消息的操作是针对本地保存的消息历史还是服务器上的消息数据？这样的话服务器有必要保存消息数据吗？

可以，记得给我的项目加上详细注释。再给我项目中代码加上详细的注释，包括api层，路由层，rpc层，业务实现具体逻辑等，注意这个注释不局限在本次生成的代码中，而是全部代码

## 2.10
- 无法加载图片 3553476455427156989.png 图片加载失败
- 无法下载文件或语音

> This XML file does not appear to have any style information associated with it. The document tree is shown below.
> <Error>
>
> <Code>AccessDenied</Code>
>
> <Message>Access Denied.</Message>
>
> <Key>file/f4c5293c-ed11-4e29-adf1-23bbe74433ba.pdf</Key>
>
> <BucketName>claran-aim</BucketName>
>
> <Resource>/claran-aim/file/f4c5293c-ed11-4e29-adf1-23bbe74433ba.pdf</Resource>
>
> <RequestId>18B009CEA4A34591</RequestId>
>
> <HostId>dd9025bab4ad464b049177c95eb6ebf374d3b3fd1af9251148b658df7ac2e3e8</HostId>
>
> </Error>
- 我严重怀疑这个项目中某些功能会占用我的C盘空间，我的C盘空间今天又少了5G，请全面检查
- 无法回复消息，显示“回复消息不存在”
- 无法编辑消息，显示“回复消息不存在”
- 无法撤回消息，显示“回复消息不存在”（根因是雪花 ID 超过 JS 安全整数范围，浏览器解析后丢精度，导致后端查不到消息）
- 没有已读回执等功能
- 请全面重构前端，从功能到UI，请全面按照APIdoc和项目接口和业务需求重写所有前端

## 2.11 
- 文件上传下载预览部分完全好了，后面就不要修改了
- 点击回复按钮没反应
- 编辑消息不应该用浏览器的弹窗
- 编辑消息后消息气泡异常
- 现在不能正常创建回话了！
- 请把目前已注册的用户的用户ID改为10位
- 已读未读功能好像不对，消息已读状态由接收消息者决定，群聊中则改为可由多人“已读”，像飞书一样记录已读人数，现在已读有问题
- 真的有广播消息的功能吗？已读回执呢？已读回执是什么？

## 2.12
请思考如何在该项目中集成消息队列

是的，我想加入消息队列就是为了当作服务间通信方法之一，我注意到很多方法涉及到不同服务间的通信，但是之前确忽略了这一点，反而把这些方法放到了api-gateway层来实现服务间通信，我认为这并非良好的措施，我觉得可以用消息队列来重构这些涉及不同服务通信的方法，请全面重构，注意合理消息队列选型，建议Kafka
我的kafkadocker信息：
name：kafka 
bitnamilegacy/kafka:3.3.2-debian-11-r11
9092:9092
9093:9093

把Kafka 配置默认开启，并扫描全项目找出存在的bug和集成消息队列后遗留的问题和bug，并新增accesstoken和refreshtoken功能，并在token里加上role，区分管理员和用户，并把设计管理层的接口使用role鉴权，虽然目前应该并没有管理层接口

再次扫描文档，寻找可能存在的bug，并思考一些深度问题：例如redis 扣减完了，但是 kafka 还没有推送的间隙，服务器挂了或者重启了咋办？或者说 kafka 丢消息了，wal 写到哪里？

另外，请写一份详细的技术文档，考虑在这个项目里集成分布式事务的可能性，用dtm框架，再加上限流和熔断

# Phase 3

## 3.1
现在请实现plan里phase2的功能，也就是集成agent并把agent用户化，记得是agent而不是ai chatbot，要有agent的长会话能力和工作能力，具体实现请参考我的bot-manage里面的代码，这是我最开始在一个agent项目里写的一个基础agent，有长会话和调用工具能力，也有websearch和rag能力

是，实施此计划，另外，记得在合理的合适的地方使用消息队列或dtm等技术栈。另外，agent创建者可以更改bot配置，包括作为用户的昵称，头像和apikey等，agent创建者也能给其他人更改权限，就像群聊的群主和管理员一样。，并且记得给所有接口都适配前端功能。可以适当仿照我的tool和graphtool方法写一些其他tool，比如写代码啥的。注意文件操作tool我已经带上了，不过需要agent有工作目录和权限，你可以看看eino如何实现这些功能

## 3.2
- 分组功能没用，可能是前端问题，分了组不能把用户修改组别，也不会显示组别，甚至可以重复分组
- @人后显示的应该只有昵称，而不是昵称+uid
- 给我解释一下agent那里，编译agent有啥用？agent级别又是什么？角色是什么？工具策略呢？
- agent运行的地方不要显示什么会话ID，我们不知道那是什么，显示会话标题！、
- agent不能运行：service discovery error: no instance remains for agent-runtime-service
- 侧边会话栏，agent信息显示不全
- 用户不应该能@自己
- 添加快捷@全体人按钮

## 3.3
有一些部分我认为有问题，就在agent总结会话那部分，系统提示词不应该让agent生成json来恢复用户，且我让他总结群聊回话，她并没有实际进行对应思考和总结，甚至没看到会话。应该完善这部分能力，也就是会话感知能力

请修改一下agent的提示词，现在当可供分析的数据过少时，agent会表示因为无有效信息而拒绝分析或总结，理想状态应该是agent总结告诉我这些会话是废话

另外，现在的agent缺少交互能力，无法多次长对话或者有类似agent-ask - user-access - agent-act的效果

agent表示当前操作目录为空：2. **目录路径可能不正确** - 路径 `D:\CodeStudy\GoProjects\src\ClaranAIM\49895688258818048` 可能不存在

根据我提到的所有问题特征，全面修复相关问题，以及类似问题

- 前端agent侧边栏包不住agent功能按钮了，需要改正，建议把这些功能按钮全都整合到二级菜单中，统合为“管理”“运行”等按钮
- agent上下文会话列表感知不全面，只能感知最近打开或者缓存过的的会话列表
- agent应该是可以连续对话的，所以在agent运行界面前端应该是输入栏在最下面，上面是历史消息记录，如codex

## 3.4

完成Phase3：

Phase 3：Agent-Native IM 原生化重构

目标：把 Agent 从“被用户点击按钮调用的 Bot”重构为 IM 的原生成员。Agent 不应只等待 HTTP `/agent/run`，而应订阅消息、文件、群事件、系统通知、任务变化等 IM 事件，在权限允许时理解上下文、选择沉默或行动、调用工具、生成结构化结果并以真实用户身份回到会话。

核心原则：

- Message as Event：IM 每一条消息、引用、@、表情反应、文件上传、语音转写、群成员变化和系统通知都应进入统一事件模型。
- Context Awareness：Agent 处理事件前必须能看到当前会话、参与者、引用关系、历史窗口、附件、群角色、任务状态、知识库命中和权限边界。
- Agent as User：Agent 使用真实系统用户身份参与私聊和群聊，可被 @、可入群、可被加好友、可发消息，但所有主动行为必须可配置、可审计、可关闭。
- Work not Chat：Agent 的差异不在“能不能聊”，而在“能不能替用户工作”：查 RAG 并带来源、整理 Git/工单/会议纪要、安排日程、提取待办并 @负责人、识别截图报错并给解决方案。
- Event First, HTTP Second：HTTP 接口保留为人工入口和管理入口，Agent 原生入口应是 Kafka/Outbox 事件订阅与异步任务。

重构任务：

- [ ] 新增统一 `im_events` 事件契约，覆盖 `message.created`、`message.edited`、`message.recalled`、`reaction.added`、`file.uploaded`、`voice.transcribed`、`group.member_joined`、`group.member_left`、`system.notice`、`task.changed`。
- [ ] 事件 payload 必须携带 `conversation_id`、`conversation_type`、`sender_id`、`participant_ids`、`mention_user_ids`、`reply_to_id`、附件引用、权限上下文、发生时间和幂等键。
- [ ] 新增 Agent Subscription / Rule 表：配置某个 Agent 订阅哪些会话、群、事件类型、关键词、命令、@规则、静默规则和主动响应策略。
- [ ] 将 agent-manager 的 @Agent consumer 重构为 Agent Event Dispatcher：消费统一 IM 事件，查订阅规则，判断触发意图，写入 dispatch 事实表，再调用 runtime。
- [ ] Dispatcher 必须支持三种决策：忽略事件、记录/入库、触发 Agent 执行；不能每条消息都刷屏回复。
- [ ] 私聊 Agent 默认触发，群聊 Agent 默认仅 @、命令或规则触发；群助手可配置定时摘要、关键风险提醒和静默知识入库。
- [ ] Agent 执行前由 Context Builder 统一构建上下文包，包含最近消息、时间窗口、引用链、附件摘要、群成员角色、相关记忆、RAG 召回、任务状态和权限说明。
- [ ] Context Builder 必须使用当前 Agent 用户和触发用户的可见性做权限裁剪，禁止把用户无权查看的消息、文件、知识库片段注入上下文。
- [ ] Agent 输出统一进入 Action Decision：纯文本回复、结构化卡片、工具调用、知识候选、任务候选、静默记录、等待用户确认。
- [ ] Agent 回复通过 msg-core-service 以 Agent 系统用户身份落库，保留 `reply_to_id`、`client_msg_id`、`source_event_id`、`agent_trace_id` 和工具调用审计引用。
- [ ] 前端会话页增加 Agent 原生状态：思考中、已完成、等待确认、静默记录、失败、被策略拦截。
- [ ] 前端增加 Agent Context Sidebar：展示当前会话摘要、相关知识、待办、风险、引用来源、正在运行的 Agent 和可执行动作。
- [ ] 文件即入口：图片、PDF、Word、Markdown、语音消息进入事件流水线，Agent 可识别、解析、入库或回答，并向用户说明来源和权限。
- [ ] 群聊隐形助理：支持群聊日/周摘要、共识总结、待办提取、错误文档提醒、风险提示，但默认低打扰、可关闭。
- [ ] 建立 Agent 行为审计：每次事件触发、上下文读取、RAG 命中、工具调用、消息回复和静默决策都必须有可追踪记录。
- [ ] 建立 Agent 原生验收脚本：私聊 Agent 普通消息响应、群聊 @Agent 带群上下文、Agent 静默入库、文件上传触发解析、任务卡片确认、重复 Kafka 投递幂等。

需要影响的模块：

- `pkg/events`：从消息推送事件扩展为 IM 统一事件契约。
- `msg-core-service`：继续作为消息事实源，同时为 Agent 提供可见性裁剪后的上下文读取。
- `agent-manager-service`：从 Agent 管理 + @消费器升级为 Agent 订阅、路由、权限、审计和 dispatch 管理面。
- `agent-runtime-service`：从同步执行接口升级为事件上下文执行器，支持异步运行、心跳、取消、checkpoint 和结构化输出。
- `rag-service / memory-service`：承接消息、文件、总结和知识候选的沉淀。
- `websocket-gateway / frontend`：展示 Agent 原生状态、上下文侧边栏、Action Card 和审批流。

阶段边界：本阶段重点是“Agent 原生事件架构”和“上下文感知执行链路”，不是完整多 Agent 编排。多 Agent Leader、子 Agent 阻塞等待和协作验收继续放在 Phase 7。
- [ ] 实现未读摘要。
- [ ] 实现“我错过了什么”会话摘要。

- [ ] 将 Agent @ 响应扩展为通用事件订阅：消息、文件、群成员变化、系统消息、任务变化均可触发 Agent 判断是否响应。
- [ ] 为 Agent 主动行为补权限边界：主动私聊、拉群、@用户、创建任务、写文件都必须有来源、授权和审计记录。
- [ ] 支持创建者邀请 Agent 入群，Agent 作为普通群成员展示，但所有主动行为附带 Agent 标记和审计来源。
- [ ] 支持 Agent 主动发起私聊或会话邀请，但必须经过创建者授权、频率限制和用户可见的操作记录。
- [ ] 完善 Agent 在线状态：区分可用、运行中、等待确认、停用和配置异常。

# Phase 4：

## 4.1
补全Phase4前的其他功能，并做phase4：

Phase 4：Agent 记忆与用户/群画像

- [x] 实现基础长会话记忆：runtime 使用 session key 与 JSONL 持久化，支持跨重启恢复基本上下文。
- [ ] 实现 memory-service。
- [ ] 在不引入向量库的前提下先实现基础事实记忆表：用户偏好、群背景、项目状态、Agent 运行摘要。
- [ ] 建立用户记忆：偏好、常用表达、长期目标、历史交互模式。
- [ ] 建立群记忆：群背景、项目状态、关键成员角色、长期讨论主题。
- [ ] 实现聊天记忆，覆盖私聊和群聊。
- [ ] 实现跨会话用户个体记忆。
- [ ] 实现用户发言习惯分析。
- [ ] 将用户发言习惯分析默认设置为仅本人可见。
- [ ] 支持用户查看、编辑、删除和关闭自己的记忆。
- [ ] 支持向量化记忆，为跨会话召回和个性化 Agent 提供基础。
- [ ] 保证不同 Agent、不同 Session 的记忆隔离，避免记忆串线。

## 4.2 
继续补全功能，rag向量数据库什么的不用做，另外，补全翻译功能，用户可以在设置里配置翻译llm的api，使用配置的llm进行实现翻译功能，也可以使用服务自带的llm，翻译默认为其他语言转中文，用户可以在设置里配置翻译功能prompt，类似地可以配置其他prompt

我建议把翻译功能整个做进msg服务里，并且把用户配置功能新建为一个“系统设置”服务，可以配置包括我提到的翻译功能配置和其他地方的配置

自动翻译不用做，然后用户可以在该服务里预设各种llm的url和api等信息，并在创建agent的时候使用这些包装好的信息，这样就需要修改bot那一部分。另外，请把项目内所有的bot相关service改名为agent

## 4.3
为什么现在的网关分为bot/和agent/？这两个组别的接口分别对应什么级别的功能？

既然你做的是分布式的微服务，那微服务间怎么能相互调用包呢？现在的api-gateway会直接调用memory-service，msg和setting的包来初始化数据库和初始化服务，微服务不该是这样，同理，各个微服务间不应该能相互调用，亲全面排查该问题并修改

我会为你开放全部权限，请将bot相关service改名为agent。对于旧的bot兼容层，请迁移到agent下，并删除旧的不用的接口

给项目的所有没有注释或注释是英文的代码都加上详细的中文注释

把我项目里的tmp站位文件全部删除

寻找项目里的潜在bug

## 4.4
- agent说话时会连续输出两次内容
- agent聊天框无法渲染md的表格
- 最小 Action Card 渲染协议疑似未能生效，我刚刚并没有看见
- 现在agent为我生成的代码文件仍然在根目录，我需要在/agent/files中
- 支持配置agent工作目录
- 我希望好友界面的分组也能像会话界面一样有分类下拉列表

## 4.5
我需要可以让用户上传skill.md或文件夹来配置skill的功能，并且写到settingservice里，注意可以注入为全局skill或者单个agent的skill

我注意到很多注释都比较水，例如很多包内部使用的方法，用于拆分主流程中的局部业务步骤，都是没有描述实现细节的注释

在配置的地方，应该还要有更多的可配置项，例如agent读取的会话上下文数量

重写D:\CodeStudy\GoProjects\src\ClaranAIM\internal\agent-manager-service\agent\prompt.go的相关Prompt，目前的prompt是阿米娅，因为agent的核心代码是我从另一个项目里复制过来的。现在你要全面重写相关提示词，让agent的人设编委ClaranAIM中的会话助手和工具人，并重写相关tool，之前的tool全是明日方舟相关的东西，现在写成一个标准的会话助手和工具人适配的tool。另外，其他与明日方舟相关的特色功能都要改成通用功能。好，还有在配置的地方，应该还要有更多的可配置项，例如agent读取的会话上下文数量

## 4.6
- 记忆管理的前端页面不好看，可以不做成卡片
- 触发规则设置界面同理
- 当用户多次向同一agent触发（且agent正在思考），agent会等待且正常工作，但前端不显示思考中的消息气泡。
- skill应该有单独的配置界面，不要和其他配置在一个界面，并且skill上传后应该能提取并显示该skill摘要，并且可以直接在前端修改skill内容
- 给agent写skill-creator Tool
- agent无法正常识别skill
- agent完成任务的横幅条会错误地显示在其他页面
- 添加LLM预设也应该可以选择内置的agent
- 请检查用户能否自己配置agent，也就是自己用自己的apikey和baseurl

# Phase 5 RAG
## 5.1
现在我想集成milvus做完整的RAG功能，做到rag-service，要包含以下内容：
- 完整的rag基础功能
- Hybrid Search 替代纯向量搜索
- 层级索引（大范围召回-精确搜索-根据质量决胜）
- Reranking ，做法是在检索和生成之间加一个精排步骤：用 Reranker 模型给每对 (query, doc) 重新打分
- Corrective RAG（CRAG）CRAG 就是 把搜到的资料过滤一遍
在检索和生成之间插一个质检员，逐个审查检索到的文档是否和问题相关，然后根据审查结果走不同的分支
打分高的，说明资料靠谱，直接喂给大模型生成答案
打分低的，说明内部知识库里压根没找着相关内容，干脆回退到 Web 搜索兜底
打分模糊的，就两边的结果合一起送进去
- Self-RAG 的思路是在整个流程中设置四个检查点，每一步都让模型自我审视：

这个问题需要检索吗（Retrieve）？
检索到的文档相关吗（IsRel）？
我的回答有文档支撑吗（IsSup）？
这个答案对用户有用吗（IsUse）？
- CRAG 和 Self-RAG 都在给 RAG 加流程来解决问题，但这样多了好几次 LLM 调用，增加了成本，Adaptive RAG 在最前面加了一个路由器（分类器），先判断问题的复杂度，然后决定走哪条路线
- 最重要的：GraphRAG，知识图谱：先把文档变成知识图谱，再基于图谱来检索和推理：用 LLM 逐篇读文档，抽取里面的实体（人、部门、产品等）和关系，构建成一张图谱
用 Leiden 算法对图谱做社区划分，把关联紧密的实体聚成一团，然后让 LLM 为每个社区生成一段摘要
提问时先定位到相关实体，沿着关系拿到子图
- Text-to-SQL RAG
如果我们的数据本身就是结构化的表格，比如销售数据、用户行为日志、财务报表这些，就不合适传统的 RAG 了，因为对表格数据做 embedding 是非常低效的。

用户问：“上个月销售额最高的产品是哪个？”

这本质上就是一条 SQL，向量搜索对这种聚合、排序、筛选类的需求完全没招。

Text-to-SQL RAG 的做法是让 LLM 直接把自然语言翻译成 SQL，执行查询，再把查询结果作为上下文来回答
- 前面每种方法都有自己擅长的场景：Hybrid Search 擅长术语密集的文档，GraphRAG 擅长多跳推理，Text-to-SQL 擅长结构化数据。

但在一个真实的系统中，这些场景可能同时存在.Agentic RAG 的做法是让一个 AI Agent 来自动调度，根据问题自主决定每一步该怎么做。给这个 Agent 配备一组检索工具，它会先搜搜看 → 看看结果够不够 → 不够就换个方式 / 换个关键词再搜 → 结果够了就生成回答

请选择合适的rag方法完善rag功能，但是必须要有graphrag

另外，既然要做graphrag，那也要做用户可视化的知识图谱功能，前端要展示rag的知识图谱

我的milvus镜像由本地目录的./deployment/docker/milvus构成，你可以在这里查询milvus镜像数据

做完所有内容后记得为我生成工作文档，在update下

## 5.2
再此检查当前rag功能，用户可以上传txt，pdf等文件构建知识库或者知识图谱，要确保后端能正确解析切片这些不同类型的文档。另外，我新建了一个apikey用来当embedding模型，(apikey省略)，记得放到env里，这个apikey对应的是我买的glm embedding3 的资源包，如图所示，url是https://open.bigmodel.cn/api/paas/v4/embeddings，curl示例为**********

有关Milvus 部分，你说版本不对，那么你可以降低Milvus SDK本地缓存的版本，不用我规定的版本。另外，在self-rag部分怎么做的？我希望用llm-router小模型来判断是否需要rag

小模型用.env里面现在正在用的llm的apikey，并在配置部分做用户可自由配置小模型/默认使用项目内置小模型，并再此检查rag部分是否可用

## 5.3
当前的hybrid search是怎么做的？我期望的是dense+BM25，并用RRF，Reciprocal Rank Fusion方式合并二者结果

我现在的分层索引和大小chunk是怎么做的？我期望的是大小chunk分层，检索时用小 chunk，精准回答时用大 chunk，完整 这比直接用大 chunk 检索更好。只检索小 chunk，返回父 chunk

这是最常见、最推荐的。

检索单位：小 chunk
返回上下文：父 chunk 或相邻小 chunk
流程：

query
  -> search level=child
  -> top child chunks
  -> group by parent_chunk_id
  -> 取 parent summery 内容 
  -> 放进 prompt

在大chunk分出是，生成对应的 summary（用小模型）。比如 parent chunk 很长，不适合直接塞 prompt，你可以提前生成parent_summary

另外，针对不同的文档，我期望的分片方式是：
Markdown 文档

按标题切：

 一级标题 -> document
 二级标题 -> parent chunk
 三级标题/段落 -> child chunk
PDF / Word

先解析成结构化文本，再按标题/段落切。标题识别不准时，按段落 + token 长度切。

代码文件

不要按固定字数切。按结构切：

package/import 简要
type struct
interface
function
method
关键注释
例如 Go：

func SendMessageExt(...)
应该作为一个 chunk，别从函数中间切开。

如果函数太长：

按逻辑块切：校验、事务写入、缓存失效、事件发布
聊天记录

按时间窗口和话题切：

连续 10-30 条消息一个 parent
单条/少量消息或摘要一个 child
聊天记录更适合先做摘要，再向量化摘要。

## 5.4
当前的rerank是怎么做的？我期望的是Model rerank: 用模型读 query + chunk 后重新打相关性分数。Hybrid Search + RRF
  -> 得到 top 30
  -> Model Reranker
  -> 得到 top 5
我会给你rerank模型的url：https://open.bigmodel.cn/api/paas/v4/rerank，密钥就用f28---------------u就行，"model": "rerank"，记得放到env里

整合文档解析功能，我会给你GLM-OCR

curl --request POST \ 
  --url https://open.bigmodel.cn/api/paas/v4/layout_parsing \
  --header 'Authorization: Bearer <token>' \
  --header 'Content-Type: application/json' \
  --data '
{
  "model": "glm-ocr",
  "file": "https://cdn.bigmodel.cn/static/logo/introduction.png"
}
'

apikey：d3--------------------------------------

## 5.5
现在项目里的CRAG是怎么实现的？我期望的是
一个好的 CRAG evaluator 不只问“相关吗”，而是问四个东西：

1. Relevance：资料是否和问题相关？
2. Coverage：资料是否覆盖了问题需要的关键点？
3. Specificity：资料是否足够具体，而不是泛泛相关？
4. Conflict：资料之间是否互相矛盾？

用户问题
  -> Router 判断需要 RAG
  -> Milvus Hybrid Search
  -> Rerank
  -> CRAG Evaluator
       ├─ correct   -> 内部资料回答
       ├─ incorrect -> Web/询问用户搜索兜底
       └─ ambiguous -> 内部 + 外部合并
  -> LLM 生成

在CRAG中，llm可以用来：判断检索结果是否相关evaluator
{
  "label": "ambiguous",
  "score": 0.56,
  "reason": "资料提到了 Agent 调度，但没有解释 event_id 和 agent_user_id 的业务含义"
}

CRAG的LLM用复用小模型即可

## 5.6
当前的self-rag是怎么实现的？我期望的是
Self-RAG 的四个典型检查点

你之前提到的四个点很关键：

Retrieve：这个问题需要检索吗？
IsRel：检索到的文档相关吗？
IsSup：我的回答有文档支撑吗？
IsUse：这个答案对用户有用吗？

Self-RAG 不是让 LLM 自己随便跑工具

这点很重要。

不要让 LLM 自己无限决定：

我要查 Milvus
我要查 Web
我要查数据库
我要跳过权限
正确做法是：

LLM 输出结构化判断
应用代码执行工具调用
例如：

{
  "need_retrieve": true,
  "retrieval_source": "project_docs",
  "query": "agent_dispatch_records event_id agent_user_id"
}
然后你的代码决定：

if decision.NeedRetrieve {
    chunks := ragService.Search(ctx, decision.Query, userScope)
}
LLM 是“判断器”，不是“权限执行者”。
## 5.7
当前的GraphRAG是怎么实现的？我期望的是：
图里有两种核心东西：

实体 Entity
关系 Relationship

GraphRAG 一般分两条链路：

离线构建 indexing
在线查询 query

对每个 chunk 调 LLM，让它抽取实体。

Prompt 大概是：

请从下面文本中抽取实体。
实体类型包括：
- Service
- DatabaseTable
- EventTopic
- API
- Module
- Concept
- Person
- Organization
- Product

返回 JSON：
{
  "entities": [
    {
      "name": "...",
      "type": "...",
      "description": "...",
      "aliases": []
    }
  ]
}
示例输出：

{
  "entities": [
    {
      "name": "msg-core-service",
      "type": "Service",
      "description": "消息核心服务，负责会话、消息写入、已读游标和 Outbox",
      "aliases": ["消息核心服务"]
    },
    {
      "name": "event_outbox",
      "type": "DatabaseTable",
      "description": "事务 Outbox 表，用于保存待发布事件",
      "aliases": []
    }
  ]
}

关系抽取

继续让 LLM 抽关系。

Prompt 类似：

基于文本和已抽取实体，抽取实体之间的关系。
关系类型包括：
- CALLS
- PUBLISHES
- CONSUMES
- STORES
- OWNS
- DEPENDS_ON
- CONFIGURES
- TRIGGERS
- READS
- WRITES

返回 JSON：
{
  "relationships": [
    {
      "source": "...",
      "target": "...",
      "type": "...",
      "description": "...",
      "evidence": "原文证据"
    }
  ]
}
示例：

{
  "relationships": [
    {
      "source": "msg-core-service",
      "target": "event_outbox",
      "type": "WRITES",
      "description": "msg-core-service 在消息事务中写入 event_outbox",
      "evidence": "发送消息时同事务写入 messages、message_user_states 和 event_outbox"
    },
    {
      "source": "websocket-gateway",
      "target": "claran.message.events",
      "type": "CONSUMES",
      "description": "websocket-gateway 消费消息事件并推送在线用户",
      "evidence": "websocket-gateway 消费 claran.message.events"
    }
  ]
}
实体归一化

这是很重要的一步。

不同 chunk 可能抽出这些名字：

msg-core-service
消息核心服务
msg core service
MessageService
它们可能指同一个实体。

所以要做 entity resolution：

判断这些实体是不是同一个
合并 aliases
保留 canonical name
可以用：

规则：小写、去符号、别名表
embedding 相似度
LLM 判断
人工审核
最终得到：

canonical entity:
  name: msg-core-service
  aliases:
    - 消息核心服务
    - MessageService
如果不做这步，图谱会碎成一堆重复节点。
Claran.: 06-01 23:54:00
构建图

图数据库里保存：

Node: Entity
Edge: Relationship
可以用：

Neo4j
NebulaGraph
ArangoDB
PostgreSQL + AGE
MySQL 表 MVP
NetworkX 离线图
MVP 用 MySQL 也可以：

knowledge_entities
- id
- name
- type
- description
- aliases_json

knowledge_relationships
- id
- source_entity_id
- target_entity_id
- relation_type
- description
- evidence_chunk_id
- confidence
如果你想做图查询更方便，后期再上 Neo4j/NebulaGraph。

Claran.: 06-01 23:54:24
 Leiden 社区划分是什么

图构建出来后，节点和边会很多。直接拿全图回答问题不现实。

Leiden 算法的作用是：

把图里关联紧密的节点聚成社区 community
比如项目图谱里可能自动聚出：

社区 1：IM 消息链路
- msg-core-service
- messages
- message_user_states
- event_outbox
- claran.message.events
- websocket-gateway

社区 2：Agent 事件链路
- agent-manager-service
- agent-runtime-service
- agent_subscription_rules
- agent_dispatch_records
- claran.im.events

社区 3：Memory/RAG
- memory-service
- memory_facts
- Milvus
- embedding
- vector_status

社区 4：文件服务
- file-service
- MinIO
- file metadata
- OCR
Leiden 比 Louvain 更稳定一些，常用于社区发现。

你可以理解成：

图谱自动分章节

Claran.: 06-01 23:54:35
社区摘要是什么

社区划分后，每个社区可能有几十个实体和上百条关系。

不能每次查询都把整个社区塞给 LLM，所以要提前生成摘要：

community_id: 1
title: IM 消息链路
summary:
  本社区描述 ClaranAIM 的消息写入、事件发布和在线推送流程。
  msg-core-service 是消息事实源，负责写 messages、message_user_states 和 event_outbox。
  Outbox worker 将 message.created 发布到 claran.message.events。
  websocket-gateway 消费该 topic 并推送在线连接。
社区摘要可以分层：

低层社区：具体模块
中层社区：服务链路
高层社区：系统域
这就形成了：

实体
关系
社区
社区摘要

Claran.: 06-01 23:54:46
查询时怎么用图谱

在线查询大概是：

用户问题
  -> 实体识别
  -> 找相关实体
  -> 扩展子图
  -> 找相关社区摘要
  -> 找原文证据 chunks
  -> LLM 回答

Claran.: 06-01 23:54:58
步骤一：从问题中识别实体

用户问：

agent_dispatch_records(event_id, agent_user_id) 到底负责什么业务？
系统识别实体：

agent_dispatch_records
event_id
agent_user_id
Agent
Kafka
可以通过：

关键词匹配
BM25
实体别名表
LLM entity linker
embedding search
定位到图谱节点：

Entity: agent_dispatch_records

Claran.: 06-01 23:55:20
沿关系扩展子图

找到实体后，不只是拿这个节点，而是向外走 1-2 跳：

agent_dispatch_records
  --owned_by-->
agent-manager-service

agent_dispatch_records
  --deduplicates-->
Agent Event Dispatcher

Agent Event Dispatcher
  --consumes-->
claran.message.events

Agent Event Dispatcher
  --calls-->
agent-runtime-service

Agent Event Dispatcher
  --writes_reply_to-->
msg-core-service
得到一个小子图。

这个子图比普通 chunk 更结构化。

Claran.: 06-01 23:55:31
步骤三：找社区摘要

这个实体属于某个社区：

社区：Agent 事件调度链路
取社区摘要：

该社区描述 Agent 如何订阅 IM 事件、判断规则、记录调度幂等、调用 runtime 并写回消息。步骤四：找原文证据

图谱里的关系应该带 evidence：

relationship.evidence_chunk_id
所以还可以回源拿原文 chunk。

最终上下文可能是：

[社区摘要]
Agent 事件调度链路...

[子图关系]
agent-manager-service -> consumes -> claran.message.events
agent-manager-service -> writes -> agent_dispatch_records
agent_dispatch_records -> prevents_duplicate_dispatch -> Agent 回复

[原文证据]
internal/agent-manager-service/eventconsumer/agent_consumer.go ...
然后 LLM 回答。

Claran.: 06-01 23:55:42
GraphRAG 和普通 RAG 怎么结合

GraphRAG 不一定替代向量 RAG。更好的方式是混合：

Vector RAG:
  找语义相近片段

GraphRAG:
  找实体关系和社区摘要

Hybrid Search:
  找关键词/字段名

Rerank:
  重新排序

LLM:
  综合回答
查询时可以并行：

query
  -> vector search topK
  -> graph search subgraph
  -> community summary search
  -> merge context
  -> rerank / context selection
  -> answer
比如用户问：

为什么 memory-service 也要接入向量数据库？
Vector RAG 找到：

memory-service、vector_status、embedding_ref 相关 chunk
GraphRAG 找到：

memory-service
  -> stores -> memory_facts
  -> indexed_by -> Milvus
  -> used_by -> agent-manager-service
  -> injects_context_to -> agent-runtime-service
两者结合后，答案会更完整。

Claran.: 06-01 23:56:37
离线社区划分

可以用 Python 跑 NetworkX / igraph / graspologic 做 Leiden。

流程：

从 MySQL 读 entities + relationships
  -> 构建图
  -> Leiden 社区划分
  -> 写 knowledge_communities
  -> LLM 生成社区摘要
社区摘要写表：

knowledge_community_summaries
- community_id
- level
- title
- summary
- key_entities_json

注意，GraphRAG不应该取代当前的主流RAG，只是作为附属功能或增强功能

## Phase 6: knowledge graph
## 6.1
我要做知识图谱可视化：这部分可以单独划分一个微服务：knowledge-service
本质上与rag-service有关联

最终效果应该是什么

你想做的东西大概长这样：

节点：
- msg-core-service
- agent-manager-service
- event_outbox
- claran.message.events
- websocket-gateway
- Milvus
- memory-service

边：
- msg-core-service WRITES event_outbox
- msg-core-service PUBLISHES claran.message.events
- websocket-gateway CONSUMES claran.message.events
- agent-manager-service CALLS agent-runtime-service
- memory-service INDEXES Milvus
前端展示：

一个可拖拽、可缩放、可搜索的图
点击节点 -> 显示实体说明、类型、相关文档、相邻节点
点击边 -> 显示关系说明、证据来源
按类型过滤 -> 只看 Service / Table / Topic / Concept
按社区过滤 -> 只看 Agent 社区 / 消息社区 / RAG 社区

前端图可视化库我建议AntV G6
如果你前端只是 dist/js/app.js 这种原生 JS 页面，也可以用 CDN 引 G6 或 Cytoscape，先做一个独立页面 图谱可视化交互应该有哪些

MVP 至少做：

搜索节点
按类型过滤
拖拽节点
缩放画布
点击节点看详情
点击边看关系说明
只显示一跳/二跳邻居
重置视图
稍微进阶：

社区颜色
路径高亮
按关系类型过滤
节点重要度大小
证据来源跳转
例如你点：

agent_dispatch_records
右侧详情显示：

类型：table
说明：Agent 调度幂等表
相关关系：
- agent-manager-service WRITES agent_dispatch_records
- agent_dispatch_records DEDUPLICATES Agent dispatch
- agent_dispatch_records LINKS_TO client_msg_id
证据：
- internal/agent-manager-service/eventconsumer/agent_consumer.go
这对学习项目很有帮助。

和 GraphRAG 怎么衔接

可视化图谱和 GraphRAG 共用同一套底层数据：

knowledge_entities
knowledge_relationships
knowledge_communities
可视化用：

nodes + edges
GraphRAG 用：

entity linking
subgraph retrieval
community summary
evidence chunks
所以你现在做可视化，不是额外功能，而是在给 GraphRAG 打底。

注意，我需要精美的前端页面，有足够动效，例如鼠标悬浮在实例上的反馈，或者示例的随机漂浮

# Phase 7: memory&websearch&conversation rag
## 7.1

现在开始做memory-service，同时也是重做之前的memory。现在需要给memory集成rag，做成rag系统的memory-service，包含Embedding + Milvus 向量召回，Metadata 权限过滤，MySQL 回源校验，不要只看向量分数。Memory 要融合：

向量相似度
重要性 importance
时效性 recency
作用域 scope boost，TopK 限制和最低分过滤，Prompt 注入策略
注入时要明确告诉 LLM：

以下是可能相关的长期记忆。
如果和当前问题无关，不要强行使用。
用户当前输入优先级高于记忆。，如果召回噪声多，可以加一个轻量过滤器：

query + memory candidates
  -> 小 LLM 判断哪些记忆真正有用，记忆候选自动抽取
从聊天或 Agent 运行结果中提取：

用户偏好
长期目标
学习背景
项目状态
反复出现的困惑，先进入：

pending memory_candidates
再由用户确认或规则自动接受。，记忆冲突处理
例如旧记忆：

用户不懂 Kafka
新记忆：

用户已经学完 Kafka 基础
需要：

更新旧记忆
降低旧记忆权重
标记过期
保留时间线。总的来说就是memory-service 最合理的是：

Dense Vector Search
+ Metadata Filter
+ MySQL Fact Check
+ Score Fusion
+ TopK / MinScore
+ Optional LLM Relevance Filter
+ Memory Governance
不需要什么复杂的rag技术

以上我提到的内容到要实现，记得总结文档

## 7.2
现在做conversation-intelligence-service，主要负责聊天记录rag归档整合成记忆

聊天记录要不要 RAG

要，但不要一开始对所有聊天记录全量 RAG。

IM 系统里，聊天记录 RAG 很有价值：

找历史讨论
总结某段时间发生了什么
问“之前我们怎么决定的”
找某个文件/任务/结论出处
Agent 根据长期群聊上下文回答
但聊天记录有几个难点：

噪声大
数量多
权限复杂
实时增长快
短消息语义弱
重复寒暄多
上下文依赖强
例如单条消息：

好
可以
那就这样
明天搞
拿去 embedding 基本没意义。

所以聊天记录 RAG 不应该直接“每条消息都入 Milvus”。

更合理的是分层：

短期上下文：直接从 msg-core 拉最近 N 条
长期聊天 RAG：对会话片段/摘要做向量化
推荐做法：

最近 20-80 条消息：
  直接查 msg-core-service，作为短期上下文

长期历史：
  按时间窗口/话题聚合
  生成 conversation_summary / decision / task / topic chunk
  再入 Milvus
也就是说，聊天记录 RAG 的对象最好是：

会话片段摘要
讨论结论
任务决策
关键消息组合
而不是每一条碎消息。

也就是说，聊天记录的rag可以做定时任务，拉取1h内所有信息或者每100条信息做一次（用户可自由配置），做conversation_summary / decision / task / topic chunk或者金句/用户要求记忆或者价值信息，再入strict rag，

只对有价值消息入库：

长文本消息
包含文件/代码/链接的消息
被引用/点赞/标记的消息
Agent 生成的总结
用户手动收藏的消息
决策类消息

自动记忆提取

Agent 对话结束后，异步判断是否应该生成新记忆：

这轮对话是否产生长期有用事实？
例如：

用户明确说“我不懂 RAG 的 rerank”
生成候选记忆：

用户正在学习 RAG，特别关注 rerank、CRAG、Adaptive RAG。
但建议先进入：

pending / user_confirmed
不要无脑写。

记忆冲突处理

比如旧记忆：

用户不懂消息队列
新记忆：

用户已经掌握 Kafka 基础
需要：

更新旧记忆
降低旧记忆权重
保留时间线
这是后期能力。
产物应该有哪些

你列的几个都很好，可以明确成几类。

conversation_summary

会话摘要，回答：

这段时间大家聊了什么？
字段：

conversation_id
start_message_id
end_message_id
start_time
end_time
summary
participants
示例：

这一小时主要讨论了 Milvus 接入方式。结论是先用 docker-compose 启动 standalone，再从 memory-service 接入向量召回。
decision

决策，回答：

这段讨论里做了什么决定？
示例：

决定先做 Memory RAG，不一开始上完整 CRAG/Self-RAG。
字段：

decision_text
reason
decided_by
source_message_ids
confidence
task

任务，回答：

接下来要做什么？
谁负责？
什么时候做？
示例：

任务：设计 memory-service 的 Milvus collection schema。
负责人：用户。
状态：todo。
字段：

task_title
assignee
due_time
status
source_message_ids
topic chunk

话题块，回答：

这段聊天属于什么主题？
示例：

主题：RAG 架构设计
关键词：Milvus, Memory RAG, Hybrid Search, Rerank, CRAG
它适合入向量库。

quote / 金句

这个可以做，但要克制。适合保存：

明确结论
用户表达偏好
重要定义
架构原则
比如：

“同步保证核心事实正确；异步处理可重试的后处理；缓存只加速可重建的读模型。”
这类很适合沉淀。

memory_candidate

候选记忆，回答：

这段聊天里有没有值得长期记住的用户信息？
例如：

用户准备开始集成 Milvus。
用户希望按教学方式理解 RAG。
用户倾向先做 Memory RAG。
注意：这类最好先进入候选状态，不要全部自动写正式记忆。
产物应该有哪些

你列的几个都很好，可以明确成几类。

conversation_summary

会话摘要，回答：

这段时间大家聊了什么？
字段：

conversation_id
start_message_id
end_message_id
start_time
end_time
summary
participants
示例：

这一小时主要讨论了 Milvus 接入方式。结论是先用 docker-compose 启动 standalone，再从 memory-service 接入向量召回。
decision

决策，回答：

这段讨论里做了什么决定？
示例：

决定先做 Memory RAG，不一开始上完整 CRAG/Self-RAG。
字段：

decision_text
reason
decided_by
source_message_ids
confidence
task

任务，回答：

接下来要做什么？
谁负责？
什么时候做？
示例：

任务：设计 memory-service 的 Milvus collection schema。
负责人：用户。
状态：todo。
字段：

task_title
assignee
due_time
status
source_message_ids
topic chunk

话题块，回答：

这段聊天属于什么主题？
示例：

主题：RAG 架构设计
关键词：Milvus, Memory RAG, Hybrid Search, Rerank, CRAG
它适合入向量库。

quote / 金句

这个可以做，但要克制。适合保存：

明确结论
用户表达偏好
重要定义
架构原则
比如：

“同步保证核心事实正确；异步处理可重试的后处理；缓存只加速可重建的读模型。”
这类很适合沉淀。

memory_candidate

候选记忆，回答：

这段聊天里有没有值得长期记住的用户信息？
例如：

用户准备开始集成 Milvus。
用户希望按教学方式理解 RAG。
用户倾向先做 Memory RAG。
注意：这类最好先进入候选状态，不要全部自动写正式记忆。
Scheduler 每分钟扫描活跃会话
  -> 找到满足条件的 conversation
  -> 创建 digest job

Worker 处理 digest job
  -> 拉取消息窗口
  -> 清洗消息
  -> LLM 提炼 summary/decision/task/topic/memory_candidate
  -> 写 MySQL
  -> embedding
  -> 写 Milvus
  -> 标记 job completed

所以是对活跃会话进入该服务层。

如果跟memory-service相似，可以考虑合并

## 7.3
现在做web-search-service，有关web search 轻量 Web RAG 更实用

如果只用搜索摘要，有时不够准。可以做轻量版：

搜索
  -> 选官方/高可信页面
  -> 抓网页正文
  -> 清洗正文
  -> 截取相关段落
  -> 给 LLM 回答
这其实已经有 RAG 味道，但不需要向量库。

你可以叫它：

Web Search Augmentation
而不是完整 Web RAG。

它不需要：

Milvus
embedding
chunk 入库
长期索引
只是在一次请求内临时检索。

## 7.4
完善这几块内容，并且有update文档产出，并更新plan.md
- [~] WebSocket 在线推送和 Outbox 事件已接入，但离线推送、上线同步、多端完整同步和乱序补偿仍不完整。
- [ ] 客户端本地数据库级缓存和云端漫游策略。
- [~] 未读摘要、“我错过了什么”
- [~] conversation-intelligence-service：可创建/处理会话归档任务，可消费消息/IM 事件推进活跃会话游标，并按时间窗口或消息数阈值自动归档。
- [~] 聊天记录长期 RAG：摘要/主题块进入 rag-service，候选记忆进入 memory-service pending 区；当前仍是规则提炼和基础调度 MVP，LLM 提炼器、失败重试队列和前端归档状态页未完成。
- [ ] API Key 加密存储
- [ ] 实体归一化、Leiden 社区划分、社区摘要持久化、图谱候选审核和专用图数据库评估。

1. 强化 conversation-intelligence-service：增加 LLM 提炼器、失败重试、归档状态前端入口、按时间范围精确拉取消息和调度指标。
2. 补齐 Agent 运行生产化：持久化审批、checkpoint/resume、取消、心跳、长期任务队列和运行审计。
3. 完善 IM 同步：离线推送、上线同步、多端游标、Ack/重试/乱序补偿和客户端本地缓存策略。
4. 深化 RAG + Knowledge Graph：图谱候选审核、实体归一化、社区摘要、RAG 来源治理和 ppt/pptx 解析。
5. 推进 Tool / Skill / MCP 生产化：工具市场、权限矩阵、审批持久化、失败重试、限流配额和 stdio MCP 安全执行沙箱。

继续完善
- [~] 实体 canonical key 归一化、Leiden-like 社区划分和社区摘要持久化已有 MVP；仍需图谱候选审核、严格 Leiden 库级实现和专用图数据库评估。
- [~] WebSocket 在线推送和 Outbox 事件已接入，但离线推送、上线同步、多端完整同步和乱序补偿仍不完整。
- [ ] 客户端本地数据库级缓存和云端漫游策略。
- [~] 全项目仍缺少统一端到端验收脚本和稳定 CI。
 Phase 9：客户端与体验扩展

- [x] Web 前端已有聊天、好友、群、Agent、Memory、Settings、RAG 和 Knowledge Graph 页面。
- [x] Markdown 消息渲染、Agent 流式对话样式、Action Card MVP 和知识图谱交互。
- [~] Agent 上下文侧边栏、运行状态、Skill 页面和触发规则页面已有 MVP，但交互细节仍需持续打磨。
- [ ] 管理端前端：媒体审核、系统消息、Agent 审计、知识候选审核和成本面板。
- [ ] 更完整的文件预览、图片查看、语音播放、表情库和组合搜索体验。
记得给我文档

# Phase8 Check&Frontend
## 8.1
重构前端页面，可以不用只有一个展示页，很多东西可以分为独立其他前端页面上，比如管理层和配制成，其他的你自己思考。另外，注意功能链路要正确。并且给前端加上一定动效和美化，让页面更生动好看

再次检查构建的文件，查找可能出现的bug，检查前端功能是否满足后端要求并添砖补瓦，检查前端按钮、悬浮窗、滑动窗口是否合理

项目中pkg里诸如websearchclient是不是没必要存在了？是之前没有kitex时构筑的旧产物吧

skill文件存储方式应该是用户名后的skill文件夹，而不是只有skill.md

## 8.2
- 如图，agent会错误地不断回复自己的消息
-  在收到图片时，应该可以尝试调用orc解释图片，而不是只把orc藏在rag里面
- 优化会话Ctx上下文侧边栏的前端展示，现在这个可读性差且丑
- 如图，在agent管理菜单里打开的skill悬浮窗很丑，而且可用性很差，记忆管理同理
- 现在会错误地生成相同的运行摘要，且都是system prompt
- 删除text-to-sql
- 联网搜索等悬浮窗都很丑，可以做成单独页面
- 上传文件构建知识库不可用，错误状态如图
- 上传文件构建知识库的清空按钮不可用

根因更像是：Agent 发送的文本消息也会生成 message.created，私聊默认逻辑会把“另一个参与者”当作目标；如果参与者列表里只有用户和 Agent，自发消息会默认触发用户而非 Agent，但 resolveBot 可能把用户 ID 当 Agent 查不到，理论上应跳过。截图说明可能是旧 outbox payload 的 participant/mention 不完整或 agent 自己 @ 了自己。我要加防御性测试：任何来自已知 Agent 用户的事件都不再触发任何 Agent，哪怕 mention/规则命中。

事件发送者是另一个 Agent 时会触发目标 Agent。这正是 Agent 原生 IM 里容易形成互相回声的问题。我会在 dispatcher 入口加“发送者是 Agent 用户则忽略触发”的防线，后面多 Agent 协作再走专门协作链路，不走普通消息触发链。

## 8.3
这里有个设计层面的根因：internal Agent 创建时把平台默认 key 快照写进 bots.api_key，后面你改 .env，旧 Agent 不会自动跟着更新。所以我准备把“internal Agent 使用当前平台默认 LLM”这件事做到运行前，而不是要求你每次换 key 后手工更新所有 Agent。

根因修复会落在 agent-manager service 层：internal Agent 的 provider 配置应该是“运行时读取平台默认配置”，不是“创建时永久冻结”。这样你以后改 .env 或平台默认 LLM 后，旧 internal Agent 不会继续拿过期 key。

## 8.4
继续修改启动脚本相关问题
- 另外，为什么有时候我启动脚本会有skip的服务呢？我明明每次测试都关掉cmd了，这种情况下不会弹出新的cmd，我也不好测试啊
- 文档入rag后，前端显示区块为一个chunk，不能正常分片，有很大的问题，也有可能是前端显示问题，我需要知道上传文件后被分成了多少chunk
- 文档检索界面做得不好，一个下拉菜单中有很大一部分空白
- Go进阶与Git使用-第2节课.md： rpc timeout: timeout=1m0s, to=rag-service, method=IngestDocument, location=kitex.rpcTimeoutMW, remote=127.0.0.1:9112，但是貌似成功了一部分
- 04 实验四 气缸缸体建模及气缸活塞杆建模.docx： rpc timeout: timeout=1m0s, to=rag-service, method=IngestDocument, location=kitex.rpcTimeoutMW, remote=127.0.0.1:9112，但是貌似成功了一部分
- 知识库上传文件（18.9mb）时，有时候会fail to fetch，可能跟文件大小有关，但是好像有有部分上传成功了
- 请检查 上传文件-rag解析-rag链路是否是异步的，用户切换页面时文件上传解析中是否有影响？
- 知识图谱的划分和实体抽取有很大的问题，全是意义不明的关系
- 知识图谱界面请用中文表明各个实体和关系
- 联网增强无法使用
- Adaptive rag不好用，有些问题可以入rag但是被拒绝
- 同一个问题，混合检索效果不好，如图
- rag检索部分除了命中信息，还要最终的检索结果，用于回答用户的问题
- 知识图谱的前端部分改一下，主要是排版和UI，画布应占据更大的部分，参数选择等功能部分可以做成小横幅或者侧边栏
- agent还是会回复自己
- agent不能正常运行我注入的测试skill，如图

## 8.5
- 前端知识台页面有问题，页面居然不是居中的，偏移了
- 我之前应该说过要加这个功能：用户自己配置rag等功能使用的LLM，embedding，ocr和routerllm，或者选择项目自带的，并且还要可以检测这些模型是否可用（联通测试，项目中自带的也要）
- 项目中rag检索系统还是有些问题，如图
- 上传过大的文件时会fail to fetch
- 还是不能按文章检索
- 应该有一个可以删除对应文章的知识图谱的功能，并帮我把现在项目中已上传的所有的知识库和知识图谱删除
- 有些按钮还是很丑
- 另外，你做了管理层的前端对应部分的吗？我没找到入口
- 机器人还是有自己回答自己的消息的情况
- 另外，会话页面“知识”侧边栏有点鸡肋或者没用，而且按文档检索rag那一部分好像也做得不好，现在点击对应文档检索只是单纯地在检索内容中加上文档标题罢了

## 8.6
- 右下角知识入库的进度条有bug，会卡住且不会更新信息，完成过后也不会显示完成
- 这些按钮还是很丑，你要一次性把类似的按钮改完
- 管理面板很丑，位置不对，有偏移，且应该有更多功能，比如封禁用户，解封用户，封禁会话（只能群聊），按需求查找用户，媒体界面可以预览照片等文件，成本界面可以折叠小信息，且做得更可视化一点，可以有当日圆饼图或者多日柱状图等。总之管理面板界面可以大改
- 知识录入界面录入时前端会一直闪烁
- 我希望只是录入界面录入文档是有更多的可视化，比如进度，时间等
- 检索文件正在执行 Adaptive RAG时，也需要更多可视化，有时候一直显示正在检索我都不知道是不是出error了还是卡了
- 检索后并没有生成由ai总结的内容，并且我希望除了ai总结的内容外也可以看被折叠的原文和命中信息
- 检索命中信息做得太不合理了，每一栏都有大量空白
- 不能正常生成知识图谱
- 记忆部分，不应该所有聊天内容都进记忆，短期记忆或者无用记忆应该被临时召回或者遗弃

## 8.7
- 系统公告内容应该全局全用户可见，在右上角做个公告小圆形铃铛按钮，按下会显示最近和总系统公告
- 完善群封禁功能
- 管理界面的媒体部分图片和视频预览应该还要可以直接看到预览图，而不是预览才能看到
- 管理界面的媒体部分不能正常对媒体类型进行查找
- 完善mcp功能：websearch表示其有错误
- 同上：summarize_conversation表示其有错误
- 管理界面的成本界面的柱状图和圆环图鼠标悬浮应该能看到具体数据才对
- 无法加载图谱，无法正常抽离实体和关系并划分社区
- 离开正在上传的知识图谱上传界面后，再次进入该界面不能正常显示上次上传进度，且一旦离开该界面上传就会异常
- 知识库上传小气泡需要可以用户拖动或者换位，不然会挡到其他东西
- 如图，agent还是会不停引用并回复自己的消息
- 翻译时，显示您的账户已达到速率限制，请您控制请求频率，可能是第三方模型的提示，但是你要返回清楚链路，用户不知道是哪个东西出问题了或者哪个第三方，如果是项目内部的限制，请解除
- 有时候用户和别人发的消息会串会话显示

请将这些问题全部一次性解决

## 8.8
仍然存在的问题：
- 翻译时显示“您的账户已达到速率限制，请您控制请求频率”

- 关于知识图谱部分，请仔细检查这几个部分：
1. 当文档上传rag时，知识图谱部分是否能正常根据上传的文档抽取并生成实体，关系等内容
2. 知识图谱抽离的实体和关系是否合理，是否能当作文档的精华
3. 知识图谱的实体关系等部分和社区划分是否正常生效

需要达到的最终效果应该是：当用户上传文档时，知识图谱能根据文档抽离精准正常的实体和关系，并标记其出处，并做与现已存在实体与关系的集成（不一定是同一文档的），最终形成精简的知识图谱系统
