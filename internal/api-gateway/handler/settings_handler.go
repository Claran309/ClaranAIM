package handler

import (
	"ClaranAIM/pkg/response"
	"ClaranAIM/pkg/settingsclient"
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

// SettingsHandler 暴露用户级系统设置接口，例如可复用的大模型配置和提示词模板。
type SettingsHandler struct {
	svc settingsclient.Service
}

const maxSkillUploadBytes int64 = 5 << 20
const maxSkillPackageFiles = 80

// gatewaySettingsService 是 api-gateway 到 settings-service 的内部客户端。
// main 启动时注入一次，handler 创建时只持有接口，避免网关 import settings-service 实现包。
var gatewaySettingsService settingsclient.Service

// InitSettingsService 将 settings-service 的客户端门面注册到 api-gateway。
func InitSettingsService(svc settingsclient.Service) {
	gatewaySettingsService = svc
}

// NewSettingsHandler 创建系统设置 HTTP handler。
func NewSettingsHandler() *SettingsHandler {
	return &SettingsHandler{svc: gatewaySettingsService}
}

// ensureService 在请求入口检查 settings-service 客户端是否完成初始化。
// 如果网关启动配置缺失，返回明确错误，避免后续 nil pointer panic。
func (h *SettingsHandler) ensureService(c *app.RequestContext) bool {
	if h.svc == nil {
		response.Error(c, "settings-service未初始化")
		return false
	}
	return true
}

// ListLLMProfiles 返回当前用户保存的可复用大模型配置。
func (h *SettingsHandler) ListLLMProfiles(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	profiles, err := h.svc.ListLLMProfiles(ctx, userID, c.DefaultQuery("usage_type", ""))
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "profiles": profiles})
}

// SaveLLMProfile 创建或更新当前用户的大模型配置。
func (h *SettingsHandler) SaveLLMProfile(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req llmProfileReq
	if err := bindSettingsJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	profile, err := h.svc.SaveLLMProfile(ctx, userID, settingsclient.SaveLLMProfileInput{
		ID:           parseSettingsNumber(req.ID),
		Name:         req.Name,
		ProviderType: req.ProviderType,
		BaseURL:      req.BaseURL,
		APIKey:       req.APIKey,
		ModelName:    req.ModelName,
		UsageType:    defaultSettingString(req.UsageType, settingsclient.ProviderTranslate),
		IsDefault:    req.IsDefault,
		Enabled:      req.Enabled,
		APIKeyAction: defaultSettingString(req.APIKeyAction, settingsclient.APIKeyActionKeep),
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "profile": profile})
}

// DeleteLLMProfile 删除当前用户名下的一条大模型配置。
func (h *SettingsHandler) DeleteLLMProfile(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的配置ID")
		return
	}
	if err := h.svc.DeleteLLMProfile(ctx, userID, id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true})
}

// ListPrompts 返回当前用户保存的提示词模板。
func (h *SettingsHandler) ListPrompts(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	prompts, err := h.svc.ListPrompts(ctx, userID)
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "prompts": prompts})
}

// SavePrompt 创建或更新当前用户的一条提示词模板。
func (h *SettingsHandler) SavePrompt(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req promptReq
	if err := bindSettingsJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	prompt, err := h.svc.SavePrompt(ctx, userID, settingsclient.SavePromptInput{
		ID:        parseSettingsNumber(req.ID),
		Type:      defaultSettingString(req.Type, settingsclient.ProviderTranslate),
		Name:      req.Name,
		Content:   req.Content,
		IsDefault: req.IsDefault,
		Enabled:   req.Enabled,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "prompt": prompt})
}

