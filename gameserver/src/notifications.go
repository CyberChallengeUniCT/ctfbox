package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"game/log"
	"net/http"
	"time"
)

var notifyHTTPClient = &http.Client{Timeout: 5 * time.Second}

// notifyFirstBlood fires the configured Discord/Telegram notifications for a
// first blood event. It never blocks the caller: each configured channel is
// sent on its own goroutine.
func notifyFirstBlood(teamID int, teamName string, service string, round uint) {
	if len(conf.FirstBloodDiscordWebhooks) == 0 && len(conf.FirstBloodTelegramNotifiers) == 0 {
		return
	}

	message := fmt.Sprintf("🔥 First Blood! Team %s (#%d) is the first to pop service \"%s\" at round %d", teamName, teamID, service, round)

	for _, webhookURL := range conf.FirstBloodDiscordWebhooks {
		go sendDiscordNotification(webhookURL, message)
	}
	for _, notifier := range conf.FirstBloodTelegramNotifiers {
		go sendTelegramNotification(notifier, message)
	}
}

func sendDiscordNotification(webhookURL string, message string) {
	payload, err := json.Marshal(map[string]string{"content": message})
	if err != nil {
		log.Errorf("Error marshaling discord first blood payload: %v", err)
		return
	}
	resp, err := notifyHTTPClient.Post(webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Errorf("Error sending discord first blood notification: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Errorf("Discord first blood notification failed with status %d", resp.StatusCode)
	}
}

func sendTelegramNotification(notifier TelegramNotifier, message string) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", notifier.BotToken)
	payload, err := json.Marshal(map[string]string{
		"chat_id": notifier.ChatID,
		"text":    message,
	})
	if err != nil {
		log.Errorf("Error marshaling telegram first blood payload: %v", err)
		return
	}
	resp, err := notifyHTTPClient.Post(apiURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Errorf("Error sending telegram first blood notification: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Errorf("Telegram first blood notification failed with status %d", resp.StatusCode)
	}
}
