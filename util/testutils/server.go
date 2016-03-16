package testutils

import (
	"net"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	Port = ":10000"
	localHost = "127.0.0.1"

	certFile = "cert/ssl.crt"
	keyFile = "cert/ssl.key"
)

type RegisterFun interface{
	Register(*grpc.Server, interface{})
}

func getLocalIp() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return localHost
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return localHost
}

type TestServer struct {
	S          *grpc.Server
	Host, Port string

	security   bool
	started    bool
}

func NewTestServer(host, port string, security bool) *TestServer {
	if host == "" {
		host = getLocalIp()
	}
	if port == "" {
		port = Port
	}
	ts := &TestServer{
		Host: host,
		Port: port,
		security: security,
	}
	var opts []grpc.ServerOption
	if security {
		creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
		if err != nil {
			panic(err)
		}
		opts = []grpc.ServerOption{grpc.Creds(creds)}
	}
	ts.S = grpc.NewServer(opts...)
	return ts
}

func (ts *TestServer) GetDialOpts() []grpc.DialOption {
	var opts []grpc.DialOption
	if ts.security {
		creds, err := credentials.NewClientTLSFromFile(certFile, "testserver")
		if err != nil {
			panic(err)
		}
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}
	return opts
}

func (ts *TestServer) RegisterRPC(rf RegisterFun, rpc interface{}) bool {
	if ts.started {
		return false
	}
	rf.Register(ts.S, rpc)
	return true
}

func (ts *TestServer) Start() error {
	l, err := net.Listen("tcp", ts.Host + ts.Port)
	if err != nil {
		return err
	}
	ts.started = true
	go ts.S.Serve(l)
	return nil
}

func (ts *TestServer) Stop() {
	ts.S.Stop()
	ts.started = false
}
