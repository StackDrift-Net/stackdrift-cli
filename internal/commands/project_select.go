package commands

import (
	"fmt"
	"strings"

	"github.com/StackDrift-Net/stackdrift-cli/internal/api"
	"github.com/StackDrift-Net/stackdrift-cli/internal/ui"
)

func chooseProject(client *api.Client) (*api.Project, error) {
	projects, err := client.ListProjects()
	if err != nil {
		return nil, err
	}

	if len(projects) == 0 {
		ui.Println("You have no systems yet. Let's create one.")
		return createProject(client)
	}

	ui.Println()
	ui.Println("Add this directory to an existing system, or create a new one?")
	for i, p := range projects {
		ui.Printf("  %2d. %s\n", i+1, p.Name)
	}
	ui.Printf("  %2d. Create a new system\n", len(projects)+1)
	ui.Println()

	choice, ok := ui.AskInt(fmt.Sprintf("Choose 1-%d: ", len(projects)+1), 1, len(projects)+1)
	if !ok {
		return nil, fmt.Errorf("no valid choice made")
	}

	if choice == len(projects)+1 {
		return createProject(client)
	}
	return &projects[choice-1], nil
}

func createProject(client *api.Client) (*api.Project, error) {
	name := ui.Ask("New system name: ")
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("a system name is required")
	}
	description := ui.Ask("Description (optional): ")

	project, err := client.CreateProject(name, description)
	if err != nil {
		return nil, err
	}
	ui.Println("Created system: " + project.Name)
	return project, nil
}
