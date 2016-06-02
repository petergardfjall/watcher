package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	// Regular expression describing a valid IPv4 address
	ipv4AddrRegexp = regexp.MustCompile("^[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}\\.[0-9]{1,3}$")
	// Regular expression describing a valid DNS host name (RFC 1123)
	hostnameRegexp = regexp.MustCompile("^(([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\\-]*[a-zA-Z0-9])\\.)*([A-Za-z0-9]|[A-Za-z0-9][A-Za-z0-9\\-]*[A-Za-z0-9])$")

	// Regular expression that describes a valid pinger name (must
	// be possible to use as a path segment in a URL)
	validPingerName = regexp.MustCompile("^[a-zA-Z0-9_\\-\\.]+$")
)

// Engine is the root type of the watcher engine configuration.
type Engine struct {
	DefaultSchedule *Schedule `json:"defaultSchedule"`
	Pingers         []Pinger  `json:"pingers"`
	Alerter         *Alerter  `json:"alerter"`
}

// A Pinger definition in an Engine config. Note that the "check" field of the
// pinger config is not parsed until the type of the pinger is known. It carries
// protocol-specific check instructions.
type Pinger struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Check    json.RawMessage `json:"check"`
	Schedule *Schedule       `json:"schedule"`
}

// A Schedule describes how often to carry out a ping check.
type Schedule struct {
	Interval *Duration `json:"interval"`
	Retries  *Retries  `json:"retries"`
}

// Retries describes the retry behavior for a pinger.
type Retries struct {
	// Total number of attempts to make for each ping.
	Attempts int `json:"attempts"`
	// Delay between attempts.
	Delay Duration `json:"delay"`
	// Whether to use exponential backoff to increase delay by a factor 2
	// with every retry.
	ExponentialBackoff bool `json:"exponentialBackoff"`
}

// HTTPCheck describes a check for a HTTP(S) pinger.
type HTTPCheck struct {
	URL        string          `json:"url"`
	VerifyCert bool            `json:"verifyCert"`
	BasicAuth  *HTTPBasicAuth  `json:"basicAuth"`
	Expect     HTTPExpectation `json:"expect"`
	Timeout    *Duration       `json:"timeout"`
}

// HTTPBasicAuth describes how to authenticate in case a HTTPCheck
// requires basic authentication.
type HTTPBasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// HTTPExpectation is the expected status code of the response in order for
// a HTTPCheck to be deemed successful.
type HTTPExpectation struct {
	StatusCode int `json:"statusCode"`
}

// SSHCheck descibres a check for an SSH pinger.
type SSHCheck struct {
	Host        string         `json:"host"`
	Port        int            `json:"port"`
	Auth        SSHAuth        `json:"auth"`
	Command     string         `json:"command"`
	CommandFile string         `json:"commandFile"`
	Expect      SSHExpectation `json:"expect"`
	Timeout     *Duration      `json:"timeout"`
}

// SSHAuth describes how to authenticate for an SSHCheck. Either
// agent forwarding, password or public key auth must be selected.
type SSHAuth struct {
	Username string  `json:"username"`
	Password *string `json:"password"`
	Key      *string `json:"key"`
	Agent    bool    `json:"agent"`
}

// SSHExpectation is the expected exit code of the script in order for
// a SSHCheck to be deemed successful.
type SSHExpectation struct {
	ExitCode int `json:"exitCode"`
}

// Alerter describes how to configure alerting.
type Alerter struct {
	// The externally reachable IP address to advertise in alerts.
	AdvertisedIP string `json:"advertisedIP"`
	// The watcher port to advertise in alerts.
	AdvertisedPort int `json:"advertisedPort"`
	// Delay between reminders on pings that fail repeatedly.
	ReminderDelay Duration `json:"reminderDelay"`
	// An email alerter to use (or nil).
	Email *Email `json:"email"`
}

// Email alerter configuration.
type Email struct {
	SMTPHost string     `json:"smtpHost"`
	SMTPPort int        `json:"smtpPort"`
	Auth     *EmailAuth `json:"auth"`
	From     string     `json:"from"`
	To       []string   `json:"to"`
}

// EmailAuth describes how to authenticate to a SMTP host.
type EmailAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// Duration is a wrapper type for JSON (un)marshalling of time.Duration
type Duration struct {
	time.Duration
}

