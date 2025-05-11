package main

import (
	"context"
	"eventbus/config"
	"eventbus/internal/service"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	grpcserver "eventbus/internal/grpc"
	"eventbus/internal/middleware"

	pb "eventbus/internal/generated/eventbus"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

const (
	successExitCode = 0
	failExitCode    = 1
)

func main() {
	dir := flag.String("config-dir", "./config", "config dir")
	flag.Parse()

	cfg, err := config.LoadConfig(*dir)
	if err != nil {
		panic("cannot load config: " + err.Error())
	}

	zcfg := zap.NewProductionConfig()
	zcfg.Encoding = cfg.LogFormat
	if cfg.LogFile != "" {
		zcfg.OutputPaths = []string{cfg.LogFile}
		zcfg.ErrorOutputPaths = []string{cfg.LogFile}
	} else {
		zcfg.OutputPaths = []string{"stdout"}
		zcfg.ErrorOutputPaths = []string{"stderr"}
	}
	logger, _ := zcfg.Build()
	defer logger.Sync()

	logger.Info("starting service",
		zap.String("app", cfg.AppName),
		zap.Int("app_port", cfg.AppPort),
		zap.Int("buffer_size", cfg.BufferSize),
		zap.Duration("shutdown_timeout", cfg.ShutdownTimeout),
		zap.String("log_format", cfg.LogFormat), // новый ключ
		zap.String("log_file", cfg.LogFile),
	)
	sub := service.NewBroker(cfg.BufferSize)

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(middleware.LoggingUnary(logger)),
		grpc.StreamInterceptor(middleware.LoggingStream(logger)),
	)
	pb.RegisterPubSubServer(grpcServer, grpcserver.NewServer(sub, logger))

	addr := fmt.Sprintf(":%d", cfg.AppPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatal("failed to listen", zap.Error(err))
	}

	go func() {
		logger.Info("gRPC server listening", zap.String("address", addr))
		if err := grpcServer.Serve(lis); err != nil {
			logger.Fatal("gRPC serve error", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop
	logger.Info("shutdown signal received")

	grpcServer.GracefulStop()
	logger.Info("gRPC stopped, closing Pub/Sub bus")

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := sub.Close(ctx); err != nil {
		logger.Error("bus.Close error", zap.Error(err))
	} else {
		logger.Info("Pub/Sub bus closed gracefully")
	}
}
