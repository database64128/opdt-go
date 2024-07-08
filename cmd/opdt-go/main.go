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
	clientServer   string
	clientPSK      byteSliceFlag
	clientBind     string
	clientInterval time.Duration
	clientAttempts int
	zapConf        string
	logLevel       zapcore.Level
)

func init() {
	flag.StringVar(&serverConfPath, "server", "", "Run as server using the specified config file")
	flag.StringVar(&clientServer, "client", "", "Run as client using the specified server address")
	flag.Var(&clientPSK, "clientPSK", "Pre-shared key in client mode")
	flag.StringVar(&clientBind, "clientBind", "", "Bind address in client mode (default: let system choose)")
	flag.DurationVar(&clientInterval, "clientInterval", 0, "Keep sending at specified interval in client mode")
	flag.IntVar(&clientAttempts, "clientAttempts", 5, "Number of attempts to send in client mode. Set to 0 to send indefinitely.")
	flag.StringVar(&zapConf, "zapConf", "", "Preset name or path to JSON configuration file for building the zap logger.\nAvailable presets: console (default), systemd, production, development")
	flag.TextVar(&logLevel, "logLevel", zapcore.InvalidLevel, "Override the logger configuration's log level.\nAvailable levels: debug, info, warn, error, dpanic, panic, fatal")
}

func main() {
	flag.Parse()

	serverMode := serverConfPath != ""
	clientMode := clientServer != ""
	if serverMode == clientMode {
		fmt.Fprintln(os.Stderr, "Either -server <path> or -client <address> must be specified.")
		flag.Usage()
		os.Exit(1)
	}

	var zc zap.Config

	switch zapConf {
	case "console", "":
		zc = logging.NewProductionConsoleConfig(false)
	case "systemd":
		zc = logging.NewProductionConsoleConfig(true)
	case "production":
		zc = zap.NewProductionConfig()
	case "development":
		zc = zap.NewDevelopmentConfig()
	default:
		if err := jsonhelper.OpenAndDecodeDisallowUnknownFields(zapConf, &zc); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to load zap logger config:", err)
			os.Exit(1)
		}
	}

	if logLevel != zapcore.InvalidLevel {
		zc.Level.SetLevel(logLevel)
	}

	logger, err := zc.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to build logger:", err)
		os.Exit(1)
	}
	defer logger.Sync()

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

		if err = s.Start(); err != nil {
			logger.Fatal("Failed to start server",
				zap.String("listenAddress", sc.ListenAddress),
				zap.Binary("psk", sc.PSK),
				zap.Error(err),
			)
		}

		logger.Info("Started server", zap.String("listenAddress", sc.ListenAddress))

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		logger.Info("Received signal, stopping...", zap.Stringer("signal", sig))
		s.Stop()
	}

	if clientMode {
		serverAddrPort, err := netip.ParseAddrPort(clientServer)
		if err != nil {
			logger.Fatal("Failed to parse server address",
				zap.String("serverAddress", clientServer),
				zap.Error(err),
			)
		}

		clientConfig := client.Config{
			ServerAddrPort: serverAddrPort,
			BindAddress:    clientBind,
			PSK:            clientPSK,
		}

		c, err := clientConfig.Client()
		if err != nil {
			logger.Fatal("Failed to initialize client",
				zap.String("serverAddress", clientServer),
				zap.String("bindAddress", clientBind),
				zap.Binary("psk", clientPSK),
				zap.Error(err),
			)
		}

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-ctx.Done()
			stop()
		}()

		if clientAttempts == 0 {
			resultCh, err := c.Run(ctx, clientInterval)
			if err != nil {
				logger.Fatal("Failed to start client",
					zap.String("serverAddress", clientServer),
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
