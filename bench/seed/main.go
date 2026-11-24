// Command seed 生成 ClaranAIM 压测所需的确定性种子数据。
// 只连接 bench 专用库；重复执行使用 upsert 或按前缀清理，避免污染开发库。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	groupmodel "ClaranAIM/internal/group-service/model"
	msgmodel "ClaranAIM/internal/msg-core-service/model"
	usermodel "ClaranAIM/internal/user-service/model"
	"ClaranAIM/pkg/outbox"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	userBaseID  int64 = 1_100_000_001
	groupBaseID int64 = 8_100_000_001
	convBaseID  int64 = 7_300_000_000_000_000_000
	// 种子消息 ID 必须小于当前雪花 ID（约 1.18e17），否则新消息会排到历史后面。
	messageBaseID int64 = 1_000_000_000_000_000
	recordBaseID  int64 = 6_500_000_000_000_000_000
)

// benchPassword 只用于本地 bench 账号，可通过 BENCH_PASSWORD 覆盖。
func benchPassword() string {
	if v := strings.TrimSpace(os.Getenv("BENCH_PASSWORD")); v != "" {
		return v
	}
	return "bench-local-pass"
}

type manifest struct {
	RunID     string              `json:"run_id"`
	CreatedAt string              `json:"created_at"`
	Users     []manifestUser      `json:"users"`
	Private   []manifestConv      `json:"private_conversations"`
	Groups    []manifestGroupConv `json:"group_conversations"`
}

type manifestUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type manifestConv struct {
	ID           int64   `json:"id"`
	Participants []int64 `json:"participants"`
}

type manifestGroupConv struct {
	ID           int64   `json:"id"`
	GroupID      int64   `json:"group_id"`
	Participants []int64 `json:"participants"`
}

