package service

import (
	"ClaranAIM/kitex_gen/settings"
	"context"
	"errors"
	"fmt"
	"strings"
)

func (s *ragServiceImpl) route(ctx context.Context, input SearchInput) RouterDecision {
	routerInput := RouterInput{
		Query:          input.Query,
		DefaultMode:    s.defaultMode,
		GroupID:        input.GroupID,
		ConversationID: input.ConversationID,
	}
	router := s.resolveUserRouter(ctx, input.ViewerID)
	decision, err := router.Route(ctx, routerInput)
	if err != nil {
		fallback, _ := HybridAdaptiveRouter{}.Route(ctx, routerInput)
		fallback.Reason = "RAG Router不可用，已降级规则路由"
		return fallback
	}
	decision = normalizeRouterDecision(decision, s.defaultMode, input.Query)
	if !decision.Retrieve {
		decision.Mode = "direct"
	}
	return decision
}

func routerNote(decision RouterDecision) string {
	return fmt.Sprintf(
		"Adaptive Router: route=%s complexity=%s need_retrieve=%t sources=%s strategy=%s reason=%s",
		decision.Route,
		decision.Complexity,
		decision.Retrieve,
		strings.Join(decision.Sources, ","),
		decision.Strategy,
		defaultString(decision.Reason, "已选择检索路线"),
	)
}

func (s *ragServiceImpl) resolveUserRouter(ctx context.Context, viewerID int64) RAGRouter {
	defaultRouter := s.router
	if defaultRouter == nil {
		defaultRouter = RuleRouter{}
	}
	if s.settings == nil || viewerID <= 0 || s.routerFactory == nil {
		return defaultRouter
	}
	profiles, err := s.listRAGRouterProfiles(ctx, viewerID)
	if err != nil || len(profiles) == 0 {
		return defaultRouter
	}
	selected := profiles[0]
	for _, profile := range profiles {
		if profile.GetIsDefault() {
			selected = profile
			break
		}
	}
	if selected.GetId() <= 0 {
		return defaultRouter
	}
	resolved, err := s.resolveLLMProfile(ctx, viewerID, selected.GetId())
	if err != nil {
		return defaultRouter
	}
	if strings.TrimSpace(resolved.APIKey) == "" || strings.TrimSpace(resolved.BaseURL) == "" || strings.TrimSpace(resolved.ModelName) == "" {
		return defaultRouter
	}
	router := s.routerFactory(resolved)
	if router == nil {
		return defaultRouter
	}
	return fallbackRouter{primary: router, fallback: defaultRouter}
}

func (s *ragServiceImpl) listRAGRouterProfiles(ctx context.Context, ownerID int64) ([]*settings.LLMProfile, error) {
	if s == nil || s.settings == nil {
		return nil, errors.New("settings-service客户端未配置")
	}
	resp, err := s.settings.ListLLMProfiles(ctx, &settings.ListLLMProfilesReq{UserId: ownerID, UsageType: "rag_router"})
	if err != nil {
		return nil, err
	}
	if !resp.GetSuccess() {
		msg := strings.TrimSpace(resp.GetMsg())
		if msg == "" {
			msg = "settings-service查询RAG Router配置失败"
		}
		return nil, errors.New(msg)
	}
	return resp.GetProfiles(), nil
}

func (s *ragServiceImpl) resolveLLMProfile(ctx context.Context, ownerID, profileID int64) (resolvedLLMConfig, error) {
	if s == nil || s.settings == nil {
		return resolvedLLMConfig{}, errors.New("settings-service客户端未配置")
	}
	resp, err := s.settings.ResolveLLMProfile(ctx, &settings.ResolveLLMProfileReq{UserId: ownerID, ProfileId: profileID})
	if err != nil {
		return resolvedLLMConfig{}, err
	}
	if !resp.GetSuccess() {
		msg := strings.TrimSpace(resp.GetMsg())
		if msg == "" {
			msg = "settings-service解析LLM配置失败"
		}
		return resolvedLLMConfig{}, errors.New(msg)
	}
	return resolvedLLMConfig{
		ProfileID:      resp.GetProfileId(),
		APIKey:         resp.GetApiKey(),
		BaseURL:        resp.GetBaseUrl(),
		ModelName:      resp.GetModelName(),
		ProviderType:   resp.GetProviderType(),
		PromptTemplate: resp.GetPromptTemplate(),
	}, nil
}

type fallbackRouter struct {
	primary  RAGRouter
	fallback RAGRouter
}

func (r fallbackRouter) Route(ctx context.Context, input RouterInput) (RouterDecision, error) {
	decision, err := r.primary.Route(ctx, input)
	if err == nil {
		return decision, nil
	}
	if r.fallback == nil {
		return RouterDecision{}, err
	}
	return r.fallback.Route(ctx, input)
}
