package service

import (
	"ClaranAIM/internal/settings-service/dao"
	"ClaranAIM/internal/settings-service/model"
	"ClaranAIM/pkg/settingsclient"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveUserLLMProfileEncryptsAPIKeyAndDoesNotExposeIt(t *testing.T) {
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{}, WithSecretEncryptionKey("unit-test-settings-secret"))

	profile, err := svc.SaveLLMProfile(context.Background(), 1001, SaveLLMProfileInput{
		Name:         "zhipu",
		UsageType:    model.ProviderTranslate,
		BaseURL:      "https://open.bigmodel.cn/api/paas/v4",
		APIKey:       "secret",
		ModelName:    "glm-4.7",
		APIKeyAction: APIKeyActionSet,
		Enabled:      boolPtr(true),
	})
	if err != nil {
		t.Fatalf("SaveLLMProfile returned error: %v", err)
	}
	if !profile.HasAPIKey {
		t.Fatal("profile should indicate api key exists")
	}
	stored, _ := repo.GetLLMProfile(context.Background(), profile.ID)
	if stored.APIKey == "secret" || !strings.HasPrefix(stored.APIKey, "enc:v1:") {
		t.Fatalf("stored api key = %q, want encrypted value", stored.APIKey)
	}
	resolved, err := svc.ResolveLLMProfile(context.Background(), 1001, profile.ID)
	if err != nil {
		t.Fatalf("ResolveLLMProfile returned error: %v", err)
	}
	if resolved.APIKey != "secret" {
		t.Fatalf("resolved api key = %q, want secret", resolved.APIKey)
	}
}

func TestSaveLLMProfileCanKeepOrClearExistingAPIKey(t *testing.T) {
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{}, WithSecretEncryptionKey("unit-test-settings-secret"))
	created, _ := svc.SaveLLMProfile(context.Background(), 1001, SaveLLMProfileInput{Name: "default", BaseURL: "https://llm.example/v1", APIKey: "old", ModelName: "m1", APIKeyAction: APIKeyActionSet})
	stored, _ := repo.GetLLMProfile(context.Background(), created.ID)
	initialCiphertext := stored.APIKey

	_, err := svc.SaveLLMProfile(context.Background(), 1001, SaveLLMProfileInput{ID: created.ID, Name: "default", ModelName: "m2", APIKeyAction: APIKeyActionKeep})
	if err != nil {
		t.Fatalf("keep returned error: %v", err)
	}
	stored, _ = repo.GetLLMProfile(context.Background(), created.ID)
	if stored.APIKey != initialCiphertext || stored.ModelName != "m2" {
		t.Fatalf("keep stored profile = %#v", stored)
	}
	resolved, err := svc.ResolveLLMProfile(context.Background(), 1001, created.ID)
	if err != nil {
		t.Fatalf("ResolveLLMProfile returned error: %v", err)
	}
	if resolved.APIKey != "old" {
		t.Fatalf("resolved api key after keep = %q, want old", resolved.APIKey)
	}

	cleared, err := svc.SaveLLMProfile(context.Background(), 1001, SaveLLMProfileInput{ID: created.ID, Name: "default", APIKeyAction: APIKeyActionClear})
	if err != nil {
		t.Fatalf("clear returned error: %v", err)
	}
	if cleared.HasAPIKey {
		t.Fatal("cleared profile should not report api key")
	}
	stored, _ = repo.GetLLMProfile(context.Background(), created.ID)
	if stored.APIKey != "" {
		t.Fatalf("stored api key after clear = %q, want empty", stored.APIKey)
	}
}

