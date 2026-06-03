// Package handler 实现 admin-service 的 Kitex RPC 入口。
package handler

import (
	adminsvc "ClaranAIM/internal/admin-service/service"
	"ClaranAIM/kitex_gen/admin"
	"context"
)

// AdminServiceImpl 把 Kitex 请求转给管理业务层。
type AdminServiceImpl struct {
	svc adminsvc.AdminService
}

func NewAdminServiceImpl(svc adminsvc.AdminService) admin.AdminService {
	return &AdminServiceImpl{svc: svc}
}

func (h *AdminServiceImpl) GetDashboard(ctx context.Context, req *admin.DashboardReq) (*admin.DashboardResp, error) {
	return h.svc.GetDashboard(ctx, req.GetAdminId())
}

func (h *AdminServiceImpl) ListUsers(ctx context.Context, req *admin.ListUsersReq) (*admin.ListUsersResp, error) {
	return h.svc.ListUsers(ctx, req)
}

func (h *AdminServiceImpl) ListGroups(ctx context.Context, req *admin.ListGroupsReq) (*admin.ListGroupsResp, error) {
	return h.svc.ListGroups(ctx, req)
}

func (h *AdminServiceImpl) UpdateGroupStatus(ctx context.Context, req *admin.UpdateGroupStatusReq) (*admin.UpdateGroupStatusResp, error) {
	return h.svc.UpdateGroupStatus(ctx, req)
}

func (h *AdminServiceImpl) ListFiles(ctx context.Context, req *admin.ListFilesReq) (*admin.ListFilesResp, error) {
	return h.svc.ListFiles(ctx, req)
}

func (h *AdminServiceImpl) ListAgents(ctx context.Context, req *admin.ListAgentsReq) (*admin.ListAgentsResp, error) {
	return h.svc.ListAgents(ctx, req)
}

func (h *AdminServiceImpl) ListBilling(ctx context.Context, req *admin.ListBillingReq) (*admin.ListBillingResp, error) {
	return h.svc.ListBilling(ctx, req)
}

func (h *AdminServiceImpl) ListReviews(ctx context.Context, req *admin.ListReviewsReq) (*admin.ListReviewsResp, error) {
	return h.svc.ListReviews(ctx, req)
}

func (h *AdminServiceImpl) ReviewItem(ctx context.Context, req *admin.ReviewReq) (*admin.ReviewResp, error) {
	return h.svc.ReviewItem(ctx, req)
}

func (h *AdminServiceImpl) ListMCPTraces(ctx context.Context, req *admin.ListMCPTracesReq) (*admin.ListMCPTracesResp, error) {
	return h.svc.ListMCPTraces(ctx, req)
}

func (h *AdminServiceImpl) SaveNotice(ctx context.Context, req *admin.SaveNoticeReq) (*admin.NoticeResp, error) {
	return h.svc.SaveNotice(ctx, req)
}

func (h *AdminServiceImpl) ListNotices(ctx context.Context, req *admin.ListNoticesReq) (*admin.ListNoticesResp, error) {
	return h.svc.ListNotices(ctx, req)
}

func (h *AdminServiceImpl) ListAuditLogs(ctx context.Context, req *admin.ListAuditLogsReq) (*admin.ListAuditLogsResp, error) {
	return h.svc.ListAuditLogs(ctx, req)
}
