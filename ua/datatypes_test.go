// Copyright 2018-2020 opcua authors. All rights reserved.
// Use of this source code is governed by a MIT-style license that can be
// found in the LICENSE file.

package ua

import (
	"encoding/binary"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBufferDateTimeWideCalendar(t *testing.T) {
	for _, stamp := range []time.Time{
		time.Date(1600, 12, 31, 23, 59, 59, 123456700, time.UTC),
		time.Date(1601, 1, 1, 0, 1, 49, 0, time.UTC),
		time.Date(1677, 1, 2, 3, 4, 5, 987654300, time.UTC),
		time.Date(1970, 1, 1, 0, 0, 0, 123456700, time.UTC),
		time.Date(2026, 10, 14, 0, 0, 0, 123456700, time.UTC),
		time.Date(2263, 1, 1, 0, 0, 0, 123456700, time.UTC),
		time.Date(9999, 12, 31, 23, 59, 59, 999999900, time.UTC),
	} {
		t.Run(stamp.Format(time.RFC3339Nano), func(t *testing.T) {
			ticks := (stamp.Unix()+11644473600)*10000000 + int64(stamp.Nanosecond())/100
			wire := binary.LittleEndian.AppendUint64(nil, uint64(ticks))
			require.Equal(t, stamp, NewBuffer(wire).ReadTime())
			buffer := NewBuffer(nil)
			buffer.WriteTime(stamp)
			require.NoError(t, buffer.Error())
			if !stamp.After(time.Date(1601, 1, 1, 0, 0, 0, 0, time.UTC)) {
				wire = make([]byte, 8)
			} else if !stamp.Before(time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)) {
				wire = binary.LittleEndian.AppendUint64(nil, math.MaxInt64)
			}
			require.Equal(t, wire, buffer.Bytes())
		})
	}
	require.True(t, NewBuffer(make([]byte, 8)).ReadTime().IsZero())
	buffer := NewBuffer(nil)
	buffer.WriteTime(time.Time{})
	require.Equal(t, make([]byte, 8), buffer.Bytes())
	require.Equal(t, time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC), NewBuffer(binary.LittleEndian.AppendUint64(nil, math.MaxInt64)).ReadTime())
	stamp := time.Date(2263, 1, 1, 0, 0, 0, 123456789, time.UTC)
	fractional := NewBuffer(nil)
	fractional.WriteTime(stamp)
	require.Equal(t, stamp.Truncate(100*time.Nanosecond), NewBuffer(fractional.Bytes()).ReadTime())
}

func TestDataValueTimestampPresenceAndPrecisionAreNotDurationFlags(t *testing.T) {
	server := time.Date(2026, 10, 14, 12, 0, 0, 123456700, time.UTC)
	ticks := (server.Unix()+11644473600)*10000000 + int64(server.Nanosecond())/100
	wire := []byte{0x3d, 0x0a}
	wire = binary.LittleEndian.AppendUint32(wire, math.Float32bits(70.5))
	wire = binary.LittleEndian.AppendUint64(wire, 109*10000000)
	wire = binary.LittleEndian.AppendUint16(wire, 1234)
	wire = binary.LittleEndian.AppendUint64(wire, uint64(ticks))
	wire = binary.LittleEndian.AppendUint16(wire, 5678)
	var value DataValue
	n, err := value.Decode(wire)
	require.NoError(t, err)
	require.Equal(t, len(wire), n)
	require.Equal(t, float32(70.5), value.Value.Value())
	require.Equal(t, time.Date(1601, 1, 1, 0, 1, 49, 0, time.UTC), value.SourceTimestamp)
	require.Equal(t, server, value.ServerTimestamp)
	require.Equal(t, uint16(1234), value.SourcePicoseconds)
	require.Equal(t, uint16(5678), value.ServerPicoseconds)
}

func TestDataValue(t *testing.T) {
	cases := []CodecTestCase{
		{
			Name: "no value",
			Struct: &DataValue{
				EncodingMask: 0x00,
				Value:        MustVariant(nil),
			},
			Bytes: []byte{
				// EncodingMask
				0x00,
			},
		},
		{
			Name: "value only",
			Struct: &DataValue{
				EncodingMask: 0x01,
				Value:        MustVariant(float32(2.50025)),
			},
			Bytes: []byte{
				// EncodingMask
				0x01,
				// Value
				0x0a,                   // type
				0x19, 0x04, 0x20, 0x40, // value
			},
		},
		{
			Name: "value, source timestamp, server timestamp",
			Struct: &DataValue{
				EncodingMask:    0x0d,
				Value:           MustVariant(float32(2.50017)),
				SourceTimestamp: time.Date(2018, time.September, 17, 14, 28, 29, 112000000, time.UTC),
				ServerTimestamp: time.Date(2018, time.September, 17, 14, 28, 29, 112000000, time.UTC),
			},
			Bytes: []byte{
				// EncodingMask
				0x0d,
				// Value
				0x0a,                   // type
				0xc9, 0x02, 0x20, 0x40, // value
				// SourceTimestamp
				0x80, 0x3b, 0xe8, 0xb3, 0x92, 0x4e, 0xd4, 0x01,
				// SeverTimestamp
				0x80, 0x3b, 0xe8, 0xb3, 0x92, 0x4e, 0xd4, 0x01,
			},
		},
		{
			Name: "source timestamp, server timestamp",
			Struct: &DataValue{
				EncodingMask:    0x0c,
				Value:           MustVariant(nil),
				SourceTimestamp: time.Date(2018, time.September, 17, 14, 28, 29, 112000000, time.UTC),
				ServerTimestamp: time.Date(2018, time.September, 17, 14, 28, 29, 112000000, time.UTC),
			},
			Bytes: []byte{
				// EncodingMask
				0x0c,
				// SourceTimestamp
				0x80, 0x3b, 0xe8, 0xb3, 0x92, 0x4e, 0xd4, 0x01,
				// SeverTimestamp
				0x80, 0x3b, 0xe8, 0xb3, 0x92, 0x4e, 0xd4, 0x01,
			},
		},
		{
			Name: "value with nil slice, source timestamp, server timestamp",
			Struct: &DataValue{
				EncodingMask:    0x0d,
				Value:           MustVariant([]string(nil)),
				SourceTimestamp: time.Date(2018, time.September, 17, 14, 28, 29, 112000000, time.UTC),
				ServerTimestamp: time.Date(2018, time.September, 17, 14, 28, 29, 112000000, time.UTC),
			},
			Bytes: []byte{
				// EncodingMask
				0x0d,
				// Value
				0x8c,                   // type
				0xff, 0xff, 0xff, 0xff, // value
				// SourceTimestamp
				0x80, 0x3b, 0xe8, 0xb3, 0x92, 0x4e, 0xd4, 0x01,
				// SeverTimestamp
				0x80, 0x3b, 0xe8, 0xb3, 0x92, 0x4e, 0xd4, 0x01,
			},
		},
	}
	RunCodecTest(t, cases)
}

