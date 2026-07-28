package api

type Me struct {
	Authenticated bool   `json:"authenticated"`
	Email         string `json:"email"`
	UserID        string `json:"userId"`
	IsAdmin       bool   `json:"isAdmin"`
	// True once the plan has fully lapsed, which is the point the server starts
	// refusing writes. An unpaid plan still inside its grace window is not
	// locked out and does not set this.
	SubscriptionLocked bool `json:"subscriptionLocked"`
	// Locked out is the same verdict for an account that never had a plan and
	// one that lost it, and only this tells them apart. Without it the CLI told
	// somebody who signed up a minute ago that their plan had lapsed.
	HasEverSubscribed bool `json:"hasEverSubscribed"`
}

type Technology struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Kernel   string `json:"kernel"`
	Category string `json:"category"`
}

type Project struct {
	ID           int          `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	Technologies []Technology `json:"technologies"`
}

type CreateProjectRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type Suggestion struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

type AddTechnologyRequest struct {
	Name     string `json:"name"`
	Version  string `json:"version,omitempty"`
	Kernel   string `json:"kernel,omitempty"`
	Category string `json:"category"`
}

type UpdateKernelRequest struct {
	Kernel string `json:"kernel"`
}

type ManifestFile struct {
	FileName string `json:"fileName"`
	Content  string `json:"content"`
}

type UploadManifestsRequest struct {
	Ecosystem string         `json:"ecosystem"`
	GroupName string         `json:"groupName"`
	Files     []ManifestFile `json:"files"`
}

type DependencyGroupInfo struct {
	ID              int    `json:"id"`
	Name            string `json:"name"`
	Ecosystem       string `json:"ecosystem"`
	DependencyCount int    `json:"dependencyCount"`
}

type DependencySummary struct {
	Groups          []DependencyGroupInfo `json:"groups"`
	VulnerableCount int                   `json:"vulnerableCount"`
	TotalCount      int                   `json:"totalCount"`
}

type UploadManifestsResponse struct {
	Summary          DependencySummary `json:"summary"`
	UnsupportedFiles []string          `json:"unsupportedFiles"`
	EmptyFiles       []string          `json:"emptyFiles"`
}

// One dependency group, named the way the person who uploaded it named it
// rather than by its ecosystem. Counts cover the whole group, not a page of it.
type ProjectGroupStat struct {
	GroupID       int    `json:"groupId"`
	Name          string `json:"name"`
	Ecosystem     string `json:"ecosystem"`
	PackageCount  int    `json:"packageCount"`
	OutdatedCount int    `json:"outdatedCount"`
	UnknownCount  int    `json:"unknownCount"`
}

type ProjectStats struct {
	TechnologyCount           int `json:"technologyCount"`
	EndOfLifeCount            int `json:"endOfLifeCount"`
	TechnologyCveCount        int `json:"technologyCveCount"`
	DependencyCount           int `json:"dependencyCount"`
	VulnerableDependencyCount int `json:"vulnerableDependencyCount"`
	DependencyCveCount        int `json:"dependencyCveCount"`
	// Absent from an older server, which decodes to nil and simply prints
	// nothing rather than claiming every group is clean.
	Groups []ProjectGroupStat `json:"groups"`
}

type DeviceAuthorization struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresInSeconds        int    `json:"expiresInSeconds"`
	IntervalSeconds         int    `json:"intervalSeconds"`
}

type DeviceToken struct {
	AccessToken string `json:"accessToken"`
	TokenType   string `json:"tokenType"`
}
