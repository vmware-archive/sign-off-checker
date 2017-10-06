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
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/go-github/github"
)

// isDCO checks if the provided CONTRIBUTING.md document is based on the
// Developer Certificate of Origin (DCO)
func isDCO(contributingDoc string) bool {
	return strings.Contains(string(contributingDoc), "Developer Certificate of Origin")
}

// getContributing returns the CONTRIBUTING.md document in the repository's root.
// if the repository does not have a CONTRIBUTING.md, returns an empty string
func getContributing(gh *github.Client, repo *github.Repository) (string, error) {
	// github.com/google/go-github doesn't wrap the Contents API yet, so we
	// have to do this manually (docs: https://developer.github.com/v3/repos/contents/)
	url := strings.Replace(repo.GetContentsURL(), "{+path}", "CONTRIBUTING.md", 1)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("Could not construct CONTRIBUTING.md request for %q: %v", repo.GetFullName(), err)
	}

	// The github client.Do expects a struct to unmarshal into
	contents := struct {
		ContentBase64 string `json:"content"`
	}{}
	resp, err := gh.Do(context.TODO(), req, &contents)
	if resp != nil && resp.StatusCode == 404 {
		// 404 is not an error, just means there's no CONTRIBUTING.md
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("Error getting CONTRIBUTING.md for %q: %v", repo.GetFullName(), err)
	}

	contributing, err := base64.StdEncoding.DecodeString(contents.ContentBase64)
	if err != nil {
		return "", fmt.Errorf("Error decoding CONTRIBUTING.md for %q: %v", repo.GetFullName(), err)
	}

	return string(contributing), nil
}
