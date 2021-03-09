// Copyright (c) 2020 Gitpod GmbH. All rights reserved.
// Licensed under the GNU Affero General Public License (AGPL).
// See License-AGPL.txt in the project root for license information.

package archive

import (
	"context"
	"io"
	"os/exec"
	"time"

	"github.com/opentracing/opentracing-go"
	"golang.org/x/xerrors"

	"github.com/gitpod-io/gitpod/common-go/log"
	"github.com/gitpod-io/gitpod/common-go/tracing"
)

// TarConfig configures tarbal creation/extraction
type TarConfig struct {
	MaxSizeBytes int64
	UIDMaps      []IDMapping
	GIDMaps      []IDMapping
}

// BuildTarbalOption configures the tarbal creation
type TarOption func(o *TarConfig)

// TarbalMaxSize limits the size of a tarbal
func TarbalMaxSize(n int64) TarOption {
	return func(o *TarConfig) {
		o.MaxSizeBytes = n
	}
}

// IDMapping maps user or group IDs
type IDMapping struct {
	ContainerID int
	HostID      int
	Size        int
}

// WithUIDMapping reverses the given user ID mapping during archive creation
func WithUIDMapping(mappings []IDMapping) TarOption {
	return func(o *TarConfig) {
		o.UIDMaps = mappings
	}
}

// WithGIDMapping reverses the given user ID mapping during archive creation
func WithGIDMapping(mappings []IDMapping) TarOption {
	return func(o *TarConfig) {
		o.GIDMaps = mappings
	}
}

// ExtractTarbal extracts an OCI compatible tar file src to the folder dst, expecting the overlay whiteout format
func ExtractTarbal(ctx context.Context, src io.Reader, dst string, opts ...TarOption) (err error) {
	var cfg TarConfig
	start := time.Now()
	for _, opt := range opts {
		opt(&cfg)
	}

	//nolint:staticcheck,ineffassign
	span, ctx := opentracing.StartSpanFromContext(ctx, "extractTarbal")
	span.LogKV("src", src, "dst", dst)
	defer tracing.FinishSpan(span, &err)

	tarcmd := exec.Command("tar", "--extract", "--preserve-permissions")
	tarcmd.Dir = dst
	tarcmd.Stdin = src

	var msg []byte
	msg, err = tarcmd.CombinedOutput()
	if err != nil {
		return xerrors.Errorf("tar %s: %s", dst, err.Error()+";"+string(msg))
	}

	log.WithField("duration", time.Since(start).Milliseconds()).Debug("untar complete")
	return nil
}
