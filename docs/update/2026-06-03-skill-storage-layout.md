# 2026-06-03 Skill 存储目录修正

## 背景

此前浏览器上传 Skill 文件夹时，api-gateway 会把路径第一层目录剥掉，例如 `Skill/SKILL.md` 会被改成 `SKILL.md`。settings-service 又只接受根目录 `SKILL.md`，导致实际落盘后 Skill 包被压扁，只剩入口文件，辅助文件夹和引用资料无法保持原结构。

## 本次修改

- api-gateway 不再剥离上传目录，`webkitdirectory` 和 zip 内的相对路径会原样交给 settings-service 二次校验。
- settings-service 支持带文件夹的 Skill 包，自动识别包含 `SKILL.md` 的包根目录，并把包内文件完整写入该目录。
- Skill 落盘路径改为用户名后的 `skill` 文件夹：
  - 全局 Skill：`storage/agent/skills/global/<username>/skill/<skill-folder>/...`
  - Agent 专属 Skill：`storage/agent/skills/agents/<username>/<agent_id>/skill/<skill-folder>/...`
- 若服务间旧调用没有传用户名，目录回退为 `user_<user_id>`，避免不同用户写入共享目录。
- `EntryFile` 继续表示相对 Skill 包根目录的入口文件，默认是 `SKILL.md`；前端编辑入口文件时不会再强制重置目录结构。

## 验证

已新增并通过测试：

- `internal/settings-service/service`：验证 `Skill/SKILL.md` 和辅助文件会写入用户名后的 `skill/Skill` 目录。
- `internal/api-gateway/handler`：验证上传路径 `Skill/SKILL.md` 不再被压扁。

相关命令：

```powershell
go test ./internal/settings-service/service
go test ./internal/api-gateway/handler -run TestNormalizeBrowserSkillPathPreservesUploadedFolder
go test ./pkg/settingsclient
go test ./internal/settings-service/handler
go test ./kitex_gen/settings
```
