package data

import "time"

type Plan struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URLs      []string  `json:"urls"`
	CreatedAt time.Time `json:"created_at"`
}
