// Package service 实现 admin-service 的管理聚合逻辑。
package service

import (
	"ClaranAIM/internal/admin-service/dao"
	"ClaranAIM/internal/admin-service/model"
	"ClaranAIM/kitex_gen/admin"
	"ClaranAIM/kitex_gen/bot"
	"ClaranAIM/kitex_gen/bot/botservice"
	"ClaranAIM/kitex_gen/file"
	"ClaranAIM/kitex_gen/file/fileservice"
	"ClaranAIM/kitex_gen/group"
	"ClaranAIM/kitex_gen/group/groupservice"
	"ClaranAIM/kitex_gen/knowledge"
	"ClaranAIM/kitex_gen/knowledge/knowledgeservice"
	"ClaranAIM/kitex_gen/mcp_gateway"
	"ClaranAIM/kitex_gen/mcp_gateway/mcpgatewayservice"
	"ClaranAIM/kitex_gen/memory"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"ClaranAIM/kitex_gen/rag"
	"ClaranAIM/kitex_gen/rag/ragservice"
	"ClaranAIM/kitex_gen/user"
	"ClaranAIM/kitex_gen/user/userservice"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AdminService 定义管理后台聚合能力。
type AdminService interface {
	GetDashboard(ctx context.Context, adminID int64) (*admin.DashboardResp, error)
	ListUsers(ctx context.Context, req *admin.ListUsersReq) (*admin.ListUsersResp, error)
	ListGroups(ctx context.Context, req *admin.ListGroupsReq) (*admin.ListGroupsResp, error)
	UpdateGroupStatus(ctx context.Context, req *admin.UpdateGroupStatusReq) (*admin.UpdateGroupStatusResp, error)
	ListFiles(ctx context.Context, req *admin.ListFilesReq) (*admin.ListFilesResp, error)
	ListAgents(ctx context.Context, req *admin.ListAgentsReq) (*admin.ListAgentsResp, error)
	ListBilling(ctx context.Context, req *admin.ListBillingReq) (*admin.ListBillingResp, error)
	ListReviews(ctx context.Context, req *admin.ListReviewsReq) (*admin.ListReviewsResp, error)
	ReviewItem(ctx context.Context, req *admin.ReviewReq) (*admin.ReviewResp, error)
	ListMCPTraces(ctx context.Context, req *admin.ListMCPTracesReq) (*admin.ListMCPTracesResp, error)
	SaveNotice(ctx context.Context, req *admin.SaveNoticeReq) (*admin.NoticeResp, error)
	ListNotices(ctx context.Context, req *admin.ListNoticesReq) (*admin.ListNoticesResp, error)
	ListAuditLogs(ctx context.Context, req *admin.ListAuditLogsReq) (*admin.ListAuditLogsResp, error)
}

type Dependencies struct {
	Users     userservice.Client
	Groups    groupservice.Client
	Files     fileservice.Client
	Agents    botservice.Client
	Memory    memoryservice.Client
	Knowledge knowledgeservice.Client
	MCP       mcpgatewayservice.Client
	RAG       ragservice.Client
}

type adminServiceImpl struct {
	repo dao.Repository
	deps Dependencies
}

func NewAdminService(repo dao.Repository, deps Dependencies) AdminService {
	return &adminServiceImpl{repo: repo, deps: deps}
}

func (s *adminServiceImpl) GetDashboard(ctx context.Context, adminID int64) (*admin.DashboardResp, error) {
	users, userTotal := s.safeUsers(ctx, &user.AdminListUsersReq{IncludeSystem: true, Limit: 1})
	groups, groupTotal := s.safeGroups(ctx, &group.AdminListGroupsReq{Limit: 1})
	files, fileTotal := s.safeFiles(ctx, &file.ListFilesReq{Limit: 1})
	agents, agentTotal := s.safeAgents(ctx, &bot.ListBotsReq{})
	docs, docTotal := s.safeDocs(ctx, &rag.ListDocumentsReq{ViewerId: adminID, Limit: 1})
	_ = users
	_ = groups
	_ = files
	_ = agents
	_ = docs
	notices, _, _ := s.repo.ListNotices(ctx, true, 5, 0)
	audits, _, _ := s.repo.ListAuditLogs(ctx, dao.AuditFilter{Limit: 8})
	return &admin.DashboardResp{
		Success: true,
		Metrics: []*admin.AdminMetric{
			{Key: "users", Label: "用户", Value: fmt.Sprintf("%d", userTotal), Hint: "含系统 Agent 用户"},
			{Key: "groups", Label: "群聊", Value: fmt.Sprintf("%d", groupTotal), Hint: "全局群组"},
			{Key: "files", Label: "文件", Value: fmt.Sprintf("%d", fileTotal), Hint: "文件元数据"},
			{Key: "agents", Label: "Agent", Value: fmt.Sprintf("%d", agentTotal), Hint: "助手配置"},
			{Key: "documents", Label: "知识文档", Value: fmt.Sprintf("%d", docTotal), Hint: "RAG 文档"},
		},
		Notices:      toRPCNotices(notices),
		RecentAudits: toRPCAuditLogs(audits),
		Msg:          "ok",
	}, nil
}

func (s *adminServiceImpl) ListUsers(ctx context.Context, req *admin.ListUsersReq) (*admin.ListUsersResp, error) {
	resp, err := s.deps.Users.AdminListUsers(ctx, &user.AdminListUsersReq{
		Keyword:       req.GetKeyword(),
		Role:          req.GetRole(),
		Status:        req.GetStatus(),
		IncludeSystem: req.GetIncludeSystem(),
		Limit:         req.GetLimit(),
		Offset:        req.GetOffset(),
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.GetSuccess() {
		return &admin.ListUsersResp{Success: false, Msg: getUserMsg(resp)}, nil
	}
	out := make([]*admin.AdminUser, 0, len(resp.GetUsers()))
	for _, u := range resp.GetUsers() {
		out = append(out, &admin.AdminUser{Id: u.Id, Username: u.Username, Nickname: u.Nickname, Avatar: u.Avatar, Status: u.Status, Role: u.Role, IsSystem: u.IsSystem, CreatedAt: u.CreatedAt})
	}
	return &admin.ListUsersResp{Success: true, Users: out, Total: resp.GetTotal(), Msg: "ok"}, nil
}

func (s *adminServiceImpl) ListGroups(ctx context.Context, req *admin.ListGroupsReq) (*admin.ListGroupsResp, error) {
	resp, err := s.deps.Groups.AdminListGroups(ctx, &group.AdminListGroupsReq{Keyword: req.GetKeyword(), OwnerId: req.GetOwnerId(), Limit: req.GetLimit(), Offset: req.GetOffset()})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.GetSuccess() {
		return &admin.ListGroupsResp{Success: false, Msg: getGroupMsg(resp)}, nil
	}
	out := make([]*admin.AdminGroup, 0, len(resp.GetGroups()))
	for _, g := range resp.GetGroups() {
		out = append(out, toAdminGroup(g))
	}
	return &admin.ListGroupsResp{Success: true, Groups: out, Total: resp.GetTotal(), Msg: "ok"}, nil
}

func (s *adminServiceImpl) UpdateGroupStatus(ctx context.Context, req *admin.UpdateGroupStatusReq) (*admin.UpdateGroupStatusResp, error) {
	status := strings.ToLower(strings.TrimSpace(req.GetStatus()))
	if status != "active" && status != "banned" {
		return &admin.UpdateGroupStatusResp{Success: false, Msg: "status只能是active或banned"}, nil
	}
	resp, err := s.deps.Groups.AdminUpdateGroupStatus(ctx, &group.AdminUpdateGroupStatusReq{
		AdminId: req.GetAdminId(),
		GroupId: req.GetGroupId(),
		Status:  status,
		Reason:  req.GetReason(),
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.GetSuccess() {
		return &admin.UpdateGroupStatusResp{Success: false, Msg: getGroupStatusMsg(resp)}, nil
	}
	s.audit(ctx, req.GetAdminId(), "update_group_status", "group", strconv.FormatInt(req.GetGroupId(), 10), status+" "+req.GetReason())
	return &admin.UpdateGroupStatusResp{Success: true, Msg: resp.GetMsg(), Group: toAdminGroup(resp.GetGroup())}, nil
}

func (s *adminServiceImpl) ListFiles(ctx context.Context, req *admin.ListFilesReq) (*admin.ListFilesResp, error) {
	resp, err := s.deps.Files.ListFiles(ctx, &file.ListFilesReq{UploaderId: req.GetUploaderId(), FileType: req.GetFileType(), Limit: req.GetLimit(), Offset: req.GetOffset()})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.GetSuccess() {
		return &admin.ListFilesResp{Success: false, Msg: getFileMsg(resp)}, nil
	}
	out := make([]*admin.AdminFile, 0, len(resp.GetFiles()))
	for _, f := range resp.GetFiles() {
		out = append(out, &admin.AdminFile{FileId: f.FileId, FileName: f.FileName, FileType: f.FileType, FileSize: f.FileSize, ContentType: f.ContentType, FileUrl: f.FileUrl, UploaderId: f.UploaderId, CreatedAt: f.CreatedAt})
	}
	return &admin.ListFilesResp{Success: true, Files: out, Total: resp.GetTotal(), Msg: "ok"}, nil
}

func (s *adminServiceImpl) ListAgents(ctx context.Context, req *admin.ListAgentsReq) (*admin.ListAgentsResp, error) {
	resp, err := s.deps.Agents.ListBots(ctx, &bot.ListBotsReq{OwnerId: req.GetOwnerId(), Type: req.GetType()})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.GetSuccess() {
		return &admin.ListAgentsResp{Success: false, Msg: getBotMsg(resp)}, nil
	}
	all := resp.GetBots()
	offset := int(req.GetOffset())
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	end := offset + limit
	if offset > len(all) {
		offset = len(all)
	}
	if end > len(all) {
		end = len(all)
	}
	out := make([]*admin.AdminAgent, 0, end-offset)
	for _, b := range all[offset:end] {
		out = append(out, &admin.AdminAgent{Id: b.Id, Name: b.Name, Type: b.Type, ModelName: b.ModelName, OwnerId: b.OwnerId, AgentUserId: b.AgentUserId, IsActive: b.IsActive, ToolPolicy: b.ToolPolicy, CreatedAt: b.CreatedAt})
	}
	return &admin.ListAgentsResp{Success: true, Agents: out, Total: int64(len(all)), Msg: "ok"}, nil
}

func (s *adminServiceImpl) ListBilling(ctx context.Context, req *admin.ListBillingReq) (*admin.ListBillingResp, error) {
	resp, err := s.deps.Agents.GetBilling(ctx, &bot.GetBillingReq{BotId: req.GetBotId(), UserId: req.GetUserId(), Limit: req.GetLimit(), Offset: req.GetOffset()})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.GetSuccess() {
		return &admin.ListBillingResp{Success: false, Msg: getBillingMsg(resp)}, nil
	}
	out := make([]*admin.AdminBillingRecord, 0, len(resp.GetRecords()))
	var totalCost float64
	for _, r := range resp.GetRecords() {
		totalCost += r.Cost
		out = append(out, &admin.AdminBillingRecord{Id: r.Id, BotId: r.BotId, UserId: r.UserId, ConversationId: r.ConversationId, InputTokens: r.InputTokens, OutputTokens: r.OutputTokens, Cost: r.Cost, ModelName: r.ModelName, CreatedAt: r.CreatedAt})
	}
	return &admin.ListBillingResp{Success: true, Records: out, Total: resp.GetTotal(), TotalCost: totalCost, Msg: "ok"}, nil
}

func (s *adminServiceImpl) ListReviews(ctx context.Context, req *admin.ListReviewsReq) (*admin.ListReviewsResp, error) {
	source := strings.TrimSpace(req.GetSource())
	if source == "" {
		source = "all"
	}
	items := make([]*admin.AdminReviewItem, 0)
	var total int64
	if source == "all" || source == "memory" {
		resp, err := s.deps.Memory.ListCandidates(ctx, &memory.ListCandidatesReq{ViewerId: 0, Filter: &memory.CandidateFilter{Status: req.GetStatus(), Limit: req.GetLimit(), Offset: req.GetOffset()}})
		if err == nil && resp != nil && resp.GetSuccess() {
			total += resp.GetTotal()
			for _, c := range resp.GetCandidates() {
				items = append(items, &admin.AdminReviewItem{Id: c.Id, Source: "memory", Title: c.Title, Content: c.Content, Status: c.Status, CreatedAt: c.CreatedAt})
			}
		}
	}
	if source == "all" || source == "graph" {
		resp, err := s.deps.Knowledge.ListGraphReviewCandidates(ctx, &knowledge.ListGraphReviewCandidatesReq{ViewerId: 0, Status: req.GetStatus(), Limit: req.GetLimit(), Offset: req.GetOffset()})
		if err == nil && resp != nil && resp.GetSuccess() {
			total += resp.GetTotal()
			for _, c := range resp.GetCandidates() {
				items = append(items, &admin.AdminReviewItem{Id: c.Id, Source: "graph", Title: c.Name, Content: c.Reason, Status: c.Status, CreatedAt: c.CreatedAt})
			}
		}
	}
	return &admin.ListReviewsResp{Success: true, Items: items, Total: total, Msg: "ok"}, nil
}

func (s *adminServiceImpl) ReviewItem(ctx context.Context, req *admin.ReviewReq) (*admin.ReviewResp, error) {
	source := strings.ToLower(strings.TrimSpace(req.GetSource()))
	action := strings.ToLower(strings.TrimSpace(req.GetAction()))
	if action != "approve" && action != "accept" && action != "reject" && action != "rejected" {
		return &admin.ReviewResp{Success: false, Msg: "action只能是approve或reject"}, nil
	}
	if action == "accept" {
		action = "approve"
	}
	if action == "rejected" {
		action = "reject"
	}
	var ok bool
	var msg string
	switch source {
	case "memory":
		var resp *memory.CandidateActionResp
		var err error
		adminReviewerID := -req.GetAdminId()
		if action == "approve" {
			resp, err = s.deps.Memory.AcceptCandidate(ctx, &memory.CandidateActionReq{ViewerId: adminReviewerID, CandidateId: req.GetItemId()})
		} else {
			resp, err = s.deps.Memory.RejectCandidate(ctx, &memory.CandidateActionReq{ViewerId: adminReviewerID, CandidateId: req.GetItemId()})
		}
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return &admin.ReviewResp{Success: false, Msg: "memory-service未返回审核结果"}, nil
		}
		ok, msg = resp.GetSuccess(), resp.GetMsg()
	case "graph":
		resp, err := s.deps.Knowledge.ReviewGraphCandidate(ctx, &knowledge.ReviewGraphCandidateReq{ViewerId: -req.GetAdminId(), CandidateId: req.GetItemId(), Action: action, Note: req.GetNote()})
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return &admin.ReviewResp{Success: false, Msg: "knowledge-service未返回审核结果"}, nil
		}
		ok, msg = resp.GetSuccess(), resp.GetMsg()
	default:
		return &admin.ReviewResp{Success: false, Msg: "source只能是memory或graph"}, nil
	}
	if ok {
		s.audit(ctx, req.GetAdminId(), "review_"+action, source, strconv.FormatInt(req.GetItemId(), 10), req.GetNote())
	}
	return &admin.ReviewResp{Success: ok, Msg: msg}, nil
}

func (s *adminServiceImpl) ListMCPTraces(ctx context.Context, req *admin.ListMCPTracesReq) (*admin.ListMCPTracesResp, error) {
	resp, err := s.deps.MCP.ListToolCalls(ctx, &mcp_gateway.ListToolCallsReq{UserId: 0, AgentId: req.GetAgentId(), ConversationId: req.GetConversationId(), Limit: req.GetLimit(), Offset: req.GetOffset()})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.GetSuccess() {
		return &admin.ListMCPTracesResp{Success: false, Msg: getMCPMsg(resp)}, nil
	}
	out := make([]*admin.AdminMCPTrace, 0, len(resp.GetTraces()))
	for _, t := range resp.GetTraces() {
		out = append(out, &admin.AdminMCPTrace{Id: t.Id, TraceId: t.TraceId, UserId: t.UserId, AgentId: t.AgentId, ConversationId: t.ConversationId, ToolName: t.ToolName, Source: t.Source, ServerName: t.ServerName, Status: t.Status, LatencyMs: t.LatencyMs, ErrorMessage: t.ErrorMessage, CreatedAt: t.CreatedAt})
	}
	return &admin.ListMCPTracesResp{Success: true, Traces: out, Total: resp.GetTotal(), Msg: "ok"}, nil
}

func (s *adminServiceImpl) SaveNotice(ctx context.Context, req *admin.SaveNoticeReq) (*admin.NoticeResp, error) {
	notice := &model.SystemNotice{}
	if req.GetNoticeId() > 0 {
		existing, err := s.repo.GetNotice(ctx, req.GetNoticeId())
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return &admin.NoticeResp{Success: false, Msg: "公告不存在"}, nil
		}
		notice = existing
	}
	notice.Title = strings.TrimSpace(req.GetTitle())
	notice.Content = strings.TrimSpace(req.GetContent())
	notice.Level = defaultString(strings.TrimSpace(req.GetLevel()), "info")
	notice.Audience = defaultString(strings.TrimSpace(req.GetAudience()), "all")
	notice.Enabled = req.GetEnabled()
	notice.CreatedBy = req.GetAdminId()
	if notice.Title == "" {
		return &admin.NoticeResp{Success: false, Msg: "公告标题不能为空"}, nil
	}
	if err := s.repo.SaveNotice(ctx, notice); err != nil {
		return nil, err
	}
	s.audit(ctx, req.GetAdminId(), "save_notice", "notice", strconv.FormatInt(notice.ID, 10), notice.Title)
	return &admin.NoticeResp{Success: true, Notice: toRPCNotice(*notice), Msg: "保存成功"}, nil
}

func (s *adminServiceImpl) ListNotices(ctx context.Context, req *admin.ListNoticesReq) (*admin.ListNoticesResp, error) {
	rows, total, err := s.repo.ListNotices(ctx, req.GetIncludeDisabled(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	return &admin.ListNoticesResp{Success: true, Notices: toRPCNotices(rows), Total: total, Msg: "ok"}, nil
}

func (s *adminServiceImpl) ListAuditLogs(ctx context.Context, req *admin.ListAuditLogsReq) (*admin.ListAuditLogsResp, error) {
	rows, total, err := s.repo.ListAuditLogs(ctx, dao.AuditFilter{Action: req.GetAction(), TargetType: req.GetTargetType(), Limit: int(req.GetLimit()), Offset: int(req.GetOffset())})
	if err != nil {
		return nil, err
	}
	return &admin.ListAuditLogsResp{Success: true, Logs: toRPCAuditLogs(rows), Total: total, Msg: "ok"}, nil
}

func (s *adminServiceImpl) audit(ctx context.Context, adminID int64, action, targetType, targetID, detail string) {
	if s.repo == nil {
		return
	}
	_ = s.repo.CreateAudit(ctx, &model.AdminAuditLog{AdminID: adminID, Action: action, TargetType: targetType, TargetID: targetID, Detail: detail})
}

func (s *adminServiceImpl) safeUsers(ctx context.Context, req *user.AdminListUsersReq) ([]*user.User, int64) {
	resp, err := s.deps.Users.AdminListUsers(ctx, req)
	if err != nil || resp == nil || !resp.GetSuccess() {
		return nil, 0
	}
	return resp.GetUsers(), resp.GetTotal()
}

func (s *adminServiceImpl) safeGroups(ctx context.Context, req *group.AdminListGroupsReq) ([]*group.Group, int64) {
	resp, err := s.deps.Groups.AdminListGroups(ctx, req)
	if err != nil || resp == nil || !resp.GetSuccess() {
		return nil, 0
	}
	return resp.GetGroups(), resp.GetTotal()
}

func (s *adminServiceImpl) safeFiles(ctx context.Context, req *file.ListFilesReq) ([]*file.FileInfo, int64) {
	resp, err := s.deps.Files.ListFiles(ctx, req)
	if err != nil || resp == nil || !resp.GetSuccess() {
		return nil, 0
	}
	return resp.GetFiles(), resp.GetTotal()
}

func (s *adminServiceImpl) safeAgents(ctx context.Context, req *bot.ListBotsReq) ([]*bot.BotInfo, int64) {
	resp, err := s.deps.Agents.ListBots(ctx, req)
	if err != nil || resp == nil || !resp.GetSuccess() {
		return nil, 0
	}
	return resp.GetBots(), int64(len(resp.GetBots()))
}

func (s *adminServiceImpl) safeDocs(ctx context.Context, req *rag.ListDocumentsReq) ([]*rag.RAGDocument, int64) {
	resp, err := s.deps.RAG.ListDocuments(ctx, req)
	if err != nil || resp == nil || !resp.GetSuccess() {
		return nil, 0
	}
	return resp.GetDocuments(), resp.GetTotal()
}

func toRPCNotices(rows []model.SystemNotice) []*admin.SystemNotice {
	out := make([]*admin.SystemNotice, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRPCNotice(row))
	}
	return out
}

func toRPCNotice(row model.SystemNotice) *admin.SystemNotice {
	return &admin.SystemNotice{Id: row.ID, Title: row.Title, Content: row.Content, Level: row.Level, Audience: row.Audience, Enabled: row.Enabled, CreatedBy: row.CreatedBy, CreatedAt: formatTime(row.CreatedAt), UpdatedAt: formatTime(row.UpdatedAt)}
}

func toRPCAuditLogs(rows []model.AdminAuditLog) []*admin.AdminAuditLog {
	out := make([]*admin.AdminAuditLog, 0, len(rows))
	for _, row := range rows {
		out = append(out, &admin.AdminAuditLog{Id: row.ID, AdminId: row.AdminID, Action: row.Action, TargetType: row.TargetType, TargetId: row.TargetID, Detail: row.Detail, CreatedAt: formatTime(row.CreatedAt)})
	}
	return out
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02 15:04:05")
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func getUserMsg(resp *user.AdminListUsersResp) string {
	if resp == nil {
		return "user-service返回空响应"
	}
	return resp.GetMsg()
}

func getGroupMsg(resp *group.AdminListGroupsResp) string {
	if resp == nil {
		return "group-service返回空响应"
	}
	return resp.GetMsg()
}

func getGroupStatusMsg(resp *group.AdminUpdateGroupStatusResp) string {
	if resp == nil {
		return "group-service返回空响应"
	}
	return resp.GetMsg()
}

func toAdminGroup(g *group.Group) *admin.AdminGroup {
	if g == nil {
		return nil
	}
	status := g.GetStatus()
	if status == "" {
		status = "active"
	}
	return &admin.AdminGroup{Id: g.Id, Name: g.Name, OwnerId: g.OwnerId, Announcement: g.Announcement, CreatedAt: g.CreatedAt, Status: status}
}

func getFileMsg(resp *file.ListFilesResp) string {
	if resp == nil {
		return "file-service返回空响应"
	}
	return resp.GetMsg()
}

func getBotMsg(resp *bot.ListBotsResp) string {
	if resp == nil {
		return "agent-manager-service返回空响应"
	}
	return resp.GetMsg()
}

func getBillingMsg(resp *bot.GetBillingResp) string {
	if resp == nil {
		return "agent-manager-service返回空计费响应"
	}
	return resp.GetMsg()
}

func getMCPMsg(resp *mcp_gateway.ListToolCallsResp) string {
	if resp == nil {
		return "mcp-gateway-service返回空响应"
	}
	return resp.GetMsg()
}

func debugJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
