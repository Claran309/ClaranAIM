# 第 18 课：Settings Service 与 LLM 配置中心

## 学习目标

这一课专题学习 settings-service。你要掌握：

- LLM profile 为什么要独立保存。
- Prompt template 如何服务翻译和未来 Agent 任务。
- 创建 Agent 时 `llm_profile_id` 如何转换为模型配置。
- 为什么 API Key 不能直接暴露给前端。

## 源码入口

重点阅读：

- `internal/settings-service/model/model.go`
- `internal/settings-service/dao/dao.go`
- `internal/settings-service/service/service.go`
- `idl/settings.thrift`
- `kitex_gen/settings/settingsservice`
- `pkg/settingsclient`
- `internal/api-gateway/handler/settings_handler.go`
- `internal/api-gateway/handler/agent_handler.go`
- `internal/msg-core-service/service/translation.go`

## LLMProfile

`llm_profiles` 保存：

- scope。
- owner_id。
- name。
- provider_type。
- base_url。
- api_key。
- model_name。
- usage_type。
- is_default。
- enabled。

API Key 在 JSON 中隐藏，前端不应拿到明文。

## 创建 Agent 时的使用

前端可以传：

```text
llm_profile_id
```

api-gateway 会：

```text
读取当前用户
  -> settings-service ResolveLLMProfile
  -> 拿到 APIKey/BaseURL/ModelName
  -> 组装 Agent 创建请求
  -> agent-manager-service 保存配置
```

这样用户不用每次创建 Agent 都重新填模型配置。

## PromptTemplate

当前重点是翻译 Prompt：

```text
请将下面内容翻译成中文。只输出译文，保留代码、链接、数字、专有名词和 Markdown 结构。
```

未来可以扩展：

- 总结 Prompt。
- 知识抽取 Prompt。
- 回复候选 Prompt。
- 代码审查 Prompt。

## 手动翻译

msg-core-service 翻译链路依赖 settings-service：

```text
/message/translate
  -> msg-core 校验消息可见性
  -> settings-service 读取翻译配置
  -> 调 LLM
  -> 缓存 translation
```

这避免把模型配置散落在各服务。

## 本课检查

你应该能回答：

- settings-service 为什么独立？
- API Key 为什么不能返回前端？
- `llm_profile_id` 如何影响 Agent 创建？
- 翻译 Prompt 为什么适合用户可配置？

## 动手任务

1. 设计一个默认 LLM profile。
2. 设计一个总结 Prompt template。
3. 追踪 `/settings/llm-profiles`。
4. 追踪创建 Agent 时 profile 解析。
