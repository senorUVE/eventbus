package service

import (
	"context"
	"sync"
)

type MessageHandler func(msg interface{})

type Subscription interface {
	Unsubscribe()
}

type SubPub interface {
	Subscribe(subject string, cb MessageHandler) (Subscription, error)
	Publish(subject string, msg interface{}) error
	Close(ctx context.Context) error
}

type subscription struct {
	subject    string
	subscriber *subscriber
	b          *broker
	once       sync.Once
}

type subscriber struct {
	handler MessageHandler
	message chan any
}

type broker struct {
	subjects   map[string][]*subscription
	closed     bool
	closedChan chan struct{}
	buf        int
	mutex      sync.RWMutex
	wg         sync.WaitGroup
}

func NewBroker(buf int) SubPub {
	return &broker{
		subjects:   make(map[string][]*subscription),
		closedChan: make(chan struct{}),
		buf:        buf,
	}
}

func (s *subscription) Unsubscribe() {
	s.once.Do(func() {
		broker := s.b
		broker.mutex.Lock()
		subs := broker.subjects[s.subject]

		for i, sub := range subs {
			if sub == s {
				broker.subjects[s.subject] = append(subs[:i], subs[i+1:]...)
				break
			}

		}
		broker.mutex.Unlock()
		close(s.subscriber.message)
	})
}

func (b *broker) Publish(subject string, msg interface{}) error {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	if b.closed {
		return ErrClosed
	}
	subs := append([]*subscription{}, b.subjects[subject]...)

	for _, sub := range subs {
		select {
		case sub.subscriber.message <- msg:
		default:
			return ErrBufferFull
		}
	}
	return nil
}

func (b *broker) Subscribe(subject string, cb MessageHandler) (Subscription, error) {
	if subject == "" || cb == nil {
		return nil, ErrInvalidInput
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	if b.closed {
		return nil, ErrClosed
	}

	subs := &subscription{
		subject: subject,
		subscriber: &subscriber{
			handler: cb,
			message: make(chan any, b.buf),
		},
		b: b,
	}
	b.subjects[subject] = append(b.subjects[subject], subs)

	b.wg.Add(1)

	go func() {
		defer b.wg.Done()
		for msg := range subs.subscriber.message {
			cb(msg)
		}
	}()

	return subs, nil
}

func (b *broker) Close(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	b.mutex.Lock()

	if b.closed {
		b.mutex.Unlock()
		return ErrClosed
	}
	b.closed = true

	var allSubs []*subscription
	for _, subs := range b.subjects {
		allSubs = append(allSubs, subs...)
	}
	b.subjects = make(map[string][]*subscription)

	b.mutex.Unlock()

	for _, sub := range allSubs {
		close(sub.subscriber.message)
	}

	done := make(chan struct{})
	go func() {
		b.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()

	}
}
