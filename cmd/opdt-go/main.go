package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"github.com/database64128/opdt-go/client"
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
	serverConfPath = flag.String("server", "", "Run as server using the specified config file")
	clientServer   = flag.String("client", "", "Run as client using the specified server address")
	clientBind     = flag.String("clientBind", "", "Bind address in client mode (default: let system choose)")
	clientInterval = flag.Duration("clientInterval", 0, "Keep sending at specified interval in client mode")
	clientAttempts = flag.Int("clientAttempts", 5, "Number of attempts to send in client mode. Set to 0 to send indefinitely.")
	zapConf        = flag.String("zapConf", "", "Preset name or path to JSON configuration file for building the zap logger.\nAvailable presets: console (default), systemd, production, development")
	logLevel       = flag.String("logLevel", "", "Override the logger configuration's log level.\nAvailable levels: debug, info, warn, error, dpanic, panic, fatal")
)

var clientPSK byteSliceFlag

func init() {
	flag.Var(&clientPSK, "clientPSK", "Pre-shared key in client mode")
}

func main() {
	flag.Parse()

	serverMode := *serverConfPath != ""
	clientMode := *clientServer != ""
	if serverMode == clientMode {
		fmt.Println("Either -server <path> or -client <address> must be specified.")
		flag.Usage()
		os.Exit(1)
	}

	var zc zap.Config

	switch *zapConf {
	case "console", "":
		zc = logging.NewProductionConsoleConfig(false)
	case "systemd":
		zc = logging.NewProductionConsoleConfig(true)
	case "production":
		zc = zap.NewProductionConfig()
	case "development":
		zc = zap.NewDevelopmentConfig()
	default:
		f, err := os.Open(*zapConf)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		d := json.NewDecoder(f)
		d.DisallowUnknownFields()
		err = d.Decode(&zc)
		f.Close()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	if *logLevel != "" {
		l, err := zapcore.ParseLevel(*logLevel)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		zc.Level.SetLevel(l)
	}

	logger, err := zc.Build()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer logger.Sync()

	if serverMode {
		f, err := os.Open(*serverConfPath)
		if err != nil {
			logger.Fatal("Failed to open config file",
				zap.Stringp("path", serverConfPath),
				zap.Error(err),
			)
		}

		d := json.NewDecoder(f)
		d.DisallowUnknownFields()
		var sc server.Config
		err = d.Decode(&sc)
		f.Close()
		if err != nil {
			logger.Fatal("Failed to decode config",
				zap.Stringp("path", serverConfPath),
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
		serverAddrPort, err := netip.ParseAddrPort(*clientServer)
		if err != nil {
			logger.Fatal("Failed to parse server address",
				zap.Stringp("serverAddress", clientServer),
				zap.Error(err),
			)
		}

		clientConfig := client.Config{
			ServerAddrPort: serverAddrPort,
			BindAddress:    *clientBind,
			PSK:            clientPSK,
		}

		c, err := clientConfig.Client()
		if err != nil {
			logger.Fatal("Failed to initialize client",
				zap.Stringp("serverAddress", clientServer),
				zap.Stringp("bindAddress", clientBind),
				zap.Binary("psk", clientPSK),
				zap.Error(err),
			)
		}

		ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

		if *clientAttempts == 0 {
			resultCh, err := c.Run(ctx, *clientInterval)
			if err != nil {
				logger.Fatal("Failed to start client",
					zap.Stringp("serverAddress", clientServer),
					zap.Stringp("bindAddress", clientBind),
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
			clientAddrPort, err := c.Get(ctx, *clientInterval, *clientAttempts)
			if err != nil {
				logger.Error("Failed to get client address", zap.Error(err))
			}
			logger.Info("Got client address", zap.String("clientAddress", clientAddrPort.String()))
		}
	}
}
