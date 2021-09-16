package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	texttemplate "text/template"

	httputil "github.com/argoproj/notifications-engine/pkg/util/http"
	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
)

type GoogleChatNotification struct {
	Cards string `json:"cards"`
}

type googleChatMessage struct {
	Text  string        `json:"text"`
	Cards []cardMessage `json:"cards"`
}

type cardMessage struct {
	Header   cardHeader    `json:"header,omitempty"`
	Sections []cardSection `json:"sections"`
}

type cardHeader struct {
	Title      string `json:"title,omitempty"`
	Subtitle   string `json:"subtitle,omitempty"`
	ImageUrl   string `json:"imageUrl,omitempty"`
	ImageStyle string `json:"imageStyle,omitempty"`
}

type cardSection struct {
	Header  string       `json:"header"`
	Widgets []cardWidget `json:"widgets"`
}

type cardWidget struct {
	TextParagraph map[string]interface{}   `json:"textParagraph,omitempty"`
	Keyvalue      map[string]interface{}   `json:"keyValue,omitempty"`
	Image         map[string]interface{}   `json:"image,omitempty"`
	Buttons       []map[string]interface{} `json:"buttons,omitempty"`
}

func (n *GoogleChatNotification) GetTemplater(name string, f texttemplate.FuncMap) (Templater, error) {
	cards, err := texttemplate.New(name).Funcs(f).Parse(n.Cards)
	if err != nil {
		return nil, fmt.Errorf("error in '%s' googlechat.cards : %w", name, err)
	}
	return func(notification *Notification, vars map[string]interface{}) error {
		if notification.GoogleChat == nil {
			notification.GoogleChat = &GoogleChatNotification{}
		}
		var cardsBuff bytes.Buffer
		if err := cards.Execute(&cardsBuff, vars); err != nil {
			return err
		}
		if val := cardsBuff.String(); val != "" {
			notification.GoogleChat.Cards = val
		}
		return nil
	}, nil
}

type GoogleChatOptions struct {
	WebhookUrls map[string]string `json:"webhooks"`
}

type googleChatService struct {
	opts GoogleChatOptions
}

func NewGoogleChatService(opts GoogleChatOptions) NotificationService {
	return &googleChatService{opts: opts}
}

type webhookReturn struct {
	Error *webhookError `json:"error"`
}

type webhookError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

func (s googleChatService) Send(notification Notification, dest Destination) error {
	webhookUrl, ok := s.opts.WebhookUrls[dest.Recipient]
	if !ok {
		return fmt.Errorf("no Google chat webhook configured for recipient %s", dest.Recipient)
	}
	transport := httputil.NewTransport(webhookUrl, false)
	client := &http.Client{
		Transport: httputil.NewLoggingRoundTripper(transport, log.WithField("service", "googlechat")),
	}
	message, err := googleChatNotificationToMessage(notification)
	if err != nil {
		return err
	}
	jsonMessage, err := json.Marshal(message)
	if err != nil {
		return err
	}
	response, err := client.Post(webhookUrl, "application/json", bytes.NewReader(jsonMessage))
	if err != nil {
		return err
	}

	defer func() {
		_ = response.Body.Close()
	}()

	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return err
	}

	body := webhookReturn{}
	err = json.Unmarshal(bodyBytes, &body)
	if err != nil {
		return err
	}
	if body.Error != nil {
		return fmt.Errorf("error with message: code=%d status=%s message=%s", body.Error.Code, body.Error.Status, body.Error.Message)
	}
	return nil
}

func googleChatNotificationToMessage(n Notification) (*googleChatMessage, error) {
	message := &googleChatMessage{}
	if n.GoogleChat != nil && n.GoogleChat.Cards != "" {
		err := yaml.Unmarshal([]byte(n.GoogleChat.Cards), &message.Cards)
		if err != nil {
			return nil, err
		}
	} else {
		message.Text = n.Message
	}
	return message, nil
}