func TestResolveLLMProfileSupportsLegacyPlaintextAPIKey(t *testing.T) {
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{}, WithSecretEncryptionKey("unit-test-settings-secret"))
	if err := repo.SaveLLMProfile(context.Background(), &model.LLMProfile{
		Scope:        model.ScopeUser,
		OwnerID:      1001,
		Name:         "legacy",
		ProviderType: "openai_compatible",
		BaseURL:      "https://llm.example/v1",
		APIKey:       "legacy-plain-key",
		ModelName:    "m1",
		UsageType:    model.ProviderTranslate,
		Enabled:      true,
	}); err != nil {
		t.Fatalf("save legacy profile: %v", err)
	}
	resolved, err := svc.ResolveTranslationConfig(context.Background(), 1001)
	if err != nil {
		t.Fatalf("ResolveTranslationConfig returned error: %v", err)
	}
	if resolved.APIKey != "legacy-plain-key" {
		t.Fatalf("resolved legacy api key = %q", resolved.APIKey)
	}
}

func TestResolveTranslationConfigUsesUserProfileAndPromptBeforeDefaults(t *testing.T) {
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{
		APIKey:  "system-key",
		BaseURL: "https://system.example/v1",
		Model:   "system-model",
	})
	_, _ = svc.SaveLLMProfile(context.Background(), 1001, SaveLLMProfileInput{Name: "my-translator", UsageType: model.ProviderTranslate, BaseURL: "https://user.example/v1", APIKey: "user-key", ModelName: "user-model", IsDefault: true, APIKeyAction: APIKeyActionSet})
	_, _ = svc.SavePrompt(context.Background(), 1001, SavePromptInput{Type: model.ProviderTranslate, Content: "翻译为{{target_language}}：{{text}}"})

	resolved, err := svc.ResolveTranslationConfig(context.Background(), 1001)
	if err != nil {
		t.Fatalf("ResolveTranslationConfig returned error: %v", err)
	}
	if resolved.APIKey != "user-key" || resolved.BaseURL != "https://user.example/v1" || resolved.ModelName != "user-model" {
		t.Fatalf("resolved provider = %#v", resolved)
	}
	if resolved.PromptTemplate != "翻译为{{target_language}}：{{text}}" {
		t.Fatalf("prompt = %q", resolved.PromptTemplate)
	}
}

func TestResolveTranslationConfigFallsBackToSystemDefault(t *testing.T) {
	svc := NewSettingsService(newFakeSettingsRepo(), DefaultLLMConfig{
		APIKey:  "system-key",
		BaseURL: "https://system.example/v1",
		Model:   "system-model",
	})
	resolved, err := svc.ResolveTranslationConfig(context.Background(), 1001)
	if err != nil {
		t.Fatalf("ResolveTranslationConfig returned error: %v", err)
	}
	if resolved.APIKey != "system-key" || resolved.BaseURL != "https://system.example/v1" || resolved.ModelName != "system-model" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if resolved.PromptTemplate != model.DefaultTranslatePrompt {
		t.Fatalf("prompt = %q", resolved.PromptTemplate)
	}
}

func TestRAGRouterLLMProfileCanBeSavedAndListedByUsage(t *testing.T) {
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{})

	_, err := svc.SaveLLMProfile(context.Background(), 1001, SaveLLMProfileInput{
		Name:         "rag-router-small",
		UsageType:    model.ProviderRAGRouter,
		BaseURL:      "https://router.example/v1",
		APIKey:       "router-key",
		ModelName:    "glm-4-flash",
		IsDefault:    true,
		APIKeyAction: APIKeyActionSet,
	})
	if err != nil {
		t.Fatalf("SaveLLMProfile returned error: %v", err)
	}

	profiles, err := svc.ListLLMProfiles(context.Background(), 1001, model.ProviderRAGRouter)
	if err != nil {
		t.Fatalf("ListLLMProfiles returned error: %v", err)
	}
	if len(profiles) != 1 {
		t.Fatalf("profiles len = %d, want 1", len(profiles))
	}
	if profiles[0].UsageType != model.ProviderRAGRouter || !profiles[0].IsDefault || !profiles[0].HasAPIKey {
		t.Fatalf("rag router profile = %#v", profiles[0])
	}
}

