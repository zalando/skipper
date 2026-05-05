package auth

import (
	"os"
	"testing"

	"github.com/zalando/skipper/secrets"
)

func TestFlowState(t *testing.T) {
	t.Run("all ok", func(t *testing.T) {
		secrets := secrets.NewRegistry()
		defer secrets.Close()

		fs := newFlowState(secrets, "testdata/authsecret")
		const u = "https://www.example.org/foo"
		s, err := fs.createState(u)
		if err != nil {
			t.Fatal(err)
		}

		st, err := fs.extractState(s)
		if err != nil {
			t.Fatal(err)
		}

		if st.RequestURL != u {
			t.Errorf("invalid redirect url: %q, expected: %q", st.RequestURL, u)
		}
	})

	t.Run("create and extract works if secret was deleted", func(t *testing.T) {
		// intially we need to hold the Encrypter
		secretsFile := "testdata/authsecret"
		b, err := os.ReadFile(secretsFile)
		if err != nil {
			t.Fatal(err)
		}
		// restore file with content
		defer func() {
			os.WriteFile(secretsFile, b, 0644)
		}()

		secrets := secrets.NewRegistry()
		defer secrets.Close()

		fs := newFlowState(secrets, secretsFile)
		const u = "https://www.example.org/foo"
		s, err := fs.createState(u)
		if err != nil {
			t.Fatal(err)
		}

		st, err := fs.extractState(s)
		if err != nil {
			t.Fatal(err)
		}

		if st.RequestURL != u {
			t.Errorf("invalid redirect url: %q, expected: %q", st.RequestURL, u)
		}

		os.Remove(secretsFile)

		s, err = fs.createState(u)
		if err != nil {
			t.Fatal(err)
		}

		st, err = fs.extractState(s)
		if err != nil {
			t.Fatal(err)
		}

		if st.RequestURL != u {
			t.Errorf("invalid redirect url: %q, expected: %q", st.RequestURL, u)
		}
	})
}
