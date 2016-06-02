package ping

import (
	"github.com/petergardfjall/watcher/config"
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"time"
)

//
// Code related to the pinger.Ssh.
//

// A SSHPinger pinger pings endpoints via the SSH protocol.
type SSHPinger struct {
	Client           *SSHClient
	Command          string
	ExpectedExitCode int
}

// NewSSHPinger creates a new ping.SSHPinger from a pinger configuration.
func NewSSHPinger(pingerConfig *config.Pinger) (Pinger, error) {
	log.Debugf("setting up ssh pinger ...")
	var sshCheck config.SSHCheck
	err := json.Unmarshal(pingerConfig.Check, &sshCheck)
	if err != nil {
		return nil, fmt.Errorf("ssh pinger: illegal check: %s", err)
	}
	if err := sshCheck.Validate(); err != nil {
		return nil, fmt.Errorf("ssh pinger: invalid check: %s", err)
	}

	command, err := loadCommand(&sshCheck)
	if err != nil {
		return nil, fmt.Errorf("ssh pinger: illegal command: %s", err)
	}

	sshClientConfig := NewSSHClientConfig(&sshCheck)
	sshClient, err := NewSSHClient(sshClientConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh pinger: failed to set up ssh client: %s", err)
	}

	pinger := &SSHPinger{
		Client:           sshClient,
		Command:          command,
		ExpectedExitCode: sshCheck.Expect.ExitCode,
	}
	return pinger, nil

}

// Ping pings the configured endpoint for this ping.SSHPinger
func (sshPinger *SSHPinger) Ping() (result Result, output *bytes.Buffer) {
	response, err := sshPinger.Client.Run(sshPinger.Command)
	if err != nil {
		result = Result{StatusNOK, fmt.Errorf("ping failed: %s", err)}
		output = nil
		return
	}

	if sshPinger.ExpectedExitCode != response.ExitStatus {
		result = Result{StatusNOK, fmt.Errorf("expected exit code (%d) differs from actual (%d)", sshPinger.ExpectedExitCode, response.ExitStatus)}
		output = response.Output
		return
	}

	result = Result{StatusOK, nil}
	output = response.Output
	return
}

// loadCommand returns the command that the pinger is configured to execute
// (either via Command or CommandFile).
func loadCommand(sshCheck *config.SSHCheck) (string, error) {
	switch {
	case sshCheck.Command != "":
		return sshCheck.Command, nil
	case sshCheck.CommandFile != "":
		bytes, err := ioutil.ReadFile(sshCheck.CommandFile)
		if err != nil {
			return "", err
		}
		return string(bytes), nil
	default:
		return "", fmt.Errorf("neither Command nor CommandFile specified")
	}

}

//
// Code related to the pinger.SSHClient.
//

const (
	// defaultSSHTimeout is the default SSH connection timeout to use.
	defaultSSHTimeout = 30 * time.Second
)

// SSHClientConfig controls the behavior of a pinger.SSHClient
type SSHClientConfig struct {
	Username        string
	Password        string
	KeyPath         string
	AgentForwarding bool
	Host            string
	Port            int
	Timeout         time.Duration
}

// A SSHClient can be used to execute commands over SSH against remote servers.
type SSHClient struct {
	Config *SSHClientConfig
}

// CommandResult holds the result of executing a command via SSHClient.Run().
type CommandResult struct {
	ExitStatus int
	Output     *bytes.Buffer
}

// NewSSHClientConfig converts a config.SSHCheck to a corresponding
// SSHClientConfig.
func NewSSHClientConfig(sshCheck *config.SSHCheck) *SSHClientConfig {
	var sshConfig = SSHClientConfig{
		Host:            sshCheck.Host,
		Port:            sshCheck.Port,
		Username:        sshCheck.Auth.Username,
		AgentForwarding: sshCheck.Auth.Agent,
	}
	if sshCheck.Auth.Password != nil {
		sshConfig.Password = *sshCheck.Auth.Password
	}
	if sshCheck.Auth.Key != nil {
		sshConfig.KeyPath = *sshCheck.Auth.Key
	}

	return &sshConfig
}