func TestSaveGlobalSkillWritesSkillMarkdownUnderGlobalRoot(t *testing.T) {
	root := t.TempDir()
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{}, WithSkillStorageRoot(root))

	skill, err := svc.SaveSkill(context.Background(), 1001, SaveSkillInput{
		Name:      "阅读助手",
		Scope:     model.SkillScopeGlobal,
		FileName:  "SKILL.md",
		Content:   []byte("# 阅读助手\n\n用于总结资料。"),
		IsDefault: true,
	})
	if err != nil {
		t.Fatalf("SaveSkill returned error: %v", err)
	}
	if skill.Scope != model.SkillScopeGlobal || skill.AgentID != 0 {
		t.Fatalf("skill scope = %q agent = %d", skill.Scope, skill.AgentID)
	}
	if !strings.HasPrefix(skill.SkillsDir, filepath.Join(root, "global")) {
		t.Fatalf("skills dir = %q, want under global root %q", skill.SkillsDir, filepath.Join(root, "global"))
	}
	if _, err := os.Stat(filepath.Join(skill.SkillsDir, "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md was not written: %v", err)
	}
}

func TestSaveAgentSkillWritesUnderAgentRoot(t *testing.T) {
	root := t.TempDir()
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{}, WithSkillStorageRoot(root))

	skill, err := svc.SaveSkill(context.Background(), 1001, SaveSkillInput{
		Name:     "代码审查",
		Scope:    model.SkillScopeAgent,
		AgentID:  9988,
		FileName: "SKILL.md",
		Content:  []byte("# 代码审查\n\n检查潜在 bug。"),
	})
	if err != nil {
		t.Fatalf("SaveSkill returned error: %v", err)
	}
	wantDir := filepath.Join(root, "agents", "user_1001", "9988", "skill", "skill")
	if skill.AgentID != 9988 || skill.SkillsDir != wantDir {
		t.Fatalf("skill dir = %q agent = %d, want %q", skill.SkillsDir, skill.AgentID, wantDir)
	}
}

func TestSaveSkillPreservesUploadedFolderUnderUsernameSkillDirectory(t *testing.T) {
	root := t.TempDir()
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{}, WithSkillStorageRoot(root))

	skill, err := svc.SaveSkill(context.Background(), 1001, SaveSkillInput{
		Username: "Claran",
		Name:     "Skill",
		Scope:    model.SkillScopeGlobal,
		Files: []settingsclient.SkillFileInput{
			{Path: "Skill/SKILL.md", Content: []byte("# Skill\n\n用于处理会话上下文。")},
			{Path: "Skill/references/guide.md", Content: []byte("# Guide")},
		},
	})
	if err != nil {
		t.Fatalf("SaveSkill returned error: %v", err)
	}
	wantDir := filepath.Join(root, "global", "Claran", "skill", "Skill")
	if skill.SkillsDir != wantDir {
		t.Fatalf("skills dir = %q, want %q", skill.SkillsDir, wantDir)
	}
	if skill.EntryFile != "SKILL.md" {
		t.Fatalf("entry file = %q, want SKILL.md relative to package root", skill.EntryFile)
	}
	if _, err := os.Stat(filepath.Join(skill.SkillsDir, "SKILL.md")); err != nil {
		t.Fatalf("nested SKILL.md was not preserved under package root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skill.SkillsDir, "references", "guide.md")); err != nil {
		t.Fatalf("nested support file was not preserved: %v", err)
	}
}

func TestSaveSkillRejectsTraversalAndMissingSkillMarkdown(t *testing.T) {
	svc := NewSettingsService(newFakeSettingsRepo(), DefaultLLMConfig{}, WithSkillStorageRoot(t.TempDir()))

	if _, err := svc.SaveSkill(context.Background(), 1001, SaveSkillInput{
		Name:     "坏路径",
		Scope:    model.SkillScopeGlobal,
		FileName: "../SKILL.md",
		Content:  []byte("# bad"),
	}); err == nil {
		t.Fatal("SaveSkill should reject traversal file name")
	}

	if _, err := svc.SaveSkill(context.Background(), 1001, SaveSkillInput{
		Name:     "非Skill",
		Scope:    model.SkillScopeGlobal,
		FileName: "README.md",
		Content:  []byte("# readme"),
	}); err == nil {
		t.Fatal("SaveSkill should reject upload without SKILL.md")
	}
}

