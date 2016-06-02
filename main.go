package main

import (
	"github.com/petergardfjall/watcher/config"
	"github.com/petergardfjall/watcher/engine"
	"github.com/petergardfjall/watcher/server"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/op/go-logging"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

const usageString = `
Description:

    %s continuously monitors a collection of servers.

Usage:

    %s [OPTIONS] <config-file>

Options:
`

var log = logging.MustGetLogger("main")

// command-line options
var (
	logLevel       = "INFO"
	port           = 8443
	advertisedIP   = ""
	advertisedPort = 0
	ipDetectionURL = "http://ipecho.net/plain"

	// Server certificate and key for HTTPS
	certFile = "/etc/watcherd/cert.pem"
	keyFile  = "/etc/watcherd/key.pem"
)

func initLogging() {
	backend := logging.NewLogBackend(os.Stdout, "", 0)
	formatter := logging.MustStringFormatter(`%{color}%{time:2006-01-02T15:04:05} %{shortfile}:%{shortfunc} ▶ [%{level}]%{color:reset} %{message}`)
	backendFormatter := logging.NewBackendFormatter(backend, formatter)
	logging.SetBackend(backendFormatter)
}

func failWithError(message string, values ...interface{}) {
	fmt.Printf("error: "+message+"\n", values...)
	os.Exit(1)
}

func setLogLevel(logLevel string) {
	level, err := logging.LogLevel(logLevel)
	if err != nil {
		failWithError("illegal log level: '%s'", logLevel)

	}
	logging.SetLevel(level, "")
}

// determineIPFromNetworkInterface tries to determine the IP address for the
// host by checking the machine's network interfaces (note that this IP address
// may not be externally reachable).
func determineIPFromNetworkInterface() (string, error) {
	log.Infof("trying to determine external IP from network interfaces ...")
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %s", err)
	}
	for _, iface := range interfaces {
		if strings.Contains(iface.Name, "docker") {
			// ignore any docker interface
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addresses {
			if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				return ipnet.IP.String(), nil
			}
		}
	}
	return "", fmt.Errorf("failed to find a non-loopback network interface")
}

// determineExternalIP tries to determine the externally reachable IP address to
// advertise for this machine by first contacting an IP detection URL service
// or, in case that check is unsuccessful, by looking at the local network
// interfaces.
func determineExternalIP() (string, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	log.Infof("attempt to determine external IP address via %s ...", ipDetectionURL)
	resp, err := client.Get(ipDetectionURL)
	if err != nil {
		log.Warningf("failed to contact %s: %s", ipDetectionURL, err)
		return determineIPFromNetworkInterface()
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Warningf("failed to read response from %s: %s", ipDetectionURL, err)
		return determineIPFromNetworkInterface()
	}
	return strings.TrimSpace(string(body)), nil
}

// determineAdvertisedIP tries to determine the externally reachable IP address
// of this machine first by checking if it was given on the command-line or,
// second, by checking a IP detection URL or, third, by checking the local
// network interfaces.
func determineAdvertisedIP() string {
	if advertisedIP != "" {
		// --advertised-ip given on command-line
		return advertisedIP
	}
	log.Infof("no --advertised-ip: determinining external IP ...")
	advertisedIP, err := determineExternalIP()
	if err != nil {
		log.Fatalf("no advertised-ip given and failed to detect external IP: %s", err)
	}

	return advertisedIP
}

func init() {
	initLogging()

	// command-line parsing
	programName := path.Base(os.Args[0])
	flag.Usage = func() {
		fmt.Fprintf(os.Stdout, usageString, programName, programName)
		flag.PrintDefaults()
	}
	flag.StringVar(&logLevel, "log-level", logLevel, "Log level to use. One of: DEBUG, INFO, NOTICE, WARNING, ERROR, CRITICAL.")
	flag.IntVar(&port, "port", port, "The HTTP port to set up server on.")
	flag.StringVar(&certFile, "certfile", certFile, "TLS certificate file (pem-formatted) for serving HTTPS traffic.")
	flag.StringVar(&keyFile, "keyfile", keyFile, "TLS key file (pem-formatted) for serving HTTPS traffic.")

	flag.StringVar(&advertisedIP, "advertised-ip", "", "The IP address/hostname advertised in alerts (unless given in config). This should be an externally facing IP address/hostname. If no IP/hostname is explicitly given, a best-effort attempt is made to determine the external IP by first checking with an external IP detection service and, if that fails, by falling back to a non-loopback interface on the local machine.")
	flag.IntVar(&advertisedPort, "advertised-port", 0, "The server port advertised in alerts (unless given in config). This should be an externally facing port that the server can be reached on. If no advertisedPort is specified in the config, and this option is left unspecified, the --port value is used as the advertised port.")
	flag.StringVar(&ipDetectionURL, "ip-detection-url", ipDetectionURL, "URL to a an external IP detection service that will be used to determine the external IP of this host in case no advertised IP is specified (via config or --advertised-ip). The URL must only respond with an IP address string, no attempt will be used to parse html output.")
}

// parseCommandLine parses the command-line and returns the configuration
// file path passed. On failure, the program will exit with an error message.
func parseCommandLine() string {
	flag.Parse()
	if len(flag.Args()) < 1 {
		failWithError("no config file given")
	}
	setLogLevel(logLevel)

	if _, err := os.Stat(certFile); err != nil {
		failWithError("TLS certificate file: %s", err)
	}
	if _, err := os.Stat(keyFile); err != nil {
		failWithError("TLS key file: %s", err)
	}

	configFile := flag.Arg(0)
	return configFile
}

func main() {
	configFile := parseCommandLine()
	configJSON, err := ioutil.ReadFile(configFile)
	if err != nil {
		failWithError("failed to read config file: %s\n", err)
	}

	var config config.Engine
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		failWithError("failed to parse %s: %s", configFile, err)
	}

	// apply default values for values not given in config
	if config.Alerter != nil && config.Alerter.AdvertisedIP == "" {
		log.Infof("no advertisedIP in config: determining advertised IP ...")
		config.Alerter.AdvertisedIP = determineAdvertisedIP()
		log.Infof("using %s as advertised IP", config.Alerter.AdvertisedIP)
	}
	if config.Alerter != nil && config.Alerter.AdvertisedPort == 0 {
		if advertisedPort != 0 {
			config.Alerter.AdvertisedPort = advertisedPort
			log.Infof("no advertisedPort in config: using --advertised-port: %d", advertisedPort)
		} else {
			config.Alerter.AdvertisedPort = port
			log.Infof("no advertisedPort in config: using --port: %d", port)
		}
	}

	if err := config.Validate(); err != nil {
		failWithError("illegal configuration: %s", err)
	}

	log.Infof("setting up engine ...")
	advertisedBaseURL := fmt.Sprintf("https://%s:%d", advertisedIP, port)
	engine, err := engine.NewEngine(&config, advertisedBaseURL)
	if err != nil {
		log.Fatalf("engine setup failed: %s", err)
	}
	log.Infof("engine set up with %d pingers", len(engine.Pingers))

	server, err := server.NewServer(engine, port, certFile, keyFile)
	if err != nil {
		failWithError("failed to create server: %s", err)
	}
	failWithError("server failed: %s", server.Start())
}
