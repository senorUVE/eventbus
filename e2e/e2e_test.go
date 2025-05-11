package e2e

import (
	"context"
	"eventbus/internal/service"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	grpcserver "eventbus/internal/grpc"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"

	pb "eventbus/internal/generated/eventbus"
)

const bufSize = 1024 * 1024

func startTestServer(t *testing.T) *bufconn.Listener {
	lis := bufconn.Listen(bufSize)
	srv := grpc.NewServer()
	bus := service.NewBroker(1000)
	logger := zap.NewNop()
	pb.RegisterPubSubServer(srv, grpcserver.NewServer(bus, logger))

	t.Log("starting in-memory gRPC server")
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Fatalf("Server exited unexpectedly: %v", err)
		}
	}()
	return lis
}

func dialBuf(lis *bufconn.Listener) *grpc.ClientConn {
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithInsecure(),
	)
	if err != nil {
		panic(err)
	}
	return conn
}

func TestEndToEndPubSub_WithLogs(t *testing.T) {
	lis := startTestServer(t)
	conn := dialBuf(lis)
	defer conn.Close()
	client := pb.NewPubSubClient(conn)

	t.Log("calling Subscribe")
	stream, err := client.Subscribe(context.Background(), &pb.SubscribeRequest{Key: "orders"})
	if err != nil {
		t.Fatalf("Subscribe RPC failed: %v", err)
	}

	t.Log("calling Publish")
	if _, err := client.Publish(context.Background(), &pb.PublishRequest{
		Key:  "orders",
		Data: "order-123",
	}); err != nil {
		t.Fatalf("Publish RPC failed: %v", err)
	}

	t.Log("waiting for event")
	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv failed: %v", err)
	}
	t.Logf("got event: %q", msg.Data)

	t.Log("stopping server")
}

func TestLoadPubSub(t *testing.T) {
	const (
		numPublishers = 5
		msgsPerPub    = 200
	)
	totalMessages := numPublishers * msgsPerPub

	lis := startTestServer(t)
	conn := dialBuf(lis)
	defer conn.Close()
	client := pb.NewPubSubClient(conn)

	t.Log("starting subscriber")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := client.Subscribe(ctx, &pb.SubscribeRequest{Key: "load"})
	if err != nil {
		t.Fatalf("Subscribe RPC failed: %v", err)
	}

	var received int
	var recvMu sync.Mutex
	done := make(chan struct{})

	go func() {
		for {
			evt, err := stream.Recv()
			if err != nil {
				break
			}
			t.Logf("subscriber got: %q", evt.Data)
			recvMu.Lock()
			received++
			recvMu.Unlock()
		}
		close(done)
	}()

	t.Logf("launching %d publishers, each sending %d messages", numPublishers, msgsPerPub)
	var wg sync.WaitGroup
	wg.Add(numPublishers)
	for p := 0; p < numPublishers; p++ {
		go func(p int) {
			defer wg.Done()
			for i := 0; i < msgsPerPub; i++ {
				_, err := client.Publish(context.Background(), &pb.PublishRequest{
					Key:  "load",
					Data: fmt.Sprintf("pub%d-msg%d", p, i),
				})
				if err != nil {
					t.Errorf("Publish error: %v", err)
				}
			}
		}(p)
	}
	wg.Wait()

	t.Log("all publishers done, waiting for subscriber to catch up")
	time.Sleep(500 * time.Millisecond)

	cancel()
	<-done

	recvMu.Lock()
	got := received
	recvMu.Unlock()

	t.Logf("subscriber received %d/%d messages", got, totalMessages)
	if got != totalMessages {
		t.Errorf("Expected %d messages, got %d", totalMessages, got)
	}
}
