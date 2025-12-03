package secrets

import (
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const refresh = 50 * time.Millisecond

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	time.Sleep(2 * refresh)
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.Remove(path))
	time.Sleep(2 * refresh)
}

func TestSecretPaths(t *testing.T) {
	sp := NewSecretPaths(refresh)
	defer sp.Close()

	checkSecret := func(t *testing.T, path string, expected string) {
		got, ok := sp.GetSecret(path)
		if assert.True(t, ok) {
			assert.Equal(t, []byte(expected), got)
		}
	}

	t.Run("empty has no secrets", func(t *testing.T) {
		_, exists := sp.GetSecret("")
		assert.False(t, exists)

		_, exists = sp.GetSecret("foo")
		assert.False(t, exists)
	})

	t.Run("errors when path does not exist", func(t *testing.T) {
		path := t.TempDir() + "/foo"

		assert.Error(t, sp.Add(path))
	})

	t.Run("errors when path is not a file or directory", func(t *testing.T) {
		path := t.TempDir() + "/foo"

		l, err := net.Listen("unix", path)
		require.NoError(t, err)
		defer l.Close()

		assert.Equal(t, sp.Add(path), ErrWrongFileType)
	})

	t.Run("refreshes file path", func(t *testing.T) {
		path := t.TempDir() + "/foo"

		writeFile(t, path, "created")

		require.NoError(t, sp.Add(path))

		checkSecret(t, path, "created")

		writeFile(t, path, "updated")
		checkSecret(t, path, "updated")

		removeFile(t, path)
		_, exists := sp.GetSecret(path)
		assert.False(t, exists)

		writeFile(t, path, "re-created")
		checkSecret(t, path, "re-created")
	})

	t.Run("errors when symlinked file does not exist", func(t *testing.T) {
		origin := t.TempDir() + "/origin"

		path := t.TempDir() + "/foo"
		require.NoError(t, os.Symlink(origin, path))

		assert.Error(t, sp.Add(path))
	})

	t.Run("refreshes symlink to file", func(t *testing.T) {
		origin := t.TempDir() + "/origin"
		writeFile(t, origin, "created")

		path := t.TempDir() + "/foo"
		require.NoError(t, os.Symlink(origin, path))

		require.NoError(t, sp.Add(path))

		_, exists := sp.GetSecret(origin)
		assert.False(t, exists)

		checkSecret(t, path, "created")

		writeFile(t, origin, "updated")
		checkSecret(t, path, "updated")

		removeFile(t, origin)
		_, exists = sp.GetSecret(path)
		assert.False(t, exists)

		writeFile(t, origin, "re-created")
		checkSecret(t, path, "re-created")
	})

	t.Run("refreshes empty directory path", func(t *testing.T) {
		dir := t.TempDir()

		require.NoError(t, sp.Add(dir))

		path := dir + "/foo"

		writeFile(t, path, "created")
		checkSecret(t, path, "created")

		writeFile(t, path, "updated")
		checkSecret(t, path, "updated")

		removeFile(t, path)
		_, exists := sp.GetSecret(path)
		assert.False(t, exists)

		writeFile(t, path, "re-created")
		checkSecret(t, path, "re-created")
	})

	t.Run("refreshes non-empty directory path", func(t *testing.T) {
		dir := t.TempDir()

		path := dir + "/foo"
		writeFile(t, path, "created")

		require.NoError(t, sp.Add(dir))

		checkSecret(t, path, "created")

		writeFile(t, path, "updated")
		checkSecret(t, path, "updated")

		removeFile(t, path)
		_, exists := sp.GetSecret(path)
		assert.False(t, exists)

		writeFile(t, path, "re-created")
		checkSecret(t, path, "re-created")
	})

	t.Run("ignores subdirectories", func(t *testing.T) {
		dir := t.TempDir()

		require.NoError(t, os.Mkdir(dir+"/subdir", 0700))

		writeFile(t, dir+"/foo", "created")
		writeFile(t, dir+"/subdir/bar", "ignored")

		require.NoError(t, sp.Add(dir))

		checkSecret(t, dir+"/foo", "created")

		_, exists := sp.GetSecret(dir + "/subdir")
		assert.False(t, exists)

		_, exists = sp.GetSecret(dir + "/subdir/")
		assert.False(t, exists)

		_, exists = sp.GetSecret(dir + "/subdir/bar")
		assert.False(t, exists)
	})

	t.Run("refreshes mounted k8s secret", func(t *testing.T) {
		mountPath := t.TempDir()
		require.NoError(t, sp.Add(mountPath))

		ls := func(path string) {
			t.Helper()
			if testing.Verbose() {
				cmd := exec.Command("ls", "-alR", path)
				out, err := cmd.CombinedOutput()
				require.NoError(t, err)
				t.Logf("\n%s", string(out))
			}
		}

		// See https://github.com/kubernetes/kubernetes/blob/d2be69ac11346d2a0fab8c3c168c4255db99c56f/pkg/volume/util/atomic_writer.go#L87-L140
		writeVersionedFile := func(name, content string) {
			timestampedDir := time.Now().UTC().Format("..2006_01_02_15_04_05.000000000")

			// ..2006_01_02_15_04_05.000000000
			require.NoError(t, os.Mkdir(mountPath+"/"+timestampedDir, 0755))
			// ..2006_01_02_15_04_05.000000000/foo
			writeFile(t, mountPath+"/"+timestampedDir+"/"+name, content)

			// ..data_tmp -> ..2006_01_02_15_04_05.000000000
			require.NoError(t, os.Symlink(timestampedDir, mountPath+"/..data_tmp"))
			// atomically rename ..data_tmp to ..data
			require.NoError(t, os.Rename(mountPath+"/..data_tmp", mountPath+"/..data"))

			// foo -> ..data/foo
			_, err := os.Readlink(mountPath + "/" + name)
			if err != nil && os.IsNotExist(err) {
				require.NoError(t, os.Symlink("..data/"+name, mountPath+"/"+name))
			}
		}

		writeVersionedFile("foo", "created")

		ls(mountPath)

		time.Sleep(2 * refresh)
		checkSecret(t, mountPath+"/foo", "created")

		writeVersionedFile("foo", "updated")

		ls(mountPath)

		time.Sleep(2 * refresh)
		checkSecret(t, mountPath+"/foo", "updated")
	})

	t.Run("trims newline", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			path := t.TempDir() + "/foo"

			writeFile(t, path, "created\n")

			require.NoError(t, sp.Add(path))

			checkSecret(t, path, "created")
		})
	})
}

func TestSecretPathsDoesNotRefreshAfterClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		sp := NewSecretPaths(refresh)

		checkSecret := func(t *testing.T, path string, expected string) {
			got, ok := sp.GetSecret(path)
			if assert.True(t, ok) {
				assert.Equal(t, []byte(expected), got)
			}
		}

		path := t.TempDir() + "/foo"

		writeFile(t, path, "created")

		require.NoError(t, sp.Add(path))

		checkSecret(t, path, "created")

		sp.Close()

		time.Sleep(2 * refresh)

		writeFile(t, path, "updated")
		checkSecret(t, path, "created")
	})
}
