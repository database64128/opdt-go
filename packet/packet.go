package packet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	MessageTypeRequest = iota
	MessageTypeResponse
)

const (
	// random nonce + unix epoch timestamp + type + AEAD tag
	RequestPacketSize = chacha20poly1305.NonceSizeX + 8 + 1 + chacha20poly1305.Overhead

	// random nonce + unix epoch timestamp + type + IP + port + AEAD tag
	ResponsePacketSize = chacha20poly1305.NonceSizeX + 8 + 1 + 16 + 2 + chacha20poly1305.Overhead
)

const (
	// MaxEpochDiff is the maximum allowed time difference between a received timestamp and system time.
	MaxEpochDiff = 30

	// MaxTimeDiff is the maximum allowed time difference between a received timestamp and system time.
	MaxTimeDiff = MaxEpochDiff * time.Second

	// ReplayWindowDuration defines the amount of time during which a nonce check is necessary.
	ReplayWindowDuration = MaxTimeDiff * 2
)

var (
	ErrBadPacketSize  = errors.New("bad packet size")
	ErrRepeatedNonce  = errors.New("repeated nonce")
	ErrBadTimestamp   = errors.New("time offset too large")
	ErrBadMessageType = errors.New("bad message type")
)

// CheckUnixEpochTimestamp checks the Unix Epoch timestamp in the buffer
// and returns an error if the timestamp exceeds the allowed time difference from system time.
//
// This function does not check buffer length. Make sure it's at least 8 bytes long.
func CheckUnixEpochTimestamp(b []byte) error {
	tsEpoch := int64(binary.BigEndian.Uint64(b))
	nowEpoch := time.Now().Unix()
	diff := tsEpoch - nowEpoch
	if diff < -MaxEpochDiff || diff > MaxEpochDiff {
		return fmt.Errorf("%w: %d - %d (now) = %d", ErrBadTimestamp, tsEpoch, nowEpoch, diff)
	}
	return nil
}
