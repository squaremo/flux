package http

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/weaveworks/common/middleware"

	"github.com/weaveworks/flux"
	"github.com/weaveworks/flux/history"
	transport "github.com/weaveworks/flux/http"
	"github.com/weaveworks/flux/http/httperror"
	"github.com/weaveworks/flux/http/websocket"
	"github.com/weaveworks/flux/integrations/github"
	"github.com/weaveworks/flux/job"
	"github.com/weaveworks/flux/policy"
	"github.com/weaveworks/flux/remote"
	"github.com/weaveworks/flux/remote/rpc"
	"github.com/weaveworks/flux/service"
	"github.com/weaveworks/flux/service/server"
	"github.com/weaveworks/flux/update"
)

func NewHandler(s *server.Server, r *mux.Router, logger log.Logger) http.Handler {
	service := HTTPService{s}
	handlers := map[string]http.HandlerFunc{
		"History":                service.History,
		"Status":                 service.Status,
		"GetConfig":              service.GetConfig,
		"SetConfig":              service.SetConfig,
		"PatchConfig":            service.PatchConfig,
		"GenerateDeployKeys":     service.GenerateKeys,
		"PostIntegrationsGithub": service.PostIntegrationsGithub,
		"IsConnected":            service.IsConnected,
	}
	transport.AddAPIHandlers(handlers, service)
	transport.AddUpstreamHandlers(handlers, service)
	for method, handlerMethod := range handlers {
		handler := logging(handlerMethod, log.NewContext(logger).With("method", method))
		r.Get(method).Handler(handler)
	}

	return middleware.Instrument{
		RouteMatcher: r,
		Duration:     requestDuration,
	}.Wrap(r)
}

type HTTPService struct {
	service *server.Server
}

func (s HTTPService) ListServices(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)
	namespace := mux.Vars(r)["namespace"]
	res, err := s.service.ListServices(inst, namespace)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}
	transport.JSONResponse(w, r, res)
}

func (s HTTPService) ListImages(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)
	service := mux.Vars(r)["service"]
	spec, err := update.ParseServiceSpec(service)
	if err != nil {
		transport.WriteError(w, r, http.StatusBadRequest, errors.Wrapf(err, "parsing service spec %q", service))
		return
	}

	d, err := s.service.ListImages(inst, spec)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	transport.JSONResponse(w, r, d)
}

func (s HTTPService) UpdateImages(w http.ResponseWriter, r *http.Request) {
	var (
		inst  = getInstanceID(r)
		vars  = mux.Vars(r)
		image = vars["image"]
		kind  = vars["kind"]
	)
	if err := r.ParseForm(); err != nil {
		transport.WriteError(w, r, http.StatusBadRequest, errors.Wrapf(err, "parsing form"))
		return
	}
	var serviceSpecs []update.ServiceSpec
	for _, service := range r.Form["service"] {
		serviceSpec, err := update.ParseServiceSpec(service)
		if err != nil {
			transport.WriteError(w, r, http.StatusBadRequest, errors.Wrapf(err, "parsing service spec %q", service))
			return
		}
		serviceSpecs = append(serviceSpecs, serviceSpec)
	}
	imageSpec, err := update.ParseImageSpec(image)
	if err != nil {
		transport.WriteError(w, r, http.StatusBadRequest, errors.Wrapf(err, "parsing image spec %q", image))
		return
	}
	releaseKind, err := update.ParseReleaseKind(kind)
	if err != nil {
		transport.WriteError(w, r, http.StatusBadRequest, errors.Wrapf(err, "parsing release kind %q", kind))
		return
	}

	var excludes []flux.ServiceID
	for _, ex := range r.URL.Query()["exclude"] {
		s, err := flux.ParseServiceID(ex)
		if err != nil {
			transport.WriteError(w, r, http.StatusBadRequest, errors.Wrapf(err, "parsing excluded service %q", ex))
			return
		}
		excludes = append(excludes, s)
	}

	jobID, err := s.service.UpdateImages(inst, update.ReleaseSpec{
		ServiceSpecs: serviceSpecs,
		ImageSpec:    imageSpec,
		Kind:         releaseKind,
		Excludes:     excludes,
		Cause: update.ReleaseCause{
			User:    r.FormValue("user"),
			Message: r.FormValue("message"),
		},
	})
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	transport.JSONResponse(w, r, jobID)
}

