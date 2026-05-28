package idgen

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"net"
	"os"
	"sync"
	"time"
)

// 下面这组常量定义当前包使用的固定取值，集中声明可以避免业务代码中散落魔法字符串或魔法数字。
const (
	// twepoch 采用项目自定义纪元，减少生成 ID 的高位浪费。
	// 2026-01-01 00:00:00 UTC = 1767225600000ms。
	twepoch int64 = 1767225600000

	workerIDBits   uint8 = 10
	sequenceBits   uint8 = 12
	maxWorkerID          = int64(-1) ^ (int64(-1) << workerIDBits)
	sequenceMask         = int64(-1) ^ (int64(-1) << sequenceBits)
	workerIDShift        = sequenceBits
	timestampShift       = sequenceBits + workerIDBits

	maxClockBackoff = 5 * time.Millisecond

	minUID10 int64 = 1000000000
	maxUID10 int64 = 9999999999
)

// Snowflake 是一个本地并发安全的 64 位分布式 ID 生成器。
//
// 位布局参考 Twitter Snowflake / 美团 Leaf-snowflake：
//   - 41 bits: 毫秒时间戳差值
//   - 10 bits: workerID
//   - 12 bits: 同毫秒内序列号
//
// 在 Redis 可用时，workerID 应由 Redis 自增分配；Redis 不可用时可使用
// FallbackWorkerID 从主机名和网卡信息得到稳定兜底值。
type Snowflake struct {
	mu            sync.Mutex
	workerID      int64
	lastTimestamp int64
	sequence      int64
}

// NewSnowflake 创建绑定指定 workerID 的雪花 ID 生成器。
func NewSnowflake(workerID int64) (*Snowflake, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, errors.New("workerID must be in [0, 1023]")
	}
	return &Snowflake{workerID: workerID}, nil
}

// NextID 返回当前 worker 的下一个单调递增雪花 ID。
// 小幅时钟回拨会通过等待容忍；大幅回拨直接报错，因为用旧时间戳继续发号会破坏趋势递增和唯一性假设。
func (s *Snowflake) NextID() (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ts := currentMillis()
	if ts < s.lastTimestamp {
		backoff := time.Duration(s.lastTimestamp-ts) * time.Millisecond
		if backoff > maxClockBackoff {
			return 0, errors.New("clock moved backwards beyond tolerated window")
		}
		time.Sleep(backoff)
		ts = currentMillis()
		if ts < s.lastTimestamp {
			return 0, errors.New("clock moved backwards")
		}
	}

	if ts == s.lastTimestamp {
		s.sequence = (s.sequence + 1) & sequenceMask
		if s.sequence == 0 {
			ts = s.waitNextMillis(s.lastTimestamp)
		}
	} else {
		s.sequence = 0
	}

	s.lastTimestamp = ts
	return ((ts - twepoch) << timestampShift) | (s.workerID << workerIDShift) | s.sequence, nil
}

// waitNextMillis 自旋等待进入下一毫秒，用于同一毫秒内序列号耗尽的情况。
func (s *Snowflake) waitNextMillis(last int64) int64 {
	ts := currentMillis()
	for ts <= last {
		ts = currentMillis()
	}
	return ts
}

// currentMillis 返回当前 Unix 毫秒时间戳。
func currentMillis() int64 {
	return time.Now().UnixNano() / int64(time.Millisecond)
}

// NewUID10 生成 10 位数字用户 UID。调用方仍然需要在 users.id 上依靠唯一索引兜底。
func NewUID10() (int64, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	n := int64(binary.BigEndian.Uint64(b[:]) % uint64(maxUID10-minUID10+1))
	return minUID10 + n, nil
}

// FallbackWorkerID 根据主机名和网卡 MAC 做稳定 hash，用于 Redis 分配 workerID 失败时兜底。
func FallbackWorkerID() int64 {
	h := fnv.New32a()
	if hostname, err := os.Hostname(); err == nil {
		_, _ = h.Write([]byte(hostname))
	}
	ifaces, _ := net.Interfaces()
	for _, iface := range ifaces {
		_, _ = h.Write([]byte(iface.HardwareAddr.String()))
	}
	return int64(h.Sum32()) & maxWorkerID
}

// 下面这组变量保存当前包需要复用的运行时状态或配置入口，调用方应通过公开函数间接使用。
var defaultSnowflake = mustDefaultSnowflake()

// mustDefaultSnowflake 创建进程级默认雪花生成器；初始化失败说明 workerID 逻辑异常，应直接暴露问题。
func mustDefaultSnowflake() *Snowflake {
	g, err := NewSnowflake(FallbackWorkerID())
	if err != nil {
		panic(err)
	}
	return g
}

// NextID 使用进程内默认生成器产生雪花 ID，适合 GORM hook 和普通业务代码直接调用。
func NextID() (int64, error) {
	return defaultSnowflake.NextID()
}
