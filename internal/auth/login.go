package auth

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

func Login(baseURL string) error {
	client := api.New(baseURL, "")

	hostname, _ := os.Hostname()
	clientName := "StackDrift CLI"
	if hostname != "" {
		clientName = "StackDrift CLI on " + hostname
	}

	auth, err := client.StartDeviceAuthorization(clientName)
	if err != nil {
		return fmt.Errorf("could not start login: %w", err)
	}

	fmt.Println()
	fmt.Println("To sign in, open this page in your browser:")
	fmt.Println("  " + auth.VerificationURIComplete)
	fmt.Println()
	fmt.Println("Then confirm this code is shown:")
	fmt.Println("  " + auth.UserCode)
	fmt.Println()
	fmt.Println("Waiting for you to approve...")

	openBrowser(auth.VerificationURIComplete)

	token, err := poll(client, auth)
	if err != nil {
		return err
	}

	return finishLogin(baseURL, token)
}

func finishLogin(baseURL, token string) error {
	me, err := api.New(baseURL, token).Me()
	if err != nil {
		return fmt.Errorf("signed in but could not read account: %w", err)
	}

	if err := config.SaveCredential(config.Credential{BaseURL: baseURL, Token: token, Email: me.Email}); err != nil {
		return fmt.Errorf("could not save credentials: %w", err)
	}

	fmt.Println()
	fmt.Println("Signed in as " + me.Email)
	return nil
}

// Swapped out in tests so the polling loop does not wait in real time.
var sleep = time.Sleep

func poll(client *api.Client, auth *api.DeviceAuthorization) (string, error) {
	interval := time.Duration(auth.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(auth.ExpiresInSeconds) * time.Second)

	for {
		if time.Now().After(deadline) {
			return "", errors.New("login timed out, please run login again")
		}

		sleep(interval)

		token, status, err := client.PollDeviceToken(auth.DeviceCode)
		if err != nil {
			continue
		}

		switch status {
		case http.StatusOK:
			return token.AccessToken, nil
		case http.StatusAccepted:
			continue
		case http.StatusForbidden:
			return "", errors.New("login was denied")
		case http.StatusGone:
			return "", errors.New("login expired, please run login again")
		default:
			return "", fmt.Errorf("unexpected login response (status %d)", status)
		}
	}
}
