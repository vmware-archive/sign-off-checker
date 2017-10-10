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

package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"

	"github.com/heptiolabs/sign-off-checker/pkg/register"
	"github.com/heptiolabs/sign-off-checker/pkg/webhook"
)

// How often do we loop through and autoregister webhooks and branch protection configuration?
var autoregisterInterval = 10 * time.Minute

func loggingMiddleware(log *log.Logger, handler http.Handler) http.Handler {
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

	token, _ := os.LookupEnv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN is not set")
	}

	autoRegisterOrgsString, _ := os.LookupEnv("AUTOREGISTER_ORGANIZATIONS")
	autoRegisterOrgs := strings.Split(autoRegisterOrgsString, ",")
	if autoRegisterOrgsString == "" {
		autoRegisterOrgs = []string{}
	}

	publicWebhookURL, _ := os.LookupEnv("PUBLIC_WEBHOOK_URL")
	if publicWebhookURL == "" && len(autoRegisterOrgs) > 0 {
		log.Fatal("PUBLIC_WEBHOOK_URL is required for AUTOREGISTER_ORGANIZATIONS")
	}

	dryRunString, _ := os.LookupEnv("DRY_RUN")
	dryRun := dryRunString != ""

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	client := github.NewClient(tc)

	// start a background thread to autoregister immediately and then once every autoregisterInterval
	go func() {
		autoregisterLog := log.New(os.Stdout, "[register] ", log.Flags())
		immediate := make(chan struct{}, 1)
		immediate <- struct{}{}
		ticker := time.NewTicker(autoregisterInterval)
		for {
			select {
			case <-immediate:
			case <-ticker.C:
			}
			start := time.Now()
			err := register.Register(autoregisterLog, client, dryRun, autoRegisterOrgs, publicWebhookURL, secretString)
			duration := time.Since(start)
			if err != nil {
				autoregisterLog.Printf("Error after %s: %v", duration, err)
			} else {
				autoregisterLog.Printf("Finished in %s", duration)
			}
		}
	}()

	// start the HTTP webhook listener
	webhookLog := log.New(os.Stdout, "[webhook] ", log.Flags())
	webhookLog.Print("Starting serving /webhook on :8080")
	mux := http.NewServeMux()
	mux.Handle("/webhook", &webhook.Handler{
		Secret: []byte(secretString),
		GitHub: client,
		Log:    webhookLog,
	})
	log.Fatal(http.ListenAndServe(":8080", loggingMiddleware(webhookLog, mux)))
}
