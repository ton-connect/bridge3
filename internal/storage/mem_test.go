package storage

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ton-connect/bridge3/internal/models"
)

func newMessage(expire time.Time, i int) message {
	return message{
		SseMessage: models.SseMessage{EventId: int64(i)},
		expireAt:   expire,
	}
}

func Test_removeExpiredMessages(t *testing.T) {

	now := time.Now()
	tests := []struct {
		name string
		ms   []message
		now  time.Time
		want []message
	}{
		{
			name: "all expired",
			ms: []message{
				newMessage(now.Add(2*time.Second), 1),
				newMessage(now.Add(3*time.Second), 2),
				newMessage(now.Add(4*time.Second), 3),
				newMessage(now.Add(5*time.Second), 4),
			},
			want: []message{},
			now:  now.Add(10 * time.Second),
		},
		{
			name: "some expired",
			ms: []message{
				newMessage(now.Add(10*time.Second), 1),
				newMessage(now.Add(9*time.Second), 2),
				newMessage(now.Add(2*time.Second), 3),
				newMessage(now.Add(1*time.Second), 4),
				newMessage(now.Add(5*time.Second), 5),
			},
			want: []message{
				newMessage(now.Add(10*time.Second), 1),
				newMessage(now.Add(9*time.Second), 2),
				newMessage(now.Add(5*time.Second), 5),
			},
			now: now.Add(4 * time.Second),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := removeExpiredMessages(tt.ms, tt.now); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("removeExpiredMessages() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStorage - removed as Add/GetMessages methods no longer exist
// Use TestMemStorage_PubSub and TestMemStorage_LastEventId instead

func TestMemStorage_watcher(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		db   map[string][]message
		want map[string][]message
	}{
		{
			db: map[string][]message{
				"1": {
					newMessage(now.Add(2*time.Second), 1),
					newMessage(now.Add(-2*time.Second), 2),
				},
				"2": {
					newMessage(now.Add(-1*time.Second), 4),
					newMessage(now.Add(-3*time.Second), 1),
				},
				"3": {
					newMessage(now.Add(1*time.Second), 4),
					newMessage(now.Add(3*time.Second), 1),
				},
			},
			want: map[string][]message{
				"1": {
					newMessage(now.Add(2*time.Second), 1),
				},
				"2": {},
				"3": {
					newMessage(now.Add(1*time.Second), 4),
					newMessage(now.Add(3*time.Second), 1),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &MemStorage{db: tt.db}
			go s.watcher()
			time.Sleep(500 * time.Millisecond)
			s.lock.Lock()
			defer s.lock.Unlock()

			if !reflect.DeepEqual(s.db, tt.want) {
				t.Errorf("GetMessages() = %v, want %v", message{}, tt.want)
			}
		})
	}
}

func TestMemStorage_PubSub(t *testing.T) {
	s := NewMemStorage()

	// Create channels to receive messages
	ch1 := make(chan models.SseMessage, 10)
	ch2 := make(chan models.SseMessage, 10)

	// Subscribe to different keys with lastEventId = 0 (get all messages)
	err := s.Sub(context.Background(), []string{"1"}, 0, ch1)
	if err != nil {
		t.Errorf("Sub() error = %v", err)
	}

	err = s.Sub(context.Background(), []string{"2"}, 0, ch2)
	if err != nil {
		t.Errorf("Sub() error = %v", err)
	}

	// Publish messages
	msg1 := models.SseMessage{EventId: 1, Message: []byte("msg1")}
	msg2 := models.SseMessage{EventId: 2, Message: []byte("msg2")}
	msg3 := models.SseMessage{EventId: 3, Message: []byte("msg3")}

	err = s.Pub(context.Background(), "1", 60, msg1)
	if err != nil {
		t.Errorf("Pub() error = %v", err)
	}

	err = s.Pub(context.Background(), "2", 60, msg2)
	if err != nil {
		t.Errorf("Pub() error = %v", err)
	}

	err = s.Pub(context.Background(), "1", 60, msg3)
	if err != nil {
		t.Errorf("Pub() error = %v", err)
	}

	// Check messages received
	select {
	case received := <-ch1:
		if received.EventId != 1 {
			t.Errorf("Expected EventId 1, got %d", received.EventId)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive message on ch1")
	}

	select {
	case received := <-ch1:
		if received.EventId != 3 {
			t.Errorf("Expected EventId 3, got %d", received.EventId)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive second message on ch1")
	}

	select {
	case received := <-ch2:
		if received.EventId != 2 {
			t.Errorf("Expected EventId 2, got %d", received.EventId)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected to receive message on ch2")
	}

	// Test unsubscribe
	err = s.Unsub(context.Background(), []string{"1"})
	if err != nil {
		t.Errorf("Unsub() error = %v", err)
	}

	// Publish another message - should not be received
	err = s.Pub(context.Background(), "1", 60, models.SseMessage{EventId: 4})
	if err != nil {
		t.Errorf("Pub() error = %v", err)
	}

	select {
	case <-ch1:
		t.Error("Should not receive message after unsubscribe")
	case <-time.After(100 * time.Millisecond):
		// Expected - no message should be received
	}
}

func TestMemStorage_LastEventId(t *testing.T) {
	s := NewMemStorage()

	// Store some messages first
	_ = s.Pub(context.Background(), "1", 60, models.SseMessage{EventId: 1})
	_ = s.Pub(context.Background(), "1", 60, models.SseMessage{EventId: 2})
	_ = s.Pub(context.Background(), "1", 60, models.SseMessage{EventId: 3})
	_ = s.Pub(context.Background(), "1", 60, models.SseMessage{EventId: 4})

	// Subscribe with lastEventId = 2 (should only get messages 3 and 4)
	ch := make(chan models.SseMessage, 10)
	err := s.Sub(context.Background(), []string{"1"}, 2, ch)
	if err != nil {
		t.Errorf("Sub() error = %v", err)
	}

	// Should receive messages with EventId > 2
	var receivedIds []int64
	timeout := time.After(200 * time.Millisecond)

	for {
		select {
		case msg := <-ch:
			receivedIds = append(receivedIds, msg.EventId)
		case <-timeout:
			goto done
		}
	}

done:
	expected := []int64{3, 4}
	if !reflect.DeepEqual(receivedIds, expected) {
		t.Errorf("Expected to receive messages %v, got %v", expected, receivedIds)
	}
}
