package service

import (
	"ClaranAIM/kitex_gen/memory"
	"ClaranAIM/kitex_gen/memory/memoryservice"
	"context"
	"errors"
	"strings"
)

type AgentMemoryService interface {
	Recall(ctx context.Context, req *memory.RecallReq) (*memory.RecallResp, error)
	CreateMemory(ctx context.Context, req *memory.CreateMemoryReq) (*memory.CreateMemoryResp, error)
	ListMemories(ctx context.Context, req *memory.ListMemoriesReq) (*memory.ListMemoriesResp, error)
}

type AgentMemoryRPC struct {
	client memoryservice.Client
}

func NewAgentMemoryRPC(client memoryservice.Client) *AgentMemoryRPC {
	return &AgentMemoryRPC{client: client}
}

func (r *AgentMemoryRPC) Recall(ctx context.Context, req *memory.RecallReq) (*memory.RecallResp, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("memory-service客户端未配置")
	}
	resp, err := r.client.Recall(ctx, req)
	if err != nil {
		return nil, err
	}
	if !resp.GetSuccess() {
		return nil, memoryRPCError(resp.GetMsg())
	}
	return resp, nil
}

func (r *AgentMemoryRPC) CreateMemory(ctx context.Context, req *memory.CreateMemoryReq) (*memory.CreateMemoryResp, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("memory-service客户端未配置")
	}
	resp, err := r.client.CreateMemory(ctx, req)
	if err != nil {
		return nil, err
	}
	if !resp.GetSuccess() {
		return nil, memoryRPCError(resp.GetMsg())
	}
	return resp, nil
}

func (r *AgentMemoryRPC) ListMemories(ctx context.Context, req *memory.ListMemoriesReq) (*memory.ListMemoriesResp, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("memory-service客户端未配置")
	}
	resp, err := r.client.ListMemories(ctx, req)
	if err != nil {
		return nil, err
	}
	if !resp.GetSuccess() {
		return nil, memoryRPCError(resp.GetMsg())
	}
	return resp, nil
}

func memoryRPCError(msg string) error {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		msg = "memory-service RPC调用失败"
	}
	return errors.New(msg)
}