func TestDataValueArray(t *testing.T) {
	cases := []CodecTestCase{
		{
			Name: "value only; value, source timestamp, server timestamp; source timestamp, server timestamp",
			Struct: []*DataValue{
				{
					EncodingMask: 0x01,
					Value:        MustVariant(float32(2.50025)),
				},
				{
					EncodingMask:    0x0d,
					Value:           MustVariant(float32(2.50017)),
					SourceTimestamp: time.Date(2018, time.September, 17, 14, 28, 29, 112000000, time.UTC),
					ServerTimestamp: time.Date(2018, time.September, 17, 14, 28, 29, 112000000, time.UTC),
				},
				{
					EncodingMask:    0x0c,
					Value:           MustVariant(nil),
					SourceTimestamp: time.Date(2018, time.September, 17, 14, 28, 29, 112000000, time.UTC),
					ServerTimestamp: time.Date(2018, time.September, 17, 14, 28, 29, 112000000, time.UTC),
				},
			},
			Bytes: []byte{
				// length
				0x03, 0x00, 0x00, 0x00,

				// EncodingMask
				0x01,
				// Value
				0x0a,
				0x19, 0x04, 0x20, 0x40,

				// EncodingMask
				0x0d,
				// Value
				0x0a,
				0xc9, 0x02, 0x20, 0x40,
				// SourceTimestamp
				0x80, 0x3b, 0xe8, 0xb3, 0x92, 0x4e, 0xd4, 0x01,
				// ServerTimestamp
				0x80, 0x3b, 0xe8, 0xb3, 0x92, 0x4e, 0xd4, 0x01,

				// EncodingMask
				0x0c,
				// SourceTimestamp
				0x80, 0x3b, 0xe8, 0xb3, 0x92, 0x4e, 0xd4, 0x01,
				// ServerTimestamp
				0x80, 0x3b, 0xe8, 0xb3, 0x92, 0x4e, 0xd4, 0x01,
			},
		},
	}
	RunCodecTest(t, cases)
}

func TestGUID(t *testing.T) {
	cases := []CodecTestCase{
		{
			Name:   "ok",
			Struct: NewGUID("AAAABBBB-CCDD-EEFF-0102-0123456789AB"),
			Bytes: []byte{
				// data1 (inverse order)
				0xbb, 0xbb, 0xaa, 0xaa,
				// data2 (inverse order)
				0xdd, 0xcc,
				// data3 (inverse order)
				0xff, 0xee,
				// data4 (same order)
				0x01, 0x02, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab,
			},
		},
		{
			Name:   "spec",
			Struct: NewGUID("72962B91-FA75-4AE6-8D28-B404DC7DAF63"),
			Bytes: []byte{
				// data1 (inverse order)
				0x91, 0x2b, 0x96, 0x72,
				// data2 (inverse order)
				0x75, 0xfa,
				// data3 (inverse order)
				0xe6, 0x4a,
				// data4 (same order)
				0x8d, 0x28, 0xb4, 0x04, 0xdc, 0x7d, 0xaf, 0x63,
			},
		},
	}
	RunCodecTest(t, cases)
}

func TestLocalizedText(t *testing.T) {
	cases := []CodecTestCase{
		{
			Name:   "nothing",
			Struct: NewLocalizedText(""),
			Bytes:  []byte{0x00},
		},
		{
			Name:   "has-locale",
			Struct: NewLocalizedTextWithLocale("", "foo"),
			Bytes: []byte{
				0x01,
				0x03, 0x00, 0x00, 0x00, 0x66, 0x6f, 0x6f,
			},
		},
		{
			Name:   "has-text",
			Struct: NewLocalizedText("bar"),
			Bytes: []byte{
				0x02,
				0x03, 0x00, 0x00, 0x00, 0x62, 0x61, 0x72,
			},
		},
		{
			Name:   "has-both",
			Struct: NewLocalizedTextWithLocale("bar", "foo"),
			Bytes: []byte{
				0x03,
				0x03, 0x00, 0x00, 0x00, 0x66, 0x6f, 0x6f,
				// second String: "bar"
				0x03, 0x00, 0x00, 0x00, 0x62, 0x61, 0x72,
			},
		},
	}
	RunCodecTest(t, cases)
}
