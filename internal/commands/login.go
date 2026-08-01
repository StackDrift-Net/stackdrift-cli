package commands

import (
	"fmt"

	"github.com/StackDrift-Net/stackdrift-cli/internal/auth"
	"github.com/StackDrift-Net/stackdrift-cli/internal/config"
)

func Login([]string) error {
	return auth.Login(config.BaseURL())
}

func Logout([]string) error {
	baseURL := config.BaseURL()
	if err := config.DeleteCredential(baseURL); err != nil {
		return err
	}
	fmt.Println("Signed out.")
	return nil
}

func Whoami([]string) error {
	_, _, me, err := authenticatedSession()
	if err != nil {
		return err
	}
	fmt.Println("Signed in as " + me.Email)
	return nil
}
