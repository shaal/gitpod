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
		subscriberChannels:   []chan *api.SubscribeResponse{},
		pendingNotifications: make(map[uint64]*pendingNotification),
	}
}

// NotificationService implements the notification service API
type NotificationService struct {
	mutex                sync.Mutex
	subscriberChannels   []chan *api.SubscribeResponse
	nextNotificationId   uint64
	pendingNotifications map[uint64]*pendingNotification
}

type pendingNotification struct {
	message         *api.SubscribeResponse
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
	log.Log.WithField("NotifyRequest", req).Info("Notify entered")
	defer log.Log.WithField("NotifyRequest", req).Info("Notify exited")
	if srv.nextNotificationId >= maxPendingNotifications {
		return nil, status.Error(codes.ResourceExhausted, "Max number of pending notifications exceeded")
	}
	var pending = srv.notifySubscribers(req)
	select {
	case resp, ok := <-pending.responseChannel:
		if !ok {
			log.Log.Error("notify response channel has been closed")
			return nil, status.Error(codes.Aborted, "response channel closed")
		}
		log.Log.WithField("NotifyResponse", resp).Info("sending notify response")
		return resp, nil
	case <-ctx.Done():
		log.Log.Info("notify cancelled")
		srv.mutex.Lock()
		defer srv.mutex.Unlock()
		close(pending.responseChannel)
		delete(srv.pendingNotifications, pending.message.RequestId)
		return nil, ctx.Err()
	}
}

func (srv *NotificationService) notifySubscribers(req *api.NotifyRequest) *pendingNotification {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	var (
		requestId = srv.nextNotificationId
		message   = &api.SubscribeResponse{
			RequestId: requestId,
			Request:   req,
		}
	)
	srv.nextNotificationId++
	for _, subscriber := range srv.subscriberChannels {
		// TODO: should we raise the channel capacity here and react to blocking?
		subscriber <- message
	}
	var channel = make(chan *api.NotifyResponse, 1)
	pending := &pendingNotification{
		message:         message,
		responseChannel: channel,
	}
	srv.pendingNotifications[requestId] = pending
	if len(req.Actions) == 0 {
		channel <- &api.NotifyResponse{}
		close(channel)
	}
	return pending
}

// subscribes to notifications that are sent to the supervisor
func (srv *NotificationService) Subscribe(req *api.SubscribeRequest, resp api.NotificationService_SubscribeServer) error {
	log.Log.WithField("SubscribeRequest", req).Info("Subscribe entered")
	defer log.Log.WithField("SubscribeRequest", req).Info("Subscribe exited")
	channel := srv.subscribeSubscriber(req, resp)
	for {
		select {
		case SubscribeResponse, ok := <-channel:
			if !ok {
				return status.Errorf(codes.Aborted, "Notfication channel closed.")
			}
			err := resp.Send(SubscribeResponse)
			if err != nil {
				return status.Errorf(codes.FailedPrecondition, "Sending notification failed. %s", err)
			}
		case <-resp.Context().Done():
			return nil
		}
	}
}

func (srv *NotificationService) subscribeSubscriber(req *api.SubscribeRequest, resp api.NotificationService_SubscribeServer) chan *api.SubscribeResponse {
	srv.mutex.Lock()
	defer srv.mutex.Unlock()
	channel := make(chan *api.SubscribeResponse, len(srv.pendingNotifications))
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
func (srv *NotificationService) Respond(ctx context.Context, req *api.RespondRequest) (*api.RespondResponse, error) {
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
	return &api.RespondResponse{}, nil
}

func isActionAllowed(action string, req *api.NotifyRequest) bool {
	if action == "" {
		// user cancelled, which is always allowed
		return true
	}
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
