// Copyright 2018-2020 opcua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

// Package opcua provides easy and painless encoding/decoding of OPC UA protocol in pure Golang.
//
// Subscription.ID safely reads a server-assigned ID across automatic reconnect.
// Prefer it to the legacy SubscriptionID field, which can change during recovery.
package opcua
