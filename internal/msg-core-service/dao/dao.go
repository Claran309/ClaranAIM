package dao

import (
	"ClaranAIM/internal/msg-core-service/model"
	"context"
	"errors"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	models := []interface{}{
		&model.Conversation{},
		&model.ConversationParticipant{},
		&model.Message{},
	}

	for _, m := range models {
		if db.Migrator().HasTable(m) {
			if err := db.Migrator().DropTable(m); err != nil {
				return nil, err
			}
		}
		if err := db.AutoMigrate(m); err != nil {
			return nil, err
		}
	}

	return db, nil
}

type MessageRepository interface {
	CreateConversation(ctx context.Context, conv *model.Conversation) error
	GetConversationByID(ctx context.Context, id int64) (*model.Conversation, error)
	UpdateConversation(ctx context.Context, conv *model.Conversation) error
	AddParticipant(ctx context.Context, p *model.ConversationParticipant) error
	GetParticipants(ctx context.Context, conversationID int64) ([]model.ConversationParticipant, error)
	GetUserConversations(ctx context.Context, userID int64) ([]model.ConversationParticipant, error)
	CreateMessage(ctx context.Context, msg *model.Message) error
	GetMessages(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.Message, error)
	SearchMessages(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.Message, error)
	FindPrivateConversation(ctx context.Context, userID1, userID2 int64) (*model.Conversation, error)
}

type messageRepositoryImpl struct {
	db *gorm.DB
}

func NewMessageRepo(db *gorm.DB) MessageRepository {
	return &messageRepositoryImpl{db: db}
}

func (r *messageRepositoryImpl) CreateConversation(ctx context.Context, conv *model.Conversation) error {
	return r.db.WithContext(ctx).Create(conv).Error
}

func (r *messageRepositoryImpl) GetConversationByID(ctx context.Context, id int64) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&conv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &conv, err
}

func (r *messageRepositoryImpl) UpdateConversation(ctx context.Context, conv *model.Conversation) error {
	return r.db.WithContext(ctx).Save(conv).Error
}

func (r *messageRepositoryImpl) AddParticipant(ctx context.Context, p *model.ConversationParticipant) error {
	return r.db.WithContext(ctx).Create(p).Error
}

func (r *messageRepositoryImpl) GetParticipants(ctx context.Context, conversationID int64) ([]model.ConversationParticipant, error) {
	var participants []model.ConversationParticipant
	err := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).Find(&participants).Error
	return participants, err
}

func (r *messageRepositoryImpl) GetUserConversations(ctx context.Context, userID int64) ([]model.ConversationParticipant, error) {
	var participants []model.ConversationParticipant
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&participants).Error
	return participants, err
}

func (r *messageRepositoryImpl) CreateMessage(ctx context.Context, msg *model.Message) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

func (r *messageRepositoryImpl) GetMessages(ctx context.Context, conversationID int64, limit, beforeID int64) ([]model.Message, error) {
	var messages []model.Message
	query := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	err := query.Order("id DESC").Limit(int(limit)).Find(&messages).Error
	return messages, err
}

func (r *messageRepositoryImpl) SearchMessages(ctx context.Context, conversationIDs []int64, keyword string, limit int64) ([]model.Message, error) {
	var messages []model.Message
	err := r.db.WithContext(ctx).
		Where("conversation_id IN ?", conversationIDs).
		Where("content LIKE ?", "%"+keyword+"%").
		Order("id DESC").
		Limit(int(limit)).
		Find(&messages).Error
	return messages, err
}

func (r *messageRepositoryImpl) FindPrivateConversation(ctx context.Context, userID1, userID2 int64) (*model.Conversation, error) {
	var conv model.Conversation
	err := r.db.WithContext(ctx).
		Where("type = ?", "private").
		Where("id IN (SELECT conversation_id FROM conversation_participants WHERE user_id = ?) AND id IN (SELECT conversation_id FROM conversation_participants WHERE user_id = ?)", userID1, userID2).
		First(&conv).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &conv, err
}
