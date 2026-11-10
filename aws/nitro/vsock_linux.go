//go:build awsnitro && linux

package nitro

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func dialVsock(cid, port uint32) (io.ReadWriteCloser, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_NONBLOCK|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("vsock socket creation failed: %w", err)
	}

	f := os.NewFile(uintptr(fd), fmt.Sprintf("vsock:%d:%d", cid, port))
	if f == nil {
		_ = unix.Close(fd)
		return nil, errors.New("failed to wrap vsock file descriptor")
	}

	sa := &unix.SockaddrVM{
		CID:  cid,
		Port: port,
	}

	rc, err := f.SyscallConn()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	var connectErr error
	err = rc.Control(func(rawFD uintptr) {
		connectErr = unix.Connect(int(rawFD), sa)
	})
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	if errors.Is(connectErr, unix.EINPROGRESS) || errors.Is(connectErr, unix.EALREADY) {
		var sockErr int
		werr := rc.Write(func(rawFD uintptr) bool {
			var gerr error
			sockErr, gerr = unix.GetsockoptInt(int(rawFD), unix.SOL_SOCKET, unix.SO_ERROR)
			if gerr != nil {
				connectErr = gerr
				return true
			}
			if sockErr != 0 {
				connectErr = syscall.Errno(sockErr)
			} else {
				connectErr = nil
			}
			return true
		})
		if werr != nil {
			_ = f.Close()
			return nil, werr
		}
	}

	if connectErr != nil {
		_ = f.Close()
		return nil, fmt.Errorf("vsock connect to cid=%d port=%d failed: %w", cid, port, connectErr)
	}

	return f, nil
}

func parseModuleFile(path string) (uint32, uint32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	return parseModuleContent(string(data))
}

func parseModuleContent(content string) (uint32, uint32, error) {
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "remote:") {
			continue
		}
		remote := strings.TrimSpace(strings.TrimPrefix(line, "remote:"))
		if !strings.HasPrefix(remote, "vsock:") {
			continue
		}
		addr := strings.TrimPrefix(remote, "vsock:")
		var (
			cid     uint32
			port    uint32
			hasCID  bool
			hasPort bool
		)
		for part := range strings.SplitSeq(addr, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			switch strings.TrimSpace(k) {
			case "cid":
				u, err := strconv.ParseUint(strings.TrimSpace(v), 10, 32)
				if err != nil {
					return 0, 0, fmt.Errorf("invalid vsock cid %q: %w", v, err)
				}
				cid = uint32(u)
				hasCID = true
			case "port":
				u, err := strconv.ParseUint(strings.TrimSpace(v), 10, 32)
				if err != nil {
					return 0, 0, fmt.Errorf("invalid vsock port %q: %w", v, err)
				}
				port = uint32(u)
				hasPort = true
			}
		}
		if hasCID {
			if !hasPort {
				port = DefaultEnclavePort
			}
			return cid, port, nil
		}
	}
	return 0, 0, fmt.Errorf("no remote:vsock:cid=<cid>;port=<port> entry found")
}

type tokenStanzaConfig struct {
	PIN      string
	CertPath string
}

func parseStanzaFiles(pattern string) map[string]tokenStanzaConfig {
	out := make(map[string]tokenStanzaConfig)
	if pattern == "" {
		return out
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return out
	}
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		parseStanzaContent(string(data), out)
	}
	return out
}

func parseStanzaContent(content string, out map[string]tokenStanzaConfig) {
	var (
		currentToken string
		currentPIN   string
		currentCert  string
	)
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "pkcs11:"); idx >= 0 {
			uriStr := line[idx:]
			for strings.HasPrefix(uriStr, "pkcs11:pkcs11:") {
				uriStr = strings.TrimPrefix(uriStr, "pkcs11:")
			}
			if end := strings.IndexAny(uriStr, "\"' \t"); end >= 0 {
				uriStr = uriStr[:end]
			}
			uriStr = strings.TrimSuffix(uriStr, ";")
			tok, pin := parsePKCS11URI(uriStr)
			if tok != "" {
				currentToken = tok
				currentPIN = pin
			}
		} else if strings.HasPrefix(line, "ssl_certificate ") || strings.HasPrefix(line, "SSLCertificateFile ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				p := strings.Trim(fields[1], "\";'")
				if p != "" {
					currentCert = p
				}
			}
		}
	}
	if currentToken != "" {
		out[currentToken] = tokenStanzaConfig{
			PIN:      currentPIN,
			CertPath: currentCert,
		}
	}
}

func parsePKCS11URI(uri string) (token, pin string) {
	rest := strings.TrimPrefix(uri, "pkcs11:")
	pathPart, queryPart, _ := strings.Cut(rest, "?")
	for part := range strings.SplitSeq(pathPart, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if k == "token" {
			if decoded, err := url.PathUnescape(v); err == nil {
				token = decoded
			} else {
				token = v
			}
		}
	}
	for part := range strings.SplitSeq(queryPart, "&") {
		for subpart := range strings.SplitSeq(part, ";") {
			k, v, ok := strings.Cut(subpart, "=")
			if !ok {
				continue
			}
			if k == "pin-value" {
				if decoded, err := url.QueryUnescape(v); err == nil {
					pin = decoded
				} else {
					pin = v
				}
			}
		}
	}
	return token, pin
}
