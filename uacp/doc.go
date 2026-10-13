// Copyright 2018-2020 opcua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

// Package uacp provides encoding/decoding and automated connection handling for
// the OPC UA Connection Protocol.
//
// To establish the connection as a client, call the Dial() function.
//
// The client handshake honors context deadlines and closes the socket when its
// context is canceled, interrupting blocked I/O even without a deadline.
// After a successful handshake, cancellation is detached and the handshake
// deadline is cleared; the caller owns the established connection's lifetime.
//
// To wait for the client to connect to, call Listen() method, and to establish
// connection with the Accept() method.
//
// Once you have a connection you can call Read() to receive full UACP messages
// including the header.
//
// In uacp, *Conn also implements Local/RemoteEndpoint() methods which returns
// EndpointURL of client or server.
//
// The data on top of UACP connection is passed as it is as long as the
// connection is established. In other words, uacp never cares the data even if
// it seems invalid. Users of this package should check the data to make sure it
// is what they want or not.
package uacp