func TestGetSkillReturnsMarkdownContentAndSummary(t *testing.T) {
	root := t.TempDir()
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{}, WithSkillStorageRoot(root))

	created, err := svc.SaveSkill(context.Background(), 1001, SaveSkillInput{
		Name:     "会议整理",
		Scope:    model.SkillScopeGlobal,
		FileName: "SKILL.md",
		Content:  []byte("# 会议整理\n\n把群聊、会议记录和待办事项整理成结论清单。\n\n## 步骤\n- 提取结论"),
	})
	if err != nil {
		t.Fatalf("SaveSkill returned error: %v", err)
	}

	got, err := svc.GetSkill(context.Background(), 1001, created.ID)
	if err != nil {
		t.Fatalf("GetSkill returned error: %v", err)
	}
	if got.Content == "" || !strings.Contains(got.Content, "结论清单") {
		t.Fatalf("content = %q, want SKILL.md content", got.Content)
	}
	if got.Summary != "把群聊、会议记录和待办事项整理成结论清单。" {
		t.Fatalf("summary = %q", got.Summary)
	}
}

func TestUpdateSkillContentRewritesSkillMarkdownInPlace(t *testing.T) {
	root := t.TempDir()
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{}, WithSkillStorageRoot(root))

	created, err := svc.SaveSkill(context.Background(), 1001, SaveSkillInput{
		Name:     "旧Skill",
		Scope:    model.SkillScopeGlobal,
		FileName: "SKILL.md",
		Content:  []byte("# 旧Skill\n\n旧说明。"),
	})
	if err != nil {
		t.Fatalf("SaveSkill returned error: %v", err)
	}

	updated, err := svc.UpdateSkillContent(context.Background(), 1001, created.ID, "新Skill", "新的描述", []byte("# 新Skill\n\n新的可执行步骤。"))
	if err != nil {
		t.Fatalf("UpdateSkillContent returned error: %v", err)
	}
	if updated.Name != "新Skill" || updated.Description != "新的描述" {
		t.Fatalf("updated metadata = %#v", updated)
	}
	data, err := os.ReadFile(filepath.Join(created.SkillsDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("read rewritten SKILL.md: %v", err)
	}
	if string(data) != "# 新Skill\n\n新的可执行步骤。" {
		t.Fatalf("rewritten content = %q", string(data))
	}
}

func TestSaveMCPServerListsSanitizedAndResolveIncludesSecret(t *testing.T) {
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{}, WithSecretEncryptionKey("unit-test-settings-secret"))

	global, err := svc.SaveMCPServer(context.Background(), 1001, SaveMCPServerInput{
		Name:         "global-mcp",
		Scope:        model.MCPScopeGlobal,
		Transport:    model.MCPTransportStreamableHTTP,
		EndpointURL:  "https://global-mcp.example/mcp",
		TrustLevel:   model.MCPTrustLow,
		Secret:       "global-secret",
		SecretAction: APIKeyActionSet,
	})
	if err != nil {
		t.Fatalf("SaveMCPServer global returned error: %v", err)
	}
	if global.Secret != "" || !global.HasSecret {
		t.Fatalf("global dto = %#v, want sanitized secret flag", global)
	}

	server, err := svc.SaveMCPServer(context.Background(), 1001, SaveMCPServerInput{
		Name:           "github-mcp",
		Scope:          model.MCPScopeAgent,
		AgentID:        9988,
		Transport:      model.MCPTransportStreamableHTTP,
		EndpointURL:    "https://mcp.example/mcp",
		HeadersJSON:    `{"X-Team":"claran"}`,
		Secret:         "secret-token",
		SecretAction:   APIKeyActionSet,
		TrustLevel:     model.MCPTrustNormal,
		AllowToolsJSON: `["github_search"]`,
	})
	if err != nil {
		t.Fatalf("SaveMCPServer returned error: %v", err)
	}
	if !server.HasSecret || server.Secret != "" {
		t.Fatalf("saved dto = %#v, want secret presence but no secret value", server)
	}
	stored, _ := repo.GetMCPServer(context.Background(), server.ID)
	if stored.Secret == "secret-token" || !strings.HasPrefix(stored.Secret, "enc:v1:") {
		t.Fatalf("stored mcp secret = %q, want encrypted value", stored.Secret)
	}

	listed, err := svc.ListMCPServers(context.Background(), 1001, model.MCPScopeAgent, 9988, -1, false)
	if err != nil {
		t.Fatalf("ListMCPServers returned error: %v", err)
	}
	if len(listed) != 1 || listed[0].Secret != "" || !listed[0].HasSecret {
		t.Fatalf("listed = %#v, want sanitized MCP config", listed)
	}

	resolved, err := svc.ResolveMCPServers(context.Background(), 1001, 9988, 0)
	if err != nil {
		t.Fatalf("ResolveMCPServers returned error: %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("resolved = %#v, want global and agent MCP configs", resolved)
	}
	secretByName := map[string]string{}
	for _, item := range resolved {
		secretByName[item.Name] = item.Secret
	}
	if secretByName["global-mcp"] != "global-secret" || secretByName["github-mcp"] != "secret-token" {
		t.Fatalf("resolved = %#v, want secrets for mcp-gateway service call", resolved)
	}
}

