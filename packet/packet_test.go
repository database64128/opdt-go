package packet

import (
	"crypto/rand"
	"net/netip"
	"testing"

	"golang.org/x/crypto/chacha20poly1305"
)

func TestClientServer(t *testing.T) {
	psk := make([]byte, chacha20poly1305.KeySize)
	if _, err := rand.Read(psk); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(psk)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(psk)
	if err != nil {
		t.Fatal(err)
	}
	req := make([]byte, RequestPacketSize)
	resp := make([]byte, ResponsePacketSize)
	clientAddrPort := netip.AddrPortFrom(netip.IPv6Unspecified(), 60000)

	client.PutRequest(req)
	if err = server.Handle(clientAddrPort, req, resp); err != nil {
		t.Fatal(err)
	}
	addrPort, err := client.ParseResponse(resp)
	if err != nil {
		t.Error(err)
	}
	if addrPort != clientAddrPort {
		t.Errorf("Got client address %s, expected %s", addrPort, clientAddrPort)
	}
}
