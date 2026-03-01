//
// Copyright 2021, Sander van Harmelen
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package gitlab

import (
	"net/http"
	"time"
)

type (
	ProjectMirrorServiceInterface interface {
		ListProjectMirror(pid any, opt *ListProjectMirrorOptions, options ...RequestOptionFunc) ([]*ProjectMirror, *Response, error)
		GetProjectMirror(pid any, mirror int64, options ...RequestOptionFunc) (*ProjectMirror, *Response, error)
		GetProjectMirrorPublicKey(pid any, mirror int64, options ...RequestOptionFunc) (*ProjectMirrorPublicKey, *Response, error)
		AddProjectMirror(pid any, opt *AddProjectMirrorOptions, options ...RequestOptionFunc) (*ProjectMirror, *Response, error)
		EditProjectMirror(pid any, mirror int64, opt *EditProjectMirrorOptions, options ...RequestOptionFunc) (*ProjectMirror, *Response, error)
		DeleteProjectMirror(pid any, mirror int64, options ...RequestOptionFunc) (*Response, error)
		ForcePushMirrorUpdate(pid any, mirror int64, options ...RequestOptionFunc) (*Response, error)
	}

	// ProjectMirrorService handles communication with the project mirror
	// related methods of the GitLab API.
	//
	// GitLAb API docs: https://docs.gitlab.com/api/remote_mirrors/
	ProjectMirrorService struct {
		client *Client
	}
)

var _ ProjectMirrorServiceInterface = (*ProjectMirrorService)(nil)

// ProjectMirror represents a project mirror configuration.
//
// GitLAb API docs: https://docs.gitlab.com/api/remote_mirrors/
type ProjectMirror struct {
	Enabled                bool       `json:"enabled"`
	ID                     int64      `json:"id"`
	LastError              string     `json:"last_error"`
	LastSuccessfulUpdateAt *time.Time `json:"last_successful_update_at"`
	LastUpdateAt           *time.Time `json:"last_update_at"`
	LastUpdateStartedAt    *time.Time `json:"last_update_started_at"`
	MirrorBranchRegex      string     `json:"mirror_branch_regex"`
	OnlyProtectedBranches  bool       `json:"only_protected_branches"`
	KeepDivergentRefs      bool       `json:"keep_divergent_refs"`
	UpdateStatus           string     `json:"update_status"`
	URL                    string     `json:"url"`
	AuthMethod             string     `json:"auth_method"`
}

type ProjectMirrorPublicKey struct {
	PublicKey string `json:"public_key"`
}

// ListProjectMirrorOptions represents the available ListProjectMirror() options.
type ListProjectMirrorOptions struct {
	ListOptions
}

