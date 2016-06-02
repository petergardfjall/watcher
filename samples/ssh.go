package main

import (
	"github.com/petergardfjall/watcher/ping"

	"flag"
	"fmt"
	"github.com/op/go-logging"
	"io/ioutil"
	"os"
	"path"
)

var log = logging.MustGetLogger("main")

// command-line arguments
var (
	username        = os.Getenv("USER")
	password        string
	keyFile         string
	agentForwarding = false
	port            = 22
	// If true, interpret command as a path
	commandFile = false
	// Either a raw command or a path to a script (if commandFile is true)
	command string
)

const usageString = `
Description:

    %s runs a command over ssh against a remote server.

Usage:

    %s [OPTIONS] <host> <command>
    %s [OPTIONS] --cmdfile <host> <file>

Options:
`

func initLogging() {
	backend := logging.NewLogBackend(os.Stdout, "", 0)
	formatter := logging.MustStringFormatter(`%{color}%{time:2006-01-02T15:04:05.999-07:00} %{shortfile}:%{shortfunc} ▶ [%{level}]%{color:reset} %{message}`)
	backendFormatter := logging.NewBackendFormatter(backend, formatter)
	logging.SetBackend(backendFormatter)
}

func init() {
	initLogging()

	// command-line parsing
	progName := path.Base(os.Args[0])
	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, usageString, progName, progName, progName)
		flag.PrintDefaults()
	}

	flag.IntVar(&port, "port", port, "SSH Port.")
	flag.StringVar(&username, "username", username, "Account user name.")
	flag.StringVar(&password, "password", "", "Account password.")
	flag.StringVar(&keyFile, "keyfile", "", "Private key file.")
	flag.BoolVar(&agentForwarding, "forward-agent", agentForwarding, "Enable forwarding of the authentication agent connection.")
	flag.BoolVar(&commandFile, "cmdfile", commandFile, "Set to interpret command as a file path to shell script.")

}

func failWithError(message string, values ...interface{}) {
	fmt.Printf("error: "+message+"\n", values...)
	os.Exit(1)

}

func main() {
	flag.Parse()
	if len(flag.Args()) < 1 {
		failWithError("no host given")
	}
	host := flag.Args()[0]

	if len(flag.Args()) < 2 {
		failWithError("no command [file] given")
	}
	var command string
	if commandFile {
		// interpret command as a file path
		filePath := flag.Args()[1]
		commandBytes, err := ioutil.ReadFile(filePath)
		if err != nil {
			log.Fatalf("failed to read command file: %s", filePath)
		}
		command = string(commandBytes)
	} else {
		command = flag.Args()[1]
	}

	if password == "" && keyFile == "" && !agentForwarding {
		failWithError("no auth mechanism given (either password, keyfile, or forward-agent should be specified")
	}

	client, err := ping.NewSSHClient(&ping.SSHClientConfig{
		Host:            host,
		Port:            port,
		Username:        username,
		Password:        password,
		KeyPath:         keyFile,
		AgentForwarding: agentForwarding,
	})
	if err != nil {
		log.Fatalf("failed to set up client: %s", err)
	}

	result, err := client.Run(command)
	if result != nil {
		log.Infof("exit status: %d", result.ExitStatus)
		log.Infof("output:\n%s", result.Output.String())
	}
	if err != nil {
		log.Fatalf("failed to run command: %s", err)
	}

}
