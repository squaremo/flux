package history

import (
	"io"
	"time"

	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux/history"
	"github.com/weaveworks/flux/service"
)

type DB interface {
	LogEvent(service.InstanceID, history.Event) error
	AllEvents(service.InstanceID, time.Time, int64) ([]history.Event, error)
	EventsForService(service.InstanceID, flux.ServiceID, time.Time, int64) ([]history.Event, error)
	GetEvent(history.EventID) (history.Event, error)
	io.Closer
}
