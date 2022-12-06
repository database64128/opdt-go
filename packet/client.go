package packet

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net/netip"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

// Client generates request packets and parses response packets.
type Client struct {
	aead cipher.AEAD
}

// NewClient creates a new client with the given PSK.
func NewClient(psk []byte) (*Client, error) {
	aead, err := chacha20poly1305.NewX(psk)
	if err != nil {
		return nil, err
	}
	return &Client{
		aead: aead,
	}, nil
}

// PutRequest writes a request packet to the first [RequestPacketSize] bytes of the given buffer.
func (c *Client) PutRequest(req []byte) {
	_ = req[RequestPacketSize-1]

	nonce := req[:chacha20poly1305.NonceSizeX]
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}

	plaintext := req[chacha20poly1305.NonceSizeX : RequestPacketSize-chacha20poly1305.Overhead]
	binary.BigEndian.PutUint64(plaintext, uint64(time.Now().Unix()))
	plaintext[8] = MessageTypeRequest
	c.aead.Seal(nonce, nonce, plaintext, nil)
}

// ParseResponse parses the response packet and returns the client IP and port.
func (c *Client) ParseResponse(resp []byte) (netip.AddrPort, error) {
	if len(resp) != ResponsePacketSize {
		return netip.AddrPort{}, ErrBadPacketSize
	}

	nonce := resp[:chacha20poly1305.NonceSizeX]
	ciphertext := resp[chacha20poly1305.NonceSizeX:]
	plaintext, err := c.aead.Open(ciphertext[:0], nonce, ciphertext, nil)
	if err != nil {
		return netip.AddrPort{}, err
	}

	if err = CheckUnixEpochTimestamp(plaintext); err != nil {
		return netip.AddrPort{}, err
	}

	if plaintext[8] != MessageTypeResponse {
		return netip.AddrPort{}, fmt.Errorf("%w: %d, expected %d", ErrBadMessageType, plaintext[8], MessageTypeResponse)
	}

	addr := netip.AddrFrom16(*(*[16]byte)(plaintext[9:])).Unmap()
	port := binary.BigEndian.Uint16(plaintext[25:])
	return netip.AddrPortFrom(addr, port), nil
}
