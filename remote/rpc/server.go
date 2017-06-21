package rpc

import (
	"fmt"
	"io"
	"net/rpc"
	"net/rpc/jsonrpc"

	"github.com/pkg/errors"

	"github.com/weaveworks/flux"
	fluxerr "github.com/weaveworks/flux/errors"
	"github.com/weaveworks/flux/job"
	"github.com/weaveworks/flux/remote"
	"github.com/weaveworks/flux/ssh"
	"github.com/weaveworks/flux/update"
)

// Server takes a platform and makes it available over RPC.
type Server struct {
	server *rpc.Server
}

// NewServer instantiates a new RPC server, handling requests on the
// conn by invoking methods on the underlying (assumed local)
// platform.
func NewServer(p remote.Platform) (*Server, error) {
	server := rpc.NewServer()
	if err := server.Register(&RPCServer{p}); err != nil {
		return nil, err
	}
	return &Server{server: server}, nil
}

func (c *Server) ServeConn(conn io.ReadWriteCloser) {
	c.server.ServeCodec(jsonrpc.NewServerCodec(conn))
}

type RPCServer struct {
	p remote.Platform
}

func (p *RPCServer) Ping(_ struct{}, _ *struct{}) error {
	return p.p.Ping()
}

func (p *RPCServer) Version(_ struct{}, resp *string) error {
	v, err := p.p.Version()
	*resp = v
	return err
}

type ExportResponse struct {
	Result []byte
	Error  *fluxerr.Error
}

func (p *RPCServer) Export(_ struct{}, resp *ExportResponse) error {
	v, err := p.p.Export()
	resp.Result = v
	if err, ok := err.(*fluxerr.Error); ok {
		resp.Error = err
	}
	return err
}

type ListServicesResponse struct {
	Result []flux.ServiceStatus
	Error  *fluxerr.Error
}

func (p *RPCServer) ListServices(namespace string, resp *ListServicesResponse) error {
	v, err := p.p.ListServices(namespace)
	resp.Result = v
	if err != nil {
		err := errors.Cause(err)
		if helperr, ok := err.(*fluxerr.Error); ok {
			fmt.Printf("DEBUG RPC server setting error %#v\n", err)
			resp.Error = helperr
			return nil
		}
	}
	return err
}

func (p *RPCServer) ListImages(spec update.ServiceSpec, resp *[]flux.ImageStatus) error {
	v, err := p.p.ListImages(spec)
	*resp = v
	return err
}

func (p *RPCServer) UpdateManifests(spec update.Spec, resp *job.ID) error {
	v, err := p.p.UpdateManifests(spec)
	*resp = v
	return err
}

func (p *RPCServer) SyncNotify(_ struct{}, _ *struct{}) error {
	return p.p.SyncNotify()
}

func (p *RPCServer) JobStatus(jobID job.ID, resp *job.Status) error {
	v, err := p.p.JobStatus(jobID)
	*resp = v
	return err
}

func (p *RPCServer) SyncStatus(cursor string, resp *[]string) error {
	v, err := p.p.SyncStatus(cursor)
	*resp = v
	return err
}

func (p *RPCServer) PublicSSHKey(regenerate bool, resp *ssh.PublicKey) error {
	v, err := p.p.PublicSSHKey(regenerate)
	*resp = v
	return err
}
