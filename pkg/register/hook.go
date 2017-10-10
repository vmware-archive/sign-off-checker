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

package register

import (
	"context"
	"fmt"

	"github.com/google/go-github/github"
)

func hasSignOffHook(gh *github.Client, org string, repo *github.Repository, url string) (bool, error) {
	opt := &github.ListOptions{PerPage: 10}
	for {
		hooks, resp, err := gh.Repositories.ListHooks(context.TODO(), org, repo.GetName(), opt)
		if resp != nil && resp.StatusCode == 404 {
			// 404 just means there are no hooks for this repo
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("Error listing hooks for %q: %v", repo.GetFullName(), err)
		}
		for _, hook := range hooks {
			// if the hook with our expected URL already exists, we're done
			if hook.Config["url"] == url {
				return true, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return false, nil
}

func addSignOffHook(gh *github.Client, org string, repo *github.Repository, url string, secret string) error {
	hook := &github.Hook{
		Name:   github.String("web"),
		Events: []string{"pull_request"},
		Active: github.Bool(true),
		Config: map[string]interface{}{
			"url":          url,
			"secret":       secret,
			"content_type": "json",
		},
	}
	_, _, err := gh.Repositories.CreateHook(context.TODO(), org, repo.GetName(), hook)
	if err != nil {
		return fmt.Errorf("Error registering webhook: %v", err)
	}
	return err
}
