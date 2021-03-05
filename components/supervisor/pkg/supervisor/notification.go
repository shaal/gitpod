// Copyright (c) 2021 Gitpod GmbH. All rights reserved.
// Licensed under the GNU Affero General Public License (AGPL).
// See License-AGPL.txt in the project root for license information.

package supervisor

import (
	"context"
	"sync"

	"github.com/gitpod-io/gitpod/common-go/log"
	"github.com/gitpod-io/gitpod/supervisor/api"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	maxPendingNotifications = 100
)

func NewNotificationService() *NotificationService {
	return &NotificationService{
		subscriberChannels:   []chan *api.SubscribeResult{},
		pendingNotifications: make(map[uint64]*pendingNotification),
	}
}

// NotificationService implements the notification service API
type NotificationService struct {
	mutex                sync.Mutex
	subscriberChannels   []chan *api.SubscribeResult
	nextNotificationId   uint64
	pendingNotifications map[uint64]*pendingNotification
}

type pendingNotification struct {
	message         *api.SubscribeResult
	responseChannel chan *api.NotifyResponse
}

// RegisterGRPC registers a gRPC service
func (srv *NotificationService) RegisterGRPC(s *grpc.Server) {
	api.RegisterNotificationServiceServer(s, srv)
}

// RegisterREST registers a REST service
func (srv *NotificationService) RegisterREST(mux *runtime.ServeMux, grpcEndpoint string) error {
	return api.RegisterNotificationServiceHandlerFromEndpoint(context.Background(), mux, grpcEndpoint, []grpc.DialOption{grpc.WithInsecure()})
}

// Sends a notification to the user
func (srv *NotificationService) Notify(ctx context.Context, req *api.NotifyRequest) (*api.NotifyResponse, error) {
	if srv.nextNotificationId >= maxPendingNotifications {
		return nil, status.Error(codes.ResourceExhausted, "Max number of pending notifications exceeded")
	}
	var channel = srv.notifySubscribers(req)
	select {
	case resp := <-channel:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (srv *NotificationService) notifySubscribers(req *api.NotifyRequest) chan *api.NotifyResponse {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	var (
		requestId = srv.nextNotificationId
		message   = &api.SubscribeResult{
			RequestId: requestId,
			Request:   req,
		}
		staleSubscribers = []int{}
	)
	srv.nextNotificationId++
	for _, subscriber := range srv.subscriberChannels {
		subscriber <- message
	}
	srv.removeSubscribers(staleSubscribers)
	var channel = make(chan *api.NotifyResponse, 1)
	srv.pendingNotifications[requestId] = &pendingNotification{
		message:         message,
		responseChannel: channel,
	}
	if len(req.Actions) == 0 {
		channel <- &api.NotifyResponse{}
		close(channel)
	}
	return channel
}

// subscribes to notifications that are sent to the supervisor
func (srv *NotificationService) Subscribe(req *api.SubscribeRequest, resp api.NotificationService_SubscribeServer) error {
	channel := srv.subscribeSubscriber(req, resp)
	for {
		select {
		case subscribeResult, ok := <-channel:
			if !ok {
				return status.Errorf(codes.Aborted, "Notfication channel closed.")
			}
			err := resp.Send(subscribeResult)
			if err != nil {
				return status.Errorf(codes.FailedPrecondition, "Sending notification failed. %s", err)
			}
		case <-resp.Context().Done():
			return nil
		}
	}
}

func (srv *NotificationService) subscribeSubscriber(req *api.SubscribeRequest, resp api.NotificationService_SubscribeServer) chan *api.SubscribeResult {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	channel := make(chan *api.SubscribeResult, len(srv.pendingNotifications))
	for id, pending := range srv.pendingNotifications {
		channel <- pending.message
		if len(pending.message.Request.Actions) == 0 {
			delete(srv.pendingNotifications, id)
		}
	}
	srv.subscriberChannels = append(srv.subscriberChannels, channel)
	return channel
}

// reports user actions as response to a notification request
func (srv *NotificationService) Respond(ctx context.Context, req *api.RespondRequest) (*api.RespondResult, error) {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	pending, ok := srv.pendingNotifications[req.RequestId]
	if !ok {
		log.Log.WithField("requestId", req.RequestId).Info("Invalid or late response to notification")
		return nil, status.Errorf(codes.InvalidArgument, "Invalid or late response to notification")
	}
	if !isActionAllowed(req.Response.Action, pending.message.Request) {
		log.Log.WithFields(map[string]interface{}{
			"Notification": pending.message,
			"Action":       req.Response.Action,
		}).Error("Invalid user action on notification")
		return nil, status.Errorf(codes.InvalidArgument, "Invalid user action on notification")
	}
	delete(srv.pendingNotifications, req.RequestId)
	pending.responseChannel <- req.Response
	close(pending.responseChannel)
	return &api.RespondResult{}, nil
}

func isActionAllowed(action string, req *api.NotifyRequest) bool {
	for _, allowedAction := range req.Actions {
		if allowedAction == action {
			return true
		}
	}
	return false
}

func (srv *NotificationService) removeSubscribers(indices []int) {
	if len(indices) == 0 {
		return
	}
	log.Log.WithFields(map[string]interface{}{
		"stale":  len(indices),
		"remain": len(srv.subscriberChannels) - len(indices),
	}).Error("Unsubscribing stale subscribers")
	n := 0
	j := 0
	for i := range srv.subscriberChannels {
		if indices[j] != i {
			srv.subscriberChannels[n] = srv.subscriberChannels[i]
			n++
		} else {
			j++
		}
	}
	srv.subscriberChannels = srv.subscriberChannels[:n]
}
