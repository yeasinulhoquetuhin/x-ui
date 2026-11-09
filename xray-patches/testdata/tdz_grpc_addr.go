package encoding

import (
	"context"
	"net"
	"net/http"

	"github.com/xtls/xray-core/transport/internet"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

func tdzGRPCClientAddr(ctx context.Context) net.Addr {
	var remote net.Addr = &net.TCPAddr{IP: net.IPv4zero}
	var local net.Addr
	if pr, ok := peer.FromContext(ctx); ok {
		remote = pr.Addr
		local = pr.LocalAddr
	}
	headers := make(http.Header)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		for _, name := range []string{"x-forwarded-for", "x-real-ip"} {
			for _, value := range md.Get(name) {
				headers.Add(name, value)
			}
		}
	}
	return internet.TDZClientAddr(remote, local, headers, nil)
}
