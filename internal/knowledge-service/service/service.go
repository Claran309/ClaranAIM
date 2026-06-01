// Package service 实现 knowledge-service 的知识图谱查询与可视化视图逻辑。
//
// 本服务不负责知识入库、embedding、RAG 检索或 GraphRAG indexing；这些仍由 rag-service
// 负责。knowledge-service 只读取底层图谱数据，整理成前端可视化需要的节点、边、社区、
// 详情和过滤 facet。Kitex handler 调用这里的接口，具体 GraphRAG 子图读取通过
// pkg/knowledgeclient.GraphSource 适配 rag-service RPC。
package service

import (
	"ClaranAIM/pkg/knowledgeclient"
	"context"
)

// KnowledgeService 是 knowledge-service 的业务接口。
type KnowledgeService interface {
	GetGraphView(ctx context.Context, viewerID int64, input knowledgeclient.GraphQuery) (*knowledgeclient.GraphView, error)
	GetNodeDetail(ctx context.Context, viewerID, nodeID int64, input knowledgeclient.GraphQuery) (*knowledgeclient.NodeDetail, error)
	GetEdgeDetail(ctx context.Context, viewerID, edgeID int64, input knowledgeclient.GraphQuery) (*knowledgeclient.EdgeDetail, error)
}

// NewKnowledgeService 创建知识图谱视图服务。
func NewKnowledgeService(source knowledgeclient.GraphSource) KnowledgeService {
	return knowledgeclient.NewRAGBackedService(source)
}
