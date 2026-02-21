package domain

import "time"

type User struct {
	ID               int64     `json:"id"`
	Age              int       `json:"age"`
	Country          string    `json:"country"`
	SubscriptionType string    `json:"subscription_type"`
	CreatedAt        time.Time `json:"created_at"`
}