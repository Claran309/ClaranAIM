package model

type User struct {
	UserID   int64  `json:"user_id" gorm:"primary_key"`
	Username string `json:"username" gorm:"column:username"`
	Email    string `json:"email" gorm:"column:email"`
	Password string `json:"password" gorm:"column:password"`
}
