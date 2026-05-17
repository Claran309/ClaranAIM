// Package dtm 封装 DTM 分布式事务客户端。
//
// 当前项目已经用 Outbox 解决“业务数据库提交成功但 Kafka 发布前崩溃”的
// 高频事件可靠性问题。DTM 在这里作为补偿事务基础设施：适合后续接入低频、
// 跨服务且需要显式回滚/补偿的业务流程，例如付费开通、资源配额调整等。
package dtm

import (
	"ClaranAIM/pkg/idgen"
	"context"
	"fmt"
	"strings"

	"github.com/dtm-labs/dtmcli"
)

// Manager 保存 DTM Server 地址，并负责创建 Saga 构建器。
type Manager struct {
	server string
}

// NewManager 创建 DTM 管理器。server 通常为 http://localhost:36789。
func NewManager(server string) *Manager {
	return &Manager{server: strings.TrimRight(server, "/")}
}

// Server 返回当前管理器连接的 DTM Server 地址，便于启动日志和测试断言。
func (m *Manager) Server() string {
	return m.server
}

// NewSaga 创建一个 Saga 分布式事务构建器。
// 兼容旧调用方式：如果 DTM Server 不可用会 panic；新业务优先使用
// NewSagaWithServerGID 或 NewSagaLocal 显式处理错误。
func (m *Manager) NewSaga() *SagaBuilder {
	builder, err := m.NewSagaWithServerGID()
	if err != nil {
		panic(err)
	}
	return builder
}

// NewSagaWithServerGID 从 DTM Server 申请全局事务 ID。
// 调用方可以显式处理 DTM 不可用错误，避免基础设施未启动时发生 panic。
func (m *Manager) NewSagaWithServerGID() (*SagaBuilder, error) {
	gid, err := GenGID(m.server)
	if err != nil {
		return nil, err
	}
	return &SagaBuilder{
		server: m.server,
		gid:    gid,
		steps:  make([]SagaStep, 0),
	}, nil
}

// NewSagaLocal 使用项目雪花 ID 生成 DTM GID，不依赖 DTM Server 的 /newGid。
// 适合测试、离线构建，或业务已经有可靠全局 ID 来源的场景。
func (m *Manager) NewSagaLocal() (*SagaBuilder, error) {
	id, err := idgen.NextID()
	if err != nil {
		return nil, err
	}
	return &SagaBuilder{
		server: m.server,
		gid:    fmt.Sprintf("%d", id),
		steps:  make([]SagaStep, 0),
	}, nil
}

// GenGID 包装 dtmcli.MustGenGid，将 panic 转换为普通 error。
func GenGID(server string) (gid string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("生成DTM GID失败: %v", recovered)
		}
	}()
	gid = dtmcli.MustGenGid(server)
	if gid == "" {
		return "", fmt.Errorf("生成DTM GID失败: 空GID")
	}
	return gid, nil
}

// SagaStep 表示一个 Saga 分支：Action 是正向接口，Compensate 是补偿接口。
// Data 会由 dtmcli 序列化后作为请求体传给分支接口。
type SagaStep struct {
	Action     string
	Compensate string
	Data       interface{}
}

// SagaBuilder 用链式 API 收集 Saga 分支，最后 Submit 到 DTM Server。
type SagaBuilder struct {
	server string
	gid    string
	steps  []SagaStep
}

// AddStep 添加一个 Saga 分支。
func (s *SagaBuilder) AddStep(action, compensate string, data interface{}) *SagaBuilder {
	s.steps = append(s.steps, SagaStep{
		Action:     action,
		Compensate: compensate,
		Data:       data,
	})
	return s
}

// WithGID 允许调用方在测试或幂等重试场景下显式指定 GID。
func (s *SagaBuilder) WithGID(gid string) *SagaBuilder {
	s.gid = gid
	return s
}

// GID 返回当前 Saga 的全局事务 ID。
func (s *SagaBuilder) GID() string {
	return s.gid
}

// Steps 返回已添加的分支副本，避免调用方直接改内部切片。
func (s *SagaBuilder) Steps() []SagaStep {
	steps := make([]SagaStep, len(s.steps))
	copy(steps, s.steps)
	return steps
}

// Submit 提交 Saga 到 DTM Server。dtmcli 当前 Submit API 不接收 context，
// 这里保留 ctx 参数是为了让业务层接口先稳定下来，后续可以统一加 tracing。
func (s *SagaBuilder) Submit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	saga := dtmcli.NewSaga(s.server, s.gid)
	for _, step := range s.steps {
		saga.Add(step.Action, step.Compensate, step.Data)
	}
	return saga.Submit()
}

// BuildURL 根据服务地址和路径构造 DTM 分支接口 URL。
func BuildURL(host string, port int, path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return fmt.Sprintf("http://%s:%d%s", host, port, path)
}
