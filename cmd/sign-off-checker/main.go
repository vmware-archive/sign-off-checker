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
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/oauth2"

	"github.com/heptiolabs/sign-off-checker/pkg/register"
	"github.com/heptiolabs/sign-off-checker/pkg/webhook"
)

// CLI entrypoint
func main() {
	rootCmd := &cobra.Command{
		Use:     "SHARED_SECRET='[...]' GITHUB_TOKEN='[...]' sign-off-checker",
		Short:   "A GitHub integration to ensure commits have \"Signed-off-by\".",
		Args:    cobra.NoArgs,
		PreRunE: func(_ *cobra.Command, _ []string) error { return validate() },
		RunE:    func(_ *cobra.Command, _ []string) error { return run() },
	}

	// $SHARED_SECRET
	viper.BindEnv("sharedSecret", "SHARED_SECRET")

	// $GITHUB_TOKEN
	viper.BindEnv("githubToken", "GITHUB_TOKEN")

	// --listen / $LISTEN
	rootCmd.Flags().String(
		"listen",
		":8080",
		"Set HTTP listen `address`",
	)
	viper.BindPFlag("listenAddress", rootCmd.Flags().Lookup("listen"))
	viper.BindEnv("listenAddress", "LISTEN")

	// --autoregister / $AUTOREGISTER_ORGANIZATIONS
	rootCmd.Flags().StringSlice(
		"autoregister",
		[]string{},
		"Autoregister all DCO repositories under this `organization` (repeat to watch more than one organization)",
	)
	viper.BindPFlag("autoregisterOrganizations", rootCmd.Flags().Lookup("autoregister"))
	viper.BindEnv("autoregisterOrganizations", "AUTOREGISTER_ORGANIZATIONS")

	// --autoregister-interval / $AUTOREGISTER_INTERVAL
	rootCmd.Flags().Duration(
		"autoregister-interval",
		10*time.Minute,
		"Rerun webhook and branch protection automatic registration every `interval`",
	)
	viper.BindPFlag("autoregisterInterval", rootCmd.Flags().Lookup("autoregister-interval"))
	viper.BindEnv("autoregisterInterval", "AUTOREGISTER_INTERVAL")

	// --public-webhook-url / $PUBLIC_WEBHOOK_URL
	rootCmd.Flags().String(
		"public-webhook-url",
		"",
		"Set the public HTTPS URL of this server (required for automatic registration)",
	)
	viper.BindPFlag("publicWebhookURL", rootCmd.Flags().Lookup("public-webhook-url"))
	viper.BindEnv("publicWebhookURL", "PUBLIC_WEBHOOK_URL")

	// --dry-run / $DRY_RUN
	rootCmd.Flags().Bool(
		"dry-run",
		false,
		"Do not change any webhook/branch configuration during automatic registration",
	)
	viper.BindPFlag("dryRun", rootCmd.Flags().Lookup("dry-run"))
	viper.BindEnv("dryRun", "DRY_RUN")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// validate that all provided parameters are valid
func validate() error {
	valid := true
	invalid := func(msg string, args ...interface{}) {
		valid = false
		fmt.Fprintf(os.Stderr, msg+"\n", args...)
	}

	if !viper.IsSet("sharedSecret") {
		invalid("$SHARED_SECRET is not set")
	}

	if !viper.IsSet("githubToken") {
		invalid("$GITHUB_TOKEN is not set")
	}

	_, _, err := net.SplitHostPort(viper.GetString("listenAddress"))
	if err != nil {
		invalid("listen address is invalid (%v)", err)
	}

	if viper.GetString("publicWebhookURL") != "" {
		url, err := url.ParseRequestURI(viper.GetString("publicWebhookURL"))
		if err != nil {
			invalid("public webhook URL is invalid (%v)", err)
		} else if url.Scheme != "https" {
			invalid("public webhook URL must be \"https://[...]\"")
		}
	} else if len(viper.GetStringSlice("autoregisterOrganizations")) > 0 {
		invalid("--public-webhook-url/$PUBLIC_WEBHOOK_URL must be set to use automatic registration")
	}

	if !valid {
		fmt.Fprintf(os.Stderr, "\n")
		return fmt.Errorf("invalid parameters")
	}
	return nil
}

// run the webhook server and autoregistration daemon
func run() error {
	gh := github.NewClient(
		oauth2.NewClient(oauth2.NoContext,
			oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: viper.GetString("githubToken")},
			),
		),
	)

	go autoregister(gh)

	return serveWebhook(gh)
}

func autoregister(gh *github.Client) {
	autoregisterLog := log.New(os.Stdout, "[register] ", log.Flags())

	if len(viper.GetStringSlice("autoregisterOrganizations")) == 0 {
		autoregisterLog.Printf("Automatic registration disabled (enable with --autoregister)")
		return
	}
	for _, org := range viper.GetStringSlice("autoregisterOrganizations") {
		autoregisterLog.Printf("Enabling automatic registration for DCO repositories under https://github.com/%s", org)
	}

	immediate := make(chan struct{}, 1)
	immediate <- struct{}{}
	ticker := time.NewTicker(viper.GetDuration("autoregisterInterval"))
	for {
		select {
		case <-immediate:
		case <-ticker.C:
		}
		start := time.Now()
		err := register.Register(
			autoregisterLog,
			gh,
			viper.GetBool("dryRun"),
			viper.GetStringSlice("autoregisterOrganizations"),
			viper.GetString("publicWebhookURL"),
			viper.GetString("sharedSecret"),
		)
		duration := time.Since(start)
		if err != nil {
			autoregisterLog.Printf("Error after %s: %v", duration, err)
		} else {
			autoregisterLog.Printf("Finished in %s", duration)
		}
	}
}

func loggingMiddleware(log *log.Logger, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func serveWebhook(gh *github.Client) error {
	// start the HTTP webhook listener
	webhookLog := log.New(os.Stdout, "[webhook] ", log.Flags())
	webhookLog.Printf("Serving /webhook on %s", viper.GetString("listenAddress"))
	mux := http.NewServeMux()
	mux.Handle("/webhook", &webhook.Handler{
		Secret: []byte(viper.GetString("sharedSecret")),
		GitHub: gh,
		Log:    webhookLog,
	})
	return http.ListenAndServe(
		viper.GetString("listenAddress"),
		loggingMiddleware(webhookLog, mux))
}
