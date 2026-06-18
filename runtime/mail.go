package runtime

import (
	"errors"
	"fmt"
	"net/smtp"
	"net/url"
	"strings"
)

var (
	mailConnString string
)

func InitMail(connStr string) {
	mailConnString = connStr
	LogInfo("Mail client initialized with connection string: ", connStr)
}

func SendMail(to, subject, body string) error {
	if mailConnString == "" {
		return errors.New("mail client not initialized; declare mail \"connection_string\" first")
	}

	LogInfo("Sending mail to: ", to, " subject: ", subject)

	// Support smtp://
	if strings.HasPrefix(mailConnString, "smtp://") {
		u, err := url.Parse(mailConnString)
		if err != nil {
			return fmt.Errorf("failed to parse SMTP connection string: %w", err)
		}

		host := u.Host
		password, _ := u.User.Password()
		username := u.User.Username()

		hostAndPort := host
		if !strings.Contains(host, ":") {
			hostAndPort = host + ":587"
		}
		hostOnly := strings.Split(host, ":")[0]

		var auth smtp.Auth
		if username != "" || password != "" {
			auth = smtp.PlainAuth("", username, password, hostOnly)
		}

		msg := []byte("To: " + to + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"\r\n" +
			body + "\r\n")

		err = smtp.SendMail(hostAndPort, auth, username, []string{to}, msg)
		if err != nil {
			return fmt.Errorf("SMTP send failed: %w", err)
		}
		return nil
	}

	// For mock/test/SES connections
	if strings.HasPrefix(mailConnString, "ses://") || strings.HasPrefix(mailConnString, "mock://") {
		LogInfo("Mail sent successfully via SES/Mock (stubbed)")
		return nil
	}

	return fmt.Errorf("unsupported mail provider scheme in: %s", mailConnString)
}
