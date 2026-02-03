package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/reyoung/agent-envs/proxy/pkg/callback"
	"github.com/reyoung/agent-envs/proxy/pkg/publisher"
	"github.com/reyoung/agent-envs/proxy/pkg/response_store"
)

func main() {
	var (
		flagListenAddr   = flag.String("listen", ":8080", "HTTP listen address")
		flagRedisDSN     = flag.String("redis", "", "Redis DSN without queue suffix")
		flagCallbackBase = flag.String("callback-base-url", "", "Base URL for callback endpoint (e.g. http://proxy:8080)")
	)
	flag.Parse()

	if *flagRedisDSN == "" {
		log.Fatalf("--redis is required")
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
	publisher, err := publisher.New(*flagRedisDSN)
	defer publisher.Close()
	if err != nil {
		log.Fatalf("failed to create publisher: %v", err)
	}

	svr := &callback.Server{
		Store: respStore,
	}
	svr.Start(*flagListenAddr)
	defer svr.Close()
	ch := make(chan os.Signal, 1) // 一般给 buffer=1，避免错过
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	log.Println("shutting down...")
}
