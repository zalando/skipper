package otel

import (
	"context"
	"net/http"
	"testing"

	"github.com/zalando/skipper/predicates"
	"go.opentelemetry.io/otel/baggage"
)

func TestOTelBaggage(t *testing.T) {
	keyProperty, err := baggage.NewKeyProperty("kproperty")
	if err != nil {
		t.Fatalf("Failed to create property: %v", err)
	}
	keyValProperty, err := baggage.NewKeyValueProperty("kvproperty", "kvalue")
	if err != nil {
		t.Fatalf("Failed to create property: %v", err)
	}
	keyPropertyExists, err := baggage.NewKeyProperty("exists")
	if err != nil {
		t.Fatalf("Failed to create property: %v", err)
	}
	keyValPropertyExists, err := baggage.NewKeyValueProperty("exists", "kvalue")
	if err != nil {
		t.Fatalf("Failed to create property: %v", err)
	}

	member, err := baggage.NewMember("exists", "val")
	if err != nil {
		t.Fatalf("Failed to create member: %v", err)
	}
	memberWithKeyProperty, err := baggage.NewMember("exists", "val", keyProperty)
	if err != nil {
		t.Fatalf("Failed to create member: %v", err)
	}
	memberWithKeyValProperty, err := baggage.NewMember("exists", "val", keyValProperty)
	if err != nil {
		t.Fatalf("Failed to create member: %v", err)
	}
	noMatchMemberWithProperties, err := baggage.NewMember("kproperty", "val", keyProperty, keyValProperty)
	if err != nil {
		t.Fatalf("Failed to create member: %v", err)
	}
	noMatchMemberWithMatchingProperties, err := baggage.NewMember("kproperty", "val", keyPropertyExists, keyValPropertyExists)
	if err != nil {
		t.Fatalf("Failed to create member: %v", err)
	}

	for _, tt := range []struct {
		name   string
		args   []interface{}
		member []baggage.Member
		want   bool
		err    error
	}{
		{
			name: "test no args",
			err:  predicates.ErrInvalidPredicateParameters,
		},
		{
			name: "test faulty args",
			args: []interface{}{5},
			err:  predicates.ErrInvalidPredicateParameters,
		},
		{
			name: "test no match member without property",
			args: []interface{}{"no-match"},
			member: []baggage.Member{
				member,
			},
			want: false,
		},
		{
			name: "test no match member with key property",
			args: []interface{}{"no-match"},
			member: []baggage.Member{
				memberWithKeyProperty,
			},
			want: false,
		},
		{
			name: "test no match member with key-val property",
			args: []interface{}{"no-match"},
			member: []baggage.Member{
				memberWithKeyValProperty,
			},
			want: false,
		},
		{
			name: "test no match member with multiple properties",
			args: []interface{}{"no-match"},
			member: []baggage.Member{
				noMatchMemberWithProperties,
			},
			want: false,
		},
		{
			name: "test no match member with multiple matching properties",
			args: []interface{}{"no-match"},
			member: []baggage.Member{
				noMatchMemberWithMatchingProperties,
			},
			want: false,
		},
		{
			name: "test match member without property",
			args: []interface{}{"exists"},
			member: []baggage.Member{
				member,
			},
			want: true,
		},
		{
			name: "test match member with key property",
			args: []interface{}{"exists"},
			member: []baggage.Member{
				memberWithKeyProperty,
			},
			want: true,
		},
		{
			name: "test match member with key-val property",
			args: []interface{}{"exists"},
			member: []baggage.Member{
				memberWithKeyValProperty,
			},
			want: true,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			spec := NewBaggage()
			pred, err := spec.Create(tt.args)
			if tt.err != nil {
				if err != tt.err {
					t.Fatalf("Failed to get expected error: %v, got: %v", tt.err, err)
				}
				return // this is fine we expected error
			}
			if err != nil {
				t.Fatalf("Failed to create predicate spec: %v", err)
			}

			b, err := baggage.New(tt.member...)
			if err != nil {
				t.Fatalf("Failed to create baggage item: %v", err)
			}
			ctx := baggage.ContextWithBaggage(context.Background(), b)
			req, err := http.NewRequestWithContext(ctx, "GET", "http://example.test/", nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			if m := pred.Match(req); m != tt.want {
				t.Fatalf("Failed to match: want: %v, got: %v", tt.want, m)
			}
		})
	}

}