// UploadSkill 接收浏览器上传的单文件 SKILL.md、zip 包或文件夹文件列表。
// 网关只负责 HTTP multipart 解析和 zip 解包，真正的 Skill 元数据、目录边界和落盘由 settings-service 负责。
func (h *SettingsHandler) UploadSkill(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	scope := defaultSettingString(string(c.FormValue("scope")), settingsclient.SkillScopeGlobal)
	name := strings.TrimSpace(string(c.FormValue("name")))
	description := strings.TrimSpace(string(c.FormValue("description")))
	agentID, err := parseOptionalSettingsInt64(string(c.FormValue("agent_id")))
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	isDefault := parseSettingsBool(string(c.FormValue("is_default")))
	form, err := c.MultipartForm()
	if err != nil || form == nil {
		response.BadRequest(c, "请上传Skill文件")
		return
	}
	files, err := collectSkillUploadFiles(form.File["file"])
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if name == "" {
		name = inferSkillName(files)
	}
	skill, err := h.svc.SaveSkill(ctx, userID, settingsclient.SaveSkillInput{
		Username:    currentUsername(c),
		Name:        name,
		Description: description,
		Scope:       scope,
		AgentID:     agentID,
		Files:       files,
		IsDefault:   isDefault,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "skill": skill})
}

// ListSkills 返回当前用户配置过的全局 Skill 或某个 Agent 的专属 Skill。
func (h *SettingsHandler) ListSkills(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	agentID, err := parseOptionalSettingsInt64(c.DefaultQuery("agent_id", "-1"))
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	skills, err := h.svc.ListSkills(ctx, userID, c.DefaultQuery("scope", ""), agentID)
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "skills": skills})
}

// GetSkill 返回单个 Skill 的详情和可编辑的 SKILL.md 正文。
func (h *SettingsHandler) GetSkill(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的Skill ID")
		return
	}
	skill, err := h.svc.GetSkill(ctx, userID, id)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "skill": skill})
}

// UpdateSkillContent 保存用户在前端编辑后的 Skill 入口 Markdown。
func (h *SettingsHandler) UpdateSkillContent(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的Skill ID")
		return
	}
	var req skillContentReq
	if err := json.Unmarshal(c.Request.Body(), &req); err != nil {
		response.BadRequest(c, "请求体格式错误")
		return
	}
	skill, err := h.svc.UpdateSkillContent(ctx, userID, id, req.Name, req.Description, []byte(req.Content))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "skill": skill})
}

// DeleteSkill 删除当前用户拥有的 Skill 元数据。
func (h *SettingsHandler) DeleteSkill(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的Skill ID")
		return
	}
	if err := h.svc.DeleteSkill(ctx, userID, id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true})
}

// ListMCPServers 返回当前用户配置的外部 MCP Server，Secret 不会回显。
func (h *SettingsHandler) ListMCPServers(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	agentID, err := parseOptionalSettingsInt64(c.DefaultQuery("agent_id", "-1"))
	if err != nil {
		response.BadRequest(c, "无效的Agent ID")
		return
	}
	conversationID, err := parseOptionalSettingsInt64(c.DefaultQuery("conversation_id", "-1"))
	if err != nil {
		response.BadRequest(c, "无效的会话ID")
		return
	}
	includeDisabled := strings.EqualFold(c.DefaultQuery("include_disabled", "false"), "true")
	servers, err := h.svc.ListMCPServers(ctx, userID, c.DefaultQuery("scope", ""), agentID, conversationID, includeDisabled)
	if err != nil {
		response.Error(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "servers": servers})
}

// SaveMCPServer 创建或更新当前用户的外部 MCP Server 配置。
func (h *SettingsHandler) SaveMCPServer(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	var req mcpServerReq
	if err := bindSettingsJSON(c, &req); err != nil {
		response.BadRequest(c, "参数错误")
		return
	}
	server, err := h.svc.SaveMCPServer(ctx, userID, settingsclient.SaveMCPServerInput{
		ID:             parseSettingsNumber(req.ID),
		AgentID:        parseSettingsNumber(req.AgentID),
		ConversationID: parseSettingsNumber(req.ConversationID),
		Scope:          defaultSettingString(req.Scope, settingsclient.MCPScopeUser),
		Name:           req.Name,
		Description:    req.Description,
		Transport:      defaultSettingString(req.Transport, settingsclient.MCPTransportStreamableHTTP),
		EndpointURL:    req.EndpointURL,
		Command:        req.Command,
		ArgsJSON:       req.ArgsJSON,
		EnvJSON:        req.EnvJSON,
		HeadersJSON:    req.HeadersJSON,
		AuthType:       req.AuthType,
		Secret:         req.Secret,
		SecretAction:   defaultSettingString(req.SecretAction, settingsclient.APIKeyActionKeep),
		Enabled:        req.Enabled,
		TrustLevel:     defaultSettingString(req.TrustLevel, settingsclient.MCPTrustLow),
		AllowToolsJSON: req.AllowToolsJSON,
		DenyToolsJSON:  req.DenyToolsJSON,
	})
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true, "server": server})
}

