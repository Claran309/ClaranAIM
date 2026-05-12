package model

import "time"

type Bot struct {
	ID          int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string    `json:"name" gorm:"size:100;not null"`
	Type        string    `json:"type" gorm:"size:20;not null;default:internal"`
	Description string    `json:"description" gorm:"type:text"`
	ModelName   string    `json:"model_name" gorm:"size:100;not null"`
	APIKey      string    `json:"api_key" gorm:"size:255"`
	BaseURL     string    `json:"base_url" gorm:"size:255"`
	SystemPrompt string   `json:"system_prompt" gorm:"type:text"`
	SkillsDir   string    `json:"skills_dir" gorm:"size:255"`
	AgentRoot   string    `json:"agent_root" gorm:"size:255"`
	OwnerID     int64     `json:"owner_id" gorm:"index;not null"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (Bot) TableName() string {
	return "bots"
}

type BotRoute struct {
	ID           int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	BotID        int64     `json:"bot_id" gorm:"index;not null"`
	RoutePattern string    `json:"route_pattern" gorm:"size:255;not null"`
	RouteType    string    `json:"route_type" gorm:"size:50;not null;default:keyword"`
	Priority     int64     `json:"priority" gorm:"default:0"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (BotRoute) TableName() string {
	return "bot_routes"
}

type BillingRecord struct {
	ID         int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	BotID      int64     `json:"bot_id" gorm:"index;not null"`
	UserID     int64     `json:"user_id" gorm:"index;not null"`
	Action     string    `json:"action" gorm:"size:50;not null"`
	TokenCount int64     `json:"token_count" gorm:"default:0"`
	Cost       float64   `json:"cost" gorm:"default:0"`
	CreatedAt  time.Time `json:"created_at" gorm:"autoCreateTime"`
}

func (BillingRecord) TableName() string {
	return "billing_records"
}
