package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/database64128/opdt-go/client"
	"github.com/database64128/opdt-go/jsonhelper"
	"github.com/database64128/opdt-go/logging"
	"github.com/database64128/opdt-go/server"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type byteSliceFlag []byte

func (b byteSliceFlag) String() string {
	return base64.StdEncoding.EncodeToString(b)
}

func (b *byteSliceFlag) Set(s string) error {
	bb, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	*b = bb
	return nil
}

var (
	serverConfPath string
	clientServer   netip.AddrPort
	clientPSK      byteSliceFlag
	clientBind     string
	clientInterval time.Duration
	clientAttempts int
	zapConf        string
	logLevel       zapcore.Level
)

func init() {
	flag.StringVar(&serverConfPath, "server", "", "Run as server using the specified config file")
	flag.TextVar(&clientServer, "client", netip.AddrPort{}, "Run as client using the specified server address")
	flag.Var(&clientPSK, "clientPSK", "Pre-shared key in client mode")
	flag.StringVar(&clientBind, "clientBind", "", "Bind address in client mode (default: let system choose)")
	flag.DurationVar(&clientInterval, "clientInterval", 0, "Keep sending at specified interval in client mode")
	flag.IntVar(&clientAttempts, "clientAttempts", 5, "Number of attempts to send in client mode. Set to 0 to send indefinitely.")
	flag.StringVar(&zapConf, "zapConf", "console", "Preset name or path to the JSON configuration file for building the zap logger.\nAvailable presets: console, console-nocolor, console-notime, systemd, production, development")
	flag.TextVar(&logLevel, "logLevel", zapcore.InfoLevel, "Log level for the console and systemd presets.\nAvailable levels: debug, info, warn, error, dpanic, panic, fatal")
}

func main() {
	flag.Parse()

	serverMode := serverConfPath != ""
	clientMode := clientServer.IsValid()
	if serverMode == clientMode {
		fmt.Fprintln(os.Stderr, "Either -server <path> or -client <address> must be specified.")
		flag.Usage()
		os.Exit(1)
	}

	logger, err := logging.NewZapLogger(zapConf, logLevel)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to build logger:", err)
		os.Exit(1)
	}
	defer logger.Sync()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()

	if serverMode {
		var sc server.Config
		if err = jsonhelper.OpenAndDecodeDisallowUnknownFields(serverConfPath, &sc); err != nil {
			logger.Fatal("Failed to load server config",
				zap.String("path", serverConfPath),
				zap.Error(err),
			)
		}

		s, err := sc.Server(logger)
		if err != nil {
			logger.Fatal("Failed to initialize server",
				zap.String("listenAddress", sc.ListenAddress),
				zap.Binary("psk", sc.PSK),
				zap.Error(err),
			)
		}

		if err = s.Start(ctx); err != nil {
			logger.Fatal("Failed to start server",
				zap.String("listenAddress", sc.ListenAddress),
				zap.Binary("psk", sc.PSK),
				zap.Error(err),
			)
		}

		logger.Info("Started server", zap.String("listenAddress", sc.ListenAddress))

		<-ctx.Done()
		s.Stop()
		logger.Info("Stopped server")
	}

	if clientMode {
		clientConfig := client.Config{
			ServerAddrPort: clientServer,
			BindAddress:    clientBind,
			PSK:            clientPSK,
		}

		c, err := clientConfig.Client()
		if err != nil {
			logger.Fatal("Failed to initialize client",
				zap.Stringer("serverAddress", clientServer),
				zap.String("bindAddress", clientBind),
				zap.Binary("psk", clientPSK),
				zap.Error(err),
			)
		}

		if clientAttempts == 0 {
			resultCh, err := c.Run(ctx, clientInterval)
			if err != nil {
				logger.Fatal("Failed to start client",
					zap.Stringer("serverAddress", clientServer),
					zap.String("bindAddress", clientBind),
					zap.Binary("psk", clientPSK),
					zap.Error(err),
				)
			}

			for result := range resultCh {
				if result.IsOk() {
					logger.Info("Got client address", zap.String("clientAddress", result.ClientAddrPort.String()))
				} else {
					logger.Warn("Failed to get client address", zap.Error(result.Err))
				}
			}
		} else {
			clientAddrPort, err := c.Get(ctx, clientInterval, clientAttempts)
			if err != nil {
				logger.Error("Failed to get client address", zap.Error(err))
			}
			logger.Info("Got client address", zap.String("clientAddress", clientAddrPort.String()))
		}
	}
}