// DeleteMCPServer 删除当前用户拥有的外部 MCP Server 配置。
func (h *SettingsHandler) DeleteMCPServer(ctx context.Context, c *app.RequestContext) {
	if !h.ensureService(c) {
		return
	}
	userID, ok := requireCurrentUserID(c)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "无效的MCP配置ID")
		return
	}
	if err := h.svc.DeleteMCPServer(ctx, userID, id); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, map[string]interface{}{"success": true})
}

// llmProfileReq 是浏览器保存 LLM 预设的 JSON 请求体。
// ID 使用 json.Number，避免 JS 大整数在网关二次解析时丢失精度。
type llmProfileReq struct {
	ID           json.Number `json:"id"`
	Name         string      `json:"name"`
	ProviderType string      `json:"provider_type"`
	BaseURL      string      `json:"base_url"`
	APIKey       string      `json:"api_key"`
	ModelName    string      `json:"model_name"`
	UsageType    string      `json:"usage_type"`
	IsDefault    bool        `json:"is_default"`
	Enabled      *bool       `json:"enabled"`
	APIKeyAction string      `json:"api_key_action"`
}

// promptReq 是浏览器保存 Prompt 模板的 JSON 请求体。
type promptReq struct {
	ID        json.Number `json:"id"`
	Type      string      `json:"type"`
	Name      string      `json:"name"`
	Content   string      `json:"content"`
	IsDefault bool        `json:"is_default"`
	Enabled   *bool       `json:"enabled"`
}

// skillContentReq 是 Skill 编辑器保存入口 Markdown 时使用的 JSON 请求体。
type skillContentReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// mcpServerReq 是浏览器保存外部 MCP Server 时使用的 JSON 请求体。
// allow_tools_json / deny_tools_json / headers_json 等字段保持 JSON 字符串，方便前端高级模式直接编辑。
type mcpServerReq struct {
	ID             json.Number `json:"id"`
	AgentID        json.Number `json:"agent_id"`
	ConversationID json.Number `json:"conversation_id"`
	Scope          string      `json:"scope"`
	Name           string      `json:"name"`
	Description    string      `json:"description"`
	Transport      string      `json:"transport"`
	EndpointURL    string      `json:"endpoint_url"`
	Command        string      `json:"command"`
	ArgsJSON       string      `json:"args_json"`
	EnvJSON        string      `json:"env_json"`
	HeadersJSON    string      `json:"headers_json"`
	AuthType       string      `json:"auth_type"`
	Secret         string      `json:"secret"`
	SecretAction   string      `json:"secret_action"`
	Enabled        *bool       `json:"enabled"`
	TrustLevel     string      `json:"trust_level"`
	AllowToolsJSON string      `json:"allow_tools_json"`
	DenyToolsJSON  string      `json:"deny_tools_json"`
}

