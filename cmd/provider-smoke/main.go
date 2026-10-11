// provider-smoke is a read-only deployment diagnostic and private test proxy.
// It never creates provider accounts, logs temporary URLs or handles raw keys.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Provider diagnostic failed; sensitive details omitted.")
		os.Exit(1)
	}
}
func run() error {
	mode := flag.String("mode", "probe", "probe or loopback-proxy")
	endpoint := flag.String("cloud-endpoint", "clouddrive2:19798", "private CloudDrive2 gRPC host:port")
	listen := flag.String("listen", "127.0.0.1:25014", "loopback-only test proxy listen address")
	target := flag.String("target", "http://server:8080", "private management service target")
	flag.Parse()
	if *mode == "loopback-proxy" {
		host, _, err := net.SplitHostPort(*listen)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			return fmt.Errorf("proxy must bind loopback")
		}
		upstream, err := url.Parse(*target)
		if err != nil || upstream.Scheme != "http" || upstream.User != nil || upstream.Host != "server:8080" || upstream.Path != "" || upstream.RawQuery != "" || upstream.Fragment != "" {
			return fmt.Errorf("only private test server target allowed")
		}
		p := httputil.NewSingleHostReverseProxy(upstream)
		p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "private test upstream unavailable", 502)
		}
		s := &http.Server{Addr: *listen, Handler: p, ReadHeaderTimeout: 5 * time.Second}
		return s.ListenAndServe()
	}
	if *mode != "probe" {
		return fmt.Errorf("unsupported diagnostic mode")
	}
	if _, _, err := net.SplitHostPort(*endpoint); err != nil {
		return err
	}
	conn, err := grpc.NewClient(*endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := pb.NewCloudDriveFileSrvClient(conn).GetSystemInfo(ctx, &emptypb.Empty{})
	report := map[string]any{"cloudGRPCReachable": err == nil, "cloudReadAuthenticated": false, "real115PlaybackVerified": false}
	if err != nil {
		report["grpcStatus"] = status.Code(err).String()
		report["authorizationRequired"] = status.Code(err) == codes.Unauthenticated || status.Code(err) == codes.PermissionDenied
	} else if result != nil {
		// Whitelist only provider readiness, not raw system/account data.
		report["providerReady"] = result.GetSystemReady()
		report["providerLoggedIn"] = result.GetIsLogin()
	}
	json.NewEncoder(os.Stdout).Encode(report)
	if err != nil && status.Code(err) != codes.Unauthenticated && status.Code(err) != codes.PermissionDenied {
		return fmt.Errorf("gRPC unavailable")
	}
	return nil
}
