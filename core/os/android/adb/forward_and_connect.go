// Copyright (C) 2017 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package adb

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/google/gapid/core/app"
	"github.com/google/gapid/core/fault"
	"github.com/google/gapid/core/fault/cause"
	"github.com/google/gapid/core/log"
)

const (
	connectionAttempts = 30
	reconnectDelay     = time.Second
	ErrServiceTimeout  = fault.Const("Timeout connecting to service")
)

// ForwardAndConnect forwards the local-abstract-socket las and connects to it.
// When the returned ReadCloser is closed the forwarded port is removed.
// The function takes care of the quirky behavior of ADB forwarded sockets.
func ForwardAndConnect(ctx log.Context, d Device, las string) (io.ReadCloser, error) {
	port, err := LocalFreeTCPPort()
	if err != nil {
		return nil, cause.Explain(ctx, err, "Finding free port")
	}

	if err := d.Forward(ctx, TCPPort(port), NamedAbstractSocket(las)); err != nil {
		return nil, cause.Explain(ctx, err, "Setting up port forwarding")
	}

	once := sync.Once{}
	unforward := func() {
		once.Do(func() { d.RemoveForward(ctx, port) })
	}

	app.AddCleanup(ctx, unforward)

	for i := 0; i < connectionAttempts; i++ {
		if sock, err := net.Dial("tcp", fmt.Sprintf("localhost:%v", port)); err == nil {
			reader := bufio.NewReader(sock)
			if _, err := reader.Peek(1); err == nil {
				close := func() error {
					unforward()
					return sock.Close()
				}

				return readerCustomCloser{reader, close}, nil
			}

			sock.Close()
		}
		time.Sleep(reconnectDelay)
	}

	return nil, cause.Wrap(ctx, ErrServiceTimeout)
}

type readerCustomCloser struct {
	io.Reader
	onClose func() error
}

func (r readerCustomCloser) Close() error {
	return r.onClose()
}
