package client

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/database64128/opdt-go/conn"
	"github.com/database64128/opdt-go/packet"
)

const (
	defaultInterval        = 2 * time.Second
	defaultOneShotAttempts = 5
)

type Error struct {
	Message      string
	PeerAddrPort netip.AddrPort
	PacketLength int
	Err          error
}

func (e Error) Unwrap() error {
	return e.Err
}

func (e Error) Error() string {
	return fmt.Sprintf("message: %s, peer address: %s, packet length: %d, error: %s", e.Message, e.PeerAddrPort.String(), e.PacketLength, e.Err.Error())
}

type Result struct {
	ClientAddrPort netip.AddrPort
	Err            Error
}

func (r Result) IsOk() bool {
	return r.ClientAddrPort.IsValid()
}

func OkResult(clientAddrPort netip.AddrPort) Result {
	return Result{ClientAddrPort: clientAddrPort}
}

func ErrResult(err Error) Result {
	return Result{Err: err}
}

type Config struct {
	ServerAddrPort netip.AddrPort
	BindAddress    string
	PSK            []byte
}

func (c Config) Client() (*Client, error) {
	handler, err := packet.NewClient(c.PSK)
	if err != nil {
		return nil, err
	}
	pc, err := net.ListenPacket("udp", c.BindAddress)
	if err != nil {
		return nil, err
	}
	return &Client{
		serverAddrPort: c.ServerAddrPort,
		serverConn:     pc.(*net.UDPConn),
		handler:        handler,
	}, nil
}

type Client struct {
	serverAddrPort netip.AddrPort
	serverConn     *net.UDPConn
	handler        *packet.Client
}

func (c *Client) Get(ctx context.Context, interval time.Duration, attempts int) (netip.AddrPort, error) {
	if interval == 0 {
		interval = defaultInterval
	}
	if attempts == 0 {
		attempts = defaultOneShotAttempts
	}

	ctx, cancel := context.WithTimeout(ctx, interval*time.Duration(attempts))
	defer cancel()

	resultCh, err := c.Run(ctx, interval)
	if err != nil {
		return netip.AddrPort{}, err
	}

	var clientErr Error

	for result := range resultCh {
		if result.IsOk() {
			return result.ClientAddrPort, nil
		}
		clientErr = result.Err
	}

	if clientErr.Err == nil {
		return netip.AddrPort{}, context.DeadlineExceeded
	}
	return netip.AddrPort{}, clientErr
}

func (c *Client) Run(ctx context.Context, interval time.Duration) (<-chan Result, error) {
	if interval == 0 {
		interval = defaultInterval
	}

	if err := c.serverConn.SetReadDeadline(time.Time{}); err != nil {
		return nil, err
	}

	resultCh := make(chan Result)
	var wg sync.WaitGroup

	wg.Go(func() {
		reqBuf := make([]byte, packet.RequestPacketSize)

		for {
			c.handler.PutRequest(reqBuf)

			if _, err := c.serverConn.WriteToUDPAddrPort(reqBuf, c.serverAddrPort); err != nil {
				resultCh <- ErrResult(Error{Message: "failed to send request", PeerAddrPort: c.serverAddrPort, PacketLength: packet.RequestPacketSize, Err: err})
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
		}
	})

	wg.Go(func() {
		respBuf := make([]byte, packet.ResponsePacketSize)

		for {
			n, _, flags, packetSourceAddrPort, err := c.serverConn.ReadMsgUDPAddrPort(respBuf, nil)
			if err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					return
				}
				resultCh <- ErrResult(Error{Message: "failed to receive packet", PeerAddrPort: packetSourceAddrPort, PacketLength: n, Err: err})
				continue
			}
			if err = conn.ParseFlagsForError(flags); err != nil {
				resultCh <- ErrResult(Error{Message: "failed to receive packet", PeerAddrPort: packetSourceAddrPort, PacketLength: n, Err: err})
				continue
			}

			clientAddrPort, err := c.handler.ParseResponse(respBuf[:n])
			if err != nil {
				resultCh <- ErrResult(Error{Message: "failed to parse response", PeerAddrPort: packetSourceAddrPort, PacketLength: n, Err: err})
				continue
			}

			resultCh <- OkResult(clientAddrPort)

			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			default:
			}
		}
	})

	_ = context.AfterFunc(ctx, func() {
		c.serverConn.SetReadDeadline(conn.ALongTimeAgo)
		wg.Wait()
		close(resultCh)
	})

	return resultCh, nil
}

func (c *Client) Close() error {
	return c.serverConn.Close()
}
