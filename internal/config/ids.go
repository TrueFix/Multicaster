package config

import "github.com/google/uuid"

func NewDestID(existingCount int) string {
	return uuid.NewString()
}
