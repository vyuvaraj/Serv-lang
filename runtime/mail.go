package runtime

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"net/url"
	"strings"
)

var (
	mailConnString   string
	notifyConnString string
)

func InitMail(connStr string) {
	mailConnString = connStr
	LogInfo("Mail client initialized with connection string: ", connStr)
}

func InitNotify(connStr string) {
	notifyConnString = connStr
	LogInfo("Notify client initialized with connection string: ", connStr)
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

func MailSend(toVal, templateVal, dataVal interface{}) interface{} {
	to := fmt.Sprint(toVal)
	templateStr := fmt.Sprint(templateVal)

	var data map[string]interface{}
	if sm, ok := dataVal.(*SafeMap); ok {
		data = sm.All()
	} else if m, ok := dataVal.(map[string]interface{}); ok {
		data = m
	}

	LogInfo(fmt.Sprintf("[Serv-lang] [mail.send] To: %s, Template: %s, Context: %+v", to, templateStr, data))
	return true
}

func Notify(channelVal, targetVal, msgVal interface{}) interface{} {
	channel := fmt.Sprint(channelVal)
	target := fmt.Sprint(targetVal)
	msg := fmt.Sprint(msgVal)

	LogInfo(fmt.Sprintf("[Serv-lang] [notify] Channel: %s, Target: %s, Message: %s", channel, target, msg))

	if strings.HasPrefix(notifyConnString, "servmail://") {
		baseURL := strings.Replace(notifyConnString, "servmail://", "http://", 1)
		sendURL := strings.TrimSuffix(baseURL, "/") + "/api/mail/send"

		payloadMap := map[string]interface{}{
			"channel":  channel,
			"target":   target,
			"template": msg,
		}
		payloadBytes, err := json.Marshal(payloadMap)
		if err != nil {
			LogWarn("[Serv-lang] [notify] Failed to marshal payload: ", err.Error())
			return false
		}

		resp, err := http.Post(sendURL, "application/json", bytes.NewReader(payloadBytes))
		if err != nil {
			LogWarn("[Serv-lang] [notify] HTTP POST failed: ", err.Error())
			return false
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			bodyBytes, _ := io.ReadAll(resp.Body)
			LogWarn("[Serv-lang] [notify] ServMail returned error: ", string(bodyBytes))
			return false
		}
		return true
	}

	return true
}
