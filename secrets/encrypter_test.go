package secrets

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testingSecretSource struct {
	getCount  int
	secretKey string
}

func (s *testingSecretSource) GetSecret() ([][]byte, error) {
	s.getCount++
	return [][]byte{[]byte(s.secretKey)}, nil
}

func (s *testingSecretSource) SetSecret(key string) {
	s.secretKey = key
}

func TestEncryptDecrypt(t *testing.T) {
	for _, tt := range []struct {
		name      string
		secretKey string
		plaintext string
		secSrc    *testingSecretSource
		wantErr   bool
	}{
		{
			name:      "shorter secret than plaintext",
			secretKey: "abc",
			plaintext: "helloworld",
			secSrc:    &testingSecretSource{},
			wantErr:   false,
		},
		{
			name:      "longer secret than plaintext",
			secretKey: "abcdefghijklmn",
			plaintext: "hello",
			secSrc:    &testingSecretSource{},
			wantErr:   false,
		},
		{
			name:      "long plaintext",
			secretKey: "mykey",
			plaintext: strings.Repeat("hello", 2000),
			secSrc:    &testingSecretSource{},
			wantErr:   false,
		},
		{
			name:      "long secret",
			secretKey: strings.Repeat("abcdefghijklmn", 2000),
			plaintext: "hello",
			secSrc:    &testingSecretSource{},
			wantErr:   false,
		},
		{
			name:      "long plaintext and secret",
			secretKey: strings.Repeat("abcdefghijklmn", 2000),
			plaintext: strings.Repeat("helloworld", 5000),
			secSrc:    &testingSecretSource{},
			wantErr:   false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.secSrc.SetSecret(tt.secretKey)
			enc := &Encrypter{
				secretSource: tt.secSrc,
			}
			enc.RefreshCiphers()

			plain := []byte(tt.plaintext)
			b, err := enc.Encrypt(plain)
			if err != nil && !tt.wantErr {
				t.Errorf("failed to encrypt data block: %v", err)
			}
			if tt.wantErr && err == nil {
				t.Fatal("wantErr while encrypting, but got no error")
			}

			decenc, err := enc.Decrypt(b)
			if err != nil && !tt.wantErr {
				t.Errorf("failed to decrypt data block: %v", err)
			}
			if string(decenc) != tt.plaintext && !tt.wantErr {
				t.Errorf("decrypted plaintext is not the same as plaintext: %s", string(decenc))
			}
			if tt.wantErr && err == nil {
				t.Fatal("wantErr while decrypting, but got no error")
			}
		})
	}
}

func TestCipherRefreshing(t *testing.T) {
	d := 1 * time.Second
	sleepD := 4 * d
	SecSource := &testingSecretSource{secretKey: "abc"}
	enc := &Encrypter{
		secretSource: SecSource,
		closer:       make(chan struct{}),
		closedHook:   make(chan struct{}),
	}
	enc.runCipherRefresher(d)
	time.Sleep(sleepD)
	enc.Close()
	<-enc.closedHook
	assert.True(t, SecSource.getCount >= 3, "secret fetched more than 3 times in %s", sleepD)
}

func Test_GetSecret(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Errorf("Failed to get CWD: %v", err)
	}

	for _, tt := range []struct {
		name    string
		args    string
		want    [][]byte
		wantErr bool
	}{
		{
			name:    "secret file does not exist",
			args:    "does-not-exist",
			want:    [][]byte{},
			wantErr: true,
		},
		{
			name: "secret file that exists",
			args: fmt.Sprintf("%s/../skptesting/enckey", wd),
			want: [][]byte{
				[]byte("very secure"),
			},
			wantErr: false,
		},
	} {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			fss := &fileSecretSource{fileName: tt.args}
			got, err := fss.GetSecret()

			if (tt.wantErr && err == nil) || (!tt.wantErr && err != nil) {
				t.Errorf("Got error but does not want an error, or the other way around: wantErr: %v, err: %v", tt.wantErr, err)
			}
			if err == nil && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Failed to GetSecret: Want %v, got %v, err %v", tt.want, got, err)
			}
		})
	}

}

func TestGetEncrypter(t *testing.T) {
	var wg sync.WaitGroup
	reg := NewRegistry()
	defer reg.Close()
	fd, err := os.CreateTemp(".", "")
	if err != nil {
		t.Fatalf("Failed to create temp file for test: %v", err)
	}
	defer func() {
		fd.Close()
		os.Remove(fd.Name())
	}()

	_, err = fd.WriteString("very secure password")
	if err != nil {
		t.Fatalf("Failed to write secret: %v", err)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			enc, err := reg.GetEncrypter(time.Second, fd.Name())
			if err != nil {
				t.Errorf("Failed to get encrypter: %v", err)
			}

			nonce, err := enc.CreateNonce()
			if err != nil {
				t.Errorf("Failed to create nonce: %v", err)
			}
			if len(nonce) == 0 {
				t.Error("Failed to create vaild nonce")
			}

			clearText := "hello"
			encText, err := enc.Encrypt([]byte(clearText))
			if err != nil {
				t.Errorf("Failed to create cipher text: %v", err)
			}

			decText, err := enc.Decrypt(encText)
			if err != nil {
				t.Errorf("Failed to decrypt cipher text: %v", err)
			}
			if s := string(decText); s != clearText {
				t.Errorf("Failed to decrypt cipher text: %s != %s", s, clearText)
			}
			wg.Done()
		}()
	}
	wg.Wait()
	//time.Sleep(2 * time.Second)
}
