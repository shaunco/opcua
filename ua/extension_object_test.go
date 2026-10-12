// Copyright 2018-2020 opcua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ua

import (
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtensionObject(t *testing.T) {
	cases := []CodecTestCase{
		{
			Name:   "anonymous-user-identity-token",
			Struct: NewExtensionObject(&AnonymousIdentityToken{PolicyID: "anonymous"}),
			Bytes: []byte{
				// TypeID
				0x01, 0x00, 0x41, 0x01,
				// EncodingMask
				0x01,
				// Length
				0x0d, 0x00, 0x00, 0x00,
				// AnonymousIdentityToken
				0x09, 0x00, 0x00, 0x00, 0x61, 0x6e, 0x6f, 0x6e, 0x79, 0x6d, 0x6f, 0x75, 0x73,
			},
		},
	}
	RunCodecTest(t, cases)
}

func unknownExtensionObjectBytes(body []byte) []byte {
	b := []byte{0x01, 0x02, 0x60, 0xea, ExtensionObjectBinary, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(b[5:], uint32(len(body)))
	return append(b, body...)
}

func TestExtensionObjectRawBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{name: "nonempty", body: []byte{0, 0xff, 1, 2, 3}},
		{name: "empty", body: []byte{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := unknownExtensionObjectBytes(tc.body)
			var e ExtensionObject
			n, err := e.Decode(append(wire, 0xab))
			require.NoError(t, err)
			require.Equal(t, len(wire), n)
			require.Equal(t, NewFourByteExpandedNodeID(2, 60000), e.TypeID)
			require.Equal(t, uint8(ExtensionObjectBinary), e.EncodingMask)
			require.Nil(t, e.Value)
			require.NotNil(t, e.RawBody)
			require.Equal(t, tc.body, e.RawBody)

			e.UpdateMask()
			require.Equal(t, uint8(ExtensionObjectBinary), e.EncodingMask)
			got, err := e.Encode()
			require.NoError(t, err)
			require.Equal(t, wire, got)
		})
	}
}

func TestExtensionObjectRawBodyOwned(t *testing.T) {
	wire := unknownExtensionObjectBytes([]byte{1, 2, 3})
	var e ExtensionObject
	_, err := e.Decode(wire)
	require.NoError(t, err)
	wire[9] = 0xff
	require.Equal(t, []byte{1, 2, 3}, e.RawBody)
	e.RawBody[1] = 0xfe
	require.Equal(t, byte(2), wire[10])

	encoded, err := e.Encode()
	require.NoError(t, err)
	encoded[11] = 0xfd
	require.Equal(t, []byte{1, 0xfe, 3}, e.RawBody)
}

func TestExtensionObjectRawBodyVariants(t *testing.T) {
	raw := &ExtensionObject{
		TypeID:       NewFourByteExpandedNodeID(2, 60000),
		EncodingMask: ExtensionObjectBinary,
		RawBody:      []byte{0, 0xff, 1, 2},
	}
	empty := &ExtensionObject{
		TypeID:       NewFourByteExpandedNodeID(3, 60001),
		EncodingMask: ExtensionObjectBinary,
		RawBody:      []byte{},
	}
	for _, tc := range []struct {
		name  string
		value interface{}
	}{
		{name: "scalar", value: raw},
		{name: "array", value: []*ExtensionObject{raw, empty}},
		{name: "nested", value: MustVariant(raw)},
		{name: "nested-array", value: []*Variant{MustVariant(raw), MustVariant([]*ExtensionObject{raw, empty})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := MustVariant(tc.value)
			wire, err := v.Encode()
			require.NoError(t, err)
			var decoded Variant
			n, err := decoded.Decode(wire)
			require.NoError(t, err)
			require.Equal(t, len(wire), n)
			require.Equal(t, v, &decoded)
			got, err := decoded.Encode()
			require.NoError(t, err)
			require.Equal(t, wire, got)
		})
	}
}

func TestExtensionObjectDecodeReset(t *testing.T) {
	known := NewExtensionObject(&AnonymousIdentityToken{PolicyID: "anonymous"})
	knownWire, err := known.Encode()
	require.NoError(t, err)
	xml := XMLElement("<vendor/>")
	xmlObject := NewExtensionObject(&xml)
	xmlWire, err := xmlObject.Encode()
	require.NoError(t, err)

	for _, tc := range []struct {
		name  string
		wire  []byte
		value interface{}
		raw   []byte
	}{
		{name: "empty-object", wire: []byte{0, 0, ExtensionObjectEmpty}},
		{name: "null-body", wire: []byte{1, 2, 0x60, 0xea, ExtensionObjectBinary, 0xff, 0xff, 0xff, 0xff}},
		{name: "empty-unknown-body", wire: unknownExtensionObjectBytes(nil), raw: []byte{}},
		{name: "empty-known-body", wire: []byte{1, 0, 0x41, 1, ExtensionObjectBinary, 0, 0, 0, 0}},
		{name: "unknown", wire: unknownExtensionObjectBytes([]byte{7}), raw: []byte{7}},
		{name: "known", wire: knownWire, value: known.Value},
		{name: "xml", wire: xmlWire, value: &xml},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, previous := range [][]byte{knownWire, unknownExtensionObjectBytes([]byte{1, 2})} {
				var e ExtensionObject
				_, err := e.Decode(previous)
				require.NoError(t, err)
				n, err := e.Decode(tc.wire)
				require.NoError(t, err)
				require.Equal(t, len(tc.wire), n)
				require.Equal(t, tc.value, e.Value)
				require.Equal(t, tc.raw, e.RawBody)
			}
		})
	}
}

func TestExtensionObjectRawBodyErrors(t *testing.T) {
	wire := unknownExtensionObjectBytes([]byte{1, 2, 3})
	for n := 0; n < len(wire); n++ {
		e := ExtensionObject{Value: &AnonymousIdentityToken{}, RawBody: []byte{9}}
		_, err := e.Decode(wire[:n])
		require.ErrorIs(t, err, io.ErrUnexpectedEOF, "prefix length %d", n)
		require.Nil(t, e.Value)
		require.Nil(t, e.RawBody)
	}

	// The complete body still contains a truncated registered structure.
	malformed := []byte{1, 0, 0x41, 1, ExtensionObjectBinary, 4, 0, 0, 0, 1, 0, 0, 0}
	var e ExtensionObject
	_, err := e.Decode(malformed)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.IsType(t, &AnonymousIdentityToken{}, e.Value)
	require.Nil(t, e.RawBody)
}

func TestExtensionObjectDecodedValuePreferred(t *testing.T) {
	xml := XMLElement("<vendor/>")
	for _, value := range []interface{}{&AnonymousIdentityToken{PolicyID: "anonymous"}, &xml} {
		e := NewExtensionObject(value)
		want, err := e.Encode()
		require.NoError(t, err)
		e.RawBody = []byte{0xff}
		e.UpdateMask()
		got, err := e.Encode()
		require.NoError(t, err)
		require.Equal(t, want, got)

		var decoded ExtensionObject
		_, err = decoded.Decode(got)
		require.NoError(t, err)
		require.Equal(t, value, decoded.Value)
		require.Nil(t, decoded.RawBody)
	}
}
