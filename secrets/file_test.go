package secrets

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func Test_secretPaths_GetSecret(t *testing.T) {
	for _, tt := range []struct {
		name   string
		known  map[string][]byte
		s      string
		want   []byte
		wantOk bool
	}{
		{
			name:   "nothing known should not find anything for empty string",
			s:      "",
			wantOk: false,
		},
		{
			name:   "nothing known should not find anything for non-empty string",
			s:      "key",
			wantOk: false,
		},
		{
			name: "something known should not find anything for wrong string",
			known: map[string][]byte{
				"dat": []byte("data"),
			},
			s:      "key",
			wantOk: false,
		},
		{
			name: "something known should find for key",
			known: map[string][]byte{
				"key": []byte("data"),
			},
			s:      "key",
			wantOk: true,
			want:   []byte("data"),
		}} {
		t.Run(tt.name, func(t *testing.T) {
			sp := &secretPaths{known: tt.known}
			got, ok := sp.GetSecret(tt.s)
			if ok != tt.wantOk {
				t.Errorf("secretPaths.GetSecret() ok = %v, want %v", ok, tt.wantOk)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("secretPaths.GetSecret() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_secretPaths_Add(t *testing.T) {
	temproot, err := ioutil.TempDir(os.TempDir(), "skipper-secrets")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
		return
	}
	defer func() {
		t.Logf("remove %s", temproot)
		os.RemoveAll(temproot)
	}()
	watchit := temproot + "/watch"
	os.MkdirAll(watchit+"/subdir", 0777)
	dat := []byte("data")
	filename := "/afile"
	if err := os.Symlink(watchit+filename, watchit+"/mysymlink"); err != nil {
		t.Errorf("Failed to create symlink")
	}
	if err := os.Symlink(watchit, watchit+"/mysymlinktodir"); err != nil {
		t.Errorf("Failed to create symlink to dir")
	}

	for _, tt := range []struct {
		name      string
		addFile   string
		writeFile string
		want      []byte
		wantOk    bool
		wantErr   bool
	}{
		{
			name:      "Should GetSecret after write to watched file",
			addFile:   watchit + filename,
			writeFile: watchit + filename,
			want:      dat,
			wantOk:    true,
			wantErr:   false,
		},
		{
			name:      "Should GetSecret after write to watched directory",
			addFile:   watchit,
			writeFile: watchit + filename + "2",
			want:      dat,
			wantOk:    true,
			wantErr:   false,
		},
		{

			name:      "Should GetSecret after write to watched symlink",
			addFile:   watchit + "/mysymlink",
			writeFile: watchit + "/mysymlink",
			want:      dat,
			wantOk:    true,
			wantErr:   false,
		},
		{

			name:      "Should GetSecret after write to watched symlinked directory",
			addFile:   watchit + "/mysymlinktodir",
			writeFile: watchit + filename + "3",
			want:      dat,
			wantOk:    true,
			wantErr:   false,
		},
		{
			name:      "Should not change secrets by a change in a not watched directory",
			addFile:   watchit,
			writeFile: watchit + "/subdir/not-watched-file",
			want:      []byte{},
			wantOk:    false,
			wantErr:   false,
		},
		{
			name:      "Should not add secret on add file that does not exist",
			addFile:   watchit + "/does-not-exist",
			writeFile: watchit + filename,
			want:      []byte{},
			wantOk:    false,
			wantErr:   true,
		}} {
		t.Run(tt.name, func(t *testing.T) {
			sp := newSecretPaths()
			err := ioutil.WriteFile(tt.writeFile, []byte(""), 0644)
			if err != nil {
				t.Errorf("Failed to create file: %v", err)
			}

			err = sp.Add(tt.addFile)
			if (err != nil) != tt.wantErr {
				t.Errorf("secretPaths.Add() error = %v, wantErr %v", err, tt.wantErr)
			}

			err = ioutil.WriteFile(tt.writeFile, dat, 0644)
			if err != nil {
				t.Errorf("Failed to write file: %v", err)
			}
			time.Sleep(100 * time.Millisecond) // wait for fsnotify

			got, ok := sp.GetSecret(filepath.Base(tt.writeFile))
			if ok != tt.wantOk {
				t.Errorf("Failed to get ok: got: %v want: %v", ok, tt.wantOk)
			}
			if string(got) != string(tt.want) {
				t.Errorf("Failed to get secret got: '%s', want: '%s'", string(got), string(tt.want))
			}
		})
	}
}

func Test_secretPaths_Close(t *testing.T) {
	temproot, err := ioutil.TempDir(os.TempDir(), "skipper-secrets-close")
	if err != nil {
		t.Errorf("Failed to create temp dir: %v", err)
		return
	}
	defer func() {
		t.Logf("remove %s", temproot)
		os.RemoveAll(temproot)
	}()
	watchit := temproot + "/watch"
	os.MkdirAll(watchit, 0777)
	dat := []byte("data")
	afile := watchit + "/afile"

	sp := newSecretPaths()
	err = ioutil.WriteFile(afile, []byte(""), 0644)
	if err != nil {
		t.Errorf("Failed to create file: %v", err)
	}
	err = sp.Add(afile)
	if err != nil {
		t.Errorf("Failed to Add file to watch list: %v", err)
	}
	err = ioutil.WriteFile(afile, dat, 0644)
	if err != nil {
		t.Errorf("Failed to write to file: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // wait for fsnotify

	got, ok := sp.GetSecret(filepath.Base(afile))
	if !ok {
		t.Errorf("Should have secret: %v", ok)
	}
	if string(got) != string(dat) {
		t.Errorf("Secret content, got: %s, want: %s", string(got), string(dat))
	}

	sp.Close()

	err = ioutil.WriteFile(afile, []byte("hello"), 0644)
	if err != nil {
		t.Errorf("Failed to write to file: %v", err)
	}
	time.Sleep(100 * time.Millisecond) // wait for fsnotify
	got, ok = sp.GetSecret(filepath.Base(afile))
	if !ok {
		t.Errorf("Should have former secret: %v", ok)
	}
	if string(got) != string(dat) {
		t.Errorf("Changed content after close, got: %s", string(got))
	}
}
