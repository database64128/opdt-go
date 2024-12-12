package server

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/database64128/opdt-go/conn"
	"github.com/database64128/opdt-go/packet"
	"go.uber.org/zap"
)

type Config struct {
	ListenAddress string `json:"listen"`
	PSK           []byte `json:"psk"`
}

func (c Config) Server(logger *zap.Logger) (*Server, error) {
	handler, err := packet.NewServer(c.PSK)
	if err != nil {
		return nil, err
	}
	return &Server{
		listenAddress: c.ListenAddress,
		handler:       handler,
		logger:        logger,
	}, nil
}

type Server struct {
	listenAddress string
	serverConn    *net.UDPConn
	handler       *packet.Server
	logger        *zap.Logger
	wg            sync.WaitGroup
}

func (s *Server) Start(ctx context.Context) error {
	var lc net.ListenConfig
	serverConn, err := lc.ListenPacket(ctx, "udp", s.listenAddress)
	if err != nil {
		return err
	}
	s.serverConn = serverConn.(*net.UDPConn)

	s.wg.Add(1)

	go func() {
		s.recv()
		s.wg.Done()
	}()

	return nil
}

func (s *Server) recv() {
	reqBuf := make([]byte, packet.RequestPacketSize)
	respBuf := make([]byte, packet.ResponsePacketSize)

	var (
		n              int
		flags          int
		clientAddrPort netip.AddrPort
		err            error
	)

	for {
		n, _, flags, clientAddrPort, err = s.serverConn.ReadMsgUDPAddrPort(reqBuf, nil)
		if err != nil {
			if errors.Is(err, os.ErrDeadlineExceeded) {
				break
			}

			s.logger.Warn("Failed to receive packet",
				zap.Stringer("clientAddress", &clientAddrPort),
				zap.Int("packetLength", n),
				zap.Error(err),
			)
			continue
		}
		if err = conn.ParseFlagsForError(flags); err != nil {
			s.logger.Warn("Failed to receive packet",
				zap.Stringer("clientAddress", &clientAddrPort),
				zap.Int("packetLength", n),
				zap.Error(err),
			)
			continue
		}

		if err = s.handler.Handle(clientAddrPort, reqBuf[:n], respBuf); err != nil {
			s.logger.Warn("Failed to handle request",
				zap.Stringer("clientAddress", &clientAddrPort),
				zap.Int("packetLength", n),
				zap.Error(err),
			)
			continue
		}

		if _, err = s.serverConn.WriteToUDPAddrPort(respBuf, clientAddrPort); err != nil {
			s.logger.Warn("Failed to send response",
				zap.Stringer("clientAddress", &clientAddrPort),
				zap.Int("packetLength", n),
				zap.Error(err),
			)
			continue
		}

		s.logger.Info("Handled request", zap.Stringer("clientAddress", &clientAddrPort))
	}
}

func (s *Server) Stop() error {
	if s.serverConn == nil {
		return nil
	}

	if err := s.serverConn.SetReadDeadline(time.Now()); err != nil {
		return err
	}

	s.wg.Wait()

	return s.serverConn.Close()
}