func (s HTTPService) SyncNotify(w http.ResponseWriter, r *http.Request) {
	instID := getInstanceID(r)
	err := s.service.SyncNotify(instID)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (s HTTPService) JobStatus(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)
	id := job.ID(mux.Vars(r)["id"])
	res, err := s.service.JobStatus(inst, id)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}
	transport.JSONResponse(w, r, res)
}

func (s HTTPService) SyncStatus(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)
	rev := mux.Vars(r)["ref"]
	res, err := s.service.SyncStatus(inst, rev)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}
	transport.JSONResponse(w, r, res)
}

func (s HTTPService) UpdatePolicies(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)

	var updates policy.Updates
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		transport.WriteError(w, r, http.StatusBadRequest, err)
		return
	}

	jobID, err := s.service.UpdatePolicies(inst, updates)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	transport.JSONResponse(w, r, jobID)
}

func (s HTTPService) LogEvent(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)

	var event history.Event
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		transport.WriteError(w, r, http.StatusBadRequest, err)
		return
	}

	err := s.service.LogEvent(inst, event)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s HTTPService) History(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)
	service := mux.Vars(r)["service"]
	spec, err := update.ParseServiceSpec(service)
	if err != nil {
		transport.WriteError(w, r, http.StatusBadRequest, errors.Wrapf(err, "parsing service spec %q", spec))
		return
	}

	before := time.Now().UTC()
	if r.FormValue("before") != "" {
		before, err = time.Parse(time.RFC3339Nano, r.FormValue("before"))
		if err != nil {
			transport.ErrorResponse(w, r, err)
			return
		}
	}
	limit := int64(-1)
	if r.FormValue("limit") != "" {
		if _, err := fmt.Sscan(r.FormValue("limit"), &limit); err != nil {
			transport.ErrorResponse(w, r, err)
			return
		}
	}

	h, err := s.service.History(inst, spec, before, limit)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	if r.FormValue("simple") == "true" {
		// Remove all the individual event data, just return the timestamps and messages
		for i := range h {
			h[i].Event = nil
		}
	}

	transport.JSONResponse(w, r, h)
}

func (s HTTPService) GetConfig(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)
	fingerprint := r.FormValue("fingerprint")
	safeConfig, err := s.service.GetConfig(inst, fingerprint)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	transport.JSONResponse(w, r, safeConfig)
}

