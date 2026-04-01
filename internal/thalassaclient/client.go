/*
Copyright 2026.

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

package thalassaclient

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
	"github.com/thalassa-cloud/client-go/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// thalassaLog is the logger for client setup (uses logf.Log; set ctrl.SetLogger before calling New* if you need output).
var thalassaLog = logf.Log.WithName("thalassa-client")

// DefaultKubernetesServiceAccountTokenPath is the default path for the projected or legacy
// Kubernetes service account JWT. Used as SubjectTokenFile for OIDC token exchange when no
// path is configured (federated workload identity in-cluster).
const DefaultKubernetesServiceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"

// defaultOIDCTokenURL returns {baseURL}/oidc/token with a single slash between host and path.
func defaultOIDCTokenURL(baseURL string) string {
	return strings.TrimSuffix(strings.TrimSpace(baseURL), "/") + "/oidc/token"
}

// readSecretFile reads a file and returns trimmed contents (for mounted secrets).
func readSecretFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read secret file %q: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

// NewClientFromEnv builds a Thalassa pkg/client using viper. Configuration is intended to come
// from process flags (see cmd/main); do not rely on THALASSA_* environment variables.
//
// Authentication precedence:
//  1. OIDC token exchange when thalassa-service-account-id and organisation are set.
//     Subject JWT from thalassa-subject-token or thalassa-subject-token-file; if neither is set,
//     DefaultKubernetesServiceAccountTokenPath is used (in-cluster workload identity).
//  2. Personal access token from thalassa-token, or from thalassa-token-file if set (file wins).
//  3. OIDC client credentials: thalassa-client-id + thalassa-client-secret or thalassa-client-secret-file.
func NewClientFromEnv() (client.Client, error) {
	baseURL := viper.GetString("thalassa-url")
	if baseURL == "" {
		baseURL = "https://api.thalassa.cloud"
	}
	org := strings.TrimSpace(viper.GetString("organisation"))
	project := viper.GetString("thalassa-project")
	personalAccessToken := viper.GetString("thalassa-token")
	tokenFile := strings.TrimSpace(viper.GetString("thalassa-token-file"))
	if tokenFile != "" {
		t, err := readSecretFile(tokenFile)
		if err != nil {
			return nil, err
		}
		personalAccessToken = t
	}
	clientID := viper.GetString("thalassa-client-id")
	clientSecret := viper.GetString("thalassa-client-secret")
	clientSecretFile := strings.TrimSpace(viper.GetString("thalassa-client-secret-file"))
	if clientSecretFile != "" {
		s, err := readSecretFile(clientSecretFile)
		if err != nil {
			return nil, err
		}
		clientSecret = s
	}
	insecure := viper.GetBool("thalassa-insecure")

	serviceAccountID := strings.TrimSpace(viper.GetString("thalassa-service-account-id"))
	subjectTokenFile := strings.TrimSpace(viper.GetString("thalassa-subject-token-file"))
	subjectToken := strings.TrimSpace(viper.GetString("thalassa-subject-token"))
	oidcTokenURL := strings.TrimSpace(viper.GetString("thalassa-oidc-token-url"))
	accessTokenLifetime := strings.TrimSpace(viper.GetString("thalassa-access-token-lifetime"))

	tokenURL := defaultOIDCTokenURL(baseURL)

	opts := []client.Option{
		client.WithBaseURL(baseURL),
		client.WithOrganisation(org),
		client.WithUserAgent(fmt.Sprintf("thalassa-dbaas-manager/%s", "0.1.0")),
	}
	if project != "" {
		opts = append(opts, client.WithProject(project))
	}
	if insecure {
		opts = append(opts, client.WithInsecure())
	}

	switch {
	case serviceAccountID != "" && org != "":
		if subjectToken == "" && subjectTokenFile == "" {
			subjectTokenFile = DefaultKubernetesServiceAccountTokenPath
		}
		if oidcTokenURL == "" {
			oidcTokenURL = tokenURL
		}
		thalassaLog.Info("initializing Thalassa client",
			"baseURL", baseURL,
			"organisation", org,
			"auth", "oidc-token-exchange",
			"serviceAccountId", serviceAccountID,
			"oidcTokenURL", oidcTokenURL,
			"subjectTokenSource", subjectTokenSource(subjectToken, subjectTokenFile),
			"insecure", insecure,
			"projectSet", project != "",
		)
		cfg := client.OIDCTokenExchangeConfig{
			TokenURL:            oidcTokenURL,
			SubjectToken:        subjectToken,
			SubjectTokenFile:    subjectTokenFile,
			OrganisationID:      org,
			ServiceAccountID:    serviceAccountID,
			AccessTokenLifetime: accessTokenLifetime,
		}
		opts = append(opts, client.WithAuthOIDCTokenExchange(cfg))
	case personalAccessToken != "":
		thalassaLog.Info("initializing Thalassa client",
			"baseURL", baseURL,
			"organisation", org,
			"auth", "personal-access-token",
			"tokenFromFile", tokenFile != "",
			"insecure", insecure,
			"projectSet", project != "",
		)
		opts = append(opts, client.WithAuthPersonalToken(personalAccessToken))
	case clientID != "" && clientSecret != "":
		thalassaLog.Info("initializing Thalassa client",
			"baseURL", baseURL,
			"organisation", org,
			"auth", "oidc-client-credentials",
			"oidcTokenURL", tokenURL,
			"clientSecretFromFile", clientSecretFile != "",
			"insecure", insecure,
			"projectSet", project != "",
		)
		if insecure {
			opts = append(opts, client.WithAuthOIDCInsecure(clientID, clientSecret, tokenURL, insecure))
		} else {
			opts = append(opts, client.WithAuthOIDC(clientID, clientSecret, tokenURL))
		}
	default:
		err := fmt.Errorf("configure OIDC token exchange (thalassa-service-account-id + organisation), thalassa-token or thalassa-token-file, or thalassa-client-id + thalassa-client-secret or thalassa-client-secret-file")
		thalassaLog.Error(err, "Thalassa client configuration invalid",
			"baseURL", baseURL,
			"organisationSet", org != "",
			"hasServiceAccountId", serviceAccountID != "",
			"hasPersonalToken", personalAccessToken != "",
			"hasTokenFile", tokenFile != "",
			"hasClientCredentials", clientID != "" && clientSecret != "",
			"hasClientSecretFile", clientSecretFile != "",
		)
		return nil, err
	}

	thalassaClient, err := client.NewClient(opts...)
	if err != nil {
		thalassaLog.Error(err, "failed to create Thalassa HTTP client", "baseURL", baseURL)
		return nil, fmt.Errorf("failed to create thalassa client: %w", err)
	}
	thalassaLog.Info("Thalassa HTTP client ready", "baseURL", baseURL)
	return thalassaClient, nil
}

// subjectTokenSource describes where the workload JWT comes from (no secret values).
func subjectTokenSource(subjectTok, subjectFile string) string {
	switch {
	case subjectTok != "":
		return "inline"
	case subjectFile != "":
		return "file"
	default:
		return "unknown"
	}
}
