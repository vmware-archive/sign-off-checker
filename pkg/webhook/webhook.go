/*
Copyright 2017 by the contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package webhook implements a webhook handler that validates Signed-Off-By
// metadata for a pull request's commits.
package webhook

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"

	"github.com/google/go-github/github"
	"github.com/heptio/sign-off-checker/pkg/constants"
)

// Handler is an http.Handler that handles GitHub pull_request hooks
// by validating that all commits in the PR have been signed-off-by
// appropriately.
type Handler struct {
	Secret []byte
	GitHub *github.Client
	Log    *log.Logger
}

var testRE = regexp.MustCompile(`(?mi)^signed-off-by:`)

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, h.Secret)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("Could not validate signature: %v", err),
			http.StatusBadRequest)
		return
	}

	hooktype := github.WebHookType(r)
	event, err := github.ParseWebHook(hooktype, payload)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("Error parsing payload: %v", err),
			http.StatusBadRequest)
		return
	}
	switch event := event.(type) {
	case *github.PullRequestEvent:
		h.handlePullRequest(event)
	case *github.PingEvent:
	default:
		h.Log.Printf("Unhandled hook type: %v", hooktype)
	}
}

func (h *Handler) handlePullRequest(event *github.PullRequestEvent) {
	owner := event.Repo.Owner.Login
	repo := event.Repo.Name
	number := event.Number

	opt := &github.ListOptions{PerPage: 10}
	allCommits := []*github.RepositoryCommit{}
	for {
		commits, resp, err := h.GitHub.PullRequests.ListCommits(context.TODO(), *owner, *repo, *number, opt)
		if err != nil {
			h.Log.Printf("Error getting commits for PR: %v", err)
			return
		}
		allCommits = append(allCommits, commits...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	signMissing := false
	for _, commit := range allCommits {
		if !testRE.MatchString(*commit.Commit.Message) {
			signMissing = true
			break
		}
	}

	for _, commit := range allCommits {
		status := github.RepoStatus{}
		status.TargetURL = github.String(fmt.Sprintf("https://github.com/%s/%s/blob/master/CONTRIBUTING.md", *owner, *repo))
		status.Context = github.String(constants.SignOffCheckerContext)
		if signMissing {
			status.State = github.String("failure")
			status.Description = github.String("A commit in PR is missing Signed-off-by")
		} else {
			status.State = github.String("success")
			status.Description = github.String("Commit has Signed-off-by")
		}

		_, _, err := h.GitHub.Repositories.CreateStatus(context.TODO(), *owner, *repo, *commit.SHA, &status)
		if err != nil {
			h.Log.Printf("Error setting status: %v", err)
		}
	}
}
