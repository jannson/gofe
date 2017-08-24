// TODO:  Better handling of errors from SSH Back-end executions.
// ExecOnly() / Exec()
// 'Process exited with status 1' Not very informative :)

package fe

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"strings"
	"time"

	"github.com/md2k/gofe/models"
	"golang.org/x/crypto/ssh"
)

const DefaultTimeout = 30 * time.Second

type SSHFileExplorer struct {
	FileExplorer
	Host     string
	User     string
	Password string
	client   *ssh.Client
}

func NewSSHFileExplorer(host string, user string, password string) *SSHFileExplorer {
	return &SSHFileExplorer{Host: host, User: user, Password: password}
}

func (fe *SSHFileExplorer) Init() error {
	sshConfig := &ssh.ClientConfig{
		User: fe.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(fe.Password),
		},
	}

	conn, err := net.DialTimeout("tcp", fe.Host, DefaultTimeout)
	if err != nil {
		return err
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, fe.Host, sshConfig)
	if err != nil {
		return err
	}
	client := ssh.NewClient(sshConn, chans, reqs)

	fe.client = client

	return nil
}

func normalizePath(path string) string {
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func (fe *SSHFileExplorer) Mkdir(path string) error {
	return fe.ExecOnly(fmt.Sprintf("mkdir -p \"%s\"", normalizePath(path)))
}

func (fe *SSHFileExplorer) ListDir(path string) ([]models.ListDirEntry, error) {
	// Most ugliest Fix, but nothing other can be done here to able parse LS in correct way and support files with spaces in names :)
	// Have another idea, but this is for later TODO
	ls, err := fe.Exec("ls --time-style=long-iso -1 -q -l --hide-control-chars " + normalizePath(path) + " | awk '{n=split($0,array,\" \")} { for (i = 1; i <= 7; i++) {printf \"%s|\",array[i]}} { for (i = 8; i <= n; i++) {printf \"%s \",array[i]};print \"\"}'")
	// ls, err := fe.Exec(fmt.Sprintf("ls --time-style=long-iso -l %s", normalizePath(path)))
	if err != nil {
		return nil, err
	}
	return parseLsOutput(string(ls)), nil
}

func (fe *SSHFileExplorer) Rename(path string, newPath string) error {
	return fe.ExecOnly(fmt.Sprintf("mv \"%s\" \"%s\"", normalizePath(path), normalizePath(newPath)))
}

func (fe *SSHFileExplorer) Move(path []string, newPath string) (err error) {
	for _, target := range path {
		err = fe.ExecOnly(fmt.Sprintf("mv \"%s\" \"%s\"", normalizePath(target), normalizePath(newPath)))
	}
	return err
}

func (fe *SSHFileExplorer) Copy(path []string, newPath string, singleFilename string) (err error) {
	for _, target := range path {
		err = fe.ExecOnly(fmt.Sprintf("cp -r \"%s\" \"%s/%s\"", normalizePath(target), normalizePath(newPath), singleFilename))
	}
	return err
}

func (fe *SSHFileExplorer) Delete(path []string) (err error) {
	for _, target := range path {
		err = fe.ExecOnly(fmt.Sprintf("rm --interactive=never -r \"%s\"", normalizePath(target)))
	}
	return err
}

func (fe *SSHFileExplorer) Chmod(path []string, code string, recursive bool) (err error) {
	for _, target := range path {
		recurs := ""
		if recursive {
			recurs = "-R"
		}
		err = fe.ExecOnly(fmt.Sprintf("chmod %s %s \"%s\"", recurs, code, normalizePath(target)))
	}
	return err
}

func (fe *SSHFileExplorer) UploadFile(destination string, part *multipart.Part) (err error) {
	// Write directly to Disk
	// dst, err := os.Create(fmt.Sprintf("%s/%s", destination, part.FileName()))
	// defer dst.Close()
	// if err != nil {
	// 	return err
	// }

	// if _, err := io.Copy(dst, part); err != nil {
	// 	return err
	// }
	// err = fe.ExecOnly(fmt.Sprintf("chmod 664 \"%s/%s\"", , destination, normalizePath(target)))
	// return nil

	// Write over SSH StdIn Pipe
	session, err := fe.client.NewSession()
	if err != nil {
		return err
	}
	// defer session.Close()

	input, err := session.StdinPipe()
	if err != nil {
		return err
	}

	// Open cat stdin input remotely
	err = session.Start(fmt.Sprintf("cat - > \"%s/%s\"", destination, part.FileName()))
	if err != nil {
		return err
	}

	// Write to Session's Stdin
	if _, err := io.Copy(input, part); err != nil {
		return err
	}
	// Close on finish (Guess it is better to send signal , so for TODO)
	session.Close()

	return err
}

func (fe *SSHFileExplorer) Close() error {
	return fe.client.Close()
}

func (fe *SSHFileExplorer) GetContent(item string) (string, error) {
	return "", errors.New("not support")
}

func (fe *SSHFileExplorer) Edit(item string, content string) error {
	return errors.New("not support")
}

// Execute cmd on the remote host and return stderr and stdout
func (fe *SSHFileExplorer) Exec(cmd string) ([]byte, error) {
	log.Println(cmd)
	session, err := fe.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.CombinedOutput(cmd)
}

func (fe *SSHFileExplorer) ExecOnly(cmd string) error {
	log.Println(cmd)
	session, err := fe.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	_, err1 := session.CombinedOutput(cmd)
	if err1 != nil {
		return err1 // + " - " + string(out)
	}
	return nil
}

func parseLsOutput(lsout string) []models.ListDirEntry {
	lines := strings.Split(lsout, "\n")
	results := []models.ListDirEntry{}
	for _, line := range lines {
		// fmt.Println(idx, line)
		if len(line) != 0 && !strings.HasPrefix(line, "total") {
			// our Dirty LS Fix with AWK return line as follow:
			// drwxr-xr-x|2|root|root|4096|2016-07-13|17:47|bin
			tmp_tokens := strings.SplitN(line, "|", 8)
			var tokens []string
			for _, token := range tmp_tokens {
				tokens = append(tokens, strings.TrimSpace(token))
			}
			// fmt.Println(tokens)
			if len(tokens) >= 8 {
				ftype := "file"
				if strings.HasPrefix(tokens[0], "d") {
					ftype = "dir"
				}
				results = append(results, models.ListDirEntry{tokens[7], tokens[0], tokens[4], tokens[5] + " " + tokens[6] + ":00", ftype})
			}
		}
	}
	return results
}