func main() {
	dsn := flag.String("dsn", os.Getenv("MYSQL_DSN"), "bench MySQL DSN")
	runID := flag.String("run-id", fmt.Sprintf("seed-%s", time.Now().Format("20060102-1504")), "run id prefix")
	userCount := flag.Int("users", 1000, "number of users")
	groupCount := flag.Int("groups", 100, "number of groups")
	messageCount := flag.Int("messages", 100000, "number of historical messages")
	reset := flag.Bool("reset", false, "delete bench rows before seeding")
	outDir := flag.String("out", "bench/results/seed", "manifest output directory")
	flag.Parse()

	if strings.TrimSpace(*dsn) == "" {
		log.Fatal("missing -dsn or MYSQL_DSN")
	}
	db, err := gorm.Open(mysql.Open(*dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("open mysql: %v", err)
	}
	if err := migrate(db); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	if *reset {
		if err := resetBench(db); err != nil {
			log.Fatalf("reset: %v", err)
		}
	}

	rng := rand.New(rand.NewSource(20261124))
	m := manifest{RunID: *runID, CreatedAt: time.Now().Format(time.RFC3339)}

	users := buildUsers(*userCount)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(users, 500).Error; err != nil {
		log.Fatalf("insert users: %v", err)
	}
	for _, u := range users {
		m.Users = append(m.Users, manifestUser{ID: u.ID, Username: u.Username})
	}
	log.Printf("users ready: %d", len(users))

	groups, members := buildGroups(*groupCount, users, rng)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(groups, 200).Error; err != nil {
		log.Fatalf("insert groups: %v", err)
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(members, 1000).Error; err != nil {
		log.Fatalf("insert group members: %v", err)
	}
	log.Printf("groups ready: %d, members: %d", len(groups), len(members))

	convs, participants, groupConvs, privateConvs := buildConversations(users, groups, members)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(convs, 500).Error; err != nil {
		log.Fatalf("insert conversations: %v", err)
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(participants, 1000).Error; err != nil {
		log.Fatalf("insert participants: %v", err)
	}
	m.Private = privateConvs
	m.Groups = groupConvs
	log.Printf("conversations ready: %d, participants: %d", len(convs), len(participants))

	messages, states := buildMessages(*runID, *messageCount, privateConvs, groupConvs, participants, rng)
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(messages, 1000).Error; err != nil {
		log.Fatalf("insert messages: %v", err)
	}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(states, 2000).Error; err != nil {
		log.Fatalf("insert message states: %v", err)
	}
	log.Printf("messages ready: %d, user states: %d", len(messages), len(states))

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(*outDir, "manifest.json")
	encoded, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		log.Fatalf("marshal manifest: %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		log.Fatalf("write manifest: %v", err)
	}
	log.Printf("manifest written: %s", path)
}

func migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&usermodel.User{}, &usermodel.Friend{}, &usermodel.FriendGroup{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(&groupmodel.Group{}, &groupmodel.GroupMember{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(
		&msgmodel.Conversation{},
		&msgmodel.ConversationParticipant{},
		&msgmodel.Message{},
		&msgmodel.MessageUserState{},
		&msgmodel.MessageEditRecord{},
		&msgmodel.MessageTranslation{},
	); err != nil {
		return err
	}
	return outbox.AutoMigrate(db)
}

// resetBench 只清空 bench 库中的业务表；不删除库、不触碰开发库。
func resetBench(db *gorm.DB) error {
	tables := []string{
		"message_user_states",
		"messages",
		"message_edit_records",
		"message_translations",
		"conversation_participants",
		"conversations",
		"group_members",
		"groups",
		"friends",
		"friend_groups",
		"users",
		"event_outbox",
		"processed_events",
	}
	for _, table := range tables {
		if !db.Migrator().HasTable(table) {
			continue
		}
		if err := db.Exec("DELETE FROM `" + table + "`").Error; err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	log.Printf("bench rows cleared")
	return nil
}

func buildUsers(count int) []usermodel.User {
	hash, err := bcrypt.GenerateFromPassword([]byte(benchPassword()), bcrypt.MinCost)
	if err != nil {
		log.Fatalf("bcrypt: %v", err)
	}
	users := make([]usermodel.User, 0, count)
	for i := 0; i < count; i++ {
		id := userBaseID + int64(i)
		users = append(users, usermodel.User{
			ID:       id,
			Username: fmt.Sprintf("bench-user-%04d", i+1),
			Password: string(hash),
			Nickname: fmt.Sprintf("Bench %04d", i+1),
			Role:     "user",
			Status:   "offline",
		})
	}
	return users
}

func buildGroups(count int, users []usermodel.User, rng *rand.Rand) ([]groupmodel.Group, []groupmodel.GroupMember) {
	groups := make([]groupmodel.Group, 0, count)
	var members []groupmodel.GroupMember
	seq := 0
	for i := 0; i < count; i++ {
		groupID := groupBaseID + int64(i)
		owner := users[i%len(users)].ID
		groups = append(groups, groupmodel.Group{
			ID:      groupID,
			Name:    fmt.Sprintf("Bench Group %03d", i+1),
			OwnerID: owner,
			Status:  "active",
		})
		members = append(members, groupmodel.GroupMember{
			ID:      recordBaseID + int64(seq),
			GroupID: groupID,
			UserID:  owner,
			Role:    "owner",
		})
		seq++
		size := 20 + rng.Intn(181)
		if size > len(users) {
			size = len(users)
		}
		seen := map[int64]struct{}{owner: {}}
		for len(seen) < size {
			candidate := users[rng.Intn(len(users))].ID
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			members = append(members, groupmodel.GroupMember{
				ID:      recordBaseID + int64(seq),
				GroupID: groupID,
				UserID:  candidate,
				Role:    "member",
			})
			seq++
		}
	}
	return groups, members
}

func buildConversations(users []usermodel.User, groups []groupmodel.Group, members []groupmodel.GroupMember) (
	[]msgmodel.Conversation,
	[]msgmodel.ConversationParticipant,
	[]manifestGroupConv,
	[]manifestConv,
) {
	membersByGroup := map[int64][]int64{}
	for _, m := range members {
		membersByGroup[m.GroupID] = append(membersByGroup[m.GroupID], m.UserID)
	}

	var convs []msgmodel.Conversation
	var participants []msgmodel.ConversationParticipant
	var groupConvs []manifestGroupConv
	var privateConvs []manifestConv
	seq := 0

	// 每个群一个群会话。
	for i, g := range groups {
		convID := convBaseID + int64(i)
		convs = append(convs, msgmodel.Conversation{ID: convID, Type: "group", GroupID: g.ID})
		memberIDs := membersByGroup[g.ID]
		for _, uid := range memberIDs {
			participants = append(participants, msgmodel.ConversationParticipant{
				ID:             recordBaseID + int64(1_000_000+seq),
				ConversationID: convID,
				UserID:         uid,
			})
			seq++
		}
		groupConvs = append(groupConvs, manifestGroupConv{ID: convID, GroupID: g.ID, Participants: memberIDs})
	}

	// 每个用户一条私聊，配成环状关系，保证 1000 个私聊会话。
	privateCount := len(users)
	for i := 0; i < privateCount; i++ {
		a := users[i].ID
		b := users[(i+1)%len(users)].ID
		if a == b {
			continue
		}
		convID := convBaseID + int64(len(groups)+i)
		convs = append(convs, msgmodel.Conversation{ID: convID, Type: "private"})
		for _, uid := range []int64{a, b} {
			participants = append(participants, msgmodel.ConversationParticipant{
				ID:             recordBaseID + int64(2_000_000+seq),
				ConversationID: convID,
				UserID:         uid,
			})
			seq++
		}
		privateConvs = append(privateConvs, manifestConv{ID: convID, Participants: []int64{a, b}})
	}
	return convs, participants, groupConvs, privateConvs
}

func buildMessages(runID string, total int, private []manifestConv, groups []manifestGroupConv, participants []msgmodel.ConversationParticipant, rng *rand.Rand) ([]msgmodel.Message, []msgmodel.MessageUserState) {
	if total <= 0 {
		return nil, nil
	}
	membersByConv := map[int64][]int64{}
	for _, p := range participants {
		membersByConv[p.ConversationID] = append(membersByConv[p.ConversationID], p.UserID)
	}

	type target struct {
		convID    int64
		members   []int64
		group     bool
		groupMsgs int
	}
	var targets []target
	// 群消息占 10%，私聊占 90%，保证私聊历史有足够深度。
	groupMessages := total / 10
	privateMessages := total - groupMessages
	if len(groups) > 0 {
		per := groupMessages / len(groups)
		for _, g := range groups {
			targets = append(targets, target{convID: g.ID, members: membersByConv[g.ID], group: true, groupMsgs: per})
		}
	}
	if len(private) > 0 {
		per := privateMessages / len(private)
		for _, c := range private {
			targets = append(targets, target{convID: c.ID, members: membersByConv[c.ID], group: false, groupMsgs: per})
		}
	}

	now := time.Now()
	var messages []msgmodel.Message
	var states []msgmodel.MessageUserState
	var msgSeq, stateSeq int64
	stateIDBase := recordBaseID + int64(3_000_000)
	for _, t := range targets {
		if len(t.members) == 0 {
			continue
		}
		for i := 0; i < t.groupMsgs; i++ {
			sender := t.members[rng.Intn(len(t.members))]
			msgID := messageBaseID + msgSeq
			created := now.Add(-time.Duration(total-int(msgSeq)) * time.Second)
			content := fmt.Sprintf("bench-%s-%d", runID, msgSeq)
			messages = append(messages, msgmodel.Message{
				ID:             msgID,
				ConversationID: t.convID,
				SenderID:       sender,
				Content:        content,
				MsgType:        "text",
				Status:         "sent",
				CreatedAt:      created,
			})
			// 群消息不写 per-user 状态，避免 100k 消息 × 百人群放大到百万行；
			// 历史查询使用 LEFT JOIN，没有状态行也能正常返回消息。
			stateTargets := t.members
			if t.group {
				stateTargets = nil
			}
			for _, uid := range stateTargets {
				state := msgmodel.MessageUserState{
					ID:             stateIDBase + stateSeq,
					ConversationID: t.convID,
					MessageID:      msgID,
					UserID:         uid,
					DeliveredAt:    &created,
				}
				if uid == sender {
					state.ReadAt = &created
				}
				states = append(states, state)
				stateSeq++
			}
			msgSeq++
		}
	}
	return messages, states
}
