/*
Package target provides a way to interact with local and remote systems.
*/
/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package target

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"intel.com/svr-info/pkg/core"
)

type Target interface {
	RunCommand(*exec.Cmd) (string, string, int, error)
	RunCommandWithTimeout(*exec.Cmd, int) (string, string, int, error)
	CreateTempDirectory(string) (string, error)
	GetArchitecture() (string, error)
	PushFile(string, string) error
	PullFile(string, string) error
	CreateDirectory(string, string) (string, error)
	RemoveDirectory(string) error
	GetName() string
	CanConnect() bool
	GetSudo() string
	SetSudo(string)
}

type LocalTarget struct {
	host string
	sudo string
}

type RemoteTarget struct {
	name        string
	host        string
	port        string
	user        string
	key         string
	pass        string
	sshpassPath string
	sudo        string
	arch        string
}

func NewRemoteTarget(name string, host string, port string, user string, key string, pass string, sshpassPath string, sudo string) *RemoteTarget {
	t := RemoteTarget{name, host, port, user, key, pass, sshpassPath, sudo, ""}
	return &t
}

func NewLocalTarget(host string, sudo string) *LocalTarget {
	t := LocalTarget{host, sudo}
	return &t
}

func (t *RemoteTarget) getSSHFlags(scp bool) (flags []string) {
	flags = []string{
		"-2",
		"-o",
		"UserKnownHostsFile=/dev/null",
		"-o",
		"StrictHostKeyChecking=no",
		"-o",
		"ConnectTimeout=10",       // This one exposes a bug in Windows' SSH client. Each connection takes
		"-o",                      // 10 seconds to establish. https://github.com/PowerShell/Win32-OpenSSH/issues/1352
		"GSSAPIAuthentication=no", // This one is not supported, but is ignored on Windows.
		"-o",
		"ServerAliveInterval=30",
		"-o",
		"ServerAliveCountMax=10", // 30 * 10 = maximum 300 seconds before disconnect on no data
		"-o",
		"ControlPath=" + filepath.Join(os.TempDir(), "%h"), // <<<<<<<<<<<<<
		"-o",
		"ControlMaster=auto",
		"-o",
		"ControlPersist=1m",
	}
	// if scp == false {
	// 	flags = append(flags, "-tt")
	// }
	if t.key != "" {
		keyFlags := []string{
			"-o",
			"PreferredAuthentications=publickey",
			"-o",
			"PasswordAuthentication=no",
			"-i",
			t.key,
		}
		flags = append(flags, keyFlags...)
	}
	if t.port != "" {
		if scp {
			flags = append(flags, "-P")
		} else {
			flags = append(flags, "-p")
		}
		flags = append(flags, t.port)
	}
	return
}

func (t *RemoteTarget) getSSHCommand(command []string) []string {
	var cmd []string
	cmd = append(cmd, "ssh")
	cmd = append(cmd, t.getSSHFlags(false)...)
	if t.user != "" {
		cmd = append(cmd, t.user+"@"+t.host)
	} else {
		cmd = append(cmd, t.host)
	}
	cmd = append(cmd, "--")
	cmd = append(cmd, command...)
	return cmd
}

func (t *RemoteTarget) getSCPCommand(src string, dstDir string, push bool) []string {
	var cmd []string
	cmd = append(cmd, "scp")
	cmd = append(cmd, t.getSSHFlags(true)...)
	if push {
		cmd = append(cmd, src)
		dst := t.host + ":" + dstDir
		if t.user != "" {
			dst = t.user + "@" + dst
		}
		cmd = append(cmd, dst)
	} else { // pull
		s := t.host + ":" + src
		if t.user != "" {
			s = t.user + "@" + s
		}
		cmd = append(cmd, s)
		cmd = append(cmd, dstDir)
	}
	return cmd
}

func (t *LocalTarget) GetSudo() (sudo string) {
	sudo = t.sudo
	return
}

func (t *RemoteTarget) GetSudo() (sudo string) {
	sudo = t.sudo
	return
}

func (t *LocalTarget) SetSudo(sudo string) {
	t.sudo = sudo
}

func (t *RemoteTarget) SetSudo(sudo string) {
	t.sudo = sudo
}

func (t *LocalTarget) RunCommandWithTimeout(cmd *exec.Cmd, timeout int) (stdout string, stderr string, exitCode int, err error) {
	log.Printf("run: %s", strings.Join(cmd.Args, " "))
	return RunLocalCommandWithTimeout(cmd, timeout)
}

func (t *LocalTarget) RunCommand(cmd *exec.Cmd) (stdout string, stderr string, exitCode int, err error) {
	return t.RunCommandWithTimeout(cmd, 0)
}

