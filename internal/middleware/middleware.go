package middleware

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
)

func LoggingUnary(logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		start := time.Now()
		logger.Info("UnaryCall start", zap.String("method", info.FullMethod))
		resp, err = handler(ctx, req)
		logger.Info("UnaryCall end",
			zap.String("method", info.FullMethod),
			zap.Duration("took", time.Since(start)),
			zap.Error(err),
		)
		return resp, err
	}
}

func LoggingStream(logger *zap.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		logger.Info("StreamCall start", zap.String("method", info.FullMethod))
		err := handler(srv, ss)
		logger.Info("StreamCall end",
			zap.String("method", info.FullMethod),
			zap.Duration("took", time.Since(start)),
			zap.Error(err),
		)
		return err
	}
}