// UnmarshalJSON implements the json.Unmarshaler interface for Duration.
func (d *Duration) UnmarshalJSON(b []byte) (err error) {
	sd := string(b[1 : len(b)-1])
	d.Duration, err = time.ParseDuration(sd)
	return
}

// MarshalJSON implements the json.Marshaler interface for Duration.
func (d Duration) MarshalJSON() (b []byte, err error) {
	return []byte(fmt.Sprintf(`"%s"`, d.String())), nil
}

// Validate validates an Engine.
func (engine *Engine) Validate() error {
	if engine.DefaultSchedule != nil {
		if err := engine.DefaultSchedule.Validate(); err != nil {
			return fmt.Errorf("engine: %s", err)
		}
	}

	takenNames := make(map[string]bool)
	for _, pinger := range engine.Pingers {
		// enforce name uniqueness.
		// note: map retrieval on missing key yields zero-value (false)
		if takenNames[pinger.Name] {
			return fmt.Errorf("engine: pinger name '%s' is used multiple times -- pinger names must be unique", pinger.Name)
		}
		takenNames[pinger.Name] = true

		if err := pinger.Validate(); err != nil {
			return fmt.Errorf("engine: %s", err)
		}
	}

	if engine.Alerter != nil {
		if err := engine.Alerter.Validate(); err != nil {
			return fmt.Errorf("engine: %s", err)
		}
	}

	return nil
}

// Validate validates the generic parts of a Pinger.
// Specific validation is carried out by the Pinger
// implementation, which depends on the value of Pinger.Type.
func (pinger *Pinger) Validate() error {
	if pinger.Name == "" {
		return fmt.Errorf("pinger: missing name")
	}
	if !ValidPingerName(pinger.Name) {
		return fmt.Errorf("pinger: illegal name: '%s' (must be of form '%s')", pinger.Name, validPingerName)
	}

	if pinger.Type == "" {
		return fmt.Errorf("pinger '%s': missing type", pinger.Name)
	}
	if len(pinger.Check) == 0 || string(pinger.Check) == "null" {
		return fmt.Errorf("pinger '%s': missing check", pinger.Name)
	}
	// note: pinger.Check is validated by the Pinger implementation
	// (determined by the value of pinger.Type)

	if pinger.Schedule != nil {
		if err := pinger.Schedule.Validate(); err != nil {
			return fmt.Errorf("pinger '%s': %s", pinger.Name, err)
		}
	}

	return nil
}

// Validate validates a Schedule.
func (schedule *Schedule) Validate() error {
	if schedule.Interval == nil {
		return fmt.Errorf("schedule: missing interval")
	}

	if schedule.Retries == nil {
		return fmt.Errorf("schedule: missing retries")
	}

	if err := schedule.Retries.Validate(); err != nil {
		return fmt.Errorf("schedule: %s", err)
	}

	return nil
}

// Validate validates a Retries.
func (retry *Retries) Validate() error {
	if retry.Attempts < 0 {
		return fmt.Errorf("retries: attempts must be a positive number")
	}

	return nil
}

// Validate validates a HTTPCheck.
func (check *HTTPCheck) Validate() error {
	if _, err := url.Parse(check.URL); err != nil {
		return fmt.Errorf("http check: invalid URL: %s", err)
	}
	if check.BasicAuth != nil {
		if err := check.BasicAuth.Validate(); err != nil {
			return fmt.Errorf("http check: %s", err)
		}
	}

	if err := check.Expect.Validate(); err != nil {
		return fmt.Errorf("http check: %s", err)
	}

	return nil
}

// Validate validates a HTTPBasicAuth.
func (auth *HTTPBasicAuth) Validate() error {
	if len(strings.TrimSpace(auth.Username)) == 0 {
		return fmt.Errorf("basicAuth: no username given")
	}
	if len(strings.TrimSpace(auth.Password)) == 0 {
		return fmt.Errorf("basicAuth: no password given")
	}

	return nil
}

// Validate validates a HTTPExpectation.
func (expect *HTTPExpectation) Validate() error {
	if !ValidHTTPStatusCode(expect.StatusCode) {
		return fmt.Errorf("http expect: illegal statusCode: %d", expect.StatusCode)
	}
	return nil
}