// collectSkillUploadFiles 将 multipart 上传转换为 settings-service 可保存的 Skill 文件列表。
func collectSkillUploadFiles(headers []*multipart.FileHeader) ([]settingsclient.SkillFileInput, error) {
	if len(headers) == 0 {
		return nil, errors.New("请上传Skill文件")
	}
	out := make([]settingsclient.SkillFileInput, 0)
	var total int64
	for _, header := range headers {
		if header == nil {
			continue
		}
		total += header.Size
		if total > maxSkillUploadBytes {
			return nil, errors.New("Skill包不能超过5MB")
		}
		file, err := header.Open()
		if err != nil {
			return nil, errors.New("读取Skill文件失败")
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxSkillUploadBytes+1))
		_ = file.Close()
		if readErr != nil {
			return nil, errors.New("读取Skill文件失败")
		}
		if strings.EqualFold(filepath.Ext(header.Filename), ".zip") {
			zipFiles, err := unzipSkillFiles(data)
			if err != nil {
				return nil, err
			}
			out = append(out, zipFiles...)
			continue
		}
		out = append(out, settingsclient.SkillFileInput{
			Path:    normalizeBrowserSkillPath(header.Filename),
			Content: data,
		})
	}
	if len(out) > maxSkillPackageFiles {
		return nil, errors.New("Skill包文件数量过多")
	}
	return out, nil
}

// unzipSkillFiles 解开 zip Skill 包，并保留包内相对路径交给 settings-service 二次校验。
func unzipSkillFiles(data []byte) ([]settingsclient.SkillFileInput, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("Skill zip包格式错误")
	}
	if len(reader.File) > maxSkillPackageFiles {
		return nil, errors.New("Skill包文件数量过多")
	}
	out := make([]settingsclient.SkillFileInput, 0, len(reader.File))
	var total int64
	for _, zf := range reader.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		total += int64(zf.UncompressedSize64)
		if total > maxSkillUploadBytes {
			return nil, errors.New("Skill包不能超过5MB")
		}
		rc, err := zf.Open()
		if err != nil {
			return nil, errors.New("读取Skill zip包失败")
		}
		content, readErr := io.ReadAll(io.LimitReader(rc, maxSkillUploadBytes+1))
		_ = rc.Close()
		if readErr != nil {
			return nil, errors.New("读取Skill zip包失败")
		}
		out = append(out, settingsclient.SkillFileInput{
			Path:    normalizeBrowserSkillPath(zf.Name),
			Content: content,
		})
	}
	return out, nil
}

// normalizeBrowserSkillPath 兼容普通文件上传、webkitdirectory 和 zip 包中的路径格式。
func normalizeBrowserSkillPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	if path == "" {
		return "SKILL.md"
	}
	return path
}

// inferSkillName 根据上传文件名推断一个给用户展示的 Skill 名称。
func inferSkillName(files []settingsclient.SkillFileInput) string {
	for _, f := range files {
		base := filepath.Base(filepath.ToSlash(f.Path))
		if strings.EqualFold(base, "SKILL.md") {
			dir := filepath.Base(filepath.Dir(filepath.ToSlash(f.Path)))
			if dir != "." && dir != "/" && dir != "" {
				return dir
			}
			return "自定义Skill"
		}
	}
	return "自定义Skill"
}

// parseOptionalSettingsInt64 解析可选 ID；空字符串按 0 处理。
func parseOptionalSettingsInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

// parseSettingsBool 解析表单中的布尔值。
func parseSettingsBool(value string) bool {
	parsed, _ := strconv.ParseBool(strings.TrimSpace(value))
	return parsed
}

// bindSettingsJSON 使用 UseNumber 解码，保护雪花 ID 不被 float64 截断。
func bindSettingsJSON(c *app.RequestContext, dest interface{}) error {
	decoder := json.NewDecoder(strings.NewReader(string(c.Request.Body())))
	decoder.UseNumber()
	return decoder.Decode(dest)
}

// parseSettingsNumber 把可选 json.Number 转成 int64。
// 空值表示创建新记录，因此返回 0；非法值也回落为 0，由业务层按创建处理。
func parseSettingsNumber(value json.Number) int64 {
	if strings.TrimSpace(value.String()) == "" {
		return 0
	}
	parsed, _ := strconv.ParseInt(value.String(), 10, 64)
	return parsed
}

// defaultSettingString 对表单和 JSON 字符串统一 trim，并在空值时使用默认值。
func defaultSettingString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
