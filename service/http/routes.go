package http

import (
	"github.com/gorilla/mux"

	transport "github.com/weaveworks/flux/http"
)

func NewServiceRouter() *mux.Router {
	r := transport.NewAPIRouter()

	r.NewRoute().Name("Automate").Methods("POST").Path("/v3/automate").Queries("service", "{service}")
	r.NewRoute().Name("Deautomate").Methods("POST").Path("/v3/deautomate").Queries("service", "{service}")
	r.NewRoute().Name("Lock").Methods("POST").Path("/v3/lock").Queries("service", "{service}")
	r.NewRoute().Name("Unlock").Methods("POST").Path("/v3/unlock").Queries("service", "{service}")
	r.NewRoute().Name("History").Methods("GET").Path("/v3/history").Queries("service", "{service}")
	r.NewRoute().Name("Status").Methods("GET").Path("/v3/status")
	r.NewRoute().Name("GetConfig").Methods("GET").Path("/v4/config")
	r.NewRoute().Name("SetConfig").Methods("POST").Path("/v4/config")
	r.NewRoute().Name("PatchConfig").Methods("PATCH").Path("/v4/config")
	r.NewRoute().Name("GenerateDeployKeys").Methods("POST").Path("/v5/config/deploy-keys")
	r.NewRoute().Name("PostIntegrationsGithub").Methods("POST").Path("/v5/integrations/github").Queries("owner", "{owner}", "repository", "{repository}")
	r.NewRoute().Name("RegisterDaemonV4").Methods("GET").Path("/v4/daemon")
	r.NewRoute().Name("RegisterDaemonV5").Methods("GET").Path("/v5/daemon")
	r.NewRoute().Name("IsConnected").Methods("HEAD", "GET").Path("/v4/ping")
	transport.AddNotFoundRoutes(r)

	return r
}
