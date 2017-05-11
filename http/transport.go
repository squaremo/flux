package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"

	"github.com/weaveworks/flux"
)

func MakeURL(endpoint string, router *mux.Router, routeName string, urlParams ...string) (*url.URL, error) {
	if len(urlParams)%2 != 0 {
		panic("urlParams must be even!")
	}

	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing endpoint %s", endpoint)
	}

	routeURL, err := router.Get(routeName).URL()
	if err != nil {
		return nil, errors.Wrapf(err, "retrieving route path %s", routeName)
	}

	v := url.Values{}
	for i := 0; i < len(urlParams); i += 2 {
		v.Add(urlParams[i], urlParams[i+1])
	}

	endpointURL.Path = path.Join(endpointURL.Path, routeURL.Path)
	endpointURL.RawQuery = v.Encode()
	return endpointURL, nil
}

func WriteError(w http.ResponseWriter, r *http.Request, code int, err error) {
	// An Accept header with "application/json" is sent by clients
	// understanding how to decode JSON errors. Older clients don't
	// send an Accept header, so we just give them the error text.
	if len(r.Header.Get("Accept")) > 0 {
		switch negotiateContentType(r, []string{"application/json", "text/plain"}) {
		case "application/json":
			body, encodeErr := json.Marshal(err)
			if encodeErr != nil {
				w.Header().Set(http.CanonicalHeaderKey("Content-Type"), "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "Error encoding error response: %s\n\nOriginal error: %s", encodeErr.Error(), err.Error())
				return
			}
			w.Header().Set(http.CanonicalHeaderKey("Content-Type"), "application/json; charset=utf-8")
			w.WriteHeader(code)
			w.Write(body)
			return
		case "text/plain":
			w.Header().Set(http.CanonicalHeaderKey("Content-Type"), "text/plain; charset=utf-8")
			w.WriteHeader(code)
			switch err := err.(type) {
			case *flux.BaseError:
				fmt.Fprint(w, err.Help)
			default:
				fmt.Fprint(w, err.Error())
			}
			return
		}
	}
	w.Header().Set(http.CanonicalHeaderKey("Content-Type"), "text/plain; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprint(w, err.Error())
}

func JSONResponse(w http.ResponseWriter, r *http.Request, result interface{}) {
	body, err := json.Marshal(result)
	if err != nil {
		ErrorResponse(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

func ErrorResponse(w http.ResponseWriter, r *http.Request, apiError error) {
	var outErr *flux.BaseError
	var code int
	err := errors.Cause(apiError)
	switch err := err.(type) {
	case flux.Missing:
		code = http.StatusNotFound
		outErr = err.BaseError
	case flux.UserConfigProblem:
		code = http.StatusUnprocessableEntity
		outErr = err.BaseError
	case flux.ServerException:
		code = http.StatusInternalServerError
		outErr = err.BaseError
	default:
		code = http.StatusInternalServerError
		outErr = flux.CoverAllError(apiError)
	}

	WriteError(w, r, code, outErr)
}
