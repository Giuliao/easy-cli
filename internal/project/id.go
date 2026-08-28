package project

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// NewProjectID returns a UUID v4 string for project identification.
func NewProjectID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(fmt.Errorf("generate project id: %w", err))
	}
	// Set version 4 and variant bits per RFC 4122.
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// NewTaskID returns a short task identifier in the form tsk_YYYYMMDD_xxxxxx
// where xxxxxx is 6 hexadecimal random digits.
func NewTaskID() string {
	now := time.Now().UTC()
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Errorf("generate task id: %w", err))
	}
	return fmt.Sprintf("tsk_%s_%s", now.Format("20060102"), hex.EncodeToString(buf))
}