func TestSaveMCPServerRejectsInvalidRemoteConfig(t *testing.T) {
	svc := NewSettingsService(newFakeSettingsRepo(), DefaultLLMConfig{})
	if _, err := svc.SaveMCPServer(context.Background(), 1001, SaveMCPServerInput{
		Name:      "bad",
		Scope:     model.MCPScopeAgent,
		AgentID:   1,
		Transport: model.MCPTransportStreamableHTTP,
	}); err == nil {
		t.Fatal("SaveMCPServer should reject remote MCP without endpoint_url")
	}
	if _, err := svc.SaveMCPServer(context.Background(), 1001, SaveMCPServerInput{
		Name:        "bad-json",
		Scope:       model.MCPScopeUser,
		Transport:   model.MCPTransportStreamableHTTP,
		EndpointURL: "https://mcp.example/mcp",
		HeadersJSON: "not json",
	}); err == nil {
		t.Fatal("SaveMCPServer should reject invalid JSON fields")
	}
}

type fakeSettingsRepo struct {
	nextID  int64
	llms    map[int64]*model.LLMProfile
	prompts map[int64]*model.PromptTemplate
	skills  map[int64]*model.AgentSkill
	mcps    map[int64]*model.MCPServerConfig
}

func newFakeSettingsRepo() *fakeSettingsRepo {
	return &fakeSettingsRepo{nextID: 1, llms: map[int64]*model.LLMProfile{}, prompts: map[int64]*model.PromptTemplate{}, skills: map[int64]*model.AgentSkill{}, mcps: map[int64]*model.MCPServerConfig{}}
}

func (r *fakeSettingsRepo) SaveLLMProfile(ctx context.Context, profile *model.LLMProfile) error {
	if profile.ID == 0 {
		profile.ID = r.nextID
		r.nextID++
	}
	cp := *profile
	r.llms[profile.ID] = &cp
	return nil
}

