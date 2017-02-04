/*
Copyright 2016 The Kubernetes Authors.

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

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"regexp"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

var client *github.Client
var secret []byte

var testRE *regexp.Regexp

func init() {
	testRE = regexp.MustCompile(`(?mi)^signed-off-by:`)
}

func loggingMiddleware(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func main() {
	secretString, _ := os.LookupEnv("SHARED_SECRET")
	if secretString == "" {
		log.Fatal("SHARED_SECRET is not set")
	}
	secret = []byte(secretString)

	token, _ := os.LookupEnv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN is not set")
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	client = github.NewClient(tc)

	log.Print("Starting serving /webhook on :8080")
	http.Handle("/webhook", loggingMiddleware(http.HandlerFunc(HandleHook)))
	err := http.ListenAndServe(":8080", nil)
	log.Fatal(err)
}

func HandleHook(w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, secret)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("Could not validate signature: %v", err),
			http.StatusBadRequest)
	}

	hooktype := github.WebHookType(r)
	event, err := github.ParseWebHook(hooktype, payload)
	if err != nil {
		http.Error(w,
			fmt.Sprintf("Error parsing payload: %v", err),
			http.StatusBadRequest)
	}
	switch event := event.(type) {
	case *github.PullRequestEvent:
		HandlePullRequest(event)
	default:
		log.Printf("Unhandled hook type: %v", hooktype)
	}
}

func HandlePullRequest(event *github.PullRequestEvent) {
	owner := event.Repo.Owner.Login
	repo := event.Repo.Name
	number := event.Number

	opt := &github.ListOptions{PerPage: 10}
	allCommits := []*github.RepositoryCommit{}
	for {
		commits, resp, err := client.PullRequests.ListCommits(*owner, *repo, *number, opt)
		if err != nil {
			log.Printf("Error getting commits for PR: %v", err)
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
		status.TargetURL = s(fmt.Sprintf("https://github.com/%s/%s/blob/master/CONTRIBUTING.md", *owner, *repo))
		status.Context = s("signed-off-by")
		if signMissing {
			status.State = s("failure")
			status.Description = s("A commit in PR is missing Signed-off-by")
		} else {
			status.State = s("success")
			status.Description = s("Commit has Signed-off-by")
		}

		_, _, err := client.Repositories.CreateStatus(*owner, *repo, *commit.SHA, &status)
		if err != nil {
			log.Printf("Error setting status: %v", err)
		}
	}
}

func s(str string) *string {
	return &str
}
