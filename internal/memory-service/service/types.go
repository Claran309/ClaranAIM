package service

// CreateMemoryInput 表示创建记忆事实的业务入参。
type CreateMemoryInput struct {
	BotID            int64
	UserID           int64
	OwnerUserID      int64
	GroupID          int64
	ConversationID   int64
	SessionID        string
	Scope            string
	Type             string
	Title            string
	Content          string
	Source           string
	Visibility       string
	Enabled          *bool
	VectorStatus     string
	EmbeddingRef     string
	Confidence       float64
	Importance       float64
	PreviousMemoryID int64
}

type UpdateMemoryInput struct {
	Scope        string
	Type         string
	Title        string
	Content      string
	Source       string
	Visibility   string
	Enabled      *bool
	VectorStatus string
	EmbeddingRef string
	Confidence   *float64
	Importance   *float64
}

type RecallInput struct {
	BotID            int64
	UserID           int64
	GroupID          int64
	ConversationID   int64
	SessionID        string
	Limit            int
	Query            string
	MinScore         float64
	VectorCandidateK int
	UseLLMFilter     bool
}

type RecallResult struct {
	Facts       []MemoryFact
	ContextText string
}

type CandidateInput struct {
	BotID              int64
	UserID             int64
	OwnerUserID        int64
	GroupID            int64
	ConversationID     int64
	SessionID          string
	Scope              string
	Type               string
	Title              string
	Content            string
	Source             string
	Evidence           string
	Confidence         float64
	Importance         float64
	ConflictMemoryIDs  []int64
	ConflictResolution string
}

type CandidateFilter struct {
	BotID  int64
	UserID int64
	Status string
	Limit  int
	Offset int
}

type MemoryFact struct {
	ID               int64
	BotID            int64
	UserID           int64
	OwnerUserID      int64
	GroupID          int64
	ConversationID   int64
	SessionID        string
	Scope            string
	Type             string
	Title            string
	Content          string
	Source           string
	Visibility       string
	Enabled          bool
	VectorStatus     string
	EmbeddingRef     string
	Confidence       float64
	Importance       float64
	VectorScore      float64
	FinalScore       float64
	ScoreReason      string
	ExpiredAt        string
	SupersededBy     int64
	PreviousMemoryID int64
	CreatedAt        string
	UpdatedAt        string
}

type MemoryCandidate struct {
	ID                 int64
	BotID              int64
	UserID             int64
	OwnerUserID        int64
	GroupID            int64
	ConversationID     int64
	SessionID          string
	Scope              string
	Type               string
	Title              string
	Content            string
	Source             string
	Evidence           string
	Confidence         float64
	Importance         float64
	Status             string
	ConflictMemoryIDs  []int64
	ConflictResolution string
	AcceptedMemoryID   int64
	CreatedAt          string
	UpdatedAt          string
}
