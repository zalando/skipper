package signer

import "strings"

// AllowedQueryHoisting is a whitelist for Build query headers. The boolean value
// represents whether or not it is a pattern.
var AllowedQueryHoisting = InclusiveRules{
	DenyList{RequiredSignedHeaders},
	Patterns{"X-Amz-"},
}

// InclusiveRules rules allow for rules to depend on one another
type InclusiveRules []Rule

// IsValid will return true if all rules are true
func (r InclusiveRules) IsValid(value string) bool {
	for _, rule := range r {
		if !rule.IsValid(value) {
			return false
		}
	}
	return true
}

// RequiredSignedHeaders is a whitelist for Build canonical headers.
var RequiredSignedHeaders = Rules{
	AllowList{
		MapRule{
			"Cache-Control":                         struct{}{},
			"Content-Disposition":                   struct{}{},
			"Content-Encoding":                      struct{}{},
			"Content-Language":                      struct{}{},
			"Content-Md5":                           struct{}{},
			"Content-Type":                          struct{}{},
			"Expires":                               struct{}{},
			"If-Match":                              struct{}{},
			"If-Modified-Since":                     struct{}{},
			"If-None-Match":                         struct{}{},
			"If-Unmodified-Since":                   struct{}{},
			"Range":                                 struct{}{},
			"X-Amz-Acl":                             struct{}{},
			"X-Amz-Copy-Source":                     struct{}{},
			"X-Amz-Copy-Source-If-Match":            struct{}{},
			"X-Amz-Copy-Source-If-Modified-Since":   struct{}{},
			"X-Amz-Copy-Source-If-None-Match":       struct{}{},
			"X-Amz-Copy-Source-If-Unmodified-Since": struct{}{},
			"X-Amz-Copy-Source-Range":               struct{}{},
			"X-Amz-Copy-Source-Server-Side-Encryption-Customer-Algorithm": struct{}{},
			"X-Amz-Copy-Source-Server-Side-Encryption-Customer-Key":       struct{}{},
			"X-Amz-Copy-Source-Server-Side-Encryption-Customer-Key-Md5":   struct{}{},
			"X-Amz-Grant-Full-control":                                    struct{}{},
			"X-Amz-Grant-Read":                                            struct{}{},
			"X-Amz-Grant-Read-Acp":                                        struct{}{},
			"X-Amz-Grant-Write":                                           struct{}{},
			"X-Amz-Grant-Write-Acp":                                       struct{}{},
			"X-Amz-Metadata-Directive":                                    struct{}{},
			"X-Amz-Mfa":                                                   struct{}{},
			"X-Amz-Request-Payer":                                         struct{}{},
			"X-Amz-Server-Side-Encryption":                                struct{}{},
			"X-Amz-Server-Side-Encryption-Aws-Kms-Key-Id":                 struct{}{},
			"X-Amz-Server-Side-Encryption-Customer-Algorithm":             struct{}{},
			"X-Amz-Server-Side-Encryption-Customer-Key":                   struct{}{},
			"X-Amz-Server-Side-Encryption-Customer-Key-Md5":               struct{}{},
			"X-Amz-Storage-Class":                                         struct{}{},
			"X-Amz-Website-Redirect-Location":                             struct{}{},
			"X-Amz-Content-Sha256":                                        struct{}{},
			"X-Amz-Tagging":                                               struct{}{},
		},
	},
	Patterns{"X-Amz-Meta-"},
}

// Patterns is a list of strings to match against
type Patterns []string

// IsValid for Patterns checks each pattern and returns if a match has
// been found
func (p Patterns) IsValid(value string) bool {
	for _, pattern := range p {
		if HasPrefixFold(value, pattern) {
			return true
		}
	}
	return false
}

func HasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[0:len(prefix)], prefix)
}

type AllowList struct {
	Rule
}

// DenyList is a generic Rule for blacklisting
type DenyList struct {
	Rule
}

// IsValid for AllowList checks if the value is within the AllowList
func (b DenyList) IsValid(value string) bool {
	return !b.Rule.IsValid(value)
}

// IsValid will iterate through all rules and see if any rules
// apply to the value and supports nested rules
func (r Rules) IsValid(value string) bool {
	for _, rule := range r {
		if rule.IsValid(value) {
			return true
		}
	}
	return false
}

// IsValid for the map Rule satisfies whether it exists in the map
func (m MapRule) IsValid(value string) bool {
	_, ok := m[value]
	return ok
}

// IsValid for AllowList checks if the value is within the AllowList
func (w AllowList) IsValid(value string) bool {
	return w.Rule.IsValid(value)
}

// IsValid for AllowList checks if the value is within the AllowList
func (b ExcludeList) IsValid(value string) bool {
	return !b.Rule.IsValid(value)
}
