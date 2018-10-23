package swarm

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeMessage(t *testing.T) {
	for _, tt := range []struct {
		msg      string
		input    []byte
		expected []byte
	}{{
		msg:      "string",
		input:    []byte("hello world"),
		expected: []byte("hello world"),
	}, {
		msg:      "string2",
		input:    []byte("hello\x00foo"),
		expected: []byte("hello\x00foo"),
	}, {
		msg:      "byte sequence",
		input:    []byte{0x05, 0x01, 0xfe, 0x00},
		expected: []byte{0x05, 0x01, 0xfe, 0x00},
	}, {
		msg:      "weird byte sequence",
		input:    []byte{0x00, 0x01, 0xff, 0x7f, 0x0d, 0x0a, 0x00},
		expected: []byte{0x00, 0x01, 0xff, 0x7f, 0x0d, 0x0a, 0x00},
	}} {

		t.Run(tt.msg, func(t *testing.T) {
			msg := &message{
				Type:   sharedValue,
				Source: "me",
				Key:    "value",
				Value:  tt.input,
			}
			encMsg, err := encodeMessage(msg)
			if err != nil {
				t.Errorf("Failed to encode message err: %v", err)
			}
			got, err := decodeMessage(encMsg)
			if err != nil {
				t.Errorf("Failed to decode message err: %v", err)
			}
			buf, ok := got.Value.([]byte)
			if !ok {
				t.Errorf("Failed to convert value data to []byte: %v", got.Value)
			}
			if !bytes.Equal(tt.expected, buf) {
				t.Errorf("Failed encode decode message: got %v, expected %v", got, tt.expected)
			}

			msg2 := &message{
				Type:   broadcast,
				Source: "me",
				Key:    "value",
				Value:  tt.input,
			}
			encMsg2, err := encodeMessage(msg2)
			if err != nil {
				t.Errorf("Failed to encode message err: %v", err)
			}
			got2, err := decodeMessage(encMsg2)
			if err != nil {
				t.Errorf("Failed to decode message err: %v", err)
			}
			buf2, ok := got2.Value.([]byte)
			if !ok {
				t.Errorf("Failed to convert value data to []byte: %v", got2.Value)
			}
			if !bytes.Equal(tt.expected, buf2) {
				t.Errorf("Failed encode decode message: got %v, expected %v", got2, tt.expected)
			}
		})
	}
}
