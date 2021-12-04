package backend

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func (backend Backend) MustBindLocalToRemoteOverSSH(local, sshStr, remote string) {
	if err := backend.BindLocalToRemoteOverSSH(local, sshStr, remote); err != nil {
		backend.logger.Fatal(err)
	}
}

// BindLocalToRemoteOverSSH connects to remote server over SSH, bind local port
// to remote port. The local and remote string should contain IP address and
// port number like this: 127.0.0.1:8080.  The SSH string should contain user
// name, identity file path, remote host and port. For example:
// root:~/.ssh/id_rsa@remote-host:22.
func (backend Backend) BindLocalToRemoteOverSSH(local, sshStr, remote string) error {
	user, key, host := parseSSH(sshStr)
	if local == "" || user == "" || key == "" || host == "" || remote == "" {
		return nil
	}
	if strings.HasPrefix(key, "~") {
		if home, _ := os.UserHomeDir(); home != "" {
			key = filepath.Join(home, strings.TrimPrefix(key, "~"))
		}
	}
	b, err := os.ReadFile(key)
	if err != nil {
		return err
	}
	signer, err := ssh.ParsePrivateKey(b)
	if err != nil {
		return err
	}
	backend.logger.Info("SSH: Connecting to", host)
	client, err := ssh.Dial("tcp", host, &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		return err
	}
	defer client.Close()
	backend.logger.Info("SSH: Connected. Listening on", remote)
	listener, err := client.Listen("tcp", remote)
	if err != nil {
		return err
	}
	defer listener.Close()
	for {
		err = accept(local, listener)
		if err == nil {
			continue
		}
		if err == io.EOF {
			return nil
		}
		backend.logger.Warning("SSH Error:", err)
		time.Sleep(1 * time.Second)
	}
	return nil
}

// ParseSSHConnStr parses strings like this:
// 127.0.0.1:8080::root:~/.ssh/id_rsa@remote-host:22::127.0.0.1:9999
func ParseSSHConnStr(connStr string) (local, ssh, remote string) {
	parts := strings.SplitN(connStr, "::", 3)
	if len(parts) == 3 {
		local = parts[0]
		ssh = parts[1]
		remote = parts[2]
	} else if len(parts) == 2 {
		local = parts[0]
		ssh = parts[1]
	} else if len(parts) == 1 {
		ssh = parts[0]
	}
	return
}

func accept(local string, listener net.Listener) error {
	client, err := listener.Accept()
	if err != nil {
		return err
	}
	defer client.Close()
	conn, err := net.Dial("tcp", local)
	if err != nil {
		return err
	}
	defer conn.Close()
	go io.Copy(client, conn)
	io.Copy(conn, client)
	return nil
}

func parseSSH(ssh string) (user, key, host string) {
	parts := strings.SplitN(ssh, "@", 2)
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		host = parts[0]
		return
	}
	partsA := strings.SplitN(parts[0], ":", 2)
	if len(partsA) == 2 {
		user = partsA[0]
		key = partsA[1]
	} else if len(partsA) == 1 {
		user = partsA[0]
	}
	host = parts[1]
	return
}
