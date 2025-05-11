package grpc

import (
	"context"
	pb "eventbus/internal/generated/eventbus"
	"eventbus/internal/service"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type server struct {
	pb.UnimplementedPubSubServer
	eb     service.SubPub
	logger *zap.Logger
}

func NewServer(eb service.SubPub, logger *zap.Logger) *server {
	return &server{
		eb:     eb,
		logger: logger,
	}
}

func (s *server) Subscribe(req *pb.SubscribeRequest, stream pb.PubSub_SubscribeServer) error {
	if req.Key == "" {
		return status.Error(codes.InvalidArgument, "key is required")
	}
	server, err := s.eb.Subscribe(req.Key, func(msg interface{}) {
		ev := &pb.Event{Data: msg.(string)}
		if err := stream.Send(ev); err != nil {
			s.logger.Warn("stream send error", zap.Error(err), zap.String("key", req.Key))
		}
	})
	if err != nil {
		s.logger.Error("Subscribe failed", zap.String("key", req.Key), zap.Error(err))
		if err == service.ErrClosed {
			return status.Error(codes.Unavailable, err.Error())
		}
		return status.Error(codes.Internal, err.Error())
	}
	defer server.Unsubscribe()

	<-stream.Context().Done()
	return stream.Context().Err()
}

func (s *server) Publish(ctx context.Context, req *pb.PublishRequest) (*emptypb.Empty, error) {
	if req.Key == "" || req.Data == "" {
		return nil, status.Error(codes.InvalidArgument, "key and data are required")
	}
	err := s.eb.Publish(req.Key, req.Data)

	if err != nil {
		s.logger.Error("Publish failed", zap.String("key", req.Key), zap.Error(err))
		switch err {
		case service.ErrClosed:
			return nil, status.Error(codes.Unavailable, err.Error())
		case service.ErrBufferFull:
			return nil, status.Error(codes.ResourceExhausted, err.Error())
		default:
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &emptypb.Empty{}, nil
}