// Validate validates a SSHCheck.
func (check *SSHCheck) Validate() error {

	if !ValidHostOrIpAddr(check.Host) {
		return fmt.Errorf("ssh check: illegal host: '%s'", check.Host)
	}

	if !ValidPort(check.Port) {
		return fmt.Errorf("ssh check: illegal port: '%d'", check.Port)
	}

	if err := check.Auth.Validate(); err != nil {
		return fmt.Errorf("ssh check: %s", err)
	}

	if err := check.Expect.Validate(); err != nil {
		return fmt.Errorf("ssh check: %s", err)
	}

	// exactly one of Command and CommandFile must be specified
	if check.Command == "" && check.CommandFile == "" {
		return fmt.Errorf("ssh check: neither command nor commandFile given")
	}
	if check.Command != "" && check.CommandFile != "" {
		return fmt.Errorf("ssh check: only one of command and commandFile is allowed, not both")
	}

	// validate that commandFile exists
	if check.CommandFile != "" {
		if _, err := os.Stat(check.CommandFile); err != nil {
			return fmt.Errorf("ssh check: command file: %s", err)
		}
	}

	return nil
}

// Validate validates an SSHAuth instance.
func (auth *SSHAuth) Validate() error {
	// ssh login name must be valid
	if ok, _ := regexp.MatchString("[a-z_][a-z0-9_-]*$", auth.Username); !ok {
		return fmt.Errorf("auth: illegal username: '%s'", auth.Username)
	}

	// at least one auth method must be specified
	if auth.Key == nil && auth.Password == nil && !auth.Agent {
		return errors.New("auth: no auth method given (at least one of password, key, or agent auth must be specified)")
	}
	return nil
}

// Validate validates an SSHExpectation instance.
func (expect *SSHExpectation) Validate() error {
	if expect.ExitCode < 0 || expect.ExitCode > 255 {
		return errors.New("expect: exitCode must be in the range [0,255]")
	}
	return nil
}

// Validate validates an Alerter configuration.
func (alerter *Alerter) Validate() error {
	if !ValidHostOrIpAddr(alerter.AdvertisedIP) {
		return fmt.Errorf("alerter: advertisedIP: illegal IP/hostname: '%s'", alerter.AdvertisedIP)
	}
	if !ValidPort(alerter.AdvertisedPort) {
		return fmt.Errorf("alerter: advertisedPort: illegal port: %d", alerter.AdvertisedPort)
	}

	if alerter.Email != nil {
		if err := alerter.Email.Validate(); err != nil {
			return fmt.Errorf("alerter: %s", err)
		}
	}
	return nil
}

// Validate validates an Email configuration.
func (email *Email) Validate() error {
	if !ValidHostOrIpAddr(email.SMTPHost) {
		return fmt.Errorf("email: illegal smtpHost: '%s'", email.SMTPHost)
	}
	if !ValidPort(email.SMTPPort) {
		return fmt.Errorf("email: illegal smtpPort: '%d'", email.SMTPPort)
	}

	if email.Auth != nil {
		if err := email.Auth.Validate(); err != nil {
			return fmt.Errorf("email: %s", err)
		}
	}

	if _, err := mail.ParseAddress(email.From); err != nil {
		return fmt.Errorf("email: illegal From address: '%s': %s",
			email.From, err)
	}

	for _, receiver := range email.To {
		if _, err := mail.ParseAddress(receiver); err != nil {
			return fmt.Errorf("email: illegal To address: '%s': %s",
				email.From, err)
		}
	}
	return nil
}

// Validate validates an EmailAuth configuration.
func (auth *EmailAuth) Validate() error {
	if ok, _ := regexp.MatchString("[a-z_][a-z0-9_-]*$", auth.Username); !ok {
		return fmt.Errorf("auth: illegal username: '%s'", auth.Username)
	}

	if len(auth.Password) == 0 {
		return fmt.Errorf("auth: no password given")
	}

	return nil
}

// ValidHostOrIpAddr determines if a given hostname/IP address is valid.
func ValidHostOrIpAddr(hostOrIp string) bool {
	return ipv4AddrRegexp.MatchString(hostOrIp) || hostnameRegexp.MatchString(hostOrIp)
}

// ValidPort determines if a given number is a valid port number.
func ValidPort(port int) bool {
	return port > 0 && port < 65535
}

// ValidHTTPStatusCode determines if a given number is a valid HTTP status code.
func ValidHTTPStatusCode(statusCode int) bool {
	return statusCode >= 100 && statusCode < 600
}

// ValidPingerName determines if a given name is a valid name for a pinger.
func ValidPingerName(name string) bool {
	return validPingerName.MatchString(name)
}
