package api

import (
	"fmt"
	"net/http"
	"net/url"
)

func (c *Client) Me() (*Me, error) {
	var me Me
	if err := c.do(http.MethodGet, "/api/auth/me", nil, &me); err != nil {
		return nil, err
	}
	return &me, nil
}

func (c *Client) ListProjects() ([]Project, error) {
	var projects []Project
	if err := c.do(http.MethodGet, "/api/projects", nil, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (c *Client) GetProject(id int) (*Project, error) {
	var project Project
	if err := c.do(http.MethodGet, fmt.Sprintf("/api/projects/%d", id), nil, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) CreateProject(name, description string) (*Project, error) {
	var project Project
	body := CreateProjectRequest{Name: name, Description: description}
	if err := c.do(http.MethodPost, "/api/projects", body, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// ReportScan tells the server this machine has just finished checking the
// system's versions, and passes on what the machine's own updater had waiting.
// The time is not sent and never will be: the server stamps it, so a machine
// with a wrong clock cannot date its own scan.
//
// A server too old to know the body ignores it, and one too old to know the
// endpoint answers 404, which the caller already swallows.
func (c *Client) ReportScan(projectID int, report ScanReportRequest) error {
	return c.do(http.MethodPost, fmt.Sprintf("/api/projects/%d/scan-report", projectID), report, nil)
}

func (c *Client) GetProjectStats(id int) (*ProjectStats, error) {
	var stats ProjectStats
	if err := c.do(http.MethodGet, fmt.Sprintf("/api/projects/%d/stats", id), nil, &stats); err != nil {
		return nil, err
	}
	return &stats, nil
}

func (c *Client) SuggestTechnologies(query string) ([]Suggestion, error) {
	var suggestions []Suggestion
	path := "/api/technologies/suggest?q=" + url.QueryEscape(query)
	if err := c.do(http.MethodGet, path, nil, &suggestions); err != nil {
		return nil, err
	}
	return suggestions, nil
}

// CliHiddenTechnologies names the catalog entries the CLI must not offer when
// it finds them on a machine. The website is not filtered by it, so this is not
// a list of things that cannot be tracked, only of things nobody should be
// nudged into tracking by a scan.
func (c *Client) CliHiddenTechnologies() ([]string, error) {
	var names []string
	if err := c.do(http.MethodGet, "/api/technologies/cli-hidden", nil, &names); err != nil {
		return nil, err
	}
	return names, nil
}

func (c *Client) GetVersions(name string) ([]string, error) {
	var versions []string
	path := "/api/technologies/versions?name=" + url.QueryEscape(name)
	if err := c.do(http.MethodGet, path, nil, &versions); err != nil {
		return nil, err
	}
	return versions, nil
}

func (c *Client) AddTechnology(projectID int, req AddTechnologyRequest) (*Project, error) {
	var project Project
	path := fmt.Sprintf("/api/projects/%d/technologies", projectID)
	if err := c.do(http.MethodPost, path, req, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (c *Client) SetKernel(technologyID int, kernel string) error {
	path := fmt.Sprintf("/api/technologies/%d/kernel", technologyID)
	return c.do(http.MethodPut, path, UpdateKernelRequest{Kernel: kernel}, nil)
}

func (c *Client) DeleteTechnology(id int) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/api/technologies/%d", id), nil, nil)
}

func (c *Client) UploadManifests(projectID int, req UploadManifestsRequest) (*UploadManifestsResponse, error) {
	var resp UploadManifestsResponse
	path := fmt.Sprintf("/api/projects/%d/dependencies", projectID)
	if err := c.do(http.MethodPost, path, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) GetDependencies(projectID int) (*DependencySummary, error) {
	var summary DependencySummary
	path := fmt.Sprintf("/api/projects/%d/dependencies", projectID)
	if err := c.do(http.MethodGet, path, nil, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (c *Client) DeleteDependencyGroup(groupID int) error {
	return c.do(http.MethodDelete, fmt.Sprintf("/api/dependencies/groups/%d", groupID), nil, nil)
}

func (c *Client) StartDeviceAuthorization(clientName string) (*DeviceAuthorization, error) {
	var auth DeviceAuthorization
	body := map[string]string{"clientName": clientName}
	if err := c.do(http.MethodPost, "/api/device/authorize", body, &auth); err != nil {
		return nil, err
	}
	return &auth, nil
}

func (c *Client) PollDeviceToken(deviceCode string) (*DeviceToken, int, error) {
	body := map[string]string{"deviceCode": deviceCode}
	encoded, err := marshal(body)
	if err != nil {
		return nil, 0, err
	}

	status, data, err := c.raw(http.MethodPost, "/api/device/token", encoded)
	if err != nil {
		return nil, 0, err
	}

	if status == http.StatusOK {
		var token DeviceToken
		if err := unmarshal(data, &token); err != nil {
			return nil, status, err
		}
		return &token, status, nil
	}

	return nil, status, nil
}
