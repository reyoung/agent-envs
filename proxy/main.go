package main

import (
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/reyoung/agent-envs/proxy/pkg/api"
	exec_v1 "github.com/reyoung/agent-envs/proxy/pkg/api/proto/agent_envs/exec/v1"
	"github.com/reyoung/agent-envs/proxy/pkg/callback"
	"github.com/reyoung/agent-envs/proxy/pkg/publisher"
	"github.com/reyoung/agent-envs/proxy/pkg/response_store"
	"google.golang.org/grpc"
)

func main() {
	var (
		flagListenAddr         = flag.String("listen", ":8080", "HTTP listen address")
		flagRedisDSN           = flag.String("redis", "", "Redis DSN without queue suffix")
		flagCallbackBase       = flag.String("callback-base-url", "", "Base URL for callback endpoint (e.g. http://proxy:8080)")
		flagCallbackListenAddr = flag.String("callback-listen", ":8081", "Callback HTTP listen address")
	)
	flag.Parse()

	log.Default().SetPrefix("[proxy] ")

	if *flagRedisDSN == "" {
		log.Fatalf("--redis is required")
	}
	if *flagCallbackListenAddr == "" {
		log.Fatalf("--callback-listen is required")
	}
	if *flagCallbackBase == "" {
		log.Fatalf("--callback-base-url is required")
	}

	callbackBase := strings.TrimRight(*flagCallbackBase, "/")
	if callbackBase == "" {
		log.Fatalf("callback base URL cannot be empty")
	}

	respStore := response_store.New()
	defer respStore.Close()
	pub, err := publisher.New(*flagRedisDSN)
	defer pub.Close()
	if err != nil {
		log.Fatalf("failed to create publisher: %v", err)
	}

	svr := &callback.Server{
		Store: respStore,
	}
	svr.Start(*flagCallbackListenAddr)
	defer svr.Close()

	grpcAPISvr := &api.Server{
		Publisher:   pub,
		Store:       respStore,
		CallbackURL: callbackBase,
	}

	lis, err := net.Listen("tcp", *flagListenAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *flagListenAddr, err)
	}
	grpcSvr := grpc.NewServer()
	exec_v1.RegisterProxyServer(grpcSvr, grpcAPISvr)
	go grpcSvr.Serve(lis)
	defer grpcSvr.Stop()
	log.Printf("gRPC server listening on %s", *flagListenAddr)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Println("shutting down...")
}