func (t *RemoteTarget) RunCommandWithTimeout(cmd *exec.Cmd, timeout int) (stdout string, stderr string, exitCode int, err error) {
	sshCommand := t.getSSHCommand(cmd.Args)
	var name string
	var args []string
	if t.key == "" && t.pass != "" {
		name = t.sshpassPath
		args = append(args, "-e")
		args = append(args, "--")
		args = append(args, sshCommand...)
	} else {
		name = sshCommand[0]
		args = sshCommand[1:]
	}
	localCommand := exec.Command(name, args...)
	if t.key == "" && t.pass != "" {
		localCommand.Env = append(localCommand.Env, "SSHPASS="+t.pass)
	}
	logOut := strings.Join(localCommand.Args, " ")
	if t.sudo != "" {
		logOut = strings.Replace(logOut, "SUDO_PASSWORD="+t.sudo, "SUDO_PASSWORD=*************", -1)
	}
	log.Printf("run: %s", logOut)
	return RunLocalCommandWithTimeout(localCommand, timeout)
}

func (t *RemoteTarget) RunCommand(cmd *exec.Cmd) (stdout string, stderr string, exitCode int, err error) {
	return t.RunCommandWithTimeout(cmd, 0)
}

func (t *LocalTarget) GetArchitecture() (arch string, err error) {
	arch = runtime.GOARCH
	return
}

func (t *RemoteTarget) GetArchitecture() (arch string, err error) {
	if t.arch == "" {
		cmd := exec.Command("uname", "-m")
		arch, _, _, err = t.RunCommand(cmd)
		if err != nil {
			return
		}
		arch = strings.TrimSpace(arch)
		t.arch = arch
	} else {
		arch = t.arch
	}
	return
}

// CreateTempDirectory creates a temporary directory on the local target in the directory
// specified by rootDir. If rootDir is an empty string, the temporary directory will be
// created in the system's default directory for temporary files, e.g. /tmp.
// The full path to the temporary directory is returned.
func (t *LocalTarget) CreateTempDirectory(rootDir string) (tempDir string, err error) {
	temp, err := os.MkdirTemp(rootDir, fmt.Sprintf("%s.tmp.", filepath.Base(os.Args[0])))
	if err != nil {
		return
	}
	tempDir, err = core.AbsPath(temp)
	return
}

// CreateTempDirectory creates a temporary directory on the remote target in the directory
// specified by rootDir. If rootDir is an empty string, the temporary directory will be
// created in the system's default directory for temporary files, e.g. /tmp.
// The full path to the temporary directory is returned.
func (t *RemoteTarget) CreateTempDirectory(rootDir string) (tempDir string, err error) {
	var root string
	if rootDir != "" {
		root = fmt.Sprintf("--tmpdir=%s", rootDir)
	}
	cmd := exec.Command("mktemp", "-d", "-t", root, fmt.Sprintf("%s.tmp.XXXXXXXXXX", filepath.Base(os.Args[0])), "|", "xargs", "realpath")
	tempDir, _, _, err = t.RunCommand(cmd)
	tempDir = strings.TrimSpace(tempDir)
	return
}

// PushFile copies file from src to dst
//
//	srcPath: full path to source file
//	dstPath: destination directory or full path to destination file
func (t *LocalTarget) PushFile(srcPath string, dstPath string) (err error) {
	srcFileStat, err := os.Stat(srcPath)
	if err != nil {
		log.Printf("failed to stat: %s", srcPath)
		return
	}
	if !srcFileStat.Mode().IsRegular() {
		err = fmt.Errorf("%s is not a regular file", srcPath)
		return
	}
	srcFile, err := os.Open(srcPath)
	if err != nil {
		log.Printf("failed to open: %s", srcPath)
		return
	}
	defer srcFile.Close()
	dstFileStat, err := os.Stat(dstPath)
	var dstFilename string
	if err == nil && dstFileStat.IsDir() {
		dstFilename = filepath.Join(dstPath, filepath.Base(srcPath))
	} else {
		dstFilename = dstPath
	}
	dstFile, err := os.Create(dstFilename)
	if err != nil {
		log.Printf("failed to create: %s", dstFilename)
		return
	}
	_, err = io.Copy(dstFile, srcFile)
	dstFile.Close()
	if err != nil {
		log.Printf("failed to copy %s to %s", srcPath, dstFilename)
	}
	err = os.Chmod(dstFilename, srcFileStat.Mode())
	if err != nil {
		log.Printf("failed to set file mode for %s", dstFilename)
	}
	return
}

