package domain

import (
	"context"
	"time"
)

type Document struct {
	ID     string    `json:"id"`
	Title  string    `json:"title"`
	Type   string    `json:"type"`
	Date   time.Time `json:"date"`
	Source string    `json:"source"`
}

type Source interface {
	Name() string
	Fetch(ctx context.Context, vin string) ([]Document, error)
}
