package service

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSubscribe(t *testing.T) {
	b := NewBroker(100)

	subj, err := b.Subscribe("test", func(msg interface{}) {})

	if err != nil {
		t.Fatalf("Failed subscribing: %v", err)
	}
	if subj == nil {
		t.Error("Subject is nil")
	}
}

func TestSubscribe_Success(t *testing.T) {
	b := NewBroker(100)
	defer b.Close(context.Background())

	done := make(chan struct{})
	sub, err := b.Subscribe("test", func(msg any) {
		close(done)
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}

	defer sub.Unsubscribe()

	b.Publish("test", "message wow")

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Error("Error receiving message")
	}
}

func TestPublishClosedSystem(t *testing.T) {
	b := NewBroker(100)
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := b.Publish("test", "message"); err != ErrClosed {
		t.Errorf("Expected ErrClosed, got %v", err)
	}
}

func TestOrder(t *testing.T) {
	b := NewBroker(10)
	defer b.Close(context.Background())

	var mutex sync.Mutex
	got := []int{}
	sub, _ := b.Subscribe("topic", func(msg any) {
		mutex.Lock()
		got = append(got, msg.(int))
		mutex.Unlock()
	})
	defer sub.Unsubscribe()

	for i := 1; i <= 5; i++ {
		if err := b.Publish("topic", i); err != nil {
			t.Fatalf("Publish %d failed: %v", i, err)
		}
	}
	time.Sleep(100 * time.Millisecond)

	for i := 1; i <= 5; i++ {
		if len(got) < i {
			t.Fatalf("Missing message %d, got only %v", i, got)
		}
		if got[i-1] != i {
			t.Errorf("Expected %d at position %d, got %d", i, i-1, got[i-1])
		}
	}
}

func TestUnsubscribeStopsMessages(t *testing.T) {
	b := NewBroker(10)
	defer b.Close(context.Background())

	received := make(chan struct{}, 1)
	sub, _ := b.Subscribe("topic", func(msg any) {
		received <- struct{}{}
	})
	sub.Unsubscribe()

	if err := b.Publish("topic", "hello"); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}
	select {
	case <-received:
		t.Error("Received message after Unsubscribe")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBufferOverflow(t *testing.T) {
	b := NewBroker(1)
	defer b.Close(context.Background())

	block := make(chan struct{})
	_, _ = b.Subscribe("topic", func(msg any) {
		<-block
	})
	if err := b.Publish("topic", "first"); err != nil {
		t.Fatalf("Publish first failed: %v", err)
	}
	if err := b.Publish("topic", "second"); err != ErrBufferFull {
		t.Errorf("Expected ErrBufferFull, got %v", err)
	}
	close(block)
}

func TestGracefulShutdown(t *testing.T) {
	b := NewBroker(10)
	ch := make(chan string, 1)
	_, _ = b.Subscribe("topic", func(msg any) {
		time.Sleep(200 * time.Millisecond)
		ch <- msg.(string)
	})
	_ = b.Publish("topic", "slow")

	start := time.Now()
	if err := b.Close(context.Background()); err != nil {
		t.Errorf("Expected successful Close, got %v", err)
	}
	if time.Since(start) < 200*time.Millisecond {
		t.Error("Close returned too early, did not wait for delivery")
	}
	select {
	case v := <-ch:
		if v != "slow" {
			t.Errorf("Expected 'slow', got %q", v)
		}
	case <-time.After(300 * time.Millisecond):
		t.Error("Message not delivered before shutdown")
	}
	b2 := NewBroker(10)
	_, _ = b2.Subscribe("topic", func(msg any) {
		time.Sleep(500 * time.Millisecond)
	})
	_ = b2.Publish("topic", "msg")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start = time.Now()
	err := b2.Close(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Expected DeadlineExceeded, got %v", err)
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Error("Close did not return immediately on timeout")
	}
}

func TestSubscribeAfterClose(t *testing.T) {
	b := NewBroker(1)
	_ = b.Close(context.Background())
	if _, err := b.Subscribe("foo", func(interface{}) {}); err != ErrClosed {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestCloseWithCanceledContext(t *testing.T) {
	b := NewBroker(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.Close(ctx); err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestDoubleClose(t *testing.T) {
	b := NewBroker(5)
	if err := b.Close(context.Background()); err != nil {
		t.Fatalf("First Close failed: %v", err)
	}
	if err := b.Close(context.Background()); err != ErrClosed {
		t.Errorf("Expected ErrClosed on second Close, got %v", err)
	}
}

func TestSubscribe_InvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		topic   string
		handler MessageHandler
		wantErr bool
	}{
		{"Empty topic", "", func(msg interface{}) {}, true},
		{"Nil handler", "valid", nil, true},
		{"Valid inputs", "valid", func(msg interface{}) {}, false},
	}

	b := NewBroker(10)
	defer b.Close(context.Background())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := b.Subscribe(tt.topic, tt.handler)
			if (err != nil) != tt.wantErr {
				t.Errorf("Subscribe(%q, %v) error = %v, wantErr %v",
					tt.topic, tt.handler, err, tt.wantErr)
			}
		})
	}
}

func TestConcurrentPublish(t *testing.T) {
	const messages = 100

	b := NewBroker(1000)
	defer b.Close(context.Background())

	var (
		counter int
		mu      sync.Mutex
		wg      sync.WaitGroup
	)

	wg.Add(messages)
	sub, err := b.Subscribe("test", func(msg interface{}) {
		mu.Lock()
		counter++
		mu.Unlock()
		wg.Done()
	})
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}
	defer sub.Unsubscribe()

	for i := 0; i < messages; i++ {
		go b.Publish("test", "message")
	}

	wg.Wait()

	if counter != messages {
		t.Errorf("Expected %d messages, got %d", messages, counter)
	}
}
