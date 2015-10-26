package main

import (
	"net/url"
	"testing"
)

func checkMedium(t *testing.T, left, right *medium, testIndex, itemIndex int) {
	if left == nil || right == nil {
		if left != right {
			t.Error("failed to parse medium")
		}

		return
	}

	if left.typ != right.typ ||
		left.path != right.path ||
		left.eskip != right.eskip {
		t.Error("failed to parse medium", left.typ, right.typ, testIndex, itemIndex)
	}

	if len(left.urls) == len(right.urls) {
		for k, u := range left.urls {
			if u.String() != right.urls[k].String() {
				t.Error("failed to parse medium urls", testIndex, itemIndex)
			}
		}
	} else {
		t.Error("failed to parse medium urls", testIndex, itemIndex)
	}

	if len(left.ids) == len(right.ids) {
		for k, id := range left.ids {
			if id != right.ids[k] {
				t.Error("failed to parse medium ids", testIndex, itemIndex)
			}
		}
	} else {
		t.Error("failed to parse medium ids", testIndex, itemIndex)
	}
}

func TestInvalidCommand(t *testing.T) {
	_, _, err := validateSelectMedia("invalid", nil)
	if err != invalidCommand {
		t.Error("failed to fail")
	}
}

func TestTooManuInputs(t *testing.T) {
}

func TestValidateSelectMedia(t *testing.T) {
	for i, item := range []struct {
		command command
		media   []*medium
		fail    bool
		err     error
		in      *medium
		out     *medium
	}{{

		// invalid command:
		"invalid",
		nil,
		true,
		invalidCommand,
		nil,
		nil,
	}, {

		// too many inputs
		"check",
		[]*medium{{}, {}},
		true,
		tooManyInputs,
		nil,
		nil,
	}, {

		// inline ids for check
		"check",
		[]*medium{{typ: inlineIds}},
		true,
		invalidInputType,
		nil,
		nil,
	}, {

		// defaults to etcd
		"check",
		nil,
		false,
		nil,
		&medium{
			typ: etcd,
			urls: []*url.URL{
				{Scheme: "http", Host: "127.0.0.1:2379"},
				{Scheme: "http", Host: "127.0.0.1:4001"}},
			path: "/skipper"},
		nil,
	}, {

		// returns input for check
		"check",
		[]*medium{{typ: stdin}},
		false,
		nil,
		&medium{typ: stdin},
		nil,
	}, {

		// returns input for print
		"print",
		[]*medium{{typ: stdin}},
		false,
		nil,
		&medium{typ: stdin},
		nil,
	}, {

		// missing input
		"upsert",
		nil,
		true,
		missingInput,
		nil,
		nil,
	}, {

		// too many inputs
		"upsert",
		[]*medium{{typ: stdin}, {typ: file}, {typ: etcd}},
		true,
		tooManyInputs,
		nil,
		nil,
	}, {

		// ids when not delete
		"upsert",
		[]*medium{{typ: inlineIds}},
		true,
		invalidInputType,
		nil,
		nil,
	}, {

		// ids accepted when delete
		"delete",
		[]*medium{{typ: inlineIds}},
		false,
		nil,
		&medium{typ: inlineIds},
		&medium{
			typ: etcd,
			urls: []*url.URL{
				{Scheme: "http", Host: "127.0.0.1:2379"},
				{Scheme: "http", Host: "127.0.0.1:4001"}},
			path: "/skipper"},
	}, {

		// output defaults to etcd when write
		"upsert",
		[]*medium{{typ: stdin}},
		false,
		nil,
		&medium{typ: stdin},
		&medium{
			typ: etcd,
			urls: []*url.URL{
				{Scheme: "http", Host: "127.0.0.1:2379"},
				{Scheme: "http", Host: "127.0.0.1:4001"}},
			path: "/skipper"},
	}, {

		// input and output specified
		"upsert",
		[]*medium{{
			typ: stdin,
		}, {
			typ: etcd,
			urls: []*url.URL{
				{Scheme: "https", Host: "etcd1.example.org:4242"},
				{Scheme: "https", Host: "etcd2.example.org:4545"}},
			path: "/skipper",
		}},
		false,
		nil,
		&medium{typ: stdin},
		&medium{
			typ: etcd,
			urls: []*url.URL{
				{Scheme: "https", Host: "etcd1.example.org:4242"},
				{Scheme: "https", Host: "etcd2.example.org:4545"}},
			path: "/skipper",
		},
	}} {
		in, out, err := validateSelectMedia(item.command, item.media)
		if item.fail {
			if err == nil {
				t.Error("failed to fail")
			}

			if item.err != nil && err != item.err {
				t.Error("invalid error")
			}
		} else {
			if err != nil {
				t.Error(err)
			}

			checkMedium(t, item.in, in, i, 0)
			checkMedium(t, item.out, out, i, 1)
		}
	}
}