func (t *RemoteTarget) PushFile(srcPath string, dstDir string) (err error) {
	scpCommand := t.getSCPCommand(srcPath, dstDir, true)
	var name string
	var args []string
	if t.key == "" && t.pass != "" {
		name = t.sshpassPath
		args = append(args, "-e")
		args = append(args, "--")
		args = append(args, scpCommand...)
	} else {
		name = scpCommand[0]
		args = scpCommand[1:]
	}
	localCommand := exec.Command(name, args...)
	if t.key == "" && t.pass != "" {
		localCommand.Env = append(localCommand.Env, "SSHPASS="+t.pass)
	}
	log.Printf("run: %s", strings.Join(localCommand.Args, " "))
	_, _, _, err = RunLocalCommand(localCommand)
	return
}

func (t *LocalTarget) PullFile(srcPath string, dstDir string) (err error) {
	err = t.PushFile(srcPath, dstDir)
	return
}

func (t *RemoteTarget) PullFile(srcPath string, dstDir string) (err error) {
	scpCommand := t.getSCPCommand(srcPath, dstDir, false)
	var name string
	var args []string
	if t.key == "" && t.pass != "" {
		name = t.sshpassPath
		args = append(args, "-e")
		args = append(args, "--")
		args = append(args, scpCommand...)
	} else {
		name = scpCommand[0]
		args = scpCommand[1:]
	}
	localCommand := exec.Command(name, args...)
	if t.key == "" && t.pass != "" {
		localCommand.Env = append(localCommand.Env, "SSHPASS="+t.pass)
	}
	log.Printf("run: %s", strings.Join(localCommand.Args, " "))
	_, _, _, err = RunLocalCommand(localCommand)
	return
}

func (t *LocalTarget) CreateDirectory(baseDir string, targetDir string) (dir string, err error) {
	dir = filepath.Join(baseDir, targetDir)
	err = os.Mkdir(dir, 0764)
	return
}

func (t *RemoteTarget) CreateDirectory(baseDir string, targetDir string) (dir string, err error) {
	dir = filepath.Join(baseDir, targetDir)
	cmd := exec.Command("mkdir", dir)
	_, _, _, err = t.RunCommand(cmd)
	return
}

func (t *LocalTarget) RemoveDirectory(targetDir string) (err error) {
	err = os.RemoveAll(targetDir)
	return
}

func (t *RemoteTarget) RemoveDirectory(targetDir string) (err error) {
	cmd := exec.Command("rm", "-rf", targetDir)
	_, _, _, err = t.RunCommand(cmd)
	return
}

func (t *LocalTarget) GetHost() (host string) {
	host = t.host
	return
}

func (t *RemoteTarget) GetHost() (host string) {
	host = t.host
	return
}

func (t *LocalTarget) GetName() (host string) {
	host = t.host //local target host and name are same field
	return
}
func (t *RemoteTarget) GetName() (host string) {
	host = t.name
	return
}

func (t *LocalTarget) CanConnect() bool {
	return true
}

func (t *RemoteTarget) CanConnect() bool {
	cmd := exec.Command("exit", "0")
	_, _, _, err := t.RunCommandWithTimeout(cmd, 5)
	return err == nil
}

func (t *LocalTarget) CanElevatePrivileges() bool {
	if os.Geteuid() == 0 {
		return true // user is root
	}
	if t.sudo != "" {
		cmd := exec.Command("sudo", "-kS", "ls")
		stdin, _ := cmd.StdinPipe()
		go func() {
			defer stdin.Close()
			io.WriteString(stdin, t.sudo+"\n")
		}()
		_, _, _, err := t.RunCommand(cmd)
		if err == nil {
			return true // sudo password works
		}
	}
	cmd := exec.Command("sudo", "-kS", "ls")
	_, _, _, err := t.RunCommand(cmd)
	return err == nil // true - passwordless sudo works
}

func RunLocalCommandWithInputWithTimeout(cmd *exec.Cmd, input string, timeout int) (stdout string, stderr string, exitCode int, err error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()
		commandWithContext := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
		commandWithContext.Env = cmd.Env
		cmd = commandWithContext
	}
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var outbuf, errbuf strings.Builder
	cmd.Stdout = &outbuf
	cmd.Stderr = &errbuf
	err = cmd.Run()
	stdout = outbuf.String()
	stderr = errbuf.String()
	if err != nil {
		exitError := &exec.ExitError{}
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		}
	}
	return
}

func RunLocalCommandWithTimeout(cmd *exec.Cmd, timeout int) (stdout string, stderr string, exitCode int, err error) {
	return RunLocalCommandWithInputWithTimeout(cmd, "", timeout)
}

func RunLocalCommandWithInput(cmd *exec.Cmd, input string) (stdout string, stderr string, exitCode int, err error) {
	return RunLocalCommandWithInputWithTimeout(cmd, input, 0)
}

func RunLocalCommand(cmd *exec.Cmd) (stdout string, stderr string, exitCode int, err error) {
	return RunLocalCommandWithInput(cmd, "")
}
