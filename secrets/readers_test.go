package secrets

import (
	"reflect"
	"testing"
)

func TestStaticSecret(t *testing.T) {
	testsecret := []byte("my-secret")
	s := StaticSecret(testsecret)
	defer s.Close()
	if sec, ok := s.GetSecret(""); !reflect.DeepEqual(sec, testsecret) || !ok {
		t.Errorf("Failed to get staticsecret: %v, got: %s, want: %s", ok, sec, testsecret)
	}
}

func TestStaticDelegateSecret(t *testing.T) {
	testsecret := []byte("my-secret")
	s := StaticSecret(testsecret)
	defer s.Close()

	key := "mykey"
	sd := NewStaticDelegateSecret(s, key)
	defer sd.Close()
	if sec, ok := sd.GetSecret(key); !reflect.DeepEqual(sec, testsecret) || !ok {
		t.Errorf("Failed to get static delegated secret: %v, got: %s, want: %s", ok, sec, testsecret)
	}
}

func TestHostSecret(t *testing.T) {
	testsecret := []byte("my-secret")
	s := StaticSecret(testsecret)
	defer s.Close()

	key := "mykey"
	sd := NewStaticDelegateSecret(s, key)
	defer sd.Close()

	hs := NewHostSecret(sd, map[string]string{
		"exist":                key,
		"http://exist/foo":     "does-not-exist",
		"http://not-exist/foo": key,
		"foo":                  key,
	})
	defer hs.Close()

	if sec, _ := hs.GetSecret(""); reflect.DeepEqual(sec, testsecret) {
		t.Errorf("Failed to get host secret got: %s, want: %s", sec, testsecret)
	}
	if sec, _ := hs.GetSecret("foo"); reflect.DeepEqual(sec, testsecret) {
		t.Errorf("Failed to get host secret got: %s, want: %s", sec, testsecret)
	}
	if sec, _ := hs.GetSecret("http://not-exist/foo"); reflect.DeepEqual(sec, testsecret) {
		t.Errorf("Failed to get host secret got: %s, want: %s", sec, testsecret)
	}

	if sec, ok := hs.GetSecret("http://exist/foo"); !reflect.DeepEqual(sec, testsecret) || !ok {
		t.Errorf("Failed to get host secret: %v, got: %s, want: %s", ok, sec, testsecret)
	}
}
