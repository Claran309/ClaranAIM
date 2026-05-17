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

// NewSnowflake creates a generator bound to one worker ID.
func NewSnowflake(workerID int64) (*Snowflake, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, errors.New("workerID must be in [0, 1023]")
	}
	return &Snowflake{workerID: workerID}, nil
}

// NextID returns the next monotonic snowflake ID for this worker.
//
// Small clock rollback is tolerated by waiting; large rollback returns an error
// because generating IDs under a stale timestamp could break ordering guarantees.
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

func (s *Snowflake) waitNextMillis(last int64) int64 {
	ts := currentMillis()
	for ts <= last {
		ts = currentMillis()
	}
	return ts
}

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

var defaultSnowflake = mustDefaultSnowflake()

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
