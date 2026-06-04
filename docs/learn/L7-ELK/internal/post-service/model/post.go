package model

type Post struct {
	PostID  int64  `json:"post_id" gorm:"column:post_id"`
	Owner   int64  `json:"owner" gorm:"column:owner"`
	Title   string `json:"title" gorm:"column:title"`
	Content string `json:"content" gorm:"column:content"`
}