// ListProjectMirror gets a list of mirrors configured on the project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/remote_mirrors/#list-a-projects-remote-mirrors
func (s *ProjectMirrorService) ListProjectMirror(pid any, opt *ListProjectMirrorOptions, options ...RequestOptionFunc) ([]*ProjectMirror, *Response, error) {
	return do[[]*ProjectMirror](s.client,
		withPath("projects/%s/remote_mirrors", ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// GetProjectMirror gets a single mirror configured on the project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/remote_mirrors/#get-a-single-projects-remote-mirror
func (s *ProjectMirrorService) GetProjectMirror(pid any, mirror int64, options ...RequestOptionFunc) (*ProjectMirror, *Response, error) {
	return do[*ProjectMirror](s.client,
		withPath("projects/%s/remote_mirrors/%d", ProjectID{pid}, mirror),
		withRequestOpts(options...),
	)
}

// GetProjectMirrorPublicKey gets the SSH public key for a single mirror configured on the project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/remote_mirrors/#get-a-single-projects-remote-mirror-public-key
func (s *ProjectMirrorService) GetProjectMirrorPublicKey(pid any, mirror int64, options ...RequestOptionFunc) (*ProjectMirrorPublicKey, *Response, error) {
	return do[*ProjectMirrorPublicKey](s.client,
		withPath("projects/%s/remote_mirrors/%d/public_key", ProjectID{pid}, mirror),
		withRequestOpts(options...),
	)
}

// AddProjectMirrorOptions contains the properties requires to create
// a new project mirror.
//
// GitLab API docs:
// https://docs.gitlab.com/api/remote_mirrors/#create-a-push-mirror
type AddProjectMirrorOptions struct {
	URL                   *string `url:"url,omitempty" json:"url,omitempty"`
	Enabled               *bool   `url:"enabled,omitempty" json:"enabled,omitempty"`
	KeepDivergentRefs     *bool   `url:"keep_divergent_refs,omitempty" json:"keep_divergent_refs,omitempty"`
	OnlyProtectedBranches *bool   `url:"only_protected_branches,omitempty" json:"only_protected_branches,omitempty"`
	MirrorBranchRegex     *string `url:"mirror_branch_regex,omitempty" json:"mirror_branch_regex,omitempty"`
	AuthMethod            *string `url:"auth_method,omitempty" json:"auth_method,omitempty"`
}

// AddProjectMirror creates a new mirror on the project.
//
// GitLab API docs:
// https://docs.gitlab.com/api/remote_mirrors/#create-a-push-mirror
func (s *ProjectMirrorService) AddProjectMirror(pid any, opt *AddProjectMirrorOptions, options ...RequestOptionFunc) (*ProjectMirror, *Response, error) {
	return do[*ProjectMirror](s.client,
		withMethod(http.MethodPost),
		withPath("projects/%s/remote_mirrors", ProjectID{pid}),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// EditProjectMirrorOptions contains the properties requires to edit
// an existing project mirror.
//
// GitLab API docs:
// https://docs.gitlab.com/api/remote_mirrors/#update-a-remote-mirrors-attributes
type EditProjectMirrorOptions struct {
	Enabled               *bool   `url:"enabled,omitempty" json:"enabled,omitempty"`
	KeepDivergentRefs     *bool   `url:"keep_divergent_refs,omitempty" json:"keep_divergent_refs,omitempty"`
	OnlyProtectedBranches *bool   `url:"only_protected_branches,omitempty" json:"only_protected_branches,omitempty"`
	MirrorBranchRegex     *string `url:"mirror_branch_regex,omitempty" json:"mirror_branch_regex,omitempty"`
	AuthMethod            *string `url:"auth_method,omitempty" json:"auth_method,omitempty"`
}

// EditProjectMirror updates a project team member to a specified access level..
//
// GitLab API docs:
// https://docs.gitlab.com/api/remote_mirrors/#update-a-remote-mirrors-attributes
func (s *ProjectMirrorService) EditProjectMirror(pid any, mirror int64, opt *EditProjectMirrorOptions, options ...RequestOptionFunc) (*ProjectMirror, *Response, error) {
	return do[*ProjectMirror](s.client,
		withMethod(http.MethodPut),
		withPath("projects/%s/remote_mirrors/%d", ProjectID{pid}, mirror),
		withAPIOpts(opt),
		withRequestOpts(options...),
	)
}

// DeleteProjectMirror deletes a project mirror.
//
// GitLab API docs:
// https://docs.gitlab.com/api/remote_mirrors/#delete-a-remote-mirror
func (s *ProjectMirrorService) DeleteProjectMirror(pid any, mirror int64, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodDelete),
		withPath("projects/%s/remote_mirrors/%d", ProjectID{pid}, mirror),
		withRequestOpts(options...),
	)
	return resp, err
}

// ForcePushMirrorUpdate triggers a manual update for a project mirror.
//
// GitLab API docs:
// https://docs.gitlab.com/api/remote_mirrors/#force-push-mirror-update
func (s *ProjectMirrorService) ForcePushMirrorUpdate(pid any, mirror int64, options ...RequestOptionFunc) (*Response, error) {
	_, resp, err := do[none](s.client,
		withMethod(http.MethodPost),
		withPath("projects/%s/remote_mirrors/%d/sync", ProjectID{pid}, mirror),
		withRequestOpts(options...),
	)
	return resp, err
}
