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

// define these as flag keys as constants so a typo is more likely to be caught
const (
	flagAutoregister         = "autoregister"
	flagAutoregisterInterval = "autoregister-interval"
	flagPublicWebhookURL     = "public-webhook-url"
	flagDryRun               = "dry-run"
	flagListen               = "listen"
	flagSharedSecret         = "shared-secret"
	flagGithubToken          = "github-token"
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

	// bind parameters from environment variables (secrets)
	viper.BindEnv(flagSharedSecret, "SHARED_SECRET")
	viper.BindEnv(flagGithubToken, "GITHUB_TOKEN")

	// bind parameters from command line flags
	rootCmd.Flags().String(
		flagListen,
		":8080",
		"Set HTTP listen `address`",
	)
	rootCmd.Flags().StringSlice(
		flagAutoregister,
		[]string{},
		"Autoregister all DCO repositories under this `organization` (repeat to watch more than one organization)",
	)
	rootCmd.Flags().Duration(
		flagAutoregisterInterval,
		10*time.Minute,
		"Rerun webhook and branch protection automatic registration every `interval`",
	)
	rootCmd.Flags().String(
		flagPublicWebhookURL,
		"",
		"Set the public HTTPS URL of this server (required for automatic registration)",
	)
	rootCmd.Flags().Bool(
		flagDryRun,
		false,
		"Do not change any webhook/branch configuration during automatic registration",
	)
	if err := viper.BindPFlags(rootCmd.Flags()); err != nil {
		log.Fatalf("error binding flags: %v", err)
	}

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

	if !viper.IsSet(flagSharedSecret) {
		invalid("$SHARED_SECRET is not set")
	}

	if !viper.IsSet(flagGithubToken) {
		invalid("$GITHUB_TOKEN is not set")
	}

	_, _, err := net.SplitHostPort(viper.GetString(flagListen))
	if err != nil {
		invalid("--%s is invalid (%v)", flagListen, err)
	}

	if viper.GetString(flagPublicWebhookURL) != "" {
		url, err := url.ParseRequestURI(viper.GetString(flagPublicWebhookURL))
		if err != nil {
			invalid("--%s is invalid (%v)", flagPublicWebhookURL, err)
		} else if url.Scheme != "https" {
			invalid("--%s must be \"https://[...]\"", flagPublicWebhookURL)
		}
	} else if len(viper.GetStringSlice(flagAutoregister)) > 0 {
		invalid("--%s must be set to use automatic registration", flagPublicWebhookURL)
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
				&oauth2.Token{AccessToken: viper.GetString(flagGithubToken)},
			),
		),
	)

	go autoregister(gh)

	return serveWebhook(gh)
}

func autoregister(gh *github.Client) {
	autoregisterLog := log.New(os.Stdout, "[register] ", log.Flags())

	if len(viper.GetStringSlice(flagAutoregister)) == 0 {
		autoregisterLog.Printf("Automatic registration disabled (enable with --%s)", flagAutoregister)
		return
	}
	for _, org := range viper.GetStringSlice(flagAutoregister) {
		autoregisterLog.Printf("Enabling automatic registration for DCO repositories under https://github.com/%s", org)
	}

	immediate := make(chan struct{}, 1)
	immediate <- struct{}{}
	ticker := time.NewTicker(viper.GetDuration(flagAutoregisterInterval))
	for {
		select {
		case <-immediate:
		case <-ticker.C:
		}
		start := time.Now()
		err := register.Register(
			autoregisterLog,
			gh,
			viper.GetBool(flagDryRun),
			viper.GetStringSlice(flagAutoregister),
			viper.GetString(flagPublicWebhookURL),
			viper.GetString(flagSharedSecret),
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
	webhookLog.Printf("Serving /webhook on %s", viper.GetString(flagListen))
	mux := http.NewServeMux()
	mux.Handle("/webhook", &webhook.Handler{
		Secret: []byte(viper.GetString(flagSharedSecret)),
		GitHub: gh,
		Log:    webhookLog,
	})
	return http.ListenAndServe(
		viper.GetString(flagListen),
		loggingMiddleware(webhookLog, mux))
}