// NewSSHClient cretes a new pinger.SSHClient with a given SSHClientConfig
// to control its behavior.
func NewSSHClient(clientConfig *SSHClientConfig) (*SSHClient, error) {
	if clientConfig.Username == "" {
		return nil, fmt.Errorf("no Username given")
	}
	if clientConfig.Password == "" && clientConfig.KeyPath == "" && !clientConfig.AgentForwarding {
		return nil, fmt.Errorf("no auth mechanism specified (must use one or more of Password, KeyPath and AgentForwarding")
	}
	if !config.ValidHostOrIpAddr(clientConfig.Host) {
		return nil, fmt.Errorf("invalid host/IP address: '%s'", clientConfig.Host)
	}
	if !config.ValidPort(clientConfig.Port) {
		return nil, fmt.Errorf("invalid port: %d", clientConfig.Port)
	}

	return &SSHClient{Config: clientConfig}, nil
}

// passwordAuth returns a password authentication method.
func passwordAuth(password string) ssh.AuthMethod {
	return ssh.Password(password)
}

// publicKeyAuth returns a public key authentication method.
func publicKeyAuth(privateKeyPath string) (ssh.AuthMethod, error) {
	buffer, err := ioutil.ReadFile(privateKeyPath)
	if err != nil {
		return nil, err
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeys(key), nil
}

// agentAuth returns a agent connection authentication method.
func agentAuth() (ssh.AuthMethod, error) {
	sshAgent, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		return nil, err
	}
	return ssh.PublicKeysCallback(agent.NewClient(sshAgent).Signers), nil
}

// clientConfig creates an ssh.ClientConfig to use for a single call of
// pinger.SSHClient.Run()
func (client *SSHClient) clientConfig() (*ssh.ClientConfig, error) {
	var authMethods = []ssh.AuthMethod{}
	if client.Config.Password != "" {
		log.Debugf("using password auth")
		authMethods = append(
			authMethods, passwordAuth(client.Config.Password))
	}
	if client.Config.KeyPath != "" {
		log.Debugf("using public key auth")
		keyAuth, err := publicKeyAuth(client.Config.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to set up public key auth: %s", err)
		}
		authMethods = append(authMethods, keyAuth)
	}
	if client.Config.AgentForwarding {
		log.Debugf("using agent forwarding auth")
		agentAuth, err := agentAuth()
		if err != nil {
			return nil, fmt.Errorf("failed to set up agent auth: %s", err)
		}
		authMethods = append(authMethods, agentAuth)
	}

	var timeout time.Duration
	if client.Config.Timeout != 0 {
		timeout = client.Config.Timeout
	} else {
		timeout = defaultSSHTimeout
	}
	return &ssh.ClientConfig{
		User:    client.Config.Username,
		Timeout: timeout,
		Auth:    authMethods,
	}, nil
}

// connect connects to a remote server (according to the config of the
// SSHClient) and establishes an SSH session.
func (client *SSHClient) connect() (*ssh.Session, error) {
	hostPort := fmt.Sprintf("%s:%d", client.Config.Host, client.Config.Port)
	clientConfig, err := client.clientConfig()
	if err != nil {
		return nil, err
	}

	log.Debugf("Connecting %s@%s ...", clientConfig.User, hostPort)
	connection, err := ssh.Dial("tcp", hostPort, clientConfig)
	if err != nil {
		return nil, fmt.Errorf("%s", err)
	}

	session, err := connection.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to establish session: %s", err)
	}
	log.Debugf("Connected.")
	return session, nil

}

// Run executes a command against a remote server (according to the config
// set for the SSHClient) and returns a CommandResult which indicates the
// command execution result. On connection problems, an error is returned.
func (client *SSHClient) Run(command string) (*CommandResult, error) {
	session, err := client.connect()
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %s", err)
	}
	defer session.Close()

	var result = CommandResult{ExitStatus: 0}
	var writer SharedWriter
	session.Stdout = &writer
	session.Stderr = &writer
	result.Output = &writer.buffer

	if err := session.Run(command); err != nil {
		log.Debugf("command failed: %s", err)
		switch err := err.(type) {
		case *ssh.ExitError:
			result.ExitStatus = err.ExitStatus()
		default:
			result.ExitStatus = -1
		}
	}

	log.Debugf("ssh: result: %d, output:\n%s", result.ExitStatus, result.Output.String())

	return &result, nil
}

// SharedWriter that appends all writes to a buffer and ensures that all
// writes to the buffer are synchronized.
type SharedWriter struct {
	buffer bytes.Buffer
	lock   sync.Mutex
}

func (w *SharedWriter) Write(p []byte) (int, error) {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.buffer.Write(p)
}
