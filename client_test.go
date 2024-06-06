package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	pb "testproxy/generated/example"

	"google.golang.org/grpc"
)

func TestGrpcNoProxy(t *testing.T) {
	// Start a mock gRPC server
	grpcServer := grpc.NewServer()
	pb.RegisterExampleServiceServer(grpcServer, &server{})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	grpcAddr := lis.Addr().String()

	// Start a mock proxy server
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Proxy should not be used, but it was used for %v", r.URL)
	}))
	defer proxy.Close()

	// Set environment variables for the proxy
	os.Setenv("HTTP_PROXY", proxy.URL)
	os.Setenv("NO_PROXY", "127.0.0.1")
	defer os.Unsetenv("HTTP_PROXY")
	defer os.Unsetenv("NO_PROXY")

	// Without custom dialer: The test should fail as the proxy will be used
	t.Run("without custom dialer", func(t *testing.T) {
		conn, err := grpc.NewClient(grpcAddr, grpc.WithInsecure())
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		client := pb.NewExampleServiceClient(conn)
		_, err = client.SayHello(context.Background(), &pb.HelloRequest{Name: "world"})
		if err != nil {
			t.Fatalf("grpc request failed: %v", err)
		}
	})

	// With custom dialer: The test should pass as the proxy should not be used
	t.Run("with custom dialer", func(t *testing.T) {
		customDialer := func(ctx context.Context, addr string) (net.Conn, error) {
			noProxy := os.Getenv("NO_PROXY")
			if noProxy != "" {
				for _, host := range strings.Split(noProxy, ",") {
					if strings.Contains(addr, host) {
						return net.Dial("tcp", addr)
					}
				}
			}

			proxyURL := os.Getenv("HTTP_PROXY")
			if proxyURL == "" {
				proxyURL = os.Getenv("HTTPS_PROXY")
			}
			if proxyURL == "" {
				return net.Dial("tcp", addr)
			}

			proxy, err := http.ProxyFromEnvironment(&http.Request{URL: &url.URL{Host: addr}})
			if err != nil {
				return nil, err
			}
			if proxy == nil {
				return net.Dial("tcp", addr)
			}

			return net.Dial("tcp", proxy.Host)
		}

		conn, err := grpc.NewClient(grpcAddr, grpc.WithContextDialer(customDialer), grpc.WithInsecure())
		if err != nil {
			t.Fatalf("failed to dial: %v", err)
		}
		defer conn.Close()

		client := pb.NewExampleServiceClient(conn)
		_, err = client.SayHello(context.Background(), &pb.HelloRequest{Name: "world"})
		if err != nil {
			t.Fatalf("grpc request failed: %v", err)
		}
	})
}
