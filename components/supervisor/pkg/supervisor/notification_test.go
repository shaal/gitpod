package supervisor

import (
	"context"
	"testing"
	"time"

	"github.com/gitpod-io/gitpod/supervisor/api"
	"google.golang.org/grpc"
)

type TestNotificationService_SubscribeServer struct {
	resps   chan *api.SubscribeResponse
	context context.Context
	cancel  context.CancelFunc
	grpc.ServerStream
}

func (subscribeServer *TestNotificationService_SubscribeServer) Send(resp *api.SubscribeResponse) error {
	subscribeServer.resps <- resp
	return nil
}

func (subscribeServer *TestNotificationService_SubscribeServer) Context() context.Context {
	return subscribeServer.context
}

func NewSubscribeServer() *TestNotificationService_SubscribeServer {
	context, cancel := context.WithCancel(context.Background())
	return &TestNotificationService_SubscribeServer{
		context: context,
		cancel:  cancel,
		resps:   make(chan *api.SubscribeResponse),
	}
}

func Test(t *testing.T) {
	t.Run("Test happy path", func(t *testing.T) {
		notificationService := NewNotificationService()
		subscriber := NewSubscribeServer()
		defer subscriber.cancel()
		go func() {
			notification := <-subscriber.resps
			notificationService.Respond(subscriber.context, &api.RespondRequest{
				RequestId: notification.RequestId,
				Response: &api.NotifyResponse{
					Action: notification.Request.Actions[0],
				},
			})
		}()
		go func() {
			notificationService.Subscribe(&api.SubscribeRequest{}, subscriber)
		}()
		notifyResponse, err := notificationService.Notify(subscriber.context, &api.NotifyRequest{
			Level:   api.NotifyRequest_INFO,
			Message: "Do you like this test?",
			Actions: []string{"yes", "no", "cancel"},
		})
		if err != nil {
			t.Errorf("error receiving user action %s", err)
		}
		if notifyResponse.Action != "yes" {
			t.Errorf("expected response 'yes' but was '%s'", notifyResponse.Action)
		}
	})

	t.Run("Notification without actions should return immediately", func(t *testing.T) {
		notificationService := NewNotificationService()
		notifyResponse, err := notificationService.Notify(context.Background(), &api.NotifyRequest{
			Level:   api.NotifyRequest_WARNING,
			Message: "You have been warned...",
		})
		if err != nil {
			t.Errorf("error receiving response %s", err)
		}
		if len(notifyResponse.Action) > 0 {
			t.Errorf("expected no response but got %s", notifyResponse.Action)
		}
	})

	t.Run("Late subscriber and pending notifications", func(t *testing.T) {
		notificationService := NewNotificationService()

		// fire notification without any subscribers
		_, err := notificationService.Notify(context.Background(), &api.NotifyRequest{
			Level:   api.NotifyRequest_INFO,
			Message: "Notification fired before subscription",
		})
		if err != nil {
			t.Errorf("error on notification %s", err)
		}

		// add a first subscriber
		firstSubscriber := NewSubscribeServer()
		defer firstSubscriber.cancel()

		// verify that late subscriber consumes cached notification
		go func() {
			notificationService.Subscribe(&api.SubscribeRequest{}, firstSubscriber)
		}()
		select {
		case _, ok := <-firstSubscriber.resps:
			if !ok {
				t.Errorf("notification stream closed")
			}
		case <-time.After(time.Second):
			t.Errorf("late subscriber did not receive pending notification")
		}

		// add a second subscriber
		secondSubscriber := NewSubscribeServer()
		defer secondSubscriber.cancel()
		go func() {
			notificationService.Subscribe(&api.SubscribeRequest{}, secondSubscriber)
		}()

		// Second subscriber should only get second message
		notificationService.Notify(context.Background(), &api.NotifyRequest{
			Level:   api.NotifyRequest_INFO,
			Message: "Notification fired before subscription",
		})
		go func() {
			// avoid blocking the delivery to the second subscriber
			<-firstSubscriber.resps
		}()
		request2, ok := <-secondSubscriber.resps
		if !ok {
			t.Errorf("notification stream closed")
		}
		if request2.Request.Message != "Notification fired before subscription" {
			t.Errorf("late subscriber received processed notification")
		}
	})

	t.Run("Wrong action is rejected", func(t *testing.T) {
		notificationService := NewNotificationService()

		// fire notification without any subscribers
		go func() {
			_, err := notificationService.Notify(context.Background(), &api.NotifyRequest{
				Level:   api.NotifyRequest_INFO,
				Message: "Notification with actions",
				Actions: []string{"ok"},
			})
			if err != nil {
				t.Errorf("error on notification %s", err)
			}
		}()

		// add a first subscriber
		subscriber := NewSubscribeServer()
		defer subscriber.cancel()
		go func() {
			notificationService.Subscribe(&api.SubscribeRequest{}, subscriber)
		}()

		// receive notification
		subscriptionRequest, ok := <-subscriber.resps
		if !ok {
			t.Errorf("notification stream closed")
		}

		// invalid reponse
		_, err := notificationService.Respond(context.Background(), &api.RespondRequest{
			RequestId: subscriptionRequest.RequestId,
			Response: &api.NotifyResponse{
				Action: "invalid",
			},
		})
		if err == nil {
			t.Errorf("expected error on invalid response")
		}

		// valid reponse
		_, err = notificationService.Respond(context.Background(), &api.RespondRequest{
			RequestId: subscriptionRequest.RequestId,
			Response: &api.NotifyResponse{
				Action: "ok",
			},
		})
		if err != nil {
			t.Errorf("error on valid response: %s", err)
		}

		// stale reponse
		_, err = notificationService.Respond(context.Background(), &api.RespondRequest{
			RequestId: subscriptionRequest.RequestId,
			Response: &api.NotifyResponse{
				Action: "ok",
			},
		})
		if err == nil {
			t.Errorf("expected error on stale response")
		}
	})
}
