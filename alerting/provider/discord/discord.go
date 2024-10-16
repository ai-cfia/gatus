package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/TwiN/gatus/v5/alerting/alert"
	"github.com/TwiN/gatus/v5/client"
	"github.com/TwiN/gatus/v5/config/endpoint"
)

// AlertProvider is the configuration necessary for sending an alert using Discord
type AlertProvider struct {
	WebhookURL string `yaml:"webhook-url"`

	// DefaultAlert is the default alert configuration to use for endpoints with an alert of the appropriate type
	DefaultAlert *alert.Alert `yaml:"default-alert,omitempty"`

	// Overrides is a list of Override that may be prioritized over the default configuration
	Overrides []Override `yaml:"overrides,omitempty"`

	// Title is the title of the message that will be sent
	Title string `yaml:"title,omitempty"`
}

// Override is a case under which the default integration is overridden
type Override struct {
	Group      string `yaml:"group"`
	WebhookURL string `yaml:"webhook-url"`
}

const (
	DISCORD_WEBHOOK_URL_PREFIX = "$"
)

// IsValid returns whether the provider's configuration is valid
func (provider *AlertProvider) IsValid() bool {
	registeredGroups := make(map[string]bool)
	if provider.Overrides != nil {
		for _, override := range provider.Overrides {
			if isAlreadyRegistered := registeredGroups[override.Group]; isAlreadyRegistered || override.Group == "" || len(override.WebhookURL) == 0 {
				return false
			}
			registeredGroups[override.Group] = true
		}
	}
	return len(provider.WebhookURL) > 0
}

// Send an alert using the provider
func (provider *AlertProvider) Send(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) error {
	buffer := bytes.NewBuffer(provider.buildRequestBody(ep, alert, result, resolved))
	request, err := http.NewRequest(http.MethodPost, provider.getWebhookURLForGroup(ep.Group), buffer)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.GetHTTPClient(nil).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode > 399 {
		body, _ := io.ReadAll(response.Body)
		return fmt.Errorf("call to provider alert returned status code %d: %s", response.StatusCode, string(body))
	}
	return err
}

type Body struct {
	Content string  `json:"content"`
	Embeds  []Embed `json:"embeds"`
}

type Embed struct {
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Color       int     `json:"color"`
	Fields      []Field `json:"fields,omitempty"`
}

type Field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// buildRequestBody builds the request body for the provider
func (provider *AlertProvider) buildRequestBody(ep *endpoint.Endpoint, alert *alert.Alert, result *endpoint.Result, resolved bool) []byte {
	var message string
	var colorCode int
	if resolved {
		message = fmt.Sprintf("An alert for **%s** has been resolved after passing successfully %d time(s) in a row", ep.DisplayName(), alert.SuccessThreshold)
		colorCode = 3066993
	} else {
		message = fmt.Sprintf("An alert for **%s** has been triggered due to having failed %d time(s) in a row", ep.DisplayName(), alert.FailureThreshold)
		colorCode = 15158332
	}
	var formattedConditionResults string
	for _, conditionResult := range result.ConditionResults {
		var prefix string
		if conditionResult.Success {
			prefix = ":white_check_mark:"
		} else {
			prefix = ":x:"
		}
		formattedConditionResults += fmt.Sprintf("%s - `%s`\n", prefix, conditionResult.Condition)
	}
	var description string
	if alertDescription := alert.GetDescription(); len(alertDescription) > 0 {
		description = ":\n> " + alertDescription
	}
	title := ":helmet_with_white_cross: Gatus"
	if provider.Title != "" {
		title = provider.Title
	}
	body := Body{
		Content: "",
		Embeds: []Embed{
			{
				Title:       title,
				Description: message + description,
				Color:       colorCode,
			},
		},
	}
	if len(formattedConditionResults) > 0 {
		body.Embeds[0].Fields = append(body.Embeds[0].Fields, Field{
			Name:   "Condition results",
			Value:  formattedConditionResults,
			Inline: false,
		})
	}
	bodyAsJSON, _ := json.Marshal(body)
	return bodyAsJSON
}

// getWebhookURLForGroup returns the appropriate Webhook URL integration to for a given group
func (provider *AlertProvider) getWebhookURLForGroup(group string) string {
	if provider.Overrides != nil {
		for _, override := range provider.Overrides {
			if group == override.Group {
				return override.WebhookURL
			}
		}
	}

	// Check if the discord Webhook url is a secret
	if strings.HasPrefix(provider.WebhookURL, DISCORD_WEBHOOK_URL_PREFIX) {
		substr := strings.ReplaceAll(provider.WebhookURL, DISCORD_WEBHOOK_URL_PREFIX, "")
		v, found := os.LookupEnv(substr)
		if !found {
			fmt.Println("error fetching discord webhook url env var")
		}

		fmt.Printf("discord webhook url value: %s\n", v)
		provider.WebhookURL = v
	}

	return provider.WebhookURL
}

// GetDefaultAlert returns the provider's default alert configuration
func (provider *AlertProvider) GetDefaultAlert() *alert.Alert {
	return provider.DefaultAlert
}
