# sign-off-checker

This is a simple Go server that listens for web hooks from GitHub for PRs.
It then looks at each commit in that PR and sets a status.
If all of the commits have a "Signed-off-by" line on them then it marks all of those commits as "success".
If any one of them is missing the "Signed-off-by" line then all are marked as "failed".

The status check points to a "CONTRIBUTING.md" file in the repo in question.

## Building

You can run `go get github.com/heptiolabs/sign-off-checker/cmd/sign-off-checker` to install the binary locally.
To build a Docker container run `make push REGISTRY=<my-gcr-registry>` from this repo.

## Running

#### Usage
```
A GitHub integration to ensure commits have "Signed-off-by".

Usage:
  SHARED_SECRET='[...]' GITHUB_TOKEN='[...]' sign-off-checker [flags]

Flags:
      --autoregister organization        Autoregister all DCO repositories under this organization (repeat to watch more than one organization)
      --autoregister-interval interval   Rerun webhook and branch protection automatic registration every interval (default 10m0s)
      --dry-run                          Do not change any webhook/branch configuration during automatic registration
  -h, --help                             help for SHARED_SECRET='[...]'
      --listen address                   Set HTTP listen address (default ":8080")
      --public-webhook-url string        Set the public HTTPS URL of this server (required for automatic registration)
```

There are two required environment variables:

* `$SHARED_SECRET`:
  Set this to a random value that you supply as the "secret" when configuring the webhook.

* `$GITHUB_TOKEN`:
  Set this to a personal access token for a github user that has access to the repo in question.
  The webhook doesn't include details of the commits so we have to fetch them.
  Unfortunately this requires full read/write `repo` access scope (even if we're not using automatic registration and are just reading).
  Create one of these at https://github.com/settings/tokens.


### Manual Registration
In this mode, sign-off-checker updates commit statuses but you must manually configure the webhook and branch protection settings you want.

To use manual registration:

 - Run the server someplace (without the `--autoregister` flag).
   It'll serve `/webhook` on the specified `--listen` address, for example `http://127.0.0.1:8080/webhook`.

 - Make sure this URL is mapped to a public HTTPS URL via some external load balancer (e.g., Kubernetes Ingress).

 - Now head on over to the settings tab of your repo and add a webhook.
   - The Payload URL should be set to the internet-accessible version of the webhook URL.
   - The content type should be `application/json`.
   - The secret should be the secret you set in `$SHARED_SECRET`.
   - Select "individual events" and check "Pull request".

 - If things are working you can check the status of the webhook from GitHub's point of view on that page.

### Automatic Registration
In this mode, the sign-off-checker automatically configures webhook and branch protection settings.
You configure it with a list of GitHub organizations that you want to scan.
The sign-off-checker server will periodically (every `--autoregister-interval`) scan all repositories in those organizations.
If it finds a repository that uses the Developer Certificate of Origin (DCO) in `CONTRIBUTING.md`, it will configure a pull request webhook.
It will also set itself as a required commit status to prevent PRs from merging without sign-off.

To use automatic registration:

 - Run the server someplace where it can expose a public HTTPS URL (as above).
   - Pass `--autoregister Org1 --autoregister Org2 [...]` to set the list of organizations you want to automatically register.
   - Pass `--public-webhook-url https://example.com/webhook` to set the internet-accessible HTTPS URL where the webhook will be served.
   - (Optional) Pass `--dry-run` to test automatic registration without modifying any repository settings.
 - That's it! You don't need to manually configure any webhook or branch protection settings..

### Build stuff
Taken from https://github.com/thockin/go-build-template