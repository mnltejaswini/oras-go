/*
Copyright The ORAS Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package content

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	"github.com/containerd/containerd/remotes"
	"github.com/containerd/containerd/remotes/docker"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	auth_opts "oras.land/oras-go/pkg/auth"
	auth "oras.land/oras-go/pkg/auth/docker"
	"oras.land/oras-go/pkg/target"
	orasdocker "oras.land/oras-go/pkg/target/docker"
)

// RegistryOptions provide configuration options to a Registry
type RegistryOptions struct {
	Configs   []string
	Username  string
	Password  string
	Insecure  bool
	PlainHTTP bool
	UserAgent string
}

// Registry provides content from a spec-compliant registry. Create an use a new one for each
// registry with unique configuration of RegistryOptions.
type Registry struct {
	remotes.Resolver
}

// NewRegistry creates a new Registry store
func NewRegistry(opts RegistryOptions) (*Registry, error) {
	return &Registry{
		Resolver: newResolver(opts.Username, opts.Password, opts.Insecure, opts.PlainHTTP, opts.Configs...),
	}, nil
}

func NewRegistryWithDiscover1(targetRef string, opts RegistryOptions) (target.Target, error) {
	obj := target.FromOCIDescriptor(targetRef, ocispec.Descriptor{}, "", nil)

	_, host, ns, _, err := obj.ReferenceSpec()
	if err != nil {
		return nil, err
	}

	loginOpts := []auth_opts.LoginOption{
		auth_opts.WithLoginHostname(host),
		auth_opts.WithLoginUsername(opts.Username),
		auth_opts.WithLoginSecret(opts.Password),
		auth_opts.WithLoginUserAgent(opts.UserAgent),
	}

	if opts.Insecure {
		loginOpts = append(loginOpts, auth_opts.WithLoginInsecure())
	}

	client, err := auth.NewRegistryWithAccessProvider(host, ns, opts.Configs, loginOpts...)
	if err != nil {
		return nil, err
	}

	// Backup resolver
	resolver, err := NewRegistry(opts)
	if err != nil {
		return nil, err
	}

	registry, err := orasdocker.FromRemotesRegistry(targetRef, client, resolver)
	if err != nil {
		return nil, err
	}

	return registry, nil
}

func NewRegistryWithDiscover(targetRef string, opts RegistryOptions) (*Registry, error) {
	registry, err := NewRegistry(opts)
	if err != nil {
		return nil, err
	}

	resolver, err := orasdocker.WithDiscover(targetRef, registry.Resolver)
	if err != nil {
		return nil, err
	}

	registry.Resolver = resolver
	return registry, nil
}

func newResolver(username, password string, insecure bool, plainHTTP bool, configs ...string) remotes.Resolver {

	opts := docker.ResolverOptions{
		PlainHTTP: plainHTTP,
	}

	client := http.DefaultClient
	if insecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}
	opts.Client = client

	if username != "" || password != "" {
		opts.Credentials = func(hostName string) (string, string, error) {
			return username, password, nil
		}
		return docker.NewResolver(opts)
	}
	cli, err := auth.NewClient(configs...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Error loading auth file: %v\n", err)
	}
	resolver, err := cli.Resolver(context.Background(), client, plainHTTP)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Error loading resolver: %v\n", err)
		resolver = docker.NewResolver(opts)
	}
	return resolver
}
