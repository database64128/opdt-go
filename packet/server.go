package packet

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net/netip"
	"time"

	"github.com/database64128/opdt-go/noncepool"
	"golang.org/x/crypto/chacha20poly1305"
)

// Server generates responses to request packets.
type Server struct {
	aead      cipher.AEAD
	noncePool *noncepool.NoncePool[[chacha20poly1305.NonceSizeX]byte]
}

// NewServer creates a new server with the given PSK.
func NewServer(psk []byte) (*Server, error) {
	aead, err := chacha20poly1305.NewX(psk)
	if err != nil {
		return nil, err
	}
	return &Server{
		aead:      aead,
		noncePool: noncepool.New[[chacha20poly1305.NonceSizeX]byte](ReplayWindowDuration),
	}, nil
}

// Handle processes the request packet and writes the response packet to the first [ResponsePacketSize] bytes of the given buffer.
func (s *Server) Handle(clientAddrPort netip.AddrPort, req []byte, resp []byte) error {
	_ = resp[ResponsePacketSize-1]

	// Process request.
	if len(req) != RequestPacketSize {
		return ErrBadPacketSize
	}

	nonce := req[:chacha20poly1305.NonceSizeX]
	reqNonce := *(*[chacha20poly1305.NonceSizeX]byte)(nonce)
	if !s.noncePool.Check(reqNonce) {
		return ErrRepeatedNonce
	}

	ciphertext := req[chacha20poly1305.NonceSizeX:]
	plaintext, err := s.aead.Open(ciphertext[:0], nonce, ciphertext, nil)
	if err != nil {
		return err
	}

	if err = CheckUnixEpochTimestamp(plaintext); err != nil {
		return err
	}

	s.noncePool.Add(reqNonce)

	if plaintext[8] != MessageTypeRequest {
		return fmt.Errorf("%w: %d, expected %d", ErrBadMessageType, plaintext[8], MessageTypeRequest)
	}

	// Generate response.
	nonce = resp[:chacha20poly1305.NonceSizeX]
	if _, err = rand.Read(nonce); err != nil {
		return err
	}

	plaintext = resp[chacha20poly1305.NonceSizeX : ResponsePacketSize-chacha20poly1305.Overhead]
	binary.BigEndian.PutUint64(plaintext, uint64(time.Now().Unix()))
	plaintext[8] = MessageTypeResponse
	*(*[16]byte)(plaintext[9:]) = clientAddrPort.Addr().As16()
	binary.BigEndian.PutUint16(plaintext[25:], clientAddrPort.Port())
	s.aead.Seal(nonce, nonce, plaintext, nil)
	return nil
}
