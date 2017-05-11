package api

import (
	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux/history"
	"github.com/weaveworks/flux/job"
	"github.com/weaveworks/flux/policy"
	"github.com/weaveworks/flux/remote"
	"github.com/weaveworks/flux/update"
)

type Token string

// API for clients connecting to the daemon or service.
type Client interface {
	ListServices(namespace string) ([]flux.ServiceStatus, error)
	ListImages(update.ServiceSpec) ([]flux.ImageStatus, error)
	UpdateImages(update.ReleaseSpec) (job.ID, error)
	SyncNotify() error
	JobStatus(job.ID) (job.Status, error)
	SyncStatus(string) ([]string, error)
	UpdatePolicies(policy.Updates) (job.ID, error)
	Export() ([]byte, error)
}

// API for daemons connecting to the service
type Upstream interface {
	RegisterDaemon(remote.Platform) error
	LogEvent(history.Event) error
}
