package notification

import (
	"context"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/notification"
	"github.com/argoproj/notifications-engine/pkg/api"
)

// Server provides an Application service
type Server struct {
	apiFactory api.Factory
}

// NewServer returns a new instance of the Application service
func NewServer(apiFactory api.Factory) notification.NotificationServiceServer {
	s := &Server{apiFactory: apiFactory}
	return s
}

// List returns list of applications
func (s *Server) List(ctx context.Context, q *notification.TriggerListRequest) (*notification.Triggers, error) {
	api, err := s.apiFactory.GetAPI()
	if err != nil {
		return nil, err
	}
	triggers := []string{}
	for trigger := range api.GetConfig().Triggers {
		triggers = append(triggers, trigger)
	}
	return &notification.Triggers{Triggers: triggers}, nil
}
