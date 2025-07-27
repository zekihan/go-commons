package session

import (
	"testing"
	"time"
)

func TestIsExpired(t *testing.T) {
	now := time.Now()

	// Test case: Cookie is not expired
	cookie := &Cookie{
		ExpireTime: now.Add(1 * time.Hour),
	}
	if cookie.IsExpired() {
		t.Errorf("Expected cookie to not be expired")
	}

	// Test case: Cookie is expired
	cookie = &Cookie{
		ExpireTime: now.Add(-1 * time.Hour),
	}
	if !cookie.IsExpired() {
		t.Errorf("Expected cookie to be expired")
	}
}

func TestNewCookie(t *testing.T) {
	// Mock configuration
	cookie := NewCookie()

	if cookie == nil {
		t.Fatalf("Expected cookie to be created")
	}

	if cookie.ID == "" {
		t.Errorf("Expected cookie ID to be set")
	}

	if cookie.Authenticated {
		t.Errorf("Expected cookie to be unauthenticated")
	}

	if !cookie.Created.After(time.Now().Add(-1*time.Second)) ||
		!cookie.Created.Before(time.Now().Add(1*time.Second)) {
		t.Errorf("Expected cookie creation time to be close to now")
	}

	expireExpected := time.Now().Add(24 * time.Hour)
	if !cookie.ExpireTime.After(expireExpected.Add(-1*time.Second)) ||
		!cookie.ExpireTime.Before(expireExpected.Add(1*time.Second)) {
		t.Errorf("Expected cookie expiration time to match session timeout")
	}
}
