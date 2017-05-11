package http

import (
	"net/http"

	"github.com/gorilla/mux"
)

func NewAPIRouter() *mux.Router {
	r := mux.NewRouter()
	AddAPIRoutes(r)
	return r
}

func AddAPIRoutes(r *mux.Router) {
	// Any versions not represented in the routes below are
	// deprecated. They are done separately so we can see them as
	// different methods in metrics and logging.
	var deprecated http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusGone, ErrorDeprecated)
	}

	for _, version := range []string{"v1", "v2"} {
		r.NewRoute().Name("Deprecated:" + version).PathPrefix("/" + version + "/").HandlerFunc(deprecated)
	}

	// These API endpoints are specifically deprecated
	for name, path := range map[string]string{
		"PostOrGetRelease": "/v4/release", // deprecated because UpdateImages and Sync{Notify,Status} supercede them, and we cannot support both
	} {
		r.NewRoute().Name("Deprecated:" + name).Path(path).HandlerFunc(deprecated)
	}

	r.NewRoute().Name("ListServices").Methods("GET").Path("/v3/services").Queries("namespace", "{namespace}") // optional namespace!
	r.NewRoute().Name("ListImages").Methods("GET").Path("/v3/images").Queries("service", "{service}")

	r.NewRoute().Name("UpdateImages").Methods("POST").Path("/v6/update-images").Queries("service", "{service}", "image", "{image}", "kind", "{kind}")
	r.NewRoute().Name("UpdatePolicies").Methods("PATCH").Path("/v4/policies")
	r.NewRoute().Name("SyncNotify").Methods("POST").Path("/v6/sync")
	r.NewRoute().Name("JobStatus").Methods("GET").Path("/v6/jobs").Queries("id", "{id}")
	r.NewRoute().Name("SyncStatus").Methods("GET").Path("/v6/sync").Queries("ref", "{ref}")
	r.NewRoute().Name("Export").Methods("HEAD", "GET").Path("/v5/export")
}

func AddNotFoundRoutes(r *mux.Router) {
	r.NewRoute().Name("NotFound").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, r, http.StatusNotFound, MakeAPINotFound(r.URL.Path))
	})
}

func NewUpstreamRouter() *mux.Router {
	r := mux.NewRouter()
	AddUpstreamRoutes(r)
	return r
}

func AddUpstreamRoutes(r *mux.Router) {
	r.NewRoute().Name("RegisterDaemonV6").Methods("GET").Path("/v6/daemon")
	r.NewRoute().Name("LogEvent").Methods("POST").Path("/v4/events")
}

// These give us some type safety, so it's less easy to forget to
// implement or register a handler.

type APIHandler interface {
	ListServices(w http.ResponseWriter, r *http.Request)
	ListImages(w http.ResponseWriter, r *http.Request)
	UpdateImages(w http.ResponseWriter, r *http.Request)
	UpdatePolicies(w http.ResponseWriter, r *http.Request)
	SyncNotify(w http.ResponseWriter, r *http.Request)
	JobStatus(w http.ResponseWriter, r *http.Request)
	SyncStatus(w http.ResponseWriter, r *http.Request)
	Export(w http.ResponseWriter, r *http.Request)
}

func AddAPIHandlers(m map[string]http.HandlerFunc, handle APIHandler) {
	for route, handler := range map[string]http.HandlerFunc{
		"ListServices":   handle.ListServices,
		"ListImages":     handle.ListImages,
		"UpdateImages":   handle.UpdateImages,
		"UpdatePolicies": handle.UpdatePolicies,
		"Export":         handle.Export,
		"SyncNotify":     handle.SyncNotify,
		"JobStatus":      handle.JobStatus,
		"SyncStatus":     handle.SyncStatus,
	} {
		m[route] = handler
	}
}

type UpstreamHandler interface {
	RegisterV6(w http.ResponseWriter, r *http.Request)
	LogEvent(w http.ResponseWriter, r *http.Request)
}

func AddUpstreamHandlers(m map[string]http.HandlerFunc, handle UpstreamHandler) {
	for route, handler := range map[string]http.HandlerFunc{
		"RegisterDaemonV6": handle.RegisterV6,
		"LogEvent":         handle.LogEvent,
	} {
		m[route] = handler
	}
}
