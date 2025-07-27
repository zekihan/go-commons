package session

import (
	"time"

	"github.com/google/uuid"

	log "github.com/sirupsen/logrus"
)

const (
	defaultSessionTimeout = 24 * time.Hour // Default session timeout of 24 hours
)

type Cookie struct {
	Created       time.Time
	ExpireTime    time.Time
	ID            string `json:"id"`
	Authenticated bool
}

func NewCookie() *Cookie {
	id, err := uuid.NewV7()
	if err != nil {
		log.Errorf("Failed to generate session ID")

		return nil
	}

	now := time.Now()

	return &Cookie{
		ID:            id.String(),
		Created:       now,
		Authenticated: false,
		ExpireTime:    now.Add(defaultSessionTimeout), // Default session timeout of 24 hours
	}
}

func (s *Cookie) IsExpired() bool {
	return s.ExpireTime.Before(time.Now())
}
