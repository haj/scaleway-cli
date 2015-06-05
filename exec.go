package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

var cmdExec = &Command{
	Exec:        runExec,
	UsageLine:   "exec [OPTIONS] SERVER COMMAND [ARGS...]",
	Description: "Run a command on a running server",
	Help:        "Run a command on a running server.",
	Examples: `
    $ scw exec myserver bash
    $ scw exec myserver 'tmux a -t joe || tmux new -s joe || bash'
    $ exec_secure=1 scw exec myserver bash
    $ scw exec -w $(scw start $(scw create ubuntu-trusty)) bash
    $ scw exec $(scw start -w $(scw create ubuntu-trusty)) bash
    $ scw exec myserver tmux new -d sleep 10
    $ scw exec myserver ls -la | grep password
`,
}

func init() {
	cmdExec.Flag.BoolVar(&execHelp, []string{"h", "-help"}, false, "Print usage")
	cmdExec.Flag.BoolVar(&execW, []string{"w", "-wait"}, false, "Wait for SSH to be ready")
}

// Flags
var execW bool    // -w, --wait flag
var execHelp bool // -h, --help flag

func NewSshExecCmd(ipAddress string, allocateTTY bool, command []string) []string {
	execCmd := []string{}

	if os.Getenv("DEBUG") != "1" {
		execCmd = append(execCmd, "-q")
	}

	if os.Getenv("exec_secure") != "1" {
		execCmd = append(execCmd, "-o", "UserKnownHostsFile=/dev/null", "-o", "StrictHostKeyChecking=no")
	}

	execCmd = append(execCmd, "-l", "root", ipAddress)

	if allocateTTY {
		execCmd = append(execCmd, "-t")
	}

	execCmd = append(execCmd, "--", "/bin/sh", "-e")

	if os.Getenv("DEBUG") == "1" {
		execCmd = append(execCmd, "-x")
	}

	execCmd = append(execCmd, "-c")

	execCmd = append(execCmd, fmt.Sprintf("%q", strings.Join(command, " ")))

	return execCmd
}

func WaitForServerState(api *ScalewayAPI, serverId string, targetState string) (*ScalewayServer, error) {
	var server *ScalewayServer
	var err error

	for {
		server, err = api.GetServer(serverId)
		if err != nil {
			return nil, err
		}
		if server.State == targetState {
			break
		}
		time.Sleep(1 * time.Second)
	}

	return server, nil
}

func WaitForTcpPortOpen(dest string) error {
	for {
		conn, err := net.Dial("tcp", dest)
		if err == nil {
			defer conn.Close()
			break
		}
		time.Sleep(1 * time.Second)
	}
	return nil
}

func WaitForServerReady(api *ScalewayAPI, serverId string) (*ScalewayServer, error) {
	server, err := WaitForServerState(api, serverId, "running")
	if err != nil {
		return nil, err
	}

	dest := fmt.Sprintf("%s:22", server.PublicAddress.IP)

	err = WaitForTcpPortOpen(dest)
	if err != nil {
		return nil, err
	}

	return server, nil
}

func serverExec(ipAddress string, command []string) error {
	execCmd := append(NewSshExecCmd(ipAddress, true, command))

	log.Debugf("Executing: ssh %s", strings.Join(execCmd, " "))
	spawn := exec.Command("ssh", execCmd...)
	spawn.Stdout = os.Stdout
	spawn.Stdin = os.Stdin
	spawn.Stderr = os.Stderr
	return spawn.Run()
}

func runExec(cmd *Command, args []string) {
	if execHelp {
		cmd.PrintUsage()
	}
	if len(args) < 2 {
		cmd.PrintShortUsage()
	}

	serverId := cmd.GetServer(args[0])

	var server *ScalewayServer
	var err error
	if execW {
		// --wait
		server, err = WaitForServerReady(cmd.API, serverId)
		if err != nil {
			log.Fatalf("Failed to wait for server to be ready, %v", err)
		}
	} else {
		// no --wait
		server, err = cmd.API.GetServer(serverId)
		if err != nil {
			log.Fatalf("Failed to get server information for %s: %v", serverId, err)
		}
	}

	err = serverExec(server.PublicAddress.IP, args[1:])
	if err != nil {
		log.Debugf("Command execution failed: %v", err)
		os.Exit(1)
	}
	log.Debugf("Command successfuly executed")
}