// Copyright 2025 Tetrate
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/tetratelabs/run"
	"github.com/tetratelabs/telemetry"

	configv1 "github.com/istio-ecosystem/authservice/config/gen/go/v1"
	oidcv1 "github.com/istio-ecosystem/authservice/config/gen/go/v1/oidc"
	"github.com/istio-ecosystem/authservice/internal"
	"github.com/istio-ecosystem/authservice/internal/oidc"
)

const (
	JwksPath = "/jwks"
	JwksPort = 10005
)

var (
	_ http.Handler = (*jwksServer)(nil)
	_ run.Service  = (*jwksServer)(nil)
)

type jwksServer struct {
	log    telemetry.Logger
	config *configv1.Config
	server *http.Server
	jwks   oidc.JWKSProvider

	// Listen allows overriding the default listener. It is meant to
	// be used in tests.
	l net.Listener
}

// NewjwksServer creates a new health server.
func NewJwksServer(config *configv1.Config) run.Unit {
	js := &jwksServer{
		log:    internal.Logger(internal.JWKS),
		config: config,
	}
	httpServer := &http.Server{Handler: js}
	js.server = httpServer
	return js
}

// Name implements run.Unit.
func (js *jwksServer) Name() string {
	return "Jwks Server"
}

// Serve implements run.Service.
func (js *jwksServer) Serve() error {
	// use test listener if set
	if js.l == nil {
		var err error
		js.l, err = net.Listen("tcp", js.getAddressAndPort())
		if err != nil {
			return err
		}
	}

	js.log.Info("starting jwks server", "addr", js.l.Addr(), "path", js.getPath())
	return js.server.Serve(js.l)
}

// GracefulStop implements run.Service.
func (js *jwksServer) GracefulStop() {
	js.log.Info("stopping jwks server")
	_ = js.server.Close()
}

func (js *jwksServer) getAddressAndPort() string {
	addr := js.config.GetJwksListenAddress()
	if addr == "" {
		addr = js.config.GetListenAddress()
	}

	port := js.config.GetHealthListenPort()
	if port == 0 {
		port = HealthzPort
	}

	return fmt.Sprintf("%s:%d", addr, port)
}

func (js *jwksServer) getPath() string {
	path := js.config.GetHealthListenPath()
	if path != "" {
		return path
	}
	return HealthzPath
}

// ServeHTTP implements http.Handler.
func (js *jwksServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := js.log.With("method", r.Method, "path", r.URL.Path)

	jwksProvider := &oidc.DefaultJWKSProvider{}
	ctx := context.Background()
	jwks, err := jwksProvider.Get(ctx, &oidcv1.OIDCConfig{})

	if err != nil {
		log.Error("failed to get jwks", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jkwsKey, ok := jwks.LookupKeyID(r.URL.Path)

	if !ok {
		http.Error(w, "key not found", http.StatusNotFound)
		return
	}

	keyJSON, err := jkwsKey.X509CertChain().MarshalJSON()
	if err != nil {
		log.Error("failed to marshal key to JSON", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write(keyJSON)
}
