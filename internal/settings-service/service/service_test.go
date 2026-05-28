package service

import (
	"ClaranAIM/internal/settings-service/dao"
	"ClaranAIM/internal/settings-service/model"
	"context"
	"testing"
)

func TestSaveUserLLMProfileDoesNotExposeAPIKey(t *testing.T) {
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{})

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
	if stored.APIKey != "secret" {
		t.Fatalf("stored api key = %q, want secret", stored.APIKey)
	}
}

func TestSaveLLMProfileCanKeepOrClearExistingAPIKey(t *testing.T) {
	repo := newFakeSettingsRepo()
	svc := NewSettingsService(repo, DefaultLLMConfig{})
	created, _ := svc.SaveLLMProfile(context.Background(), 1001, SaveLLMProfileInput{Name: "default", BaseURL: "https://llm.example/v1", APIKey: "old", ModelName: "m1", APIKeyAction: APIKeyActionSet})

	_, err := svc.SaveLLMProfile(context.Background(), 1001, SaveLLMProfileInput{ID: created.ID, Name: "default", ModelName: "m2", APIKeyAction: APIKeyActionKeep})
	if err != nil {
		t.Fatalf("keep returned error: %v", err)
	}
	stored, _ := repo.GetLLMProfile(context.Background(), created.ID)
	if stored.APIKey != "old" || stored.ModelName != "m2" {
		t.Fatalf("keep stored profile = %#v", stored)
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

type fakeSettingsRepo struct {
	nextID  int64
	llms    map[int64]*model.LLMProfile
	prompts map[int64]*model.PromptTemplate
}

func newFakeSettingsRepo() *fakeSettingsRepo {
	return &fakeSettingsRepo{nextID: 1, llms: map[int64]*model.LLMProfile{}, prompts: map[int64]*model.PromptTemplate{}}
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

func boolPtr(v bool) *bool { return &v }
