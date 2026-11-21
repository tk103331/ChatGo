// Command relayd runs the self-hosted ChatGo relay.
//
// It terminates one outbound WebSocket tunnel per desktop, pairs phones with
// a desktop, and forwards /api requests over that tunnel. Deploy it on a
// public host with TLS; desktops never need an inbound port.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"chatgo/internal/relay"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8787", "listen address")
	data := flag.String("data", "relay.db", "SQLite database path")
	public := flag.String("public-url", "", "public URL phones use, e.g. https://relay.example.com")
	cert := flag.String("tls-cert", "", "TLS certificate file")
	key := flag.String("tls-key", "", "TLS key file")
	apnsKey := flag.String("apns-key", "", "APNs .p8 key file")
	apnsKeyID := flag.String("apns-key-id", "", "APNs key ID")
	apnsTeamID := flag.String("apns-team-id", "", "Apple team ID")
	apnsTopic := flag.String("apns-topic", "", "APNs topic (the iOS bundle id)")
	apnsEnv := flag.String("apns-env", "prod", "APNs environment: prod or dev")
	fcmAccount := flag.String("fcm-service-account", "", "FCM service account JSON file")
	flag.Parse()

	if (*cert == "") != (*key == "") {
		log.Fatal("relayd: -tls-cert and -tls-key must be given together")
	}

	pusher, err := relay.NewPusher(relay.PushConfig{
		APNsKeyPath:    *apnsKey,
		APNsKeyID:      *apnsKeyID,
		APNsTeamID:     *apnsTeamID,
		APNsTopic:      *apnsTopic,
		APNsEnv:        *apnsEnv,
		FCMAccountPath: *fcmAccount,
	})
	if err != nil {
		log.Fatalf("relayd: %v", err)
	}
	srv, err := relay.New(relay.Config{
		Addr:      *addr,
		DataPath:  *data,
		PublicURL: *public,
		Pusher:    pusher,
	})
	if err != nil {
		log.Fatalf("relayd: %v", err)
	}
	defer srv.Close()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("relayd: listen %s: %v", *addr, err)
	}
	httpSrv := &http.Server{
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 15 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdown)
	}()

	log.Printf("ChatGo relay on %s (data %s)", *addr, *data)
	if *public != "" {
		log.Printf("public URL: %s", *public)
	}
	if *cert != "" {
		err = httpSrv.ServeTLS(ln, *cert, *key)
	} else {
		log.Printf("warning: TLS is off; put a TLS reverse proxy in front for production")
		err = httpSrv.Serve(ln)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
