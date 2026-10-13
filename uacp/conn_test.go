// Copyright 2018-2020 opcua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package uacp

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gopcua/opcua/errors"
	"github.com/stretchr/testify/require"
)

func handshakePair(t *testing.T) (*Conn, *Conn) {
	t.Helper()
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })
	clientTCP, err := net.DialTCP("tcp", nil, listener.Addr().(*net.TCPAddr))
	require.NoError(t, err)
	t.Cleanup(func() { clientTCP.Close() })
	serverTCP, err := listener.AcceptTCP()
	require.NoError(t, err)
	t.Cleanup(func() { serverTCP.Close() })
	client, err := NewConn(clientTCP, nil)
	require.NoError(t, err)
	server, err := NewConn(serverTCP, nil)
	require.NoError(t, err)
	require.NoError(t, server.SetDeadline(time.Now().Add(2*time.Second)))
	return client, server
}

func startHandshake(t *testing.T, client *Conn, ctx context.Context) <-chan error {
	t.Helper()
	result := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		result <- client.Handshake(ctx, "opc.tcp://localhost:4840")
	}()
	t.Cleanup(func() {
		client.TCPConn.Close()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("handshake goroutine did not stop after connection close")
		}
	})
	return result
}

func TestHandshakeCancellation(t *testing.T) {
	for _, withDeadline := range []bool{false, true} {
		name := "without_deadline"
		if withDeadline {
			name = "long_deadline"
		}
		t.Run(name, func(t *testing.T) {
			client, server := handshakePair(t)
			var ctx context.Context
			var cancel context.CancelFunc
			if withDeadline {
				ctx, cancel = context.WithTimeout(context.Background(), time.Hour)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()
			result := startHandshake(t, client, ctx)
			hello, err := server.Receive()
			require.NoError(t, err)
			require.Equal(t, "HELF", string(hello[:4]))
			cancel()
			select {
			case err := <-result:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				t.Fatal("cancellation did not interrupt silent-peer handshake")
			}
		})
	}
}

func TestHandshakeDeadline(t *testing.T) {
	client, server := handshakePair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	result := startHandshake(t, client, ctx)
	_, err := server.Receive()
	require.NoError(t, err)
	select {
	case err := <-result:
		require.Error(t, err)
		if !errors.Is(err, context.DeadlineExceeded) {
			var timeout net.Error
			require.ErrorAs(t, err, &timeout)
			require.True(t, timeout.Timeout())
		}
	case <-time.After(time.Second):
		t.Fatal("deadline did not interrupt silent-peer handshake")
	}
}

func TestHandshakeSuccessDetachesCancellation(t *testing.T) {
	client, server := handshakePair(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := startHandshake(t, client, ctx)
	_, err := server.Receive()
	require.NoError(t, err)
	require.NoError(t, server.Send("ACKF", DefaultClientACK))
	select {
	case err := <-result:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("normal handshake did not complete")
	}
	<-ctx.Done()
	require.NoError(t, server.Send("MSGF", &Message{Data: []byte{1, 2, 3}}))
	require.NoError(t, client.SetReadDeadline(time.Now().Add(time.Second)))
	message, err := client.Receive()
	require.NoError(t, err, "completed handshake context must not close the connection or leave its old deadline")
	require.Equal(t, []byte{1, 2, 3}, message[hdrlen:])
}

func TestHandshakeAlreadyCanceled(t *testing.T) {
	client, _ := handshakePair(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, client.Handshake(ctx, "opc.tcp://localhost:4840"), context.Canceled)
}

func TestConn(t *testing.T) {
	t.Run("server exists ", func(t *testing.T) {
		ep := "opc.tcp://127.0.0.1:4840/foo/bar"
		ln, err := Listen(context.Background(), ep, nil)
		require.NoError(t, err)
		defer ln.Close()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan struct{})
		acceptErr := make(chan error, 1)
		go func() {
			c, err := ln.Accept(ctx)
			if err != nil {
				acceptErr <- err
				return
			}
			defer c.Close()
			close(done)
		}()

		if _, err = Dial(ctx, ep); err != nil {
			t.Error(err)
		}

		select {
		case <-done:
		case err := <-acceptErr:
			require.Fail(t, "accept fail: %v", err)
		case <-time.After(time.Second):
			require.Fail(t, "timed out")
		}
	})

	t.Run("Address resolves, but does not implement a opcua-server", func(t *testing.T) {
		ep := "opc.tcp://example.com:56789"

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := Dial(ctx, ep)
		var operr *net.OpError
		if errors.As(err, &operr) && !operr.Timeout() {
			t.Error(err)
		}
	})
}

func TestClientWrite(t *testing.T) {
	ep := "opc.tcp://127.0.0.1:4840/foo/bar"
	ln, err := Listen(context.Background(), ep, nil)
	require.NoError(t, err, "Listen failed")
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var srvConn *Conn
	done := make(chan int)
	acceptErr := make(chan error, 1)
	go func() {
		defer ln.Close()
		var err error
		srvConn, err = ln.Accept(ctx)
		if err != nil {
			acceptErr <- err
			return
		}
		done <- 0
	}()

	cliConn, err := Dial(ctx, ep)
	require.NoError(t, err, "Dial failed")

	for {
		select {
		case _, ok := <-done:
			require.True(t, ok, "failed to setup secure channel")
			goto NEXT
		case err := <-acceptErr:
			require.Fail(t, "accept fail: %v", err)
		case <-time.After(time.Second):
			require.Fail(t, "timed out")
		}
	}

NEXT:
	msg := &Message{Data: []byte{0xde, 0xad, 0xbe, 0xef}}
	err = cliConn.Send("MSGF", msg)
	require.NoError(t, err, "Send failed")

	got, err := srvConn.Receive()
	require.NoError(t, err, "Receive failed")

	got = got[hdrlen:]

	want, err := msg.Encode()
	require.NoError(t, err, "Encode failed")

	require.Equal(t, want, got)
}

func TestServerWrite(t *testing.T) {
	ep := "opc.tcp://127.0.0.1:4840/foo/bar"
	ln, err := Listen(context.Background(), ep, nil)
	require.NoError(t, err, "Listen failed")
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var srvConn *Conn
	done := make(chan int)
	acceptErr := make(chan error, 1)
	go func() {
		defer ln.Close()
		var err error
		srvConn, err = ln.Accept(ctx)
		if err != nil {
			acceptErr <- err
			return
		}
		done <- 0
	}()

	cliConn, err := Dial(ctx, ep)
	require.NoError(t, err, "Dial failed")

	for {
		select {
		case _, ok := <-done:
			require.True(t, ok, "failed to setup secure channel")
			goto NEXT
		case err := <-acceptErr:
			require.Fail(t, "accept fail: %v", err)
		case <-time.After(time.Second):
			require.Fail(t, "timed out")
		}
	}

NEXT:
	want := []byte{0xde, 0xad, 0xbe, 0xef}
	_, err = srvConn.Write(want)
	require.NoError(t, err, "Write failed")

	got := make([]byte, cliConn.ReceiveBufSize())
	n, err := cliConn.Read(got)
	require.NoError(t, err, "Read failed")

	got = got[:n]
	require.Equal(t, want, got)
}