func (r *fakeSettingsRepo) GetLLMProfile(ctx context.Context, id int64) (*model.LLMProfile, error) {
	if p := r.llms[id]; p != nil {
		cp := *p
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeSettingsRepo) GetLLMProfileByName(ctx context.Context, scope string, ownerID int64, name string) (*model.LLMProfile, error) {
	for _, p := range r.llms {
		if p.Scope == scope && p.OwnerID == ownerID && p.Name == name {
			cp := *p
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *fakeSettingsRepo) ListLLMProfiles(ctx context.Context, filter dao.LLMProfileFilter) ([]model.LLMProfile, error) {
	var out []model.LLMProfile
	for _, p := range r.llms {
		if filter.Scope != "" && p.Scope != filter.Scope {
			continue
		}
		if filter.OwnerID >= 0 && p.OwnerID != filter.OwnerID {
			continue
		}
		if filter.UsageType != "" && p.UsageType != filter.UsageType {
			continue
		}
		if filter.Enabled != nil && p.Enabled != *filter.Enabled {
			continue
		}
		out = append(out, *p)
	}
	return out, nil
}

func (r *fakeSettingsRepo) DeleteLLMProfile(ctx context.Context, id int64) error {
	delete(r.llms, id)
	return nil
}

func (r *fakeSettingsRepo) SavePrompt(ctx context.Context, prompt *model.PromptTemplate) error {
	if prompt.ID == 0 {
		prompt.ID = r.nextID
		r.nextID++
	}
	cp := *prompt
	r.prompts[prompt.ID] = &cp
	return nil
}

func (r *fakeSettingsRepo) GetPromptByType(ctx context.Context, scope string, ownerID int64, promptType string) (*model.PromptTemplate, error) {
	for _, p := range r.prompts {
		if p.Scope == scope && p.OwnerID == ownerID && p.Type == promptType {
			cp := *p
			return &cp, nil
		}
	}
	return nil, nil
}

func (r *fakeSettingsRepo) ListPrompts(ctx context.Context, filter dao.PromptFilter) ([]model.PromptTemplate, error) {
	return nil, nil
}

func (r *fakeSettingsRepo) SaveSkill(ctx context.Context, skill *model.AgentSkill) error {
	if skill.ID == 0 {
		skill.ID = r.nextID
		r.nextID++
	}
	cp := *skill
	r.skills[skill.ID] = &cp
	return nil
}

func (r *fakeSettingsRepo) GetSkill(ctx context.Context, id int64) (*model.AgentSkill, error) {
	if s := r.skills[id]; s != nil {
		cp := *s
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeSettingsRepo) ListSkills(ctx context.Context, filter dao.SkillFilter) ([]model.AgentSkill, error) {
	var out []model.AgentSkill
	for _, s := range r.skills {
		if filter.OwnerID >= 0 && s.OwnerID != filter.OwnerID {
			continue
		}
		if filter.Scope != "" && s.Scope != filter.Scope {
			continue
		}
		if filter.AgentID >= 0 && s.AgentID != filter.AgentID {
			continue
		}
		if filter.Enabled != nil && s.Enabled != *filter.Enabled {
			continue
		}
		out = append(out, *s)
	}
	return out, nil
}

func (r *fakeSettingsRepo) DeleteSkill(ctx context.Context, id int64) error {
	delete(r.skills, id)
	return nil
}

func (r *fakeSettingsRepo) SaveMCPServer(ctx context.Context, server *model.MCPServerConfig) error {
	if server.ID == 0 {
		server.ID = r.nextID
		r.nextID++
	}
	cp := *server
	r.mcps[server.ID] = &cp
	return nil
}

func (r *fakeSettingsRepo) GetMCPServer(ctx context.Context, id int64) (*model.MCPServerConfig, error) {
	if server := r.mcps[id]; server != nil {
		cp := *server
		return &cp, nil
	}
	return nil, nil
}

func (r *fakeSettingsRepo) ListMCPServers(ctx context.Context, filter dao.MCPServerFilter) ([]model.MCPServerConfig, error) {
	var out []model.MCPServerConfig
	for _, server := range r.mcps {
		if filter.OwnerID >= 0 && server.OwnerID != filter.OwnerID {
			continue
		}
		if filter.Scope != "" && server.Scope != filter.Scope {
			continue
		}
		if filter.AgentID >= 0 && server.AgentID != filter.AgentID {
			continue
		}
		if filter.ConversationID >= 0 && server.ConversationID != filter.ConversationID {
			continue
		}
		if filter.Enabled != nil && server.Enabled != *filter.Enabled {
			continue
		}
		out = append(out, *server)
	}
	return out, nil
}

func (r *fakeSettingsRepo) DeleteMCPServer(ctx context.Context, id int64) error {
	delete(r.mcps, id)
	return nil
}

func boolPtr(v bool) *bool { return &v }
