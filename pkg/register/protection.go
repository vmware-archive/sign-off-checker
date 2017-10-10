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
	"github.com/heptiolabs/sign-off-checker/pkg/constants"
)

func hasBranchProtection(gh *github.Client, org string, repo *github.Repository) (bool, error) {
	contexts, resp, err := gh.Repositories.ListRequiredStatusChecksContexts(
		context.TODO(),
		org,
		repo.GetName(),
		repo.GetDefaultBranch(),
	)
	if resp != nil && resp.StatusCode == 404 {
		// 404 means no branch protection has been configured at all
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("Error getting branch protection configuration for %q: %v", repo.GetFullName(), err)
	}

	// look for any required status check with our context
	for _, context := range contexts {
		if context == constants.SignOffCheckerContext {
			return true, nil
		}
	}
	return false, nil
}

// addBranchProtection adds the expected branch protection configuration to
// a repository's default branch (usually "master"). This is less straightforward
// because of the way it's intertwined with other branch protection settings.
// The GH API forces us to get+modify+set which means this could race with other
// concurrent modifications.
func addBranchProtection(gh *github.Client, org string, repo *github.Repository) error {
	existing, resp, err := gh.Repositories.GetBranchProtection(context.TODO(), org, repo.GetName(), repo.GetDefaultBranch())
	if err != nil && resp != nil && resp.StatusCode != 404 {
		// if there was an error other than a 404, bail out
		return fmt.Errorf("Error getting branch protection configuration for %q: %v", repo.GetFullName(), err)
	}

	// if there is no branch protection at all in place yet, set some defaults
	if resp.StatusCode == 404 {
		_, _, err = gh.Repositories.UpdateBranchProtection(
			context.TODO(),
			org,
			repo.GetName(),
			"master",
			&github.ProtectionRequest{
				EnforceAdmins: true,
				RequiredStatusChecks: &github.RequiredStatusChecks{
					Strict:   false,
					Contexts: []string{constants.SignOffCheckerContext},
				},
			})
		if err != nil {
			return fmt.Errorf("Error setting branch protection configuration for %q: %v", repo.GetFullName(), err)
		}
		return nil
	}

	// if there was some existing branch protection configured, but no required
	// status checks, fill in a default
	if existing.RequiredStatusChecks == nil {
		existing.RequiredStatusChecks = &github.RequiredStatusChecks{
			Contexts: []string{},
			Strict:   false,
		}
	}

	// append our context to the list of required contexts
	existing.RequiredStatusChecks.Contexts = append(
		existing.RequiredStatusChecks.Contexts,
		constants.SignOffCheckerContext)

	// update the branch protection
	_, _, err = gh.Repositories.UpdateBranchProtection(
		context.TODO(),
		org,
		repo.GetName(),
		"master",
		&github.ProtectionRequest{
			EnforceAdmins:        existing.EnforceAdmins.Enabled,
			RequiredStatusChecks: existing.RequiredStatusChecks,
		})
	if err != nil {
		return fmt.Errorf("Error updating branch protection configuration for %q: %v", repo.GetFullName(), err)
	}
	return nil
}
