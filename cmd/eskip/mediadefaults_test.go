// Copyright 2015 Zalando SE
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"net/url"
	"testing"
)

func TestAddDefaultMedia(t *testing.T) {
	for i, item := range []struct {
		cmd       command
		in        *medium
		out       *medium
		err       error
		inResult  *medium
		outResult *medium
	}{{
		// should use default input
		cmd: print,
		in:  nil,
		out: nil,
		err: nil,
		inResult: &medium{
			typ: etcd,
			urls: []*url.URL{
				{Scheme: "http", Host: "127.0.0.1:2379"},
				{Scheme: "http", Host: "127.0.0.1:4001"}},
			path: "/skipper"},
		outResult: nil,
	}, {
		// should use specified input
		cmd: print,
		in: &medium{
			typ: etcd,
			urls: []*url.URL{
				{Scheme: "https", Host: "etcd1.example.org:2379"},
				{Scheme: "https", Host: "etcd2.example.org:4001"}},
			path: "/skipper"},
		out: nil,
		err: nil,
		inResult: &medium{
			typ: etcd,
			oauthToken: "",
			urls: []*url.URL{
				{Scheme: "https", Host: "etcd1.example.org:2379"},
				{Scheme: "https", Host: "etcd2.example.org:4001"}},
			path: "/skipper"},
		outResult: nil,
	}, {
		// should use default output and specified input
		cmd: reset,
		in: &medium{
			typ: stdin,
		},
		out: nil,
		err: nil,
		inResult: &medium{
			typ: stdin,
		},
		outResult: &medium{
			typ: etcd,
			oauthToken: "",
			urls: []*url.URL{
				{Scheme: "http", Host: "127.0.0.1:2379"},
				{Scheme: "http", Host: "127.0.0.1:4001"}},
			path: "/skipper"},
	}, {
		// should use specified output and input
		cmd: reset,
		in: &medium{
			typ: stdin,
		},
		out: &medium{
			typ: etcd,
			urls: []*url.URL{
				{Scheme: "https", Host: "etcd1.example.org:2379"},
				{Scheme: "https", Host: "etcd2.example.org:4001"}},
			path: "/skipper"},
		err: nil,
		inResult: &medium{
			typ: stdin,
		},
		outResult: &medium{
			typ: etcd,
			oauthToken: "",
			urls: []*url.URL{
				{Scheme: "https", Host: "etcd1.example.org:2379"},
				{Scheme: "https", Host: "etcd2.example.org:4001"}},
			path: "/skipper"},
	}} {
		cmdArgs, error := addDefaultMedia(item.cmd, cmdArgs{in: item.in, out: item.out})
		if error != item.err {
			t.Error("wrong error for index: ", i)
		}
		//t.Error("XXX: ", input.urls[0], input.urls[1])
		checkMedium(t, cmdArgs.in, item.inResult, 0, i)
		checkMedium(t, cmdArgs.out, item.outResult, 1, i)
	}
}
