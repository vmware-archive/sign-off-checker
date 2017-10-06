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

// Package register implements automatic registration of webhooks and branch
// protections on all DCO repositories in a set of organizations.
package register

import (
	"context"
	"fmt"
	"log"

	"github.com/google/go-github/github"
)

// Register walks the provided organization, finds repositories that use the
// Developer Certificate of Origin (in CONTRIBUTING.md), and registers the
// sign-off-checker webhook and required commit statuses in each repository.
func Register(log *log.Logger, gh *github.Client, dryRun bool, organizations []string, webhookURL string, webhookSecret string) error {
	dryRunMsg := ""
	if dryRun {
		dryRunMsg = " (DRY RUN)"
	}

	for _, org := range organizations {
		log.Printf("checking all repos in the %q organization", org)
		repos, err := listOrgRepos(gh, org)
		if err != nil {
			return err
		}

		for _, repo := range repos {
			contributing, err := getContributing(gh, repo)
			if err != nil {
				return err
			}
			if !isDCO(contributing) {
				continue
			}

			hasHook, err := hasSignOffHook(gh, org, repo, webhookURL)
			if err != nil {
				return err
			}
			if !hasHook {
				log.Printf("Installing webhook for %s%s", repo.GetHTMLURL(), dryRunMsg)
				if !dryRun {
					err = addSignOffHook(gh, org, repo, webhookURL, webhookSecret)
					if err != nil {
						return err
					}
				}
			}

			hasProtection, err := hasBranchProtection(gh, org, repo)
			if err != nil {
				return err
			}
			if !hasProtection {
				log.Printf("Configuring branch protection for %s%s", repo.GetHTMLURL(), dryRunMsg)
				if !dryRun {
					err = addBranchProtection(gh, org, repo)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// listOrgRepos collects all pages of the RepositoryListByOrgOptions results.
func listOrgRepos(gh *github.Client, org string) ([]*github.Repository, error) {
	opt := &github.RepositoryListByOrgOptions{
		Type:        "all",
		ListOptions: github.ListOptions{PerPage: 10},
	}
	result := []*github.Repository{}
	for {
		repos, resp, err := gh.Repositories.ListByOrg(context.TODO(), org, opt)
		if err != nil {
			return nil, fmt.Errorf("Error getting repositories for organization %q: %v", org, err)
		}
		result = append(result, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return result, nil
}