func (s HTTPService) SetConfig(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)

	var config service.UnsafeInstanceConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		transport.WriteError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := s.service.SetConfig(inst, config); err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s HTTPService) PatchConfig(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)

	var patch service.ConfigPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		transport.WriteError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := s.service.PatchConfig(inst, patch); err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s HTTPService) GenerateKeys(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)
	err := s.service.GenerateDeployKey(inst)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s HTTPService) PostIntegrationsGithub(w http.ResponseWriter, r *http.Request) {
	var (
		inst  = getInstanceID(r)
		vars  = mux.Vars(r)
		owner = vars["owner"]
		repo  = vars["repository"]
		tok   = r.Header.Get("GithubToken")
	)

	if repo == "" || owner == "" || tok == "" {
		transport.WriteError(w, r, http.StatusUnprocessableEntity, errors.New("repo, owner or token is empty"))
		return
	}

	// Generate deploy key
	err := s.service.GenerateDeployKey(inst)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	// FIXME FIXME FIXME
	// Obtain the generated key
	// cfg, err := s.service.GetConfig(inst, "")
	// if err != nil {
	// 	transport.ErrorResponse(w, r, err)
	// 	return
	// }
	// publicKey := cfg.Git.HideKey().Key
	publicKey := "abc123"

	// Use the Github API to insert the key
	// Have to create a new instance here because there is no
	// clean way of injecting without significantly altering
	// the initialisation (at the top)
	gh := github.NewGithubClient(tok)
	err = gh.InsertDeployKey(owner, repo, publicKey)
	if err != nil {
		httpErr, isHttpErr := err.(*httperror.APIError)
		code := http.StatusInternalServerError
		if isHttpErr {
			code = httpErr.StatusCode
		}
		transport.WriteError(w, r, code, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s HTTPService) Status(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)
	status, err := s.service.Status(inst)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	transport.JSONResponse(w, r, status)
}

func (s HTTPService) RegisterV6(w http.ResponseWriter, r *http.Request) {
	s.doRegister(w, r, func(conn io.ReadWriteCloser) platformCloser {
		return rpc.NewClientV6(conn)
	})
}

type platformCloser interface {
	remote.Platform
	io.Closer
}

type platformCloserFn func(io.ReadWriteCloser) platformCloser

func (s HTTPService) doRegister(w http.ResponseWriter, r *http.Request, newRPCFn platformCloserFn) {
	inst := getInstanceID(r)

	// This is not client-facing, so we don't do content
	// negotiation here.

	// Upgrade to a websocket
	ws, err := websocket.Upgrade(w, r, nil)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, err.Error())
		return
	}

	// Set up RPC. The service is a websocket _server_ but an RPC
	// _client_.
	rpcClient := newRPCFn(ws)

	// Make platform available to clients
	// This should block until the daemon disconnects
	// TODO: Handle the error here
	s.service.RegisterDaemon(inst, rpcClient)

	// Clean up
	// TODO: Handle the error here
	rpcClient.Close() // also closes the underlying socket
}

func (s HTTPService) IsConnected(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)

	err := s.service.IsDaemonConnected(inst)
	if err == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	switch err.(type) {
	case flux.UserConfigProblem:
		// NB this has a specific contract for "cannot contact" -> // "404 not found"
		transport.WriteError(w, r, http.StatusNotFound, err)
	default:
		transport.ErrorResponse(w, r, err)
	}
}

func (s HTTPService) Export(w http.ResponseWriter, r *http.Request) {
	inst := getInstanceID(r)
	status, err := s.service.Export(inst)
	if err != nil {
		transport.ErrorResponse(w, r, err)
		return
	}

	transport.JSONResponse(w, r, status)
}

// --- end handlers

func logging(next http.Handler, logger log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		begin := time.Now()
		cw := &codeWriter{w, http.StatusOK}
		tw := &teeWriter{cw, bytes.Buffer{}}

		next.ServeHTTP(tw, r)

		requestLogger := log.NewContext(logger).With(
			"url", mustUnescape(r.URL.String()),
			"took", time.Since(begin).String(),
			"status_code", cw.code,
		)
		if cw.code != http.StatusOK {
			requestLogger = requestLogger.With("error", strings.TrimSpace(tw.buf.String()))
		}
		requestLogger.Log()
	})
}

func getInstanceID(req *http.Request) service.InstanceID {
	s := req.Header.Get(service.InstanceIDHeaderKey)
	return service.InstanceID(s)
}

// codeWriter intercepts the HTTP status code. WriteHeader may not be called in
// case of success, so either prepopulate code with http.StatusOK, or check for
// zero on the read side.
type codeWriter struct {
	http.ResponseWriter
	code int
}

func (w *codeWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *codeWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response does not implement http.Hijacker")
	}
	return hj.Hijack()
}

// teeWriter intercepts and stores the HTTP response.
type teeWriter struct {
	http.ResponseWriter
	buf bytes.Buffer
}

func (w *teeWriter) Write(p []byte) (int, error) {
	w.buf.Write(p) // best-effort
	return w.ResponseWriter.Write(p)
}

func (w *teeWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response does not implement http.Hijacker")
	}
	return hj.Hijack()
}

func mustUnescape(s string) string {
	if unescaped, err := url.QueryUnescape(s); err == nil {
		return unescaped
	}
	return s
}