// Package model 定义 user-service 的数据库模型
// 每个模型对应一张数据库表，通过 GORM 的 AutoMigrate 自动建表
package model

import (
	"ClaranAIM/pkg/idgen"
	"time"

	"gorm.io/gorm"
)

// User 用户表模型
// 存储用户的基本信息和认证数据
// Password 字段使用 json:"-" 防止在 API 响应中泄露密码
type User struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement:false"`     // 用户UID，10位数字，便于复制和添加好友
	Username  string    `json:"username" gorm:"uniqueIndex;size:50;not null"` // 用户名，唯一索引，用于登录
	Password  string    `json:"-" gorm:"size:255;not null"`                   // bcrypt 加密后的密码，不序列化到 JSON
	Nickname  string    `json:"nickname" gorm:"size:50"`                      // 昵称，显示名称
	Avatar    string    `json:"avatar" gorm:"size:255"`                       // 头像 URL
	Cover     string    `json:"cover" gorm:"size:255"`                        // 个人主页头图 URL
	Signature string    `json:"signature" gorm:"size:120"`                    // 个性签名，展示在个人资料页和资料卡
	Bio       string    `json:"bio" gorm:"size:500"`                          // 个人简介，承载更长的自我介绍
	Location  string    `json:"location" gorm:"size:80"`                      // 所在地
	Website   string    `json:"website" gorm:"size:255"`                      // 个人网站或主页链接
	Gender    string    `json:"gender" gorm:"size:20"`                        // 性别/展示身份，前端不做强约束
	Birthday  string    `json:"birthday" gorm:"size:20"`                      // 生日，使用 YYYY-MM-DD 字符串避免时区转换影响展示
	Email     string    `json:"email" gorm:"size:100"`                        // 邮箱
	Phone     string    `json:"phone" gorm:"size:20"`                         // 手机号
	Role      string    `json:"role" gorm:"size:20;default:user;index"`       // 系统角色：user/admin，用于管理层接口鉴权
	Status    string    `json:"status" gorm:"size:20;default:offline"`        // 在线状态：online/offline
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`             // 创建时间
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`             // 更新时间
}

// BeforeCreate 在用户创建前生成 10 位数字 UID。
//
// 用户 UID 不使用普通雪花 ID，是为了类似 QQ 号一样便于复制、搜索和添加好友。
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID != 0 {
		return nil
	}
	uid, err := idgen.NewUID10()
	if err != nil {
		return err
	}
	u.ID = uid
	return nil
}

// TableName 指定表名为 "users"
func (User) TableName() string {
	return "users"
}

// Friend 好友关系表模型
// 好友关系是双向的：A 添加 B 时会插入两条记录（A→B 和 B→A）
// 这样查询任一方的好友列表都只需一次查询
type Friend struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement:false"` // 记录ID，雪花ID
	UserID    int64     `json:"user_id" gorm:"index;not null"`            // 用户ID，索引
	FriendID  int64     `json:"friend_id" gorm:"index;not null"`          // 好友ID，索引
	GroupID   int64     `json:"group_id" gorm:"index"`                    // 好友分组ID，0 表示默认分组
	Remark    string    `json:"remark" gorm:"size:50"`                    // 好友备注名
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`         // 添加时间
}

// BeforeCreate 在好友关系写入前补充分布式雪花 ID。
func (f *Friend) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&f.ID)
}

// TableName 指定表名为 "friends"
func (Friend) TableName() string {
	return "friends"
}

// FriendGroup 好友分组表模型
// 用于对好友进行分类管理，如"同事""家人""同学"
type FriendGroup struct {
	ID        int64     `json:"id" gorm:"primaryKey;autoIncrement:false"` // 分组ID，雪花ID
	UserID    int64     `json:"user_id" gorm:"index;not null"`            // 所属用户ID，索引
	Name      string    `json:"name" gorm:"size:50;not null"`             // 分组名称
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`         // 创建时间
}

// BeforeCreate 在好友分组写入前补充分布式雪花 ID。
func (g *FriendGroup) BeforeCreate(tx *gorm.DB) error {
	return fillSnowflakeID(&g.ID)
}

// TableName 指定表名为 "friend_groups"
func (FriendGroup) TableName() string {
	return "friend_groups"
}

func fillSnowflakeID(id *int64) error {
	if *id != 0 {
		return nil
	}
	nextID, err := idgen.NextID()
	if err != nil {
		return err
	}
	*id = nextID
	return nil
}
