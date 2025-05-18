package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"schemaregistry/internal/rest"
	"syscall"
	"time"

	natsd "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type config struct {
	NATSURL      string
	HTTPAddr     string
	SchemaBucket string
	ConfigBucket string
	Debug        bool
	TestMode     bool
}

func (c *config) load() {
	flag.StringVar(&c.NATSURL, "nats-url", getEnv("NATS_URL", nats.DefaultURL), "NATS server URL")
	flag.StringVar(&c.HTTPAddr, "http-addr", getEnv("HTTP_ADDR", ":8081"), "HTTP server address")
	flag.StringVar(&c.SchemaBucket, "schema-bucket", getEnv("SCHEMA_BUCKET", "SCHEMAS"), "JetStream KV bucket for schemas")
	flag.StringVar(&c.ConfigBucket, "config-bucket", getEnv("CONFIG_BUCKET", "CONFIG"), "JetStream KV bucket for configs")
	flag.BoolVar(&c.Debug, "debug", getEnvBool("DEBUG", false), "Enable debug logging")
	flag.BoolVar(&c.TestMode, "test", getEnvBool("TEST_MODE", false), "Enable test mode with embedded NATS server")
}

type server struct {
	cfg          config
	js           nats.JetStreamContext
	kvSchemas    nats.KeyValue
	kvConfig     nats.KeyValue
	http         *http.Server
	natsServer   *natsd.Server
	embeddedNATS bool
}

func newServer(cfg config) *server {
	return &server{
		cfg:  cfg,
		http: &http.Server{Addr: cfg.HTTPAddr, Handler: rest.Routes()},
	}
}

func main() {
	cfg := config{}
	cfg.load()
	flag.Parse()

	// Configure logging
	logLevel := slog.LevelInfo
	if cfg.Debug {
		logLevel = slog.LevelDebug
	}

	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(logHandler))

	slog.Info("Starting schema registry server", "config", cfg)

	srv := newServer(cfg)
	if err := srv.setup(); err != nil {
		slog.Error("Failed to setup server", "error", err)
		// Continue with HTTP server even if NATS setup fails
		slog.Warn("Continuing with limited functionality (no persistent storage)")
	}

	// Initialize REST handlers with schema registry
	rest.Init(srv.kvSchemas, srv.kvConfig)

	go func() {
		slog.Info("HTTP server listening", "addr", cfg.HTTPAddr)
		if err := srv.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
			os.Exit(1)
		}
	}()

	srv.gracefulShutdown(5 * time.Second)
}

func (s *server) startEmbeddedNATS() error {
	slog.Info("Starting embedded NATS server for testing")

	tmpDir, err := os.MkdirTemp("", "nats-data-*")
	if err != nil {
		return fmt.Errorf("create temp directory: %w", err)
	}

	opts := &natsd.Options{
		JetStream:  true,
		Port:       4222,
		Host:       "127.0.0.1",
		StoreDir:   tmpDir,
		MaxPayload: 8 * 1024 * 1024, // 8MB
	}

	// Create the server
	ns, err := natsd.NewServer(opts)
	if err != nil {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("create embedded NATS server: %w", err)
	}

	// Start the server in a separate goroutine
	go ns.Start()

	// Wait for server to be ready
	if !ns.ReadyForConnections(5 * time.Second) {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("embedded NATS server failed to start")
	}

	// Wait for JetStream to be ready
	timeout := time.Now().Add(5 * time.Second)
	for time.Now().Before(timeout) {
		if ns.JetStreamEnabled() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !ns.JetStreamEnabled() {
		os.RemoveAll(tmpDir)
		return fmt.Errorf("JetStream failed to start")
	}

	slog.Info("Embedded NATS server started successfully")
	s.natsServer = ns
	s.embeddedNATS = true

	return nil
}

func (s *server) setup() error {
	slog.Debug("Connecting to NATS", "url", s.cfg.NATSURL)

	// Connect to NATS with more options for better error messages
	nc, err := nats.Connect(s.cfg.NATSURL,
		nats.Name("Schema Registry"),
		nats.Timeout(5*time.Second),
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			slog.Error("NATS error", "error", err)
		}),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Error("NATS disconnected", "error", err)
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			slog.Info("NATS reconnected")
		}),
	)

	// If connection fails and test mode is enabled, start embedded NATS server
	if err != nil && s.cfg.TestMode {
		slog.Info("Failed to connect to external NATS server, starting embedded server")

		if err := s.startEmbeddedNATS(); err != nil {
			return fmt.Errorf("start embedded NATS server: %w", err)
		}

		// Try to connect to the embedded server
		nc, err = nats.Connect(nats.DefaultURL,
			nats.Name("Schema Registry"),
			nats.Timeout(5*time.Second),
			nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
				slog.Error("NATS error", "error", err)
			}),
		)

		if err != nil {
			return fmt.Errorf("connect to embedded NATS: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("connect to NATS: %w", err)
	}

	slog.Info("Connected to NATS")

	// Create JetStream context
	slog.Debug("Creating JetStream context")
	s.js, err = nc.JetStream(nats.PublishAsyncMaxPending(256))
	if err != nil {
		return fmt.Errorf("JetStream context: %w", err)
	}

	// Create buckets with retries
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		slog.Debug("Setting up schema bucket", "name", s.cfg.SchemaBucket, "attempt", i+1)
		if s.kvSchemas, err = s.makeBucket(s.cfg.SchemaBucket, "Schema records"); err != nil {
			if i == maxRetries-1 {
				return fmt.Errorf("create schema bucket: %w", err)
			}
			slog.Debug("Retrying bucket creation", "error", err)
			time.Sleep(time.Second)
			continue
		}
		break
	}

	for i := 0; i < maxRetries; i++ {
		slog.Debug("Setting up config bucket", "name", s.cfg.ConfigBucket, "attempt", i+1)
		if s.kvConfig, err = s.makeBucket(s.cfg.ConfigBucket, "Config records"); err != nil {
			if i == maxRetries-1 {
				return fmt.Errorf("create config bucket: %w", err)
			}
			slog.Debug("Retrying bucket creation", "error", err)
			time.Sleep(time.Second)
			continue
		}
		break
	}

	slog.Info("NATS setup completed successfully")
	return nil
}

func (s *server) makeBucket(name, desc string) (nats.KeyValue, error) {
	kv, err := s.js.KeyValue(name)
	if err == nats.ErrBucketNotFound {
		slog.Debug("Bucket not found, creating", "name", name)
		return s.js.CreateKeyValue(&nats.KeyValueConfig{
			Bucket:      name,
			Description: desc,
			Storage:     nats.FileStorage,
			History:     5,
		})
	}
	return kv, err
}

func (s *server) gracefulShutdown(timeout time.Duration) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	slog.Info("Shutting down server...")
	if err := s.http.Shutdown(ctx); err != nil {
		slog.Error("Server shutdown error", "error", err)
	}

	// Shutdown the embedded NATS server if it's running
	if s.embeddedNATS && s.natsServer != nil {
		slog.Info("Shutting down embedded NATS server")
		s.natsServer.Shutdown()
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1" || v == "yes"
	}
	return def
}
