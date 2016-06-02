package alerter

import (
	"github.com/petergardfjall/watcher/config"
	"encoding/json"
	"fmt"
	"net/smtp"
)

// An EmailAlerter sends alerts over the SMTP protocol to a group of receivers.
type EmailAlerter struct {
	Config *config.Email
}

// NewEmailAlerter creates a new EmailAlerter from a configuration.
func NewEmailAlerter(emailConfig *config.Email) (*EmailAlerter, error) {
	if emailConfig == nil {
		return nil, fmt.Errorf("cannot create email alerter: config is nil")
	}
	return &EmailAlerter{Config: emailConfig}, nil
}

// Alert sends an alert over the SMTP protocol to the server and recipients
// configured for the EmailAlerter.
func (emailAlerter *EmailAlerter) Alert(update PingerUpdate) error {
	conf := emailAlerter.Config
	smtpServer := fmt.Sprintf("%s:%d", conf.SMTPHost, conf.SMTPPort)

	var auth smtp.Auth
	if conf.Auth != nil {
		auth = smtp.PlainAuth("", conf.Auth.Username, conf.Auth.Password, conf.SMTPHost)
	}

	log.Debugf("sending email to %s ...", smtpServer)

	message, err := emailAlerter.message(&update)
	if err != nil {
		return fmt.Errorf("failed to send mail: %s", err)
	}

	err = smtp.SendMail(smtpServer, auth, conf.From, conf.To, message)
	if err != nil {
		return fmt.Errorf("failed to send mail: %s", err)
	}
	log.Debugf("email to %s sent.", smtpServer)

	return nil
}

func (emailAlerter *EmailAlerter) message(update *PingerUpdate) ([]byte, error) {
	conf := emailAlerter.Config

	status := "OK"
	if !update.Status.OK {
		status = "NOT OK"
	}

	subject := fmt.Sprintf("[watcherd] pinger [%s] is %s", update.Name, status)
	headers := fmt.Sprintf("From: %s\r\nSubject: %s\r\n", conf.From, subject)

	body, err := json.MarshalIndent(update, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("failed to produce alert message: %s", err)
	}

	return []byte(headers + "\r\n" + string(body) + "\r\n"), nil
}
